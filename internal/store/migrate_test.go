package store

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"modernc.org/sqlite"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/secret"
)

// SQLite extended result codes, as `SQLITE_CONSTRAINT | (n<<8)`. The tests below
// assert the extended code rather than "some error": a CHECK that was never
// created and a UNIQUE that fires instead both produce a non-nil error, and only
// the code says which constraint actually did the work.
const (
	sqliteConstraintCheck      = 275  // SQLITE_CONSTRAINT_CHECK
	sqliteConstraintForeignKey = 787  // SQLITE_CONSTRAINT_FOREIGNKEY
	sqliteConstraintPrimaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	sqliteConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
	sqliteConstraintDataType   = 3091 // SQLITE_CONSTRAINT_DATATYPE, a STRICT table
)

// requireSQLiteCode asserts err is a *sqlite.Error carrying exactly want.
func requireSQLiteCode(t *testing.T, err error, want int, what string) {
	t.Helper()
	var serr *sqlite.Error
	if !errors.As(err, &serr) {
		t.Fatalf("%s error = %v (%T), want *sqlite.Error with code %d", what, err, err, want)
	}
	if serr.Code() != want {
		t.Fatalf("%s error code = %d (%v), want %d", what, serr.Code(), err, want)
	}
}

// migrated opens a fresh database under the test's own temp dir and migrates it
// up. The pool is closed when the test ends: on Windows an open handle makes
// t.TempDir()'s cleanup fail, and the WAL sidecars count.
func migrated(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(t.Context(), dbPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

// fkViolation is one row of PRAGMA foreign_key_check: the child table, the
// child's rowid, the parent table the reference missed, and the index of the
// offending foreign key within the child's PRAGMA foreign_key_list.
type fkViolation struct {
	table  string
	rowid  int64
	parent string
	fkid   int64
}

func (v fkViolation) String() string {
	return fmt.Sprintf("{table:%s rowid:%d parent:%s fkid:%d}", v.table, v.rowid, v.parent, v.fkid)
}

// foreignKeyCheck runs the Phase 1 gate's pragma and returns every row it
// reported, in the order SQLite reported them.
func foreignKeyCheck(t *testing.T, db *sql.DB) []fkViolation {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_check: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close: %v", err)
		}
	}()
	var got []fkViolation
	for rows.Next() {
		var v fkViolation
		if err := rows.Scan(&v.table, &v.rowid, &v.parent, &v.fkid); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return got
}

func formatViolations(vs []fkViolation) string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.String()
	}
	return "[" + strings.Join(out, " ") + "]"
}

