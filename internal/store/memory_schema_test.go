package store

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// TestMemoryItemsCarriesTheShapeTheReadingAsked. Migration 00004 replaces the
// Phase 1 memory_items, and the memory spec rev.2's M-2 decision 5 says exactly
// why the old one could not be kept: no host column, so UNIQUE (project_id, key)
// makes the two hosts collide on a key they share, and project_id NOT NULL with
// a foreign key into projects, which neither host's memory can satisfy - Claude
// Code keys on a git root this database may have no row for and Codex carries a
// per-entry cwd no event ever came from.
//
// The whole column list is compared rather than the columns this test happens to
// care about, for the reason TestEventsFTSCarriesExactlyTheDecidedOptions gives:
// a subset check cannot assert that a column is absent, and the columns that are
// absent here are the point.
func TestMemoryItemsCarriesTheShapeTheReadingAsked(t *testing.T) {
	db := migrated(t)
	rows, err := db.QueryContext(t.Context(), `SELECT name, type, "notnull" FROM pragma_table_info('memory_items')`)
	if err != nil {
		t.Fatalf("pragma_table_info(memory_items): %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var name, typ string
		var notNull int
		if err := rows.Scan(&name, &typ, &notNull); err != nil {
			t.Fatalf("scan a column: %v", err)
		}
		got = append(got, fmt.Sprintf("%s %s notnull=%d", name, typ, notNull))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the columns: %v", err)
	}
	want := []string{
		"id TEXT notnull=1",
		"host TEXT notnull=1",
		"kind TEXT notnull=1",
		"source_path TEXT notnull=1",
		"entry_key TEXT notnull=1",
		"project_path TEXT notnull=1",
		"project_id TEXT notnull=0",
		"title TEXT notnull=1",
		"body TEXT notnull=1",
		"host_modified_at INTEGER notnull=0",
		"privacy_class TEXT notnull=1",
		"redaction_version INTEGER notnull=1",
		"indexed_at INTEGER notnull=1",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("memory_items columns =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestMemoryItemsIsKeyedByTheThingThatIdentifiesAnItem. The uniqueness that
// matters is the host, the file it came from and the block within that file -
// which is what a re-scan has to collide on for the upsert to be an update
// rather than a second row. project_id is explicitly not part of it: a memory
// item is scoped by the path the host wrote (M-2 decision 8), and the foreign
// key is a convenience that is empty whenever no projects row happens to match.
func TestMemoryItemsIsKeyedByTheThingThatIdentifiesAnItem(t *testing.T) {
	db := migrated(t)
	var ddl string
	if err := db.QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_schema WHERE name = 'memory_items'`).Scan(&ddl); err != nil {
		t.Fatalf("read the memory_items DDL: %v", err)
	}
	if !strings.Contains(strings.Join(strings.Fields(ddl), " "), "UNIQUE (host, source_path, entry_key)") {
		t.Fatalf("memory_items carries no UNIQUE (host, source_path, entry_key):\n%s", ddl)
	}
}

// TestDeletingAProjectLeavesItsMemoryBehind. Every other foreign key to projects
// cascades (spec 6); this one sets null, and the reason is that a cascade here
// would be a promise the code cannot keep. The memory file is the host's and
// stays on disk, so the next collection tick re-indexes the row a cascade had
// just deleted. Setting the convenience key to null is what actually happens and
// is therefore what is declared.
func TestDeletingAProjectLeavesItsMemoryBehind(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	if n := countRows(t, db, "memory_items"); n != 1 {
		t.Fatalf("before delete: memory_items rows = %d, want 1", n)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, seedProject); err != nil {
		t.Fatalf("DELETE FROM projects: %v", err)
	}
	if n := countRows(t, db, "memory_items"); n != 1 {
		t.Fatalf("after delete: memory_items rows = %d, want the row to survive", n)
	}
	var projectID *string
	if err := db.QueryRowContext(ctx, `SELECT project_id FROM memory_items`).Scan(&projectID); err != nil {
		t.Fatalf("read memory_items.project_id: %v", err)
	}
	if projectID != nil {
		t.Fatalf("memory_items.project_id = %q after the project was deleted, want NULL", *projectID)
	}
}

// TestMemoryFTSCarriesExactlyTheDecidedOptions is the memory side of
// TestEventsFTSCarriesExactlyTheDecidedOptions and exists for the same reason:
// the argument list is where the decisions are, and a substring check cannot see
// a clause that was added.
//
// The tokenizer is the events index's, to the word. M3 compares recall across
// the two indexes, and a comparison between indexes that tokenise differently
// measures the tokenizer rather than the corpus.
func TestMemoryFTSCarriesExactlyTheDecidedOptions(t *testing.T) {
	db := migrated(t)
	var ddl string
	if err := db.QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_schema WHERE name = 'memory_fts'`).Scan(&ddl); err != nil {
		t.Fatalf("read the memory_fts DDL: %v", err)
	}
	lparen, rparen := strings.Index(ddl, "("), strings.LastIndex(ddl, ")")
	if lparen < 0 || rparen < lparen {
		t.Fatalf("the memory_fts DDL has no argument list: %s", ddl)
	}
	var got []string
	for _, clause := range strings.Split(ddl[lparen+1:rparen], ",") {
		got = append(got, strings.Join(strings.Fields(clause), " "))
	}
	want := []string{
		"body",
		"content = 'memory_items'",
		"tokenize = 'unicode61 remove_diacritics 2'",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("memory_fts options = %q, want %q\nfull DDL: %s", got, want, ddl)
	}
}

// TestTheMemoryIndexFollowsItsTable. External content stores no copy of the
// text, so an index that the triggers do not keep up with is an index that
// answers about rows the table no longer holds - and against a base row that is
// gone, FTS5 returns some rows and then fails in rows.Err() rather than at the
// call. All three triggers are exercised here because insert alone passing is
// exactly the state that hides a missing update or delete.
func TestTheMemoryIndexFollowsItsTable(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	matches := func(term string) int64 {
		t.Helper()
		var n int64
		if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM memory_fts WHERE memory_fts MATCH ?`, term).Scan(&n); err != nil {
			t.Fatalf("MATCH %q: %v", term, err)
		}
		return n
	}
	if got := matches(`"exclusively"`); got != 1 {
		t.Fatalf("after insert, MATCH exclusively = %d, want 1", got)
	}
	if _, err := db.ExecContext(ctx, `UPDATE memory_items SET body = ?`, "one connection, held forever"); err != nil {
		t.Fatalf("UPDATE memory_items: %v", err)
	}
	if got := matches(`"exclusively"`); got != 0 {
		t.Fatalf("after update, MATCH exclusively = %d, want 0", got)
	}
	if got := matches(`"forever"`); got != 1 {
		t.Fatalf("after update, MATCH forever = %d, want 1", got)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM memory_items`); err != nil {
		t.Fatalf("DELETE FROM memory_items: %v", err)
	}
	if got := matches(`"forever"`); got != 0 {
		t.Fatalf("after delete, MATCH forever = %d, want 0", got)
	}
}
