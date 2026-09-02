package store

import (
	"database/sql"
	"testing"
)

// Two places where `encoding/json` and SQLite's JSON parser answer differently
// for the same bytes, found on 2026-09-03 by a review that asked whether they
// could and then measured instead of reasoning.
//
// # Why they are pinned here and not fixed
//
// Every derived column in this package is written twice - once in Go on the way
// in, once in SQL by the migration that backfills the rows predating it - and
// TestTheTwoWalksAgree and TestTheTwoDerivedWalksAgree exist because a
// divergence is otherwise silent. These two shapes are the divergence those
// tests were built to find, and neither is fixable by making one side match the
// other:
//
//   - **Go is right about the surrogate.** An unpaired surrogate is not a
//     character, and substituting U+FFFD is what produces valid UTF-8. SQLite
//     emits the WTF-8 encoding of the code point instead. Making Go match
//     SQLite would mean writing our own string unescaper and deliberately
//     putting invalid UTF-8 in the database; making SQLite match Go is not
//     ours to do.
//   - **Neither is right about the duplicated key**, because JSON does not say.
//     Go's decoder takes the last, SQLite's json_extract takes the first, and
//     both are defensible readings of a document nobody should have written.
//
// So the choice is which wrongness to carry, and that is a design decision
// rather than a repair. Backlog **39** and **40** carry them. What this file
// does is make the current behaviour a fact the suite asserts, so that a
// dependency upgrade or a rewrite of either walk cannot change it quietly - and
// so that whoever takes the decision is arguing with an assertion rather than a
// memory.
//
// # What each one costs today
//
// The surrogate one is the older and the worse: it splits [Leaves], which is
// what `events_fts` is built over, so an event ingested live and the same event
// backfilled by migration 00002 are indexed differently. It has been shipped
// since 00002. The duplicated key splits only [Derive], whose columns are a
// ranking input that nothing reads out, and it arrived with migration 00005.
// Neither is reachable from either host's own encoder; the surrogate is
// reachable from a tool's output, because Windows permits an unpaired surrogate
// in a file name.

// TestTheTwoJSONParsersDivergeOnALoneSurrogate pins the older of the two, on
// both walks.
//
// The assertion is on the exact bytes each side produces, not on the fact that
// they differ. "They differ" would still pass if both sides changed to two new
// and equally wrong answers.
func TestTheTwoJSONParsersDivergeOnALoneSurrogate(t *testing.T) {
	t.Parallel()

	// U+FFFD is what encoding/json substitutes for the unpaired surrogate.
	const goSide = "lone � surrogate"
	// The WTF-8 encoding of U+D800, which is what SQLite hands back. Written
	// from its bytes because it is not valid UTF-8 and no Go string literal
	// spells it without an escape this file cannot carry.
	sqlSide := "lone " + string([]byte{0xED, 0xA0, 0x80}) + " surrogate"

	if goSide == sqlSide {
		t.Fatal("the two expected values are equal; this test compares nothing")
	}

	payload := loneSurrogate()

	if got := Derive(payload).Cmd; got != goSide {
		t.Errorf("Derive: got %q, want the U+FFFD substitution %q", got, goSide)
	}
	if got := Leaves(payload); got != goSide {
		t.Errorf("Leaves: got %q, want the U+FFFD substitution %q", got, goSide)
	}

	db := migrated(t)
	if got := sqlExtractText(t, db, payload, `$.tool_input.command`); got != sqlSide {
		t.Errorf("json_extract: got %q, want the WTF-8 bytes %q", got, sqlSide)
	}
}

// TestTheTwoJSONParsersDivergeOnARawInvalidByte is the same defect reached
// through a different door, named separately because the mechanism is not the
// same: above it is an escape naming half a surrogate pair, here it is a byte
// that is not valid UTF-8 sitting raw inside a JSON string. Neither validator
// rejects it - Go's scanner and SQLite's json_valid both check structure rather
// than the encoding of string contents - so [sqliteWillParse] cannot remove the
// difference either.
//
// It belongs to backlog 39 with the surrogate, and it was named by the review
// that found the other two rather than by this file's author.
func TestTheTwoJSONParsersDivergeOnARawInvalidByte(t *testing.T) {
	t.Parallel()

	// 0xFF cannot begin a UTF-8 sequence. Built from bytes for the same
	// reason the surrogate is: it does not survive being a literal.
	payload := append([]byte(`{"tool_input":{"command":"a`), 0xFF)
	payload = append(payload, []byte(`b"}}`)...)

	const goSide = "a�b"
	sqlSide := "a" + string([]byte{0xFF}) + "b"
	if goSide == sqlSide {
		t.Fatal("the two expected values are equal; this test compares nothing")
	}

	if got := Derive(payload).Cmd; got != goSide {
		t.Errorf("Derive: got %q, want the U+FFFD substitution %q", got, goSide)
	}
	db := migrated(t)
	if got := sqlExtractText(t, db, payload, `$.tool_input.command`); got != sqlSide {
		t.Errorf("json_extract: got %q, want the raw byte preserved %q", got, sqlSide)
	}
}

// TestTheTwoJSONParsersDivergeOnADuplicatedKey pins the newer one.
//
// It is asserted on [Derive] and on json_extract rather than on [Leaves],
// because [Leaves] does not diverge here and TestTheTwoWalksAgree already
// carries the shape that says so: a walk that emits every string leaf visits
// both values on both sides, where a walk that extracts one member has to
// choose.
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

// sqlExtractText asks SQLite for one member of payload through the same
// json_type guard and json_extract that migration 00005's backfill uses, so
// what it reports is what the backfill would have written.
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
