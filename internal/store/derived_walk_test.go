package store

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/wotjr1649/engramux/internal/fixtures"
)

// derivedPayloads are the shapes that separate [Derive] from migration 00005's
// backfill, on top of the JSON-validity shapes [agreementPayloads] already
// carries. Every one of them is a place where a json_extract without its
// json_type guard, or a COALESCE written as a join, would put different bytes in
// a column on the two sides.
var derivedPayloads = []namedPayload{
	{"a command and nothing else", []byte(`{"tool_input":{"command":"go test -p 1 ./..."}}`)},
	{"a command that is a number", []byte(`{"tool_input":{"command":42}}`)},
	{"a command that is an object", []byte(`{"tool_input":{"command":{"argv":["go","test"]}}}`)},
	{"a command that is an array", []byte(`{"tool_input":{"command":["go","test"]}}`)},
	{"an empty command", []byte(`{"tool_input":{"command":""}}`)},
	{"a tool_input that is not an object", []byte(`{"tool_input":"go test"}`)},
	{"an input path only", []byte(`{"tool_input":{"file_path":"C:/x/a.go"}}`)},
	{"a response path only", []byte(`{"tool_response":{"filePath":"C:/x/b.go"}}`)},
	{"both paths, which name the same file", []byte(`{"tool_input":{"file_path":"C:/x/a.go"},"tool_response":{"filePath":"C:/x/a.go"}}`)},
	{"both paths disagreeing, so precedence decides", []byte(`{"tool_input":{"file_path":"C:/x/a.go"},"tool_response":{"filePath":"C:/x/b.go"}}`)},
	{"an empty input path falling through to the response path", []byte(`{"tool_input":{"file_path":""},"tool_response":{"filePath":"C:/x/b.go"}}`)},
	{"stdout only", []byte(`{"tool_response":{"stdout":"ok\ndone","stderr":"","interrupted":false}}`)},
	{"stdout and content together", []byte(`{"tool_response":{"stdout":"from stdout","content":"from content"}}`)},
	{"an empty stdout falling through to content", []byte(`{"tool_response":{"stdout":"","content":"from content"}}`)},
	{"a response that is itself a string", []byte(`{"tool_response":"the whole answer"}`)},
	{"a response object with neither field", []byte(`{"tool_response":{"userModified":false,"structuredPatch":[]}}`)},
	{"a response that is an array", []byte(`{"tool_response":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}`)},
	{"a content that is an array", []byte(`{"tool_response":{"content":[{"type":"text","text":"a"}]}}`)},
	{"all three columns at once", []byte(`{"tool_input":{"command":"cat x.go","file_path":"C:/x/x.go"},"tool_response":{"stdout":"package x"}}`)},
	{"a shallow command beside a nesting SQLite refuses", deepBesideACommand(sqliteJSONDepthLimit + 1)},
	{"a shallow command beside a nesting SQLite accepts", deepBesideACommand(sqliteJSONDepthLimit - 4)},
	{"text that looks like an error, in stdout", []byte(`{"tool_response":{"stdout":"error: cannot find package\nexit status 1"}}`)},

	// Added 2026-09-03, when a review asked whether the shapes where
	// encoding/json and SQLite's JSON parser each have an opinion could split
	// these two walks. A NUL does not, and that is the answer rather than the
	// absence of one. The two that do are not here: they are pinned in
	// TestTheTwoJSONParsersDivergeOnADuplicatedKey and
	// TestTheTwoJSONParsersDivergeOnALoneSurrogate, with the reason each is
	// pinned rather than fixed.
	//
	// Neither is written as a literal, and that is not style: a NUL survives
	// being typed into this file as a real NUL, which is an illegal character
	// in Go source. It was tried the other way first and the file stopped
	// compiling.
	{"an escaped NUL inside the command", escapedNUL("tool_input", "command")},
	{"an escaped NUL in the output column, the one that carries whole tool outputs",
		escapedNUL("tool_response", "stdout")},

	// [agreementPayloads] already carries a number too large for float64
	// between two leaves, and it did not catch this: that payload has no
	// tool_input, so both sides answered the zero value for different reasons
	// and the comparison passed on a coincidence. The number has to sit beside
	// something derivable for the difference to be visible, which is what this
	// payload is. Raised by a review, 2026-09-03.
	{"a number too large for float64 beside a derivable command",
		[]byte(`{"tool_input":{"command":"go test"},"n":1e400}`)},
	{"a number too large for float64 beside a derivable path and output",
		[]byte(`{"tool_input":{"file_path":"C:/x/a.go"},"tool_response":{"stdout":"ok"},"n":-1e400}`)},
}