// TestForeignKeyCheckReportsADanglingRow is written before the clean case and
// exists to make the clean case mean something. An empty foreign_key_check is
// what a correct schema looks like and equally what a check that does nothing
// looks like, so this test builds the violation deliberately - foreign_keys off,
// an events row whose project_id and session_id both point at nothing - and
// asserts the pragma names that exact row, both foreign keys, by name and rowid.
func TestForeignKeyCheckReportsADanglingRow(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)

	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("PRAGMA foreign_keys = OFF: %v", err)
	}
	// Prove the setup took. A pragma that silently did nothing would leave the
	// INSERT below failing for the right reason and this test passing for the
	// wrong one.
	var enforcing int64
	if err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&enforcing); err != nil {
		t.Fatalf("PRAGMA foreign_keys readback: %v", err)
	}
	if enforcing != 0 {
		t.Fatalf("PRAGMA foreign_keys = %d after turning it off, want 0", enforcing)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO events (id, project_id, session_id, host, source, event_name,
		                    payload, privacy_class, redaction_version, received_at)
		VALUES ('e-dangling', 'no-such-project', 'no-such-session', 'codex', 'pipe',
		        'PostToolUse', '{}', '', 1, 1000)`); err != nil {
		t.Fatalf("INSERT dangling event: %v", err)
	}

	got := foreignKeyCheck(t, db)
	// fkid indexes PRAGMA foreign_key_list, which SQLite reports in reverse
	// declaration order: session_id is declared after project_id, so it is 0.
	want := []fkViolation{
		{table: "events", rowid: 1, parent: "sessions", fkid: 0},
		{table: "events", rowid: 1, parent: "projects", fkid: 1},
	}
	if formatViolations(got) != formatViolations(want) {
		t.Fatalf("foreign_key_check = %s, want %s", formatViolations(got), formatViolations(want))
	}
}

// seed inserts one row into every table, so that every foreign key in the schema
// is exercised by a reference that is actually satisfied: sessions and
// memory_items into projects, events into projects and sessions, observations
// into projects and events. Six references, five rows.
//
// memory_items is the one whose reference is optional - 00004 made project_id
// nullable, because neither host's memory can promise a projects row exists for
// it - so it is seeded *with* a project on purpose. A satisfied reference is
// what foreign_key_check has to walk, and a NULL would leave that key unchecked.
//
// privacy_class and redaction_version are written through internal/secret rather
// than as literals, because the column exists to hold what that package produces
// and a literal would not notice the two drifting apart.
func seed(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := t.Context()
	stmts := []struct {
		what string
		sql  string
		args []any
	}{
		{"projects", `INSERT INTO projects (id, root, name, created_at)
			VALUES (?, ?, ?, ?)`,
			[]any{seedProject, `D:\work\engramux`, "engramux", int64(1000)}},
		{"sessions", `INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at, ended_at)
			VALUES (?, ?, ?, ?, ?, ?, NULL)`,
			[]any{seedSession, seedProject, "codex", "0198f0c1-0000-7000-8000-000000000001", "active", int64(1001)}},
		{"events", `INSERT INTO events (id, project_id, session_id, host, source, event_name,
			                            tool_name, tool_use_id, payload, privacy_class,
			                            redaction_version, received_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[]any{seedEvent, seedProject, seedSession, "codex", "pipe", "PostToolUse",
				"Bash", "toolu_01", `{"hook_event_name":"PostToolUse"}`,
				seedPrivacyClass.String(), int64(secret.Version), int64(1002)}},
		{"observations", `INSERT INTO observations (id, project_id, event_id, kind, body, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			[]any{"o-1", seedProject, seedEvent, "file-touched", "internal/store/store.go", int64(1003)}},
		memoryItemsSeed(t, db),
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("INSERT into %s: %v", s.what, err)
		}
	}
}

// memoryItemsSeed picks the statement that matches the memory_items this
// database actually has. The table has two shapes and which one is present is a
// migration version: 00001's, which two tests below reach on purpose by running
// UpTo(1) so that they can insert rows that predate the leaves column, and
// 00004's, which is what every other caller sees.
//
// Detecting rather than passing a version in is deliberate. The two version-1
// tests are about events and would have to learn about memory_items to pass a
// flag, and a flag is a second thing to keep in step with the schema. The column
// is the fact.
func memoryItemsSeed(t *testing.T, db *sql.DB) struct {
	what string
	sql  string
	args []any
} {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM pragma_table_info('memory_items') WHERE name = 'host'`).Scan(&n); err != nil {
		t.Fatalf("read the memory_items columns: %v", err)
	}
	if n == 0 {
		return struct {
			what string
			sql  string
			args []any
		}{"memory_items", `INSERT INTO memory_items (id, project_id, key, body, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			[]any{"m-1", seedProject, "convention", "one connection, held exclusively", int64(1004), int64(1004)}}
	}
	return struct {
		what string
		sql  string
		args []any
	}{"memory_items", `INSERT INTO memory_items (id, host, kind, source_path, entry_key,
			                                     project_path, project_id, title, body,
			                                     host_modified_at, privacy_class,
			                                     redaction_version, indexed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[]any{"m-1", "codex", "rollout-summary", `D:\work\engramux\MEMORY.md`, "0198f0c1",
			`D:\work\engramux`, seedProject, "one connection", "one connection, held exclusively",
			int64(1004), seedPrivacyClass.String(), int64(secret.Version), int64(1004)}}
}

const (
	seedProject = "p-0198f0c1"
	seedSession = "s-codex-0198f0c1"
	seedEvent   = "0198f0c1-1111-7222-8333-444444444444"
)

// seedPrivacyClass is deliberately more than one class: Set.String() joins, and
// a single-element set would round-trip through a code path that never touched
// the separator.
var seedPrivacyClass = secret.Set{secret.ClassAPIKey, secret.ClassUserPath}

// countRows returns the number of rows in table. table is a literal from this
// file, never a value.
func countRows(t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// queryStrings runs a query returning one TEXT column and collects it.
func queryStrings(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close: %v", err)
		}
	}()
	var got []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return got
}

