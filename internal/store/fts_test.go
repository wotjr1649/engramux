package store

import (
	"database/sql"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/fixtures"
)

// sqliteCorruptVTab is SQLITE_CORRUPT_VTAB, `SQLITE_CORRUPT | (1<<8)`. It is
// what FTS5's `integrity-check` raises when the index and its content table
// disagree - the same extended-code assertion migrate_test.go makes about the
// constraint codes, and for the same reason: "some error" cannot tell a
// desynced index from a typo in the table name.
const sqliteCorruptVTab = 267

// Tokens that occur in exactly one Phase 1 fixture each, so a MATCH for one is
// an assertion about which document was found and not merely that something
// was. Each is a camelCase identifier inside that fixture's tool_input.command;
// unicode61 keeps it as one token, so the query and the document tokenize the
// same way.
// gosec's G101 fires on these: identifiers ending in Token that hold a string literal. They are
// camelCase command names out of the fixtures and not credentials.
//
//nolint:gosec // G101: false positive, see above
const (
	arrayOnlyToken  = "renderFixtureList"  // codex-posttooluse-array.json
	stringOnlyToken = "parseFixtureRow"    // codex-posttooluse-string.json
	objectOnlyToken = "fixtureCacheWarmup" // claude-code-posttooluse-object.json
)

// integrityCheck runs FTS5's own consistency check over events_fts and returns
// what it said.
//
// rank is a parameter because the difference between the two values is the
// point: with rank=1 the check reads the content table and compares it against
// the index, and with rank=0 - which is also what the no-argument form does -
// it only verifies the index is internally well formed, which a desynced index
// still is. [TestTheIntegrityCheckCatchesADroppedTrigger] asserts both.
func integrityCheck(t *testing.T, db *sql.DB, rank int) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO events_fts(events_fts, rank) VALUES('integrity-check', ?)`, rank)
	return err
}

// matchRowids returns the events rowids matching query, in rank order.
func matchRowids(t *testing.T, db *sql.DB, query string) []int64 {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT rowid FROM events_fts WHERE events_fts MATCH ? ORDER BY rank`, query)
	if err != nil {
		t.Fatalf("MATCH %q: %v", query, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close: %v", err)
		}
	}()
	var got []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return got
}

// requireMatch asserts query matches exactly the rowids in want, in any order.
func requireMatch(t *testing.T, db *sql.DB, query string, want []int64, what string) {
	t.Helper()
	got := matchRowids(t, db, query)
	slices.Sort(got)
	sorted := slices.Clone(want)
	slices.Sort(sorted)
	if !slices.Equal(got, sorted) {
		t.Fatalf("%s: MATCH %q = rowids %v, want %v", what, query, got, sorted)
	}
}

// ftsID is the event id the nth fixture ingest of these tests uses.
func ftsID(n int) string { return fmt.Sprintf("fts-%06d", n) }

// ingestFixtures stores all four Phase 1 fixtures through the production path
// and returns each one's events rowid, keyed by fixture file name. The rowid is
// the join key an external-content FTS5 table indexes by, so it is what these
// tests assert on rather than events.id.
func ingestFixtures(t *testing.T, db *sql.DB) map[string]int64 {
	t.Helper()
	now := time.Now()
	rowids := make(map[string]int64, len(fixtures.All()))
	for i, f := range fixtures.All() {
		b, err := f.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", f.File, err)
		}
		status, err := Ingest(t.Context(), db, ingestEnv(ftsID(i), b), SourcePipe, now)
		requireCommitted(t, status, err, "ingest "+f.File)
		rowids[f.File] = rowidOf(t, db, ftsID(i))
	}
	return rowids
}

// rowidOf returns the implicit rowid of the events row with this id. events has
// a TEXT primary key under STRICT and is therefore an ordinary rowid table,
// which is what lets an external-content index address it at all.
func rowidOf(t *testing.T, db *sql.DB, id string) int64 {
	t.Helper()
	var rowid int64
	if err := db.QueryRowContext(t.Context(), `SELECT rowid FROM events WHERE id = ?`, id).
		Scan(&rowid); err != nil {
		t.Fatalf("rowid of %q: %v", id, err)
	}
	return rowid
}

// TestTheIntegrityCheckPassesOnAFreshlyIndexedDatabase is the Phase 4 schema
// gate: `integrity-check` with rank=1, which is the form that compares the
// index against the content table.
//
// The MATCH beside it is not decoration. An empty index passes an
// integrity-check by itself - it agrees with a content table it never read -
// so the check only means something next to an assertion that a token from one
// document finds that document and no other.
func TestTheIntegrityCheckPassesOnAFreshlyIndexedDatabase(t *testing.T) {
	db := migrated(t)
	rowids := ingestFixtures(t, db)

	if err := integrityCheck(t, db, 1); err != nil {
		t.Fatalf("integrity-check rank=1 on a freshly indexed database: %v", err)
	}
	requireMatch(t, db, arrayOnlyToken,
		[]int64{rowids[fixtures.CodexPostToolUseArray]}, "after ingest")
	requireMatch(t, db, stringOnlyToken,
		[]int64{rowids[fixtures.CodexPostToolUseString]}, "after ingest")
	requireMatch(t, db, objectOnlyToken,
		[]int64{rowids[fixtures.ClaudePostToolUseObject]}, "after ingest")
}

