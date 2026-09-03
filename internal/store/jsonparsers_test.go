package store

import (
	"database/sql"
	"testing"
)

// Where `encoding/json` and SQLite's JSON parser answer differently for the same
// bytes, asked about the *derived* columns.
//
// # What is here and what is not, because the first version of this file got it
// wrong
//
// The ill-formed-Unicode half of this was written on 2026-09-03 as a new
// finding. It is not one. The 1.0 spec §7.1 has recorded since Phase 4 that
// [Leaves] and `json_tree` diverge on invalid UTF-8 and on a lone surrogate,
// [TestLeavesCoercesWhatIsNotWellFormed] has pinned it since then over three
// shapes rather than two, and TestTheTokenizerReadsBothIllFormedShapesTheSameWay
// measured the consequence: all three spellings of the ill-formed run index the
// same two tokens, so on the `leaves` side the divergence **cannot change a
// search result**. None of that needed re-finding and the backlog row that
// claimed it did was withdrawn.
//
// What is left is the part those measurements do not reach, and it is the reason
// this file survives rather than being deleted with the row:
//
//   - The derived columns are compared with `LIKE`, not tokenized, so
//     "the tokenizer cannot tell the spellings apart" says nothing about them.
//   - `json_extract` resolves a duplicated member where `json_tree` visits both,
//     so [Derive] has a divergence [Leaves] structurally cannot have.
//
// # Neither is fixed, and that is a decision rather than neglect
//
// **Go is right about ill-formed Unicode.** An unpaired surrogate is not a
// character and U+FFFD is what keeps the column valid UTF-8; matching SQLite
// would mean writing our own unescaper and storing invalid UTF-8 on purpose.
// **Nobody is right about the duplicated key**, because JSON does not say - Go's
// decoder takes the last, `json_extract` takes the first, and both are readings
// of a document nobody should have written. Backlog **40** carries the one open
// decision. What these tests do is make the current behaviour an assertion
// rather than a memory, on the exact bytes, so a dependency upgrade cannot move
// it quietly.
//
// Neither shape is reachable from either host's own encoder. Ill-formed Unicode
// is reachable from a tool's output, because Windows permits an unpaired
// surrogate in a file name.

// TestDeriveCoercesIllFormedUnicodeWhereJSONExtractDoesNot pins the derived-column
// half of a divergence the `leaves` half already owns.
//
// Both shapes are asserted on the exact bytes each side produces rather than on
// the fact that they differ: "they differ" would still hold if both sides moved
// to two new and equally wrong answers.
//
// Neither payload is written as a literal. A raw invalid byte and a lone
// surrogate do not survive being typed into this file, which is what AGENTS.md's
// rule about escapes and file-write tools is for; both were tried the other way
// first.
func TestDeriveCoercesIllFormedUnicodeWhereJSONExtractDoesNot(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		payload []byte
		goSide  string
		sqlSide string
	}{
		{
			name:    "a lone surrogate escape, which is well-formed JSON and is not a character",
			payload: loneSurrogate(),
			// U+FFFD is what encoding/json substitutes.
			goSide: "lone � surrogate",
			// The WTF-8 encoding of U+D800, which SQLite hands back.
			sqlSide: "lone " + string([]byte{0xED, 0xA0, 0x80}) + " surrogate",
		},
		{
			name:    "a raw byte that cannot begin a UTF-8 sequence",
			payload: rawInvalidByte(),
			goSide:  "a�b",
			sqlSide: "a" + string([]byte{0xFF}) + "b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.goSide == tc.sqlSide {
				t.Fatal("the two expected values are equal; this case compares nothing")
			}
			if got := Derive(tc.payload).Cmd; got != tc.goSide {
				t.Errorf("Derive: got % x, want the U+FFFD substitution % x", got, tc.goSide)
			}
			db := migrated(t)
			if got := sqlExtractText(t, db, tc.payload, `$.tool_input.command`); got != tc.sqlSide {
				t.Errorf("json_extract: got % x, want the bytes preserved % x", got, tc.sqlSide)
			}
		})
	}
}

// TestTheTwoJSONParsersDivergeOnADuplicatedKey pins the one divergence that is
// [Derive]'s alone.
//
// It is not asserted on [Leaves], and the reason is structural rather than an
// omission: a walk that emits every string leaf visits both values on both
// sides, where a walk that extracts one member has to choose. TestTheTwoWalksAgree
// carries the payload that says so.
func TestTheTwoJSONParsersDivergeOnADuplicatedKey(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"tool_input":{"command":"the first value","command":"the second value"}}`)

	if got := Derive(payload).Cmd; got != "the second value" {
		t.Errorf("Derive: got %q, want the last value %q", got, "the second value")
	}

	db := migrated(t)
	if got := sqlExtractText(t, db, payload, `$.tool_input.command`); got != "the first value" {
		t.Errorf("json_extract: got %q, want the first value %q", got, "the first value")
	}
}

// rawInvalidByte builds a payload carrying a byte that cannot begin a UTF-8
// sequence, from its bytes so that nothing in the editing path decodes it on the
// way in.
func rawInvalidByte() []byte {
	out := append([]byte(`{"tool_input":{"command":"a`), 0xFF)
	return append(out, []byte(`b"}}`)...)
}

// sqlExtractText asks SQLite for one member of payload through the same
// json_type guard and json_extract that migration 00005's backfill uses, so what
// it reports is what the backfill would have written.
func sqlExtractText(t *testing.T, db *sql.DB, payload []byte, path string) string {
	t.Helper()
	var got string
	err := db.QueryRowContext(t.Context(), `
		SELECT CASE
		    WHEN json_type(?, ?) = 'text' THEN json_extract(?, ?)
		    ELSE ''
		END`, string(payload), path, string(payload), path).Scan(&got)
	if err != nil {
		t.Fatalf("json_extract %s: %v", path, err)
	}
	return got
}
