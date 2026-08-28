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
// see. Indexing the raw payload bytes makes `session`, `id`, `hook`, `event`,
// `name` and `cwd` tokens of 901 of 902 captured documents, against 76-277 for
// any word a person wrote, so a search for one of those keys returns the entire
// corpus (spec 5.7). The structure only ever raises recall, and it destroys
// precision to do it.
//
// A payload that is valid JSON but not a container yields the leaf itself - a
// bare string - or nothing - a bare number, `true`, `null`. A payload that is
// not JSON at all yields nothing, which is what the migration's backfill also
// answers for it: json_tree raises `malformed JSON`, so the backfill guards
// with json_valid and stores the empty string.
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
			// Not JSON, truncated, or carrying trailing bytes. Any
			// leaf collected so far is discarded rather than
			// returned: json_valid answers 0 for the same bytes and
			// the backfill then stores '', so a partial walk would
			// index one thing on the way in and another on upgrade.
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
