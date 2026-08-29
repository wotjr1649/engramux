package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// leafSeparator joins two string leaves in the indexed text.
//
// Some separator is required: without one, the last token of a leaf and the
// first token of the next fuse into a single token that is in neither.
//
// That it is a newline rather than a space is the migration: the backfill joins
// with char(10), and the two walks must produce byte-identical text. It is not a
// tokenizer decision and buys nothing there - unicode61 drops a newline exactly
// as it drops a space, so the two leaves' tokens are adjacent by position and a
// phrase query spans them. `"alpha beta"` matches a document indexed as
// `alpha\nbeta`. TestANewlineIsNotAPhraseBoundary measures both halves.
//
// The exposure that leaves, and why it is accepted for 1.0: matchExpression in
// internal/search never emits a multi-token phrase out of separate words a
// person typed - one quoted prefix phrase per whitespace-delimited token. A
// phrase of more than one token arises only from a single typed token the
// tokenizer splits on internal punctuation, `main_test.go` into `main` `test`
// `go`. Such a phrase can span a leaf boundary, and that cross-leaf false
// positive is the whole of it.
const leafSeparator = "\n"

// sqliteJSONDepthLimit is how many containers may be open at once in a payload
// this walk will read. It is SQLite's number and not Go's: encoding/json accepts
// ten times as much, so nothing on this side fails on its own at the depth that
// matters.
//
// Measured on SQLite 3.53.3 through modernc.org/sqlite v1.57.0, identically for
// nested objects, nested arrays and the two alternating - json_valid answers 1
// at a depth of 1000 open containers and 0 at 1001, where json.Valid answers
// true at 10000 and false only at 10001. A bare scalar is depth 0.
//
// Past the limit the migration's backfill stores the empty string, because its
// CASE tests json_valid first, so this answers "" for the same payload too.
// Without the guard the two walks disagree over the whole range between the two
// limits. The guard cannot be moved into the backfill instead: json_tree over
// such a payload raises `malformed JSON` rather than returning no rows.
//
// Re-verify when the driver moves - TestSQLiteRefusesJSONNestedPastTheDepthGuard
// asks SQLite for both sides of this number, and TestTheTwoWalksAgree carries a
// payload on each side of it in all three shapes.
const sqliteJSONDepthLimit = 1000

// Leaves is what a payload contributes to the search index: every JSON string
// leaf, in document order, joined by [leafSeparator]. Object keys are structure
// rather than content and are skipped.
//
// This is the whole answer to a precision problem the recall gate could not
// see: indexing the raw payload bytes indexes the structure, and a JSON key is
// then a token of nearly every document. Spec 5.7 holds the measurement.
//
// A payload that is valid JSON but not a container yields the leaf itself - a
// bare string - or nothing - a bare number, `true`, `null`. Anything that is
// not exactly one JSON value yields nothing, which is what the migration's
// backfill also answers for it: json_tree raises `malformed JSON`, so the
// backfill guards with json_valid and stores the empty string.
//
// "Exactly one" is why this checks [json.Valid] before walking rather than
// relying on the decode to fail. A [json.Decoder] streams: it reads
// `{"a":"x"}{"b":"y"}` as two values and never errors, where json_valid answers
// 0 for the same bytes. Without the check in front, that payload - and `"a" "b"`,
// and `{"a":"x"} 42` - would index text on the way in and nothing on upgrade,
// which is the one divergence TestTheTwoWalksAgree exists to prevent. The check
// costs a second scan of the same bytes and no second decode.
//
// The empty string means "this payload has no string leaves" and is a different
// answer from SQL NULL, which means "not computed" and only the migration's
// backfill ever sees.
//
// events.leaves is a derived column and nothing recomputes it: the FTS triggers
// carry its old and new values, they do not call this. Nothing in 1.0 updates
// an event, and whatever first does has to write both columns in the one
// statement - an UPDATE that touched payload alone would leave the index
// faithfully holding the previous text, with integrity-check still passing
// because the index and the column it indexes would still agree.
//
// # Why a token stream and not a map
//
// internal/secret walks a decoded map[string]any, which loses document order:
// Go randomises map iteration and a sorted walk imposes an order the document
// did not have. Order is what makes the two walks comparable at all - the
// migration's json_tree walk is in document order and cannot be made to be in
// any other - and it is also what keeps adjacent text adjacent. So this decodes
// the token stream instead, tracking whether the next string inside an object
// is a key or a value.
func Leaves(payload []byte) string {
	// Exactly one JSON value, or nothing - see the doc comment. This is the
	// same predicate json_valid applies in the backfill, and it has to be
	// applied here because the decoder below would not.
	if !json.Valid(payload) {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	// Numbers as text, so that a number this walk never looks at cannot end
	// it. Without this the decoder converts every number to float64 on the
	// way past, and `1e400` - which json_valid and json.Valid both accept,
	// and which json_tree walks straight over - overflows and fails the
	// whole stream. The switch below only ever reads string tokens, so
	// json.Number in place of float64 changes nothing else.
	dec.UseNumber()

	// One entry per open container. atKey is true while the next string
	// token in an object is its key; inside an object the two alternate, and
	// a nested container consumes the value slot so that the token after it
	// closes is a key again. An array frame is never in key position.
	type frame struct{ object, atKey bool }
	var stack []frame

	var out []string
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// No input is known to reach this, and "unreachable"
			// is what the comment here used to claim: json.Valid
			// above was said to have ruled out every failure the
			// token stream has. It had not. `{"a":"alpha",
			// "n":1e400,"b":"beta"}` is valid to json.Valid and to
			// json_valid, and the decoder failed on it anyway,
			// converting a number this walk never reads into a
			// float64 that overflows - so the branch ran, returned
			// "", and the backfill answered `alpha\nbeta` for the
			// same bytes. UseNumber above is what closed that.
			//
			// So the branch stays: what is left is a claim about a
			// dependency's behaviour and not a proof, and the empty
			// answer is the right one either way, because the
			// alternative is a partial walk and a partial walk is
			// the shape that disagrees with the backfill.
			return ""
		}

		top := len(stack) - 1
		isKey := top >= 0 && stack[top].object && stack[top].atKey
		if top >= 0 && stack[top].object {
			stack[top].atKey = !stack[top].atKey
		}

		switch v := tok.(type) {
		case json.Delim:
			// Openers and closers are split here rather than in a
			// switch over the four delimiters, so that the depth check
			// sits on the only path that can raise the depth. In the
			// switch it also ran after a pop, where len(stack) had just
			// shrunk and the comparison could never fire (backlog 11).
			//
			// It stays one check for both openers, because SQLite counts
			// them together - see [sqliteJSONDepthLimit].
			if v == '{' || v == '[' {
				stack = append(stack, frame{object: v == '{', atKey: v == '{'})
				if len(stack) > sqliteJSONDepthLimit {
					return ""
				}
			} else { // '}' or ']', which only close a frame this loop opened
				stack = stack[:len(stack)-1]
			}
		case string:
			if !isKey {
				out = append(out, v)
			}
		}
	}
	return strings.Join(out, leafSeparator)
}