// schemaSnapshot is every row of sqlite_schema, with its full DDL text. Nothing
// is filtered out: an index, a trigger or a table that appears on one migration
// pass and not another is exactly what this is for.
func schemaSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	return strings.Join(queryStrings(t, db,
		`SELECT type || '|' || name || '|' || tbl_name || '|' || COALESCE(sql, '')
		 FROM sqlite_schema ORDER BY type, name`), "\n")
}

// TestMigrateCreatesTheDeclaredTables names them, so that adding or losing one
// is a test change rather than a silent one. goose_db_version is goose's own
// bookkeeping, and the four events_fts_* are FTS5's shadow tables - the index
// itself, its structure record, its docsize record and its options - which the
// virtual table creates and owns. They are listed because they are what an
// external-content index costs on disk, and because losing one is how an index
// silently stops being an index.
func TestMigrateCreatesTheDeclaredTables(t *testing.T) {
	db := migrated(t)
	got := queryStrings(t, db,
		`SELECT name FROM sqlite_schema
		 WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	want := []string{
		"events", "events_fts", "events_fts_config", "events_fts_data",
		"events_fts_docsize", "events_fts_idx", "goose_db_version",
		"memory_fts", "memory_fts_config", "memory_fts_data",
		"memory_fts_docsize", "memory_fts_idx",
		"memory_items", "observations", "projects", "sessions",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tables = %v, want %v", got, want)
	}
}

// TestEventsFTSCarriesExactlyTheDecidedOptions. Everything inside events_fts's
// argument list is a spec 5.7 decision, and until this existed nothing in the
// normal suite held any of them: the tokenizer was compared only by the corpus
// benchmark, which skips without .capture/, and the two absences - no prefix
// index, no second column - were held by nothing at all.
//
// The whole argument list is compared, not a substring. A substring check for
// "unicode61" passes on a tokenizer that also lost remove_diacritics or that
// regained the porter stemmer in front of it, and no substring check can assert
// that a clause is *absent* without naming every clause anyone might add.
// Splitting on commas is safe because no clause this table may carry contains
// one; a clause that did would land here as an unreadable diff, which is the
// right way to find out.
//
// Whole-list comparison is what caught the Phase 4 tokenizer change: dropping
// porter from the migration failed here first, by name, before any search test
// noticed.
func TestEventsFTSCarriesExactlyTheDecidedOptions(t *testing.T) {
	db := migrated(t)
	var ddl string
	if err := db.QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_schema WHERE name = 'events_fts'`).Scan(&ddl); err != nil {
		t.Fatalf("read the events_fts DDL: %v", err)
	}
	lparen, rparen := strings.Index(ddl, "("), strings.LastIndex(ddl, ")")
	if lparen < 0 || rparen < lparen {
		t.Fatalf("the events_fts DDL has no argument list: %s", ddl)
	}
	var got []string
	for _, clause := range strings.Split(ddl[lparen+1:rparen], ",") {
		got = append(got, strings.Join(strings.Fields(clause), " "))
	}
	want := []string{
		"leaves",
		"content = 'events'",
		"tokenize = 'unicode61 remove_diacritics 2'",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("events_fts options = %q, want %q\nfull DDL: %s", got, want, ddl)
	}
}

