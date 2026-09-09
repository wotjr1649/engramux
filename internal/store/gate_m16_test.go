package store

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Gate M16 (memory spec 5): whether events.leaves can stop being a stored second
// copy of the payload's string text and become a VIRTUAL generated column.
//
// # What the question was, and where it was expected to fail
//
// events_fts indexes one column, leaves, and nothing else reads it - excerpt.go
// recomputes the same walk from the payload at read time - so the stored column
// is a duplicate measured at 36% of the file. The risk the spec named was FTS5:
// whether an external-content index would accept a generated column as the
// column it indexes, given that content='events' reads the content table on
// rebuild and integrity-check.
//
// That risk is not the one that bites, and this file measures both halves so the
// answer cannot be misattributed later. FTS5 accepts a virtual generated column,
// rebuilds over it and matches through it. SQLite refuses the column itself:
// **subqueries are prohibited in generated columns**, and the leaves walk cannot
// be written without one, because json_tree is a table-valued function that has
// to stand in a FROM clause and group_concat is an aggregate. The refusal is
// upstream of FTS5 and nothing about the index can move it.
//
// The measured message names the subquery, which is the first of three
// prohibitions the walk trips. sqlite.org/gencol.html, read 2026-09-09, names
// all three in one sentence: the expression "may not use subqueries, aggregate
// functions, window functions, or table-valued functions". So the walk is not
// one rule away from legal but three, which is why no rewriting reaches a legal
// form and why this gate pins the refusal rather than looking for one.
//
// # Why the refusal is pinned rather than merely recorded
//
// The measurement is a fact about SQLite 3.53.3 through modernc.org/sqlite
// v1.57.0. A driver that lifted the restriction would make M16 a live question
// again, and a comment in a spec would not notice. This goes red when it does,
// which is the notification.
//
// The walk is read out of the embedded migration rather than written out here,
// for [Tokenizer]'s reason: 00002 is the truth, and a copy in a test drifts
// silently once it has stopped being the same expression.
//
//	go test -p 1 -count=1 -run TestGateM16 -v ./internal/store/
func TestGateM16LeavesCannotBecomeAGeneratedColumn(t *testing.T) {
	walk := backfillExpression(t)
	if !strings.Contains(walk, "json_tree") || !strings.Contains(walk, "group_concat") {
		t.Fatalf("the expression taken from %s is not the leaves walk; the extraction has drifted",
			ftsMigration)
	}

	db := m16DB(t, filepath.Join(t.TempDir(), "m16.db"))
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE events (
		id      TEXT NOT NULL PRIMARY KEY,
		payload TEXT NOT NULL
	) STRICT`); err != nil {
		t.Fatalf("create the base table: %v", err)
	}

	// The first half: the walk, exactly as 00002 computes it, as a virtual
	// generated column.
	err := m16AddGenerated(t, db, "leaves", walk)
	if err == nil {
		t.Fatalf("SQLite accepted the leaves walk as a generated column. M16's blocker has been lifted "+
			"and the question is live again: re-measure it against %s and correct memory spec 5.",
			ftsMigration)
	}
	if !strings.Contains(err.Error(), m16Refusal) {
		t.Errorf("the walk was refused with %q, and M16's recorded reason is %q. A different refusal is "+
			"a different finding - re-measure it rather than widening this comparison.", err, m16Refusal)
	}
	t.Logf("M16: the leaves walk is refused as a generated column: %v", err)

	// The mechanism, not the expression: a bare subquery and json_tree on
	// its own are refused the same way, so no rewriting of the walk reaches
	// a legal form. Nothing in stock SQLite computes group_concat over
	// json_tree without a FROM clause.
	for _, tc := range []struct{ name, expr string }{
		{"a bare subquery", `(SELECT payload)`},
		{"json_tree alone", `(SELECT value FROM json_tree(payload) LIMIT 1)`},
	} {
		err := m16AddGenerated(t, db, "probe_"+strings.ReplaceAll(tc.name, " ", "_"), tc.expr)
		if err == nil || !strings.Contains(err.Error(), m16Refusal) {
			t.Errorf("%s as a generated column: %v, want %q. The refusal is meant to be the "+
				"mechanism rather than something about the walk in particular", tc.name, err, m16Refusal)
		}
	}

	// The second half, and the one the spec expected to be the blocker: an
	// external-content FTS5 index over a virtual generated column. It is
	// asked over an expression SQLite does accept, so that a refusal here
	// would be FTS5's and not the generated column's.
	if err := m16AddGenerated(t, db, "extracted", `json_extract(payload, '$.k')`); err != nil {
		t.Fatalf("a subquery-free generated column was refused too, so the arm below asks nothing: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE VIRTUAL TABLE events_fts USING fts5(
		extracted, content = 'events', tokenize = 'unicode61 remove_diacritics 2')`); err != nil {
		t.Fatalf("FTS5 external content over a virtual generated column: %v", err)
	}
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO events(id, payload) VALUES ('a', '{"k":"alpha beta"}')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO events_fts(events_fts) VALUES('rebuild')`); err != nil {
		t.Fatalf("rebuild over a virtual generated column: %v", err)
	}
	var n int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM events_fts WHERE events_fts MATCH 'alpha'`).Scan(&n); err != nil {
		t.Fatalf("match through the index: %v", err)
	}
	if n != 1 {
		t.Fatalf("the index over a virtual generated column matched %d rows, want 1; the rebuild read "+
			"nothing out of the generated column", n)
	}
	t.Log("M16: FTS5 external content accepts a virtual generated column, rebuilds over it and matches " +
		"through it - so the refusal above is SQLite's and not the index's")
}

