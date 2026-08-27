// Package secret decides what counts as a secret in a hook payload, tags it,
// and masks it on the way out.
//
// A secret is tagged, not destroyed (I-10). Erasing it would destroy exactly
// the memory this product exists to keep - "I put the key in .env" is useful
// context and a row reading [redacted] is not - so the database keeps the
// original bytes and carries the tag, and every path out of the machine filters
// on the tag. The consequence is that a false positive costs nothing and a
// false negative is the only expensive outcome, so every rule here is
// deliberately generous (spec 6.1).
//
// # Detection walks the decoded JSON, not the raw bytes
//
// [Detect] and [Mask] decode the payload and visit string leaves. Scanning the
// encoded bytes instead is wrong in both directions:
//
//   - It misses. A Windows path is backslash-escaped by every JSON encoder, and
//     user identity is the class that actually fires - 1,714 matches across 900
//     of 902 captures, against 4 files for every credential class combined. A
//     host is also free to write any character as \uXXXX, which splits a
//     credential across an escape no byte scan can see. Missing is the
//     expensive direction.
//   - It over-reaches into structure. A byte scan matches inside object keys
//     and across the quotes and braces holding the document together, and a
//     mask built from those spans produces JSON that no longer parses.
//
// Object keys are structure, not content, with one exception: a key that names
// a credential tags its own string value, because that is where a structured
// payload puts one.
//
// A payload that does not parse as JSON is scanned as text rather than reported
// clean, for the same reason: reporting clean is the expensive direction.
package secret

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
)

// Version is the stored redaction_version: the identity of the ruleset that
// judged a row, so a later ruleset can re-scan without ambiguity about what an
// old row was judged against (spec 6.1). Bump it whenever a rule below changes
// what it matches.
const Version = 1

// Class is one of spec 6.1's kinds of secret.
type Class string

const (
	ClassAPIKey           Class = "api-key"
	ClassAuthorization    Class = "authorization"
	ClassConnectionString Class = "connection-string"
	ClassCredential       Class = "credential"
	ClassDotenv           Class = "dotenv"
	ClassOpaque           Class = "opaque"
	ClassPrivateKey       Class = "private-key"
	ClassUserPath         Class = "user-path"
)

// Set is the stored form of privacy_class: the classes that matched, without
// duplicates, in a canonical order.
//
// [Set.String] renders it for the TEXT column as comma-separated names, and the
// empty set as the empty string; [ParseSet] reads that back. Sorting is what
// makes two rows that matched the same classes hold the same bytes, so equality
// on the column is a usable comparison. No class name is a substring of another
// - a test holds that - so a LIKE filter for a single class is unambiguous too.
type Set []Class

// String renders s for storage.
func (s Set) String() string {
	names := make([]string, len(s))
	for i, c := range s {
		names[i] = string(c)
	}
	return strings.Join(names, ",")
}

// ParseSet reads back what [Set.String] wrote. It does not reject names it does
// not know: a row written by a later ruleset is still readable, which is what
// the stored [Version] alongside it is for.
func ParseSet(v string) Set {
	if v == "" {
		return nil
	}
	out := make(Set, 0, strings.Count(v, ",")+1)
	for _, name := range strings.Split(v, ",") {
		if name != "" {
			out = append(out, Class(name))
		}
	}
	return canonical(out)
}

// Classes returns every class this ruleset can report, in the order Set uses.
func Classes() []Class {
	out := make(Set, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.class)
	}
	return canonical(out)
}

func canonical(s Set) Set {
	slices.Sort(s)
	return slices.Compact(s)
}

// placeholder is what masking leaves behind. It carries the class so a masked
// log line still says what was removed, and it is deliberately shaped so that
// re-scanning it finds nothing: see [isPlaceholder].
const redactedPrefix = "[redacted-"

func placeholder(c Class) string { return redactedPrefix + string(c) + "]" }