// TestANewlineIsNotAPhraseBoundary measures what [leafSeparator] buys and what
// it does not. Both halves were claimed in a doc comment and neither had been
// measured; the second one was false.
//
// Four rows, differing only in what sits between the same two tokens, and the
// queries are run against the index rather than against a belief about
// unicode61:
//
//   - nothing between them, and they are one token that is neither. That is what
//     the separator is for.
//   - a newline, which is what [Leaves] writes.
//   - a space, the counterfactual the old comment ruled out.
//   - a third token before the newline, which is the only thing that actually
//     stops the phrase.
//
// The newline row and the space row answer a phrase query identically, and
// identically again under NEAR/0, which asks the same question about token
// positions directly. So the newline is not a phrase boundary: unicode61 drops
// it exactly as it drops a space and the tokens are adjacent by position across
// the join. The third row is the control that says the phrase query is doing
// anything at all.
func TestANewlineIsNotAPhraseBoundary(t *testing.T) {
	db := migrated(t)
	seed(t, db)

	const first, second, between = "alphaPhraseOne", "betaPhraseTwo", "gammaPhraseMid"
	rows := []struct{ name, leaves string }{
		{"no separator at all", first + second},
		{"the newline Leaves writes", first + "\n" + second},
		{"a space instead", first + " " + second},
		{"a third token before the newline", first + " " + between + "\n" + second},
	}
	rowids := make([]int64, len(rows))
	for i, r := range rows {
		rowids[i] = insertSeededLeaves(t, db, fmt.Sprintf("phrase-%d", i), r.leaves, int64(4000+i))
	}
	separated := []int64{rowids[1], rowids[2], rowids[3]}
	spanned := []int64{rowids[1], rowids[2]}

	// What the separator is for: with nothing between them the two words are
	// a third token, so neither of them finds that row and the fused token
	// finds nothing else.
	requireMatch(t, db, first, separated, "the first token alone")
	requireMatch(t, db, second, separated, "the second token alone")
	requireMatch(t, db, first+second, []int64{rowids[0]}, "the token the two fused into")

	// What it is not: a phrase boundary. The newline row and the space row
	// answer the same, and only a token actually standing between them ends
	// the phrase.
	requireMatch(t, db, `"`+first+` `+second+`"`, spanned, "a phrase query across the separator")
	requireMatch(t, db, fmt.Sprintf(`NEAR("%s" "%s", 0)`, first, second), spanned,
		"NEAR/0 across the separator")
}

// TestTheIndexSecureDeletes. The index holds the same unredacted text the table
// does, and FTS5 has its own switch for it: without this, a deleted row leaves
// recoverable entries in the index until an `optimize` runs, and nothing runs
// one (spec 5.7). The setting is persistent, written into the table's %_config
// shadow table at migration time, so this reads it back rather than trusting
// that the statement was issued.
func TestTheIndexSecureDeletes(t *testing.T) {
	db := migrated(t)
	var v int64
	if err := db.QueryRowContext(t.Context(),
		`SELECT v FROM events_fts_config WHERE k = 'secure-delete'`).Scan(&v); err != nil {
		t.Fatalf("read events_fts_config['secure-delete']: %v", err)
	}
	if v != 1 {
		t.Fatalf("events_fts secure-delete = %d, want 1", v)
	}
}

// TestTheIntegrityCheckCatchesADroppedTrigger is why the gate carries rank=1.
//
// The index is desynced deliberately - the AFTER INSERT trigger is dropped and
// one more event ingested - and then all three forms of the check are run over
// the same database. rank=1 is the only one that fails. The two that pass are
// the control, and they are asserted rather than described: without them a
// later "simplification" to the short form would look harmless, because the
// short form also returns nil on an index that is correct.
func TestTheIntegrityCheckCatchesADroppedTrigger(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	ingestFixtures(t, db)

	if _, err := db.ExecContext(ctx, `DROP TRIGGER events_ai`); err != nil {
		t.Fatalf("DROP TRIGGER events_ai: %v", err)
	}

	// A fifth event, carrying a token none of the fixtures do, so that the
	// row missing from the index is identifiable by more than a count.
	const desyncToken = "desyncedAfterTheTriggerWent"
	payload := payloadOf(t, map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "00000000-0000-4000-8000-000000000005",
		"cwd":             `C:\Users\fixture\workspace\fixture-project`,
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "echo " + desyncToken},
	})
	status, err := Ingest(ctx, db, ingestEnv(ftsID(4), payload), SourcePipe, time.Now())
	requireCommitted(t, status, err, "ingest past the dropped trigger")

	requireSQLiteCode(t, integrityCheck(t, db, 1), sqliteCorruptVTab,
		"integrity-check rank=1 over a desynced index")
	requireMatch(t, db, desyncToken, nil, "the row the dropped trigger never indexed")

	// The control. Both of these run over the same desynced index and both
	// report nothing wrong, which is the whole reason the gate is the third
	// form and not one of these.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO events_fts(events_fts) VALUES('integrity-check')`); err != nil {
		t.Errorf("the no-argument integrity-check reported %v over a desynced index, want nil - "+
			"if this form now catches it, the gate can be simplified", err)
	}
	if err := integrityCheck(t, db, 0); err != nil {
		t.Errorf("integrity-check rank=0 reported %v over a desynced index, want nil - "+
			"if this form now catches it, the gate can be simplified", err)
	}
}