// escapedNUL builds a payload whose one value carries a NUL, and lets
// encoding/json write the escape rather than this file spelling it.
func escapedNUL(outer, inner string) []byte {
	b, err := json.Marshal(map[string]any{
		outer: map[string]any{inner: "before" + string(rune(0)) + "after"},
	})
	if err != nil { // two strings and two maps; there is nothing here to fail
		panic(err)
	}
	return b
}

// loneSurrogate builds a payload carrying an escape that names half a surrogate
// pair. encoding/json cannot be asked to emit one - it substitutes U+FFFD - so
// the escape is assembled from its code points, which is also what keeps
// anything in the editing path from decoding it on the way in.
func loneSurrogate() []byte {
	esc := string(rune(0x5C)) + "ud800"
	return []byte(`{"tool_input":{"command":"lone ` + esc + ` surrogate"}}`)
}

// deepBesideACommand is the shape that made [sqliteWillParse] necessary: a
// command at depth two, which Go reads without difficulty, beside a sibling
// nested deeply enough that json_valid answers 0 for the whole payload and the
// backfill skips the row.
func deepBesideACommand(depth int) []byte {
	head := []byte(`{"tool_input":{"command":"go test"},"deep":`)
	body := nestedJSON(depth, "[")
	return append(append(head, body...), '}')
}

// TestTheTwoDerivedWalksAgree is to migration 00005 what TestTheTwoWalksAgree is
// to 00002, and it exists for the same reason at one more column's remove.
// [Derive] fills the three columns on the way in; the migration's backfill fills
// them for every row that was already there. If they disagree, the live
// installation's 17,043 captured events rank differently from every event after
// them - and nothing else would say so, because a ranking input has no integrity
// check and a wrong boost looks exactly like a boost that did not help.
//
// It is run as the upgrade path rather than as a comparison of two expressions:
// the database is built at version 1, filled, and only then migrated, so what is
// read back is what the migration's own statement wrote.
func TestTheTwoDerivedWalksAgree(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, dbPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)

	p, err := provider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatalf("UpTo(1): %v", err)
	}
	seed(t, db)

	want := make(map[string]namedPayload)
	for _, f := range fixtures.All() {
		b, err := f.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", f.File, err)
		}
		want[insertAtVersionOne(t, db, len(want), b)] = namedPayload{f.File, b}
	}
	for _, np := range derivedPayloads {
		want[insertAtVersionOne(t, db, len(want), np.payload)] = np
	}
	for _, np := range agreementPayloads {
		want[insertAtVersionOne(t, db, len(want), np.payload)] = np
	}
	corpus := corpusPayloads(t)
	for _, np := range corpus {
		want[insertAtVersionOne(t, db, len(want), np.payload)] = np
	}
	t.Logf("comparing %d payloads over three columns: %d fixtures, %d derived shapes, %d validity shapes, %d corpus captures",
		len(want), len(fixtures.All()), len(derivedPayloads), len(agreementPayloads), len(corpus))

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate the rest of the way up: %v", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT id, payload, derived_cmd, derived_paths, derived_output FROM events`)
	if err != nil {
		t.Fatalf("read back the backfilled columns: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close: %v", err)
		}
	}()
	var compared, nonEmpty int
	for rows.Next() {
		var id, payload string
		var cmd, paths, output sql.NullString
		if err := rows.Scan(&id, &payload, &cmd, &paths, &output); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		np, ok := want[id]
		if !ok {
			np = namedPayload{name: "the seed row", payload: []byte(payload)}
		}
		// NOT NULL DEFAULT '' means a NULL here is the column having
		// gone missing, which is a different failure from the backfill
		// having written the wrong text.
		if !cmd.Valid || !paths.Valid || !output.Valid {
			t.Fatalf("%s: a derived column is NULL after the migration", np.name)
		}
		got := Derive(np.payload)
		sqlSide := Derived{Cmd: cmd.String, Paths: paths.String, Output: output.String}
		if got != sqlSide {
			t.Errorf("%s: the two walks disagree\n  Go:  %#v\n  SQL: %#v", np.name, got, sqlSide)
		}
		if got != (Derived{}) {
			nonEmpty++
		}
		compared++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if compared != len(want)+1 { // +1 for seed's own events row
		t.Fatalf("compared %d rows, want %d", compared, len(want)+1)
	}
	// A backfill that wrote the empty string into every row would agree with
	// a Go walk that also did, and the test above would pass having compared
	// nothing. The corpus alone carries 534 commands, so this bound is far
	// below what a working pair produces and far above what a broken one does.
	if nonEmpty == 0 {
		t.Fatalf("every one of %d rows derived to the zero value; the pair agrees about nothing", compared)
	}
	t.Logf("%d of %d rows derived something on both sides", nonEmpty, compared)
}