// isPlaceholder reports whether a candidate span is something masking already
// wrote. Skipping those is what makes [Mask] idempotent: several rules keep
// their surrounding context and mask only the token inside it, so a second pass
// sees "password=[redacted-credential]" and would otherwise re-mask the
// placeholder as a different class.
//
// The prefix alone decides. [bound] excludes ']', so what a second pass
// captures is "[redacted-credential" without its closing bracket - requiring
// the bracket here made masking append one on every pass.
func isPlaceholder(v string) bool { return strings.HasPrefix(v, redactedPrefix) }

// bound is the character class every masked token stops at. Spec 6.1 names it
// for the Authorization token and gives the reason: \S+ swallows the closing
// quote and the closing brace, and a leaf that is itself a JSON document - which
// tool_response is, for Codex (spec 4.4) - stops parsing. The reason applies to
// every span this package masks, so every bounded rule uses this class.
const bound = `[^\s"',}\]]`

type rule struct {
	class Class
	re    *regexp.Regexp
}

// rules is spec 6.1's table. A rule with a capturing group masks the group and
// keeps the rest as context; a rule without one masks the whole match.
var rules = []rule{
	// sk- subsumes the spec's sk-ant- and sk-proj- entries, because '-' is in
	// the token charset.
	{ClassAPIKey, regexp.MustCompile(
		`\b(?:sk-|gh[pousr]_|github_pat_|xox[baprs]-)[A-Za-z0-9_-]+` +
			`|\bAKIA[A-Z0-9]{16}`)},

	// The END marker is optional so a truncated block still matches; spec 6.1
	// asks for "any -----BEGIN ... PRIVATE KEY----- block".
	{ClassPrivateKey, regexp.MustCompile(
		`(?s)-----BEGIN[ A-Z]*PRIVATE KEY-----(?:.*?-----END[ A-Z]*PRIVATE KEY-----)?`)},

	// The optional quotes let this fire inside a JSON document carried as a
	// string. "Authorization" is tried with the Bearer prefix first, or
	// `Authorization: Bearer x` would mask the word Bearer and leave x.
	{ClassAuthorization, regexp.MustCompile(
		`(?i)(?:authorization"?\s*[:=]\s*"?(?:bearer\s+)?|bearer\s+)(` + bound + `+)`)},

	// No leading word boundary: refresh_token= and access_token= are the common
	// real shapes, and a boundary would miss both.
	{ClassCredential, regexp.MustCompile(
		`(?i)(?:passwd|password|secret|api[_-]?key|token)"?\s*[:=]\s*"?(` + bound + `+)`)},

	{ClassConnectionString, regexp.MustCompile(
		`[a-zA-Z][a-zA-Z0-9+.-]*://([^\s/:@"']+:[^\s/@"']*)@`)},

	{ClassDotenv, regexp.MustCompile(
		`(?m)^[A-Z][A-Z0-9_]*=.*(?:\r?\n[A-Z][A-Z0-9_]*=.*)+`)},

	{ClassOpaque, regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)},

	// One alternative covers C:\Users\x, C:/Users/x, \\host\Users\x and
	// /c/Users/x, at the cost of also matching a /users/ path segment in a URL.
	// That is the direction this package is supposed to err in.
	{ClassUserPath, regexp.MustCompile(`(?i)[\\/]users[\\/]+([^\s"',}\]\\/]+)`)},
}

// credentialKey matches an object key that names a credential. Its whole string
// value is then a credential, because a structured payload puts the value there
// rather than in a name=value string.
var credentialKey = regexp.MustCompile(`(?i)passwd|password|secret|api[_-]?key|token`)

// span is one run of bytes inside one string leaf.
type span struct {
	class      Class
	start, end int
	order      int // rule index, so equal spans have a deterministic winner
}