// TestForeignKeyCheckIsEmpty is Phase 1 gate clause 2. It only means anything
// alongside TestForeignKeyCheckReportsADanglingRow, which proves the pragma
// reports a violation when there is one; on its own an empty result set cannot
// tell a clean schema from a check that does nothing.
func TestForeignKeyCheckIsEmpty(t *testing.T) {
	db := migrated(t)
	seed(t, db)
	if got := foreignKeyCheck(t, db); len(got) != 0 {
		t.Fatalf("foreign_key_check = %s, want []", formatViolations(got))
	}
}

// TestMigrateDownUpRestoresTheSameSchema. A migration that cannot be re-run is a
// migration nobody can fix in place. The Down half is asserted separately, since
// a Down that dropped nothing would make the comparison trivially true.
//
// DownTo(0) rather than Down: Down rolls back exactly one migration, so with
// more than one in the set it would leave the earlier ones applied and the
// assertion below would be about the wrong thing.
func TestMigrateDownUpRestoresTheSameSchema(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	before := schemaSnapshot(t, db)

	p, err := provider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("DownTo(0): %v", err)
	}
	if got := queryStrings(t, db,
		`SELECT name FROM sqlite_schema
		 WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`); strings.Join(got, ",") != "goose_db_version" {
		t.Fatalf("tables after Down = %v, want [goose_db_version]", got)
	}

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate (second up): %v", err)
	}
	if after := schemaSnapshot(t, db); after != before {
		t.Fatalf("schema after down-up:\n%s\nwant:\n%s", after, before)
	}
}

