package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/project"
)

// upsertNow is the timestamp every test below writes. A fixed value, because
// created_at is asserted by value and I-06 says wall-clock time orders nothing
// anyway.
var upsertNow = time.UnixMilli(1_700_000_000_000)

// repoDir creates a directory with a .git directory in it and returns it, so
// the worktree walk stops there whatever lives above the machine's temp
// directory.
func repoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Join(dir, ".git"), err)
	}
	return dir
}

// inTx runs fn inside one transaction and commits it. The pool holds exactly
// one connection, so a transaction left open wedges every later one; the
// deferred Rollback is what covers fn calling t.Fatalf, and it is a no-op once
// Commit has succeeded.
func inTx(t *testing.T, db *sql.DB, fn func(tx *sql.Tx)) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	fn(tx)
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// projectRow returns one project row's id, root and name.
func projectRow(t *testing.T, db *sql.DB) (id, root, name string) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(),
		`SELECT id, root, name FROM projects`).Scan(&id, &root, &name); err != nil {
		t.Fatalf("read the project row: %v", err)
	}
	return id, root, name
}

// sessionRow returns a session's status and ended_at. ended_at is the only
// nullable column in the table, so it is the only one that needs a Null type.
func sessionRow(t *testing.T, db *sql.DB, id string) (status string, endedAt sql.NullInt64) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(),
		`SELECT status, ended_at FROM sessions WHERE id = ?`, id).Scan(&status, &endedAt); err != nil {
		t.Fatalf("read session %q: %v", id, err)
	}
	return status, endedAt
}

// TestUpsertProjectFoldsSpellingsIntoOneRow is the gate: paths differing only in
// drive-letter case, whole-path case and a trailing separator are ONE project
// row with ONE id - not "a row exists", which passes on four rows as happily as
// on one.
//
// projects.root is UNIQUE, so this also proves the normalisation reaches the
// stored string and not only the derived id: two spellings that normalise
// differently would fail the constraint rather than quietly making two rows.
//
// project.Identify is called at the call site, the way [Ingest] calls it: the
// resolution is filesystem work and does not belong inside the transaction.
func TestUpsertProjectFoldsSpellingsIntoOneRow(t *testing.T) {
	db := migrated(t)
	repo := repoDir(t)
	vol := filepath.VolumeName(repo)
	rest := repo[len(vol):]

	spellings := []string{
		repo,
		strings.ToLower(vol) + rest,
		strings.ToUpper(vol) + rest,
		repo + string(filepath.Separator),
		strings.ToUpper(repo),
	}

	ids := make([]string, len(spellings))
	inTx(t, db, func(tx *sql.Tx) {
		for i, spelling := range spellings {
			id, err := UpsertProject(t.Context(), tx, project.Identify(spelling), upsertNow)
			if err != nil {
				t.Fatalf("UpsertProject(%q): %v", spelling, err)
			}
			ids[i] = id
		}
	})

	for i, id := range ids {
		if id != ids[0] {
			t.Errorf("UpsertProject(%q) = %q, but UpsertProject(%q) = %q", spellings[i], id, spellings[0], ids[0])
		}
	}
	if n := countRows(t, db, "projects"); n != 1 {
		t.Fatalf("projects has %d rows after %d spellings of one worktree, want 1", n, len(spellings))
	}

	gotID, gotRoot, gotName := projectRow(t, db)
	if gotID != ids[0] {
		t.Errorf("stored id = %q, want the returned %q", gotID, ids[0])
	}
	if want := strings.ToLower(repo); gotRoot != want {
		t.Errorf("stored root = %q, want %q", gotRoot, want)
	}
	if want := strings.ToLower(filepath.Base(repo)); gotName != want {
		t.Errorf("stored name = %q, want %q", gotName, want)
	}
}

// TestUpsertProjectTwiceLeavesOneRow: resolving the same project again is not
// an error and does not insert a second row. Separate transactions, because
// that is how two events arrive.
func TestUpsertProjectTwiceLeavesOneRow(t *testing.T) {
	db := migrated(t)
	repo := repoDir(t)

	var first, second string
	inTx(t, db, func(tx *sql.Tx) {
		var err error
		if first, err = UpsertProject(t.Context(), tx, project.Identify(repo), upsertNow); err != nil {
			t.Fatalf("UpsertProject: %v", err)
		}
	})
	inTx(t, db, func(tx *sql.Tx) {
		var err error
		if second, err = UpsertProject(t.Context(), tx, project.Identify(repo), upsertNow.Add(time.Hour)); err != nil {
			t.Fatalf("UpsertProject, second call: %v", err)
		}
	})

	if first != second {
		t.Errorf("ids differ across calls: %q then %q", first, second)
	}
	if n := countRows(t, db, "projects"); n != 1 {
		t.Fatalf("projects has %d rows, want 1", n)
	}

	var createdAt int64
	if err := db.QueryRowContext(t.Context(),
		`SELECT created_at FROM projects WHERE id = ?`, first).Scan(&createdAt); err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	if want := upsertNow.UnixMilli(); createdAt != want {
		t.Errorf("created_at = %d, want %d - the second call overwrote the first sighting", createdAt, want)
	}
}

