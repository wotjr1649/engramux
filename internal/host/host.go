// Package host classifies which host - Claude Code or Codex - produced a
// captured hook payload. The relay is told nothing about which host launched
// it: the same binary path is configured under both hosts, so argv is not
// trustworthy (I-12). Detection reads the payload instead.
package host

import "strings"

// Detect classifies payload per the three ordered steps of spec 4.3 and
// returns "claude-code", "codex", or "unknown". It never returns an error:
// I-04 requires an unclassifiable event to be stored as unknown rather than
// dropped, so there is nothing here for a caller to treat as "discard this."
//
// Detect takes only the payload already decoded from the hook's stdin, and
// this file imports nothing that could reach argv - so a caller cannot even
// accidentally route os.Args into the decision (I-12).
func Detect(payload map[string]any) string {
	if present(payload, "prompt_id") || present(payload, "effort") {
		return "claude-code"
	}
	if present(payload, "model") || present(payload, "turn_id") {
		return "codex"
	}
	switch transcriptDir(payload) {
	case ".claude":
		return "claude-code"
	case ".codex":
		return "codex"
	}
	return "unknown"
}

// present reports whether key is a key of m, regardless of its value.
//
// encoding/json decodes both a missing key and an explicit "key": null into a
// nil interface{} inside a map[string]any, so a value-only check (m[key] !=
// nil) cannot tell "absent" and "explicit null" apart. The two-value map
// index below can: it reports ok=true for an explicit null, because the key
// really is in the map with a nil value, and ok=false only when the key was
// never in the source document.
//
// This package chooses to treat an explicit null as present. Spec 4.3 reads
// a key's presence as a host signal, and a host's JSON encoder commonly
// serializes an optional field as null rather than omitting it - Codex's
// Rust toolchain does this for an Option<T> field unless told not to. The
// key appearing in the document at all reflects that host's payload schema,
// regardless of whether this particular event happened to carry a value for
// it; falling through to step 3 (or to unknown) just because one event's
// value was null would discard a real signal the document already gave us.
// Verified against the 902-capture corpus: no real capture has an explicit
// null for prompt_id, effort, model, or turn_id, so this choice does not
// move the spec 4.3 counts - it only decides an edge case the corpus never
// exercises. TestDetectPresentNullBoundary tests it directly.
func present(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

// transcriptDir returns the directory component of payload's transcript_path
// that step 3 keys on (".claude" or ".codex"), or "" if transcript_path is
// absent, not a string, or names neither directory. It matches a whole path
// component, not a substring, so a project directory like ".claude-notes"
// cannot be mistaken for the ".claude" host directory.
func transcriptDir(payload map[string]any) string {
	raw, ok := payload["transcript_path"].(string)
	if !ok {
		return ""
	}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\\' || r == '/' }) {
		if part == ".claude" || part == ".codex" {
			return part
		}
	}
	return ""
}
