package host

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"strings"
)

// hooksKey is the member of a host configuration document that holds the hook
// table. Both hosts spell it the same way - spec 4.2 differs on the hook
// object, not on where it lives - so one name serves both.
const hooksKey = "hooks"

// hookIndent is what both host configuration files are written with. Two spaces
// is what the installer this replaces used and what the hosts' own writers use;
// changing it would rewrite every line of a user's file for no reason.
const hookIndent = "  "

// getMember returns one member of a JSON object as raw bytes, and whether it
// was there. Everything is read as [jsontext.Value] rather than decoded, so a
// member this code does not understand can still be carried around and written
// back byte for byte.
func getMember(obj jsontext.Value, name string) (jsontext.Value, bool, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(obj))
	if _, err := dec.ReadToken(); err != nil {
		return nil, false, fmt.Errorf("host: open an object: %w", err)
	}
	for dec.PeekKind() != '}' {
		key, err := dec.ReadValue()
		if err != nil {
			return nil, false, fmt.Errorf("host: read a member name: %w", err)
		}
		if unquote(key) != name {
			if err := dec.SkipValue(); err != nil {
				return nil, false, fmt.Errorf("host: skip a member: %w", err)
			}
			continue
		}
		value, err := dec.ReadValue()
		if err != nil {
			return nil, false, fmt.Errorf("host: read member %q: %w", name, err)
		}
		return value.Clone(), true, nil
	}
	return nil, false, nil
}

// setMember returns obj with one member set, **preserving the position and the
// exact bytes of every other member**. A member already present is rewritten
// where it stands; one that is not is appended, so a file a person reads still
// starts the way they left it. A nil value removes the member.
//
// This is the whole reason the port does not decode into a Go map. A map loses
// member order, so writing a user's settings file back would alphabetise it and
// make their diff the entire file; and encoding/json would unquote and requote
// every string on the way, turning an escape into the character it denotes and
// a plain character into an HTML escape. Here a member that is not the target
// is copied as bytes and cannot change.
func setMember(obj jsontext.Value, name string, value jsontext.Value) (jsontext.Value, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(obj))
	var out bytes.Buffer
	// PreserveRawStrings, because the encoder canonicalises string escapes
	// otherwise: a file carrying an escape comes back with the character it
	// denotes, and a character this encoder escapes for HTML comes back as an
	// escape. Both rewrite lines of a user's file that this function did not
	// touch, which is the one thing it exists not to do.
	enc := jsontext.NewEncoder(&out, jsontext.PreserveRawStrings(true))

	if k := dec.PeekKind(); k != '{' {
		return nil, fmt.Errorf("host: expected a JSON object, got %v", k)
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, fmt.Errorf("host: open an object: %w", err)
	}
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return nil, fmt.Errorf("host: open the result: %w", err)
	}

	replaced := false
	for dec.PeekKind() != '}' {
		key, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("host: read a member name: %w", err)
		}
		// Cloned at once: ReadValue reuses its buffer, so the next read below
		// invalidates this one, and what reaches the encoder is a truncated
		// name rather than a name.
		key = key.Clone()
		if unquote(key) == name {
			replaced = true
			if err := dec.SkipValue(); err != nil {
				return nil, fmt.Errorf("host: skip the replaced member: %w", err)
			}
			if value == nil {
				continue
			}
			if err := writeMember(enc, key, value); err != nil {
				return nil, err
			}
			continue
		}
		kept, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("host: read a member: %w", err)
		}
		if err := writeMember(enc, key, kept); err != nil {
			return nil, err
		}
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, fmt.Errorf("host: close an object: %w", err)
	}

	if !replaced && value != nil {
		if err := writeMember(enc, jsontext.Value(quote(name)), value); err != nil {
			return nil, err
		}
	}
	if err := enc.WriteToken(jsontext.EndObject); err != nil {
		return nil, fmt.Errorf("host: close the result: %w", err)
	}
	if _, err := dec.ReadToken(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("host: trailing bytes after the object: %w", err)
	}
	return out.Bytes(), nil
}