// TestMigrateDownToOneLeavesVersionOneExactly. TestMigrateDownUpRestoresTheSameSchema
// goes all the way to 0, where 00001's `DROP TABLE events` removes the events
// row that carries the leaves column and, with it, the evidence that 00002's
// Down forgot to drop something. Every one of 00002's objects hangs off events:
// a missing DROP is invisible one step further down.
//
// So this stops at 1 and names what may remain. The trigger and column checks
// are separate from the table list because sqlite_schema lists them under
// different types, and a trigger left behind would otherwise be reported as
// "the tables are fine".
func TestMigrateDownToOneLeavesVersionOneExactly(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)

	p, err := provider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.DownTo(ctx, 1); err != nil {
		t.Fatalf("DownTo(1): %v", err)
	}
	if v, err := p.GetDBVersion(ctx); err != nil || v != 1 {
		t.Fatalf("db version after DownTo(1) = %d (err %v), want 1", v, err)
	}

	got := queryStrings(t, db,
		`SELECT name FROM sqlite_schema
		 WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	want := []string{
		"events", "goose_db_version", "memory_items", "observations", "projects", "sessions",
	}
	if !slices.Equal(got, want) {
		t.Errorf("tables after DownTo(1) = %v, want %v", got, want)
	}
	if triggers := queryStrings(t, db,
		`SELECT name FROM sqlite_schema WHERE type = 'trigger' ORDER BY name`); len(triggers) != 0 {
		t.Errorf("triggers after DownTo(1) = %v, want none", triggers)
	}
	if cols := queryStrings(t, db,
		`SELECT name FROM pragma_table_info('events') ORDER BY name`); slices.Contains(cols, "leaves") {
		t.Errorf("events still carries the leaves column after DownTo(1): %v", cols)
	}
}

// TestMigrateIsIdempotent: the service migrates on every start, so the second
// call must be a no-op rather than an error, and must not advance the version.
func TestMigrateIsIdempotent(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate (second call): %v", err)
	}
	p, err := provider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	v, err := p.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("GetDBVersion: %v", err)
	}
	// The literal is bumped by hand with every migration added, so a
	// migration that arrives without anyone noticing fails here. 6 is
	// 00006, backlog 49's re-scan of every stored event's host against
	// spec 4.3's corrected rule.
	if v != 6 {
		t.Fatalf("db version = %d, want 6", v)
	}
}

// TestEventsCheckConstraints. internal/host.Detect hands Go a bare string, and
// the service sets source from the ingest path; nothing in Go's type system
// stops a fourth value reaching either column, so the CHECK is the only thing
// that does. Each accepted value is asserted too - a CHECK that rejected
// everything would pass a test that only tried the bad values.
func TestEventsCheckConstraints(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	insert := func(id, host, source string) error {
		_, err := db.ExecContext(ctx, `
			INSERT INTO events (id, project_id, session_id, host, source, event_name,
			                    payload, privacy_class, redaction_version, received_at)
			VALUES (?, ?, ?, ?, ?, 'PostToolUse', '{}', '', 1, 1000)`,
			id, seedProject, seedSession, host, source)
		return err
	}

	// host is I-04's domain: unknown is reachable and is not an error.
	for _, host := range []string{"claude-code", "codex", "unknown"} {
		if err := insert("ok-host-"+host, host, "pipe"); err != nil {
			t.Fatalf("INSERT host=%q: %v", host, err)
		}
	}
	for _, source := range []string{"pipe", "spool"} {
		if err := insert("ok-source-"+source, "codex", source); err != nil {
			t.Fatalf("INSERT source=%q: %v", source, err)
		}
	}

	for _, bad := range []struct{ what, host, source string }{
		{"host", "claude-mem", "pipe"},
		{"host", "Codex", "pipe"},
		{"host", "", "pipe"},
		{"source", "codex", "disk"},
		{"source", "codex", "Pipe"},
		{"source", "codex", ""},
	} {
		err := insert("bad-"+bad.what+"-"+bad.host+bad.source, bad.host, bad.source)
		requireSQLiteCode(t, err, sqliteConstraintCheck,
			fmt.Sprintf("INSERT %s host=%q source=%q", bad.what, bad.host, bad.source))
	}
}

// TestEventIDIsTheIdempotencyKey. I-05 rests on events.id being unique: it is
// the relay-minted UUIDv7 and there is no idempotency_key column, so a second
// insert of the same id has to be refused by the database rather than noticed by
// whoever remembered to check first.
func TestEventIDIsTheIdempotencyKey(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	_, err := db.ExecContext(ctx, `
		INSERT INTO events (id, project_id, session_id, host, source, event_name,
		                    payload, privacy_class, redaction_version, received_at)
		VALUES (?, ?, ?, 'codex', 'spool', 'PostToolUse', '{"different":true}', '', 1, 9999)`,
		seedEvent, seedProject, seedSession)
	requireSQLiteCode(t, err, sqliteConstraintPrimaryKey, "INSERT a duplicate events.id")

	if n := countRows(t, db, "events"); n != 1 {
		t.Fatalf("events rows = %d, want 1", n)
	}
}

// TestSessionsUniqueOnHostAndHostSessionID. sessions.id combines host and host
// session id; the UNIQUE is the same statement made about the parts, so a bug in
// how they are combined surfaces here instead of as two rows for one session.
func TestSessionsUniqueOnHostAndHostSessionID(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	_, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at)
		VALUES ('s-different-id', ?, 'codex', '0198f0c1-0000-7000-8000-000000000001', 'active', 2000)`,
		seedProject)
	requireSQLiteCode(t, err, sqliteConstraintUnique, "INSERT a second row for one host session")
}

// TestDeletingAProjectCascades. Every foreign key to projects carries ON DELETE
// CASCADE (spec 6). Project purge is not in 1.0 and no command exposes it, so
// this test is the only thing that will ever notice the cascade being dropped -
// declared and unproven is how a schema promise rots.
func TestDeletingAProjectCascades(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	// memory_items is deliberately absent. Its key to projects sets null rather
	// than cascading, for the reason 00004 gives, and
	// TestDeletingAProjectLeavesItsMemoryBehind is what holds that half.
	children := []string{"sessions", "events", "observations"}
	for _, table := range children {
		if n := countRows(t, db, table); n != 1 {
			t.Fatalf("before delete: %s rows = %d, want 1", table, n)
		}
	}

	res, err := db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, seedProject)
	if err != nil {
		t.Fatalf("DELETE FROM projects: %v", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("DELETE affected %d rows (err %v), want 1", n, err)
	}

	for _, table := range children {
		if n := countRows(t, db, table); n != 0 {
			t.Fatalf("after delete: %s rows = %d, want 0", table, n)
		}
	}
	if got := foreignKeyCheck(t, db); len(got) != 0 {
		t.Fatalf("foreign_key_check after cascade = %s, want []", formatViolations(got))
	}
}