// TestUpsertSessionIsCreatedByANonSessionStartEvent is the gate that keeps lazy
// creation honest: the only event this session ever sees is a PostToolUse. In
// the real corpus only 9 of 19 sessions carry a SessionStart and three Claude
// Code sessions have none, so a design that waits for one loses them.
func TestUpsertSessionIsCreatedByANonSessionStartEvent(t *testing.T) {
	db := migrated(t)
	const hostSessionID = "0198f0c1-0000-7000-8000-00000000000a"

	var projectID, sessionID string
	inTx(t, db, func(tx *sql.Tx) {
		var err error
		if projectID, err = UpsertProject(t.Context(), tx, project.Identify(repoDir(t)), upsertNow); err != nil {
			t.Fatalf("UpsertProject: %v", err)
		}
		if sessionID, err = UpsertSession(t.Context(), tx,
			projectID, "claude-code", hostSessionID, "PostToolUse", upsertNow); err != nil {
			t.Fatalf("UpsertSession: %v", err)
		}
	})

	if want := "claude-code:" + hostSessionID; sessionID != want {
		t.Errorf("session id = %q, want %q", sessionID, want)
	}
	if n := countRows(t, db, "sessions"); n != 1 {
		t.Fatalf("sessions has %d rows, want 1", n)
	}

	var gotID, gotProject, gotHost, gotHostSession, gotStatus string
	var gotCreated int64
	var gotEnded sql.NullInt64
	if err := db.QueryRowContext(t.Context(),
		`SELECT id, project_id, host, host_session_id, status, created_at, ended_at FROM sessions`).
		Scan(&gotID, &gotProject, &gotHost, &gotHostSession, &gotStatus, &gotCreated, &gotEnded); err != nil {
		t.Fatalf("read the session row: %v", err)
	}
	if gotID != sessionID {
		t.Errorf("stored id = %q, want %q", gotID, sessionID)
	}
	if gotProject != projectID {
		t.Errorf("stored project_id = %q, want %q", gotProject, projectID)
	}
	if gotHost != "claude-code" {
		t.Errorf("stored host = %q, want %q", gotHost, "claude-code")
	}
	if gotHostSession != hostSessionID {
		t.Errorf("stored host_session_id = %q, want %q", gotHostSession, hostSessionID)
	}
	if gotStatus != "active" {
		t.Errorf("stored status = %q, want %q", gotStatus, "active")
	}
	if want := upsertNow.UnixMilli(); gotCreated != want {
		t.Errorf("stored created_at = %d, want %d", gotCreated, want)
	}
	if gotEnded.Valid {
		t.Errorf("stored ended_at = %d, want NULL", gotEnded.Int64)
	}
}

// TestUpsertSessionTwiceLeavesOneRow: the second call for one session is not an
// error and does not insert a second row. sessions is UNIQUE (host,
// host_session_id) as well as on the id, so a bug in how the two are combined
// surfaces here as a constraint failure rather than as a duplicate.
func TestUpsertSessionTwiceLeavesOneRow(t *testing.T) {
	db := migrated(t)
	const hostSessionID = "0198f0c1-0000-7000-8000-00000000000b"

	var first, second string
	inTx(t, db, func(tx *sql.Tx) {
		projectID, err := UpsertProject(t.Context(), tx, project.Identify(repoDir(t)), upsertNow)
		if err != nil {
			t.Fatalf("UpsertProject: %v", err)
		}
		if first, err = UpsertSession(t.Context(), tx,
			projectID, "codex", hostSessionID, "PreToolUse", upsertNow); err != nil {
			t.Fatalf("UpsertSession: %v", err)
		}
		if second, err = UpsertSession(t.Context(), tx,
			projectID, "codex", hostSessionID, "PostToolUse", upsertNow.Add(time.Minute)); err != nil {
			t.Fatalf("UpsertSession, second call: %v", err)
		}
	})

	if first != second {
		t.Errorf("session ids differ across calls: %q then %q", first, second)
	}
	if n := countRows(t, db, "sessions"); n != 1 {
		t.Fatalf("sessions has %d rows, want 1", n)
	}

	var createdAt int64
	if err := db.QueryRowContext(t.Context(),
		`SELECT created_at FROM sessions WHERE id = ?`, first).Scan(&createdAt); err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	if want := upsertNow.UnixMilli(); createdAt != want {
		t.Errorf("created_at = %d, want %d - created_at means the first sighting", createdAt, want)
	}
}

