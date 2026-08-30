package store

import (
	"encoding/json"
	"testing"
)

// TestTheTwoWalksAgreeOnWhatIsValid hunts for the divergence backlog 14 named:
// a payload encoding/json accepts and SQLite refuses, for a reason other than
// depth.
//
// It matters because [Leaves] gates on json.Valid and on the depth limit and on
// nothing else. Any shape where Go says yes and SQLite says no is a payload the
// Go walk indexes and the migration's backfill stores "" for - the same silent
// disagreement the depth guard exists to close, through a door nobody checked.
//
// Depth is deliberately absent: it is the one divergence already known and
// [TestSQLiteRefusesJSONNestedPastTheDepthGuard] owns it. What is here is every
// other way the two parsers could differ that is cheap to state - escapes with
// no character behind them, bytes that are not UTF-8, numbers outside a
// float64, and the degenerate shapes a hand-written parser gets wrong.
//
// A shape both parsers refuse is as much a result as one both accept, so the
// set carries refusals on purpose: without them the assertion would pass against
// a [jsonValid] that returned 1 for everything, and the count check at the end
// is what makes that a guarantee rather than an intention.
func TestTheTwoWalksAgreeOnWhatIsValid(t *testing.T) {
	db := migrated(t)

	// Assembled rather than written as literals, and both reasons are real. A
	// literal NUL or byte-order mark is not legal Go source at all. And an
	// escape case has to be the SIX characters a host would write - backslash,
	// u, four digits - which a source literal spelling them is at constant risk
	// of turning into the one character they denote, silently, at some layer
	// between the editor and the file.
	bs := string([]byte{'\\'})
	jsonEsc := func(seq string) string { return "{\"k\":\"a" + bs + seq + "b\"}" }
	bom := string([]byte{0xEF, 0xBB, 0xBF})

	var agreedValid, agreedInvalid int
	for _, tc := range []struct{ name, payload string }{
		// Escapes with nothing behind them. TestLeavesCoercesWhatIsNotWellFormed
		// pins what TEXT these produce; what is asked here is whether both
		// parsers call them JSON at all.
		{"a lone high surrogate escape", jsonEsc("uD800")},
		{"a lone low surrogate escape", jsonEsc("uDC00")},
		{"a NUL written as a JSON escape", jsonEsc("u0000")},
		{"a noncharacter written as a JSON escape", jsonEsc("uFFFE")},

		// Bytes that are not UTF-8 at all, and a raw NUL. Go's scanner does
		// not validate UTF-8, so this is the likeliest place for the two to
		// part.
		{"invalid UTF-8 inside a string", "{\"k\":\"a\xff\xfeb\"}"},
		{"invalid UTF-8 inside a key", "{\"a\xffb\":\"v\"}"},
		{"a raw NUL inside a string", "{\"k\":\"a\x00b\"}"},

		// Numbers no float64 holds. 1e400 is the one that already fooled this
		// walk once: valid to both, and the decoder failed on it anyway until
		// UseNumber.
		{"a number past float64", `{"k":1e400}`},
		{"a number under float64", `{"k":1e-400}`},
		{"a very long integer", `{"k":123456789012345678901234567890123456789012345678901234567890}`},

		// Degenerate but legal.
		{"an empty key", `{"":"v"}`},
		{"a duplicate key", `{"a":1,"a":2}`},
		{"negative zero", `{"k":-0}`},
		{"a top-level null", `null`},
		{"a top-level bare number", `42`},
		{"an empty array", `[]`},

		// Things at least one of them should refuse. Their job is to keep the
		// agreement assertion from being vacuous.
		{"a raw control character in a string", "{\"k\":\"a\x01b\"}"},
		{"a byte-order mark in front", bom + "{}"},
		{"a trailing comma", `{"a":1,}`},
		{"a single-quoted string", `{'a':1}`},
		{"an unclosed object", `{"a":1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			goSays := json.Valid([]byte(tc.payload))
			sqliteSays := jsonValid(t, db, []byte(tc.payload)) == 1
			if goSays != sqliteSays {
				t.Errorf("json.Valid = %v but json_valid = %v - Leaves gates on the first "+
					"and the backfill on the second, so this payload is indexed by one walk "+
					"and stored as the empty string by the other", goSays, sqliteSays)
				return
			}
			if goSays {
				agreedValid++
			} else {
				agreedInvalid++
			}
		})
	}

	t.Logf("agreed valid: %d, agreed invalid: %d", agreedValid, agreedInvalid)
	if agreedValid == 0 || agreedInvalid == 0 {
		t.Fatalf("the set is one-sided (%d valid, %d invalid), so agreement means nothing",
			agreedValid, agreedInvalid)
	}
}