// TestForeignKeysAreEnforced. The cascade test above deletes a parent; this one
// is the other half - a child that names a parent which does not exist is
// refused while foreign_keys is on, which is the production setting Open reads
// back (I-11).
func TestForeignKeysAreEnforced(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	_, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at)
		VALUES ('s-orphan', 'no-such-project', 'codex', 'orphan', 'active', 2000)`)
	requireSQLiteCode(t, err, sqliteConstraintForeignKey, "INSERT a session under no project")
}

// TestPayloadRoundTripsByteForByte over the four Phase 1 fixtures. The payload
// column is TEXT and the host's bytes go in unparsed and unmarshalled; the
// assertion is on the bytes, because a column that stored a re-encoded form
// would still satisfy "the insert worked" and "the row is there".
//
// privacy_class and redaction_version ride along: they are asserted through
// internal/secret's own parser, so the stored form is checked against the thing
// that produces it rather than against a literal copied out of it.
func TestPayloadRoundTripsByteForByte(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	for i, f := range fixtures.All() {
		want, err := f.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", f.File, err)
		}
		id := fmt.Sprintf("fixture-%d", i)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO events (id, project_id, session_id, host, source, event_name,
			                    payload, privacy_class, redaction_version, received_at)
			VALUES (?, ?, ?, ?, 'pipe', ?, ?, ?, ?, ?)`,
			id, seedProject, seedSession, f.Host, f.Event, string(want),
			seedPrivacyClass.String(), int64(secret.Version), int64(1000+i)); err != nil {
			t.Fatalf("%s: INSERT: %v", f.File, err)
		}

		var payload string
		var class string
		var version int
		if err := db.QueryRowContext(ctx,
			`SELECT payload, privacy_class, redaction_version FROM events WHERE id = ?`, id).
			Scan(&payload, &class, &version); err != nil {
			t.Fatalf("%s: SELECT: %v", f.File, err)
		}
		if !bytes.Equal([]byte(payload), want) {
			t.Fatalf("%s: payload round-tripped as %d bytes, want %d; first difference at %d",
				f.File, len(payload), len(want), firstDiff([]byte(payload), want))
		}
		if got := secret.ParseSet(class); !slices.Equal(got, seedPrivacyClass) {
			t.Fatalf("%s: privacy_class = %q -> %v, want %v", f.File, class, got, seedPrivacyClass)
		}
		if version != secret.Version {
			t.Fatalf("%s: redaction_version = %d, want %d", f.File, version, secret.Version)
		}
	}
}

// firstDiff is the index of the first differing byte, or -1.
func firstDiff(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return min(len(a), len(b))
	}
	return -1
}

// TestStrictTablesRejectAWrongType. Every table is STRICT, which is what makes
// the declared types mean something: without it 'not-a-number' lands in an
// INTEGER column as text and reads back that way.
func TestStrictTablesRejectAWrongType(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	_, err := db.ExecContext(ctx, `
		INSERT INTO events (id, project_id, session_id, host, source, event_name,
		                    payload, privacy_class, redaction_version, received_at)
		VALUES ('e-typed', ?, ?, 'codex', 'pipe', 'PostToolUse', '{}', '', 'not-a-number', 1000)`,
		seedProject, seedSession)
	requireSQLiteCode(t, err, sqliteConstraintDataType, "INSERT text into events.redaction_version")
}