// marshalValues joins raw JSON values into an array without re-encoding any of
// them, which encoding/json would: its Marshal escapes for HTML, so a value
// carrying an angle bracket comes back with an escape the file never had.
func marshalValues(values []jsontext.Value) (jsontext.Value, error) {
	var out bytes.Buffer
	enc := jsontext.NewEncoder(&out, jsontext.PreserveRawStrings(true))
	if err := enc.WriteToken(jsontext.BeginArray); err != nil {
		return nil, fmt.Errorf("host: open an array: %w", err)
	}
	for _, v := range values {
		if err := enc.WriteValue(v); err != nil {
			return nil, fmt.Errorf("host: write an array element: %w", err)
		}
	}
	if err := enc.WriteToken(jsontext.EndArray); err != nil {
		return nil, fmt.Errorf("host: close an array: %w", err)
	}
	return out.Bytes(), nil
}

// unmarshalValues splits a raw JSON array into its elements, or fails when it
// is not an array.
func unmarshalValues(v jsontext.Value) ([]jsontext.Value, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(v))
	if k := dec.PeekKind(); k != '[' {
		return nil, fmt.Errorf("host: expected a JSON array, got %v", k)
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, fmt.Errorf("host: open an array: %w", err)
	}
	var out []jsontext.Value
	for dec.PeekKind() != ']' {
		e, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("host: read an array element: %w", err)
		}
		out = append(out, e.Clone())
	}
	return out, nil
}

func writeMember(enc *jsontext.Encoder, key, value jsontext.Value) error {
	if err := enc.WriteValue(key); err != nil {
		return fmt.Errorf("host: write a member name: %w", err)
	}
	if err := enc.WriteValue(value); err != nil {
		return fmt.Errorf("host: write a member value: %w", err)
	}
	return nil
}

// quote and unquote go through the JSON encoder rather than through strconv,
// so a name carrying a character either of them would escape survives the round
// trip as the same name.
func quote(s string) string {
	b, err := jsontext.AppendQuote(nil, s)
	if err != nil { // unreachable for a Go string, which is always valid UTF-8 after coercion
		return `""`
	}
	return string(b)
}

func unquote(v jsontext.Value) string {
	b, err := jsontext.AppendUnquote(nil, v)
	if err != nil {
		return ""
	}
	return string(b)
}

// isEngramux reports whether a hook object inside an entry is one of ours.
//
// The rule is the previous installer's and is deliberately loose: a `command`
// member whose string contains "engramux", case-insensitively. It has to
// recognise every spelling this product has ever written - forward slashes,
// backslashes, quoted, unquoted - because what it is really for is finding the
// hooks a PREVIOUS version installed so they can be dropped before the current
// one is added. A precise rule would leave those behind and the file would grow
// on every upgrade.
//
// The cost is that a hook belonging to somebody else whose command happens to
// contain the word is dropped. Accepted, and it is the same trade the installer
// this replaces made.
func isEngramux(hook jsontext.Value) bool {
	command, ok, err := getMember(hook, "command")
	if err != nil || !ok || command.Kind() != '"' {
		return false
	}
	return strings.Contains(strings.ToLower(unquote(command)), "engramux")
}