// TestSessionStatusFollowsTheLastEventUntilItEnds walks what spec 6 means by
// "status advances on SessionEnd and Stop", in both directions.
//
// Stop returns to active, because Stop fires at the end of every turn and not
// at the end of the session: in the 902-capture corpus 11 sessions fire Stop,
// two of them 5 and 7 times, and 9 of the 11 go on producing events after their
// first Stop - one of them 686 more. A status that only ever climbed would
// label a session "stopped" through all 686 of them.
//
// SessionEnd does not return, because ordering is partial (I-06) and the spool
// drains long after the fact: an event replayed after the session ended must
// not resurrect it.
func TestSessionStatusFollowsTheLastEventUntilItEnds(t *testing.T) {
	db := migrated(t)
	const hostSessionID = "0198f0c1-0000-7000-8000-00000000000c"

	var projectID string
	inTx(t, db, func(tx *sql.Tx) {
		var err error
		if projectID, err = UpsertProject(t.Context(), tx, project.Identify(repoDir(t)), upsertNow); err != nil {
			t.Fatalf("UpsertProject: %v", err)
		}
	})

	// send is one event, in its own transaction, at its own time.
	send := func(event string, at time.Time) string {
		t.Helper()
		var id string
		inTx(t, db, func(tx *sql.Tx) {
			var err error
			if id, err = UpsertSession(t.Context(), tx,
				projectID, "claude-code", hostSessionID, event, at); err != nil {
				t.Fatalf("UpsertSession(%s): %v", event, err)
			}
		})
		return id
	}

	endedAtMillis := upsertNow.Add(5 * time.Minute).UnixMilli()
	steps := []struct {
		event      string
		at         time.Time
		wantStatus string
		wantEnded  sql.NullInt64
	}{
		{"UserPromptSubmit", upsertNow, "active", sql.NullInt64{}},
		// SubagentStop ends a subagent, not the session.
		{"SubagentStop", upsertNow.Add(1 * time.Minute), "active", sql.NullInt64{}},
		{"Stop", upsertNow.Add(2 * time.Minute), "stopped", sql.NullInt64{}},
		// The turn after the one that fired Stop.
		{"PostToolUse", upsertNow.Add(3 * time.Minute), "active", sql.NullInt64{}},
		{"Stop", upsertNow.Add(4 * time.Minute), "stopped", sql.NullInt64{}},
		{"SessionEnd", upsertNow.Add(5 * time.Minute), "ended", sql.NullInt64{Int64: endedAtMillis, Valid: true}},
		// A spooled event draining after the session ended.
		{"PostToolUse", upsertNow.Add(6 * time.Minute), "ended", sql.NullInt64{Int64: endedAtMillis, Valid: true}},
		{"Stop", upsertNow.Add(7 * time.Minute), "ended", sql.NullInt64{Int64: endedAtMillis, Valid: true}},
	}
	for _, step := range steps {
		id := send(step.event, step.at)
		gotStatus, gotEnded := sessionRow(t, db, id)
		if gotStatus != step.wantStatus {
			t.Errorf("after %s: status = %q, want %q", step.event, gotStatus, step.wantStatus)
		}
		if gotEnded != step.wantEnded {
			t.Errorf("after %s: ended_at = %+v, want %+v", step.event, gotEnded, step.wantEnded)
		}
	}

	if n := countRows(t, db, "sessions"); n != 1 {
		t.Fatalf("sessions has %d rows after %d events, want 1", n, len(steps))
	}
}

// TestUpsertSessionSeparatesHosts: the id combines host and host session id, so
// two hosts that mint the same session id are two sessions. Without the host in
// the id they would be one row, and the UNIQUE (host, host_session_id) that is
// supposed to catch that would never fire.
func TestUpsertSessionSeparatesHosts(t *testing.T) {
	db := migrated(t)
	const hostSessionID = "0198f0c1-0000-7000-8000-00000000000d"

	var claude, codex string
	inTx(t, db, func(tx *sql.Tx) {
		projectID, err := UpsertProject(t.Context(), tx, project.Identify(repoDir(t)), upsertNow)
		if err != nil {
			t.Fatalf("UpsertProject: %v", err)
		}
		if claude, err = UpsertSession(t.Context(), tx,
			projectID, "claude-code", hostSessionID, "PostToolUse", upsertNow); err != nil {
			t.Fatalf("UpsertSession(claude-code): %v", err)
		}
		if codex, err = UpsertSession(t.Context(), tx,
			projectID, "codex", hostSessionID, "PostToolUse", upsertNow); err != nil {
			t.Fatalf("UpsertSession(codex): %v", err)
		}
	})

	if claude == codex {
		t.Fatalf("both hosts got session id %q", claude)
	}
	if n := countRows(t, db, "sessions"); n != 2 {
		t.Fatalf("sessions has %d rows, want 2", n)
	}
}