// m16Refusal is the message SQLite answers with, measured 2026-09-09 against
// SQLite 3.53.3 through modernc.org/sqlite v1.57.0. It is compared as a
// substring because the driver wraps it with a code and a table name.
const m16Refusal = "subqueries prohibited in generated columns"

// m16Snapshot is a copy of the installed database, if one has been taken. The
// refusal above is a parse-time property of the expression and cannot depend on
// the data, but the table it was asked over is not the installed one - so
// [TestGateM16OverTheInstalledSchema] asks it again over the real schema, with
// its twelve columns, its index and its triggers.
var m16Snapshot = filepath.Join("..", "..", ".capture", "m15", "m16.db")

// TestGateM16OverTheInstalledSchema repeats M16's refusal against a copy of the
// installed database, so the finding does not rest on a two-column table a test
// built.
//
// A refused ALTER writes nothing, and this runs against a copy either way. It
// skips when no copy has been taken.
func TestGateM16OverTheInstalledSchema(t *testing.T) {
	if _, err := os.Stat(m16Snapshot); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no snapshot at %s; stop the service and copy the .db and .db-wal pair", m16Snapshot)
	} else if err != nil {
		t.Fatalf("stat the snapshot: %v", err)
	}

	db := m16DB(t, m16Snapshot)
	var events int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM events`).Scan(&events); err != nil {
		t.Fatalf("count the snapshot's events: %v", err)
	}

	err := m16AddGenerated(t, db, "leaves_generated", backfillExpression(t))
	if err == nil {
		t.Fatalf("the installed schema accepted the leaves walk as a generated column, and a temporary "+
			"table did not. M16 is live again over %d events.", events)
	}
	if !strings.Contains(err.Error(), m16Refusal) {
		t.Errorf("over the installed schema the walk was refused with %q, want %q", err, m16Refusal)
	}
	t.Logf("M16: over the installed schema and its %d events, the same refusal: %v", events, err)
}

// m16DB opens a database and closes it when the test ends. The WAL sidecar keeps
// a handle too, which is what makes t.TempDir cleanup fail on Windows.
func m16DB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open %s: %v", filepath.Base(path), err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	return db
}

// m16AddGenerated adds one VIRTUAL generated column and returns what SQLite said.
//
// STORED is not asked for and could not be: SQLite's ALTER TABLE ADD COLUMN
// accepts a VIRTUAL generated column and refuses a STORED one, and a stored
// column would be the duplicate M16 exists to remove anyway.
//
// The name and the expression are interpolated because neither can be a bind
// parameter in DDL; both are this file's own constants.
func m16AddGenerated(t *testing.T, db *sql.DB, name, expr string) error {
	t.Helper()
	//nolint:gosec // G202: name and expr are this test's literals, never data
	_, err := db.ExecContext(t.Context(),
		`ALTER TABLE events ADD COLUMN `+name+` TEXT GENERATED ALWAYS AS (`+expr+`) VIRTUAL`)
	return err
}

// backfillCase cuts 00002's backfill expression out of the migration: everything
// between `SET leaves = ` and the `END` that closes it. There is one CASE in
// that statement and it is not nested, so the first END closes it.
var backfillCase = regexp.MustCompile(`(?s)SET leaves = (CASE.*?\nEND)`)

// backfillExpression is the SQL twin of [Leaves], taken from the migration that
// owns it.
func backfillExpression(t *testing.T) string {
	t.Helper()
	file, err := fs.ReadFile(migrationFiles, migrationDir+"/"+ftsMigration)
	if err != nil {
		t.Fatalf("read the embedded migration: %v", err)
	}
	m := backfillCase.FindStringSubmatch(string(file))
	if m == nil {
		t.Fatalf("%s no longer carries a `SET leaves = CASE ... END` backfill; this gate reads the walk "+
			"out of the migration and has nothing to read", ftsMigration)
	}
	return m[1]
}