// MergeHooks rewrites the hook table of a host configuration document and
// leaves every other VALUE of it alone.
//
// Value and not byte, and the distinction is the whole of what this promises.
// Member order, string escapes, number spellings and characters encoding/json
// would escape for HTML all survive exactly. Whitespace does not: the result is
// re-indented at two spaces from end to end, so a file that used four, or none,
// comes back re-laid-out. An earlier version of this comment said "every other
// byte" and a review caught it.
//
// events is the set of member names under the hook table to rewrite. A member
// not in it is copied verbatim, so a host carrying hooks this product does not
// capture keeps them.
//
// entryFor returns the hook entry to install for one event. **A nil entryFor is
// how removal is spelled** - so install and remove cannot drift apart, because
// they are one path.
//
// One limit on what removal reaches, found by review: an event whose value is
// not an array is returned untouched (see [rewriteEntries]), so an Engramux
// hook buried inside a shape this code does not recognise survives a removal.
// That is the same trade as preserving it on an install - overwriting a shape
// nobody understands loses whatever it held - and it is stated rather than
// implied.
//
// The result is indented once at the end rather than as it is built. That is
// measured, not assumed: [jsontext.Value.Indent] changes whitespace between
// tokens and nothing else, so member order, number spellings, string escapes
// and characters encoding/json would escape for HTML all survive it.
// TestIndentingChangesOnlyWhitespace holds that.
func MergeHooks(src []byte, events []string, entryFor func(event string) jsontext.Value) ([]byte, error) {
	doc := jsontext.Value(src)
	if !doc.IsValid() {
		return nil, fmt.Errorf("host: the configuration file is not valid JSON")
	}

	table, ok, err := getMember(doc, hooksKey)
	if err != nil {
		return nil, err
	}
	if !ok {
		table = jsontext.Value(`{}`)
	}

	for _, event := range events {
		entries, _, err := getMember(table, event)
		if err != nil {
			return nil, err
		}
		if entries == nil {
			entries = jsontext.Value(`[]`)
		}
		next, err := rewriteEntries(entries, event, entryFor)
		if err != nil {
			return nil, err
		}
		// An event left with nothing is removed rather than written as an
		// empty array: an empty array is still a hook table the host walks,
		// and the file it came from did not have one.
		if table, err = setMember(table, event, next); err != nil {
			return nil, err
		}
	}

	if doc, err = setMember(doc, hooksKey, table); err != nil {
		return nil, err
	}
	if err := doc.Indent(jsontext.WithIndent(hookIndent)); err != nil {
		return nil, fmt.Errorf("host: indent the result: %w", err)
	}
	return append([]byte(doc), '\n'), nil
}

// rewriteEntries drops every Engramux hook from one event's entry list and
// appends one entry holding the current hook, or returns nil when nothing is
// left to write.
//
// Ours is appended last so that a gate or guard the user already had still runs
// first.
func rewriteEntries(entries jsontext.Value, event string, entryFor func(string) jsontext.Value) (jsontext.Value, error) {
	list, err := unmarshalValues(entries)
	if err != nil {
		// Not an array of values. The host wrote something this does not
		// understand, and overwriting it would lose it, so it is returned
		// unchanged - the same rule spec 4.4 imposes on an unrecognised
		// tool_response shape.
		return entries, nil
	}

	kept := make([]jsontext.Value, 0, len(list)+1)
	for _, entry := range list {
		inner, ok, err := getMember(entry, hooksKey)
		if err != nil || !ok {
			// No hooks member, or a shape this does not read. Not ours to
			// judge: kept exactly as it was.
			kept = append(kept, entry)
			continue
		}
		hooks, err := unmarshalValues(inner)
		if err != nil {
			kept = append(kept, entry)
			continue
		}
		theirs := make([]jsontext.Value, 0, len(hooks))
		for _, hook := range hooks {
			if !isEngramux(hook) {
				theirs = append(theirs, hook)
			}
		}
		if len(theirs) == 0 {
			// The entry held only ours, so the entry goes with it.
			continue
		}
		rebuilt, err := marshalValues(theirs)
		if err != nil {
			return nil, fmt.Errorf("host: rebuild an entry's hooks: %w", err)
		}
		next, err := setMember(entry, hooksKey, rebuilt)
		if err != nil {
			return nil, err
		}
		kept = append(kept, next)
	}

	if entryFor != nil {
		kept = append(kept, entryFor(event))
	}
	if len(kept) == 0 {
		return nil, nil
	}
	out, err := marshalValues(kept)
	if err != nil {
		return nil, fmt.Errorf("host: rebuild an event's entries: %w", err)
	}
	return out, nil
}
