package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Derived is what a payload contributes to the three ranking columns of
// `events` (memory spec M-3): rule-based text beside the payload, never a
// rewrite of it (I-10).
//
// The three are what the corpus actually carries, and that is a correction to
// M-3's own list rather than a subset of it chosen for convenience. Measured
// 2026-09-03 over the 902 captures: `tool_input.command` on 534,
// `tool_input.file_path` on 120 and `tool_response.filePath` on 54, a
// non-empty `tool_response.stdout` on 220. Against that, **there is no exit
// code and no success flag to read** - `tool_response.stderr` is present on
// 241 documents and non-empty on none of them, `success` appears on 3, and
// exactly one key in the whole corpus matches /exit|return.?code|errno/. So
// M-3's "commands and exit codes" and "success flag" are commands and paths
// here, and its "error spans" live in the output column: 227 documents carry
// error-shaped text and 62 of those carry it in stdout, in prose rather than
// in a field.
//
// # Every rule here has an exact SQL twin, and that is the constraint
//
// Migration 00005 backfills these columns over the rows that predate them, and
// its backfill is SQL. So each rule below is a json_type guard and a
// json_extract, in an order a COALESCE can reproduce - which is why the paths
// column takes the first of two sources rather than joining them, and why the
// output column checks that a response is text before reading it whole.
// TestTheTwoDerivedWalksAgree is what holds the pair together; the leaves
// column has the same arrangement and spec 5.7 says why it matters.
type Derived struct {
	// Cmd is the command line a tool was asked to run.
	Cmd string
	// Paths is the file the tool touched - the one the input named, or
	// failing that the one the response named.
	Paths string
	// Output is the text the tool answered with. It is where an error
	// message and a stack frame are in this corpus, because no field holds
	// them.
	Output string
}

// Derive reads the three columns out of one payload.
//
// A payload that is not exactly one JSON value yields the zero value, which is
// what the backfill's json_valid guard answers for the same bytes. So does one
// that is valid and has none of the three. The zero value means "this payload
// said nothing here" and is a different answer from SQL NULL, which means "not
// computed" and only the backfill ever sees.
func Derive(payload []byte) Derived {
	// The same question the backfill's json_valid guard asks, and it has to
	// be asked here for the same reason [Leaves] asks it: SQLite refuses a
	// payload Go would accept, and the two would then answer differently
	// for one document.
	if !sqliteWillParse(payload) {
		return Derived{}
	}
	// UseNumber, and it is the same reason [Leaves] gives one layer down: a
	// plain json.Unmarshal into map[string]any converts every number to
	// float64 on the way past, so a payload carrying `1e400` anywhere at all
	// fails to decode - and this walk never reads a number. Without it, a
	// payload with a perfectly ordinary `tool_input.command` beside a large
	// number derives nothing here and derives correctly in the backfill,
	// because SQLite's json_extract walks past a number it is not asked for.
	//
	// [agreementPayloads] has carried that number since 00002 and did not
	// catch this: its payload has no tool_input, so both sides answered the
	// zero value for different reasons and the comparison passed on a
	// coincidence. Raised by a review on 2026-09-03; the two payloads that
	// make the difference visible are in [derivedPayloads].
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var fields map[string]any
	if err := dec.Decode(&fields); err != nil {
		// A valid JSON value that is not an object - a bare string, a
		// number, an array. json_extract of a member path over one
		// returns NULL, so the backfill answers empty too.
		//
		// Decode does not object to trailing data, which is why this is
		// not the guard against two concatenated values: json.Valid
		// inside sqliteWillParse above is, and it is what json_valid
		// answers 0 for.
		return Derived{}
	}

	toolInput, _ := fields["tool_input"].(map[string]any)
	toolResponse := fields["tool_response"]
	responseObject, _ := toolResponse.(map[string]any)

	return Derived{
		Cmd:   text(toolInput, "command"),
		Paths: firstText(text(toolInput, "file_path"), text(responseObject, "filePath")),
		// stdout, then content, then a response that is itself a
		// string. The last is guarded on the value being text and not
		// on the two above being absent, because json_extract of
		// `$.tool_response` over an object or an array returns that
		// container's JSON *text* - which would put a serialised
		// structuredPatch in a ranking column. The Go type switch here
		// and json_type() there are the same guard.
		Output: firstText(text(responseObject, "stdout"), text(responseObject, "content"), asText(toolResponse)),
	}
}

// sqliteWillParse reports whether SQLite's JSON parser would accept payload,
// which is a narrower question than [encoding/json.Valid] alone.
//
// Two things separate them and both are already measured on [sqliteJSONDepthLimit]:
// SQLite stops at 1000 open containers where Go stops at 10000, and json_valid
// answers 0 for two concatenated values where a [encoding/json.Decoder] reads
// them as two. The backfill's CASE tests json_valid before it extracts
// anything, so it answers the empty string for both shapes - and without this,
// a payload carrying a shallow `tool_input.command` and a deeply nested
// sibling would be derived here and not there, for one row, silently.
//
// It is a second walk of the same bytes rather than a value [Leaves] hands
// over, because [Leaves] answers "" both for a payload SQLite refuses and for
// one that simply has no string leaves, and only the first of those means this
// function's answer. At spec 7.4's payload sizes the walk is microseconds.
func sqliteWillParse(payload []byte) bool {
	if !json.Valid(payload) {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	// Numbers as text, for the reason [Leaves] gives: without it a number
	// this walk never looks at - 1e400 - overflows float64 and ends the
	// stream, which would read as a refusal SQLite does not make.
	dec.UseNumber()
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			// io.EOF is the end of the one value json.Valid already
			// confirmed. Anything else is a stream this walk cannot
			// finish, and the honest answer is the same as SQLite's
			// for a payload it cannot finish either.
			return errors.Is(err, io.EOF)
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
				if depth > sqliteJSONDepthLimit {
					return false
				}
			} else {
				depth--
			}
		}
	}
}

// text returns fields[key] when it is a string, and "" for every other shape.
// It is [field] with a nil-safe receiver: a payload with no tool_input at all
// hands a nil map in here rather than being special-cased at every call.
func text(fields map[string]any, key string) string {
	v, _ := fields[key].(string)
	return v
}

// asText returns v when it is a string, and "" otherwise. It is the Go half of
// json_type(...) = 'text'.
func asText(v any) string {
	s, _ := v.(string)
	return s
}

// firstText returns the first argument that is not empty, and "" when none is.
// It is the Go half of a COALESCE over each source passed through NULLIF
// against the empty string.
func firstText(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			return c
		}
	}
	return ""
}