// TestIngestWritesTheDerivedColumns is the third walk, and it is a third walk
// rather than a restatement of the first two. TestTheTwoDerivedWalksAgree
// compares [Derive] against the migration's backfill over rows that were already
// there; this compares what [Ingest] actually put in the row against what
// [Derive] answers for the same bytes. A migration that backfills perfectly and
// an insert that never binds the columns produce a database whose old events
// rank and whose new ones do not, which is the failure with the longest fuse:
// it appears only as the boost quietly ceasing to apply to anything recent.
func TestIngestWritesTheDerivedColumns(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, dbPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var checked int
	for i, np := range derivedPayloads {
		want := Derive(np.payload)
		ingestOne(t, db, i, np.payload)
		var got Derived
		if err := db.QueryRowContext(ctx,
			`SELECT derived_cmd, derived_paths, derived_output FROM events WHERE id = ?`, ckptID(i),
		).Scan(&got.Cmd, &got.Paths, &got.Output); err != nil {
			t.Fatalf("%s: read back: %v", np.name, err)
		}
		if got != want {
			t.Errorf("%s: Ingest stored\n  %#v\nwhere Derive answers\n  %#v", np.name, got, want)
		}
		checked++
	}
	if checked != len(derivedPayloads) {
		t.Fatalf("checked %d payloads, want %d", checked, len(derivedPayloads))
	}
}

// TestTheDerivedBackfillLeavesTheFTSIndexAlone is the other half of migration
// 00005's promise. The columns are a ranking input and never indexed text
// (memory spec M-2, decision 7), so the backfill must not touch events_fts -
// and it would, silently, through the external-content update trigger, if that
// trigger were left in place around an UPDATE of 17,043 rows.
//
// Two things are asserted and both are needed. integrity-check says the index
// and the table still agree, which catches a trigger that was dropped and not
// put back. And the trigger being present afterwards is what catches the
// opposite: a migration that left it dropped would pass integrity-check today
// and stop maintaining the index forever.
func TestTheDerivedBackfillLeavesTheFTSIndexAlone(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, dbPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)

	p, err := provider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatalf("UpTo(1): %v", err)
	}
	seed(t, db)
	for i, np := range derivedPayloads {
		insertAtVersionOne(t, db, i, np.payload)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// rank=1 is what 00002 set, so integrity-check compares the index
	// against the content table rather than only checking itself.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO events_fts(events_fts) VALUES('integrity-check')`); err != nil {
		t.Fatalf("integrity-check after the derived backfill: %v", err)
	}

	var triggers int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_schema WHERE type = 'trigger' AND name = 'events_au'`,
	).Scan(&triggers); err != nil {
		t.Fatalf("count events_au: %v", err)
	}
	if triggers != 1 {
		t.Fatalf("events_au is present %d times after the migration, want 1", triggers)
	}

	// And it has to still work: an UPDATE of leaves must reach the index.
	// A trigger that exists and was recreated with the wrong body would
	// pass the count above and fail this.
	var rowid int64
	if err := db.QueryRowContext(ctx, `SELECT rowid FROM events LIMIT 1`).Scan(&rowid); err != nil {
		t.Fatalf("pick a row: %v", err)
	}
	const marker = "zzqqxxderivedtriggerprobe"
	if _, err := db.ExecContext(ctx, `UPDATE events SET leaves = ? WHERE rowid = ?`, marker, rowid); err != nil {
		t.Fatalf("update leaves: %v", err)
	}
	var found int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM events_fts WHERE events_fts MATCH ?`, marker,
	).Scan(&found); err != nil {
		t.Fatalf("search for the probe: %v", err)
	}
	if found != 1 {
		t.Fatalf("the probe is in %d indexed documents after an UPDATE, want 1; events_au was recreated with the wrong body", found)
	}
}