// spansIn returns every span of leaf that some rule matched. Spans may overlap:
// two classes matching the same bytes is two tags, and [maskText] resolves the
// overlap only when it comes to rewriting.
func spansIn(key, leaf string) []span {
	var out []span
	for i, r := range rules {
		for _, m := range r.re.FindAllStringSubmatchIndex(leaf, -1) {
			start, end := m[0], m[1]
			if r.re.NumSubexp() > 0 && m[2] >= 0 {
				start, end = m[2], m[3]
			}
			if end <= start || isPlaceholder(leaf[start:end]) {
				continue
			}
			out = append(out, span{r.class, start, end, i})
		}
	}
	if leaf != "" && !isPlaceholder(leaf) && credentialKey.MatchString(key) {
		out = append(out, span{ClassCredential, 0, len(leaf), len(rules)})
	}
	return out
}

// maskText replaces every matched span of leaf with its class placeholder.
// Overlapping spans are resolved leftmost first, then longest, then by rule
// order, so the result is deterministic and no span is written twice.
func maskText(key, leaf string) string {
	spans := spansIn(key, leaf)
	if len(spans) == 0 {
		return leaf
	}
	slices.SortFunc(spans, func(a, b span) int {
		switch {
		case a.start != b.start:
			return a.start - b.start
		case a.end != b.end:
			return b.end - a.end
		default:
			return a.order - b.order
		}
	})
	var b strings.Builder
	last := 0
	for _, s := range spans {
		if s.start < last {
			continue // already inside a span that was written
		}
		b.WriteString(leaf[last:s.start])
		b.WriteString(placeholder(s.class))
		last = s.end
	}
	b.WriteString(leaf[last:])
	return b.String()
}

// Detect returns the classes that matched anywhere in payload. The result is
// the value to store in privacy_class, alongside [Version] in
// redaction_version. It never fails: a payload that does not parse as JSON is
// scanned as text, because a detector that answers "clean" on bad input is a
// detector callers will skip on bad input.
func Detect(payload []byte) Set {
	var found Set
	collect := func(key, leaf string) string {
		for _, s := range spansIn(key, leaf) {
			found = append(found, s.class)
		}
		return leaf
	}
	if v, ok := decode(payload); ok {
		apply(v, "", collect)
	} else {
		collect("", string(payload))
	}
	return canonical(found)
}

// Mask returns payload with every matched span replaced by a placeholder. It is
// the egress half of I-10: the database keeps the original, and what leaves the
// machine goes through here.
//
// The result still parses. Masking rewrites decoded string leaves and re-encodes
// the document, so the payload's own quotes and braces are never inside a span;
// the bounded token patterns are what keep a JSON document *carried inside* a
// leaf parseable too. A payload with nothing to mask is returned unchanged, down
// to the byte.
func Mask(payload []byte) []byte {
	v, ok := decode(payload)
	if !ok {
		return []byte(maskText("", string(payload)))
	}
	changed := false
	v = apply(v, "", func(key, leaf string) string {
		masked := maskText(key, leaf)
		if masked != leaf {
			changed = true
		}
		return masked
	})
	if !changed {
		return payload
	}
	out, err := json.Marshal(v)
	if err != nil {
		// Unreachable: v came out of encoding/json. Masking the raw text is
		// still better than the alternative, which is returning the secret.
		return []byte(maskText("", string(payload)))
	}
	return out
}

// MaskString is [Mask] for a value that is not a whole payload - a log message,
// one attribute, a field on its way to an egress that is not JSON.
func MaskString(v string) string { return maskText("", v) }

// decode parses payload as one JSON value. UseNumber keeps 1700000000 from
// coming back out as 1.7e+09. Trailing data means this is not a payload, so it
// is reported as not-JSON and gets scanned as text.
func decode(payload []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return v, true
}

// apply replaces every string leaf of v with f's result. key is the object key
// that reached the leaf, empty at the root; an array passes down the key that
// named the array, so {"tokens":["..."]} reaches f as key "tokens".
func apply(v any, key string, f func(key, leaf string) string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = apply(e, k, f)
		}
	case []any:
		for i, e := range x {
			x[i] = apply(e, key, f)
		}
	case string:
		return f(key, x)
	}
	return v
}
