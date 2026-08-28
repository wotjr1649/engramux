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
// A newline rather than a space, because unicode61 splits on both but a phrase
// query does not: with a space, "alpha beta" would match a document whose only
// connection between the two words is that one leaf ended and the next began.
// The newline keeps two leaves from being read as adjacent text.
const leafSeparator = "\n"

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
			// Unreachable as written - json.Valid above has
			// already ruled out every input the token stream can
			// fail on. It returns the same empty answer anyway,
			// because the alternative is a partial walk, and a
			// partial walk is the shape that would disagree with
			// the backfill.
			return ""
		}

		top := len(stack) - 1
		isKey := top >= 0 && stack[top].object && stack[top].atKey
		if top >= 0 && stack[top].object {
			stack[top].atKey = !stack[top].atKey
		}

		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				stack = append(stack, frame{object: true, atKey: true})
			case '[':
				stack = append(stack, frame{})
			default: // '}' or ']', which only close a frame this loop opened
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