// TestTheMigrationIndexesRowsThatWereAlreadyThere is the live installation's
// upgrade path: about 1,800 events were captured before the index existed, and
// the migration's own `rebuild` is what indexes them. Nothing else ever will -
// the triggers only see rows written after they are created.
//
// The database is built at version 1 through the package-private provider,
// filled, and only then brought the rest of the way up.
func TestTheMigrationIndexesRowsThatWereAlreadyThere(t *testing.T) {
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
	// Straight into events rather than through Ingest, because at version 1
	// there is no leaves column for Ingest to fill and it would fail on the
	// way in. That is the situation this test is about: these rows predate
	// the column as well as the index, so the migration's backfill and its
	// rebuild are the only things that will ever reach them.
	seed(t, db)
	rowids := make(map[string]int64, len(fixtures.All()))
	for i, f := range fixtures.All() {
		b, err := f.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", f.File, err)
		}
		rowids[f.File] = rowidOf(t, db, insertAtVersionOne(t, db, i, b))
	}

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate the rest of the way up: %v", err)
	}

	if err := integrityCheck(t, db, 1); err != nil {
		t.Fatalf("integrity-check rank=1 after indexing pre-existing rows: %v", err)
	}
	requireMatch(t, db, arrayOnlyToken,
		[]int64{rowids[fixtures.CodexPostToolUseArray]}, "a row that predates the index")
	requireMatch(t, db, objectOnlyToken,
		[]int64{rowids[fixtures.ClaudePostToolUseObject]}, "a row that predates the index")
}

// TestACascadingDeleteKeepsTheIndexConsistent. Deleting a project cascades to
// events (spec 6), and a cascade is not a DELETE statement against events - it
// is SQLite removing the child rows itself. That row triggers fire on it is
// documented, and documented is not measured: without this test the AFTER
// DELETE trigger is only known to work on the deletes a test writes by hand,
// and project purge is the one path that will ever produce the other kind.
func TestACascadingDeleteKeepsTheIndexConsistent(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	rowids := ingestFixtures(t, db)
	requireMatch(t, db, arrayOnlyToken,
		[]int64{rowids[fixtures.CodexPostToolUseArray]}, "before the cascade")

	res, err := db.ExecContext(ctx, `DELETE FROM projects`)
	if err != nil {
		t.Fatalf("DELETE FROM projects: %v", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("DELETE FROM projects affected %d rows (err %v), want 1", n, err)
	}
	requireCount(t, db, "events", 0)

	if err := integrityCheck(t, db, 1); err != nil {
		t.Fatalf("integrity-check rank=1 after a cascading delete: %v", err)
	}
	requireMatch(t, db, arrayOnlyToken, nil, "after the cascade")
	requireMatch(t, db, objectOnlyToken, nil, "after the cascade")
}

// TestAnUpdateKeepsTheIndexConsistent is the AFTER UPDATE trigger, and the one
// the FTS5 documentation's recipe is most easily got wrong: a plain delete on
// an external-content index reads the old values back from the content table,
// where an AFTER trigger finds the new ones. The old token still matching would
// be that bug, and integrity-check would still pass, so both are asserted.
//
// Nothing in 1.0 updates events. The trigger exists because the index would be
// silently wrong the first time something does.
//
// leaves is written alongside payload, which is what any future updater has to
// do: it is a derived column and no trigger recomputes it, so an update that
// touched payload alone would leave the index correct about leaves and both of
// them stale about the payload. [Leaves] is called rather than a literal, so
// the test and the ingest path derive it the same way.
func TestAnUpdateKeepsTheIndexConsistent(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	rowids := ingestFixtures(t, db)
	target := rowids[fixtures.CodexPostToolUseString]

	const afterToken = "rewrittenByAnUpdateTrigger"
	after := []byte(`{"hook_event_name":"PostToolUse","note":"` + afterToken + `"}`)
	if _, err := db.ExecContext(ctx, `UPDATE events SET payload = ?, leaves = ? WHERE rowid = ?`,
		string(after), Leaves(after), target); err != nil {
		t.Fatalf("UPDATE events: %v", err)
	}

	if err := integrityCheck(t, db, 1); err != nil {
		t.Fatalf("integrity-check rank=1 after an update: %v", err)
	}
	requireMatch(t, db, stringOnlyToken, nil, "the payload the update replaced")
	requireMatch(t, db, afterToken, []int64{target}, "the payload the update wrote")
}
