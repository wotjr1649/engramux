package store

import (
	"database/sql"
	"testing"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/secret"
)

// The shape migration 00006 exists to take apart, built by hand at version 5
// because the code that would build it no longer exists: detection was
// corrected in the same change, so ingesting these payloads today produces the
// right answer and none of the wreckage.
const (
	rescanProject = "p-rescan"
	rescanHSID    = "99bcd57b-0000-4000-8000-000000000001"
	rescanPhantom = "codex:" + rescanHSID
	rescanReal    = "claude-code:" + rescanHSID
	rescanCodex   = "codex:" + "11111111-0000-4000-8000-000000000002"

	// The phantom is created at the real session's start, six seconds
	// before the first event that classified correctly. That ordering is
	// not decoration: it is what made `sessions` report a Codex session
	// beginning before the Claude Code one it was taken from.
	rescanT0 = int64(1_700_000_000_000)
)

// TestMigration00006MovesTheEventsTheOldRuleMisjudged is backlog 49's repair,
// asserted on all four things it has to get right and one it must not touch.
//
// A count-only assertion passes a migration that moved the wrong rows, so every
// row here is named: the misjudged event moves and is re-hosted, the session it
// leaves is deleted, the session it joins keeps its own id and gains the
// earlier start, and an event the old rule judged correctly is not rewritten.
func TestMigration00006MovesTheEventsTheOldRuleMisjudged(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatalf("UpTo(5): %v", err)
	}

	exec(t, db, `INSERT INTO projects (id, root, name, created_at) VALUES (?,?,?,?)`,
		rescanProject, `D:\work\rescan`, "rescan", rescanT0)

	// The phantom: created at the real start, holding one event, never ended.
	insertSession(t, db, rescanPhantom, "codex", rescanHSID, "active", rescanT0, nil)
	// The real one, whose first correctly-judged event arrived six seconds later.
	ended := rescanT0 + 60_000
	insertSession(t, db, rescanReal, "claude-code", rescanHSID, "ended", rescanT0+6_000, &ended)
	// A Codex session that was never misjudged and must come through untouched.
	insertSession(t, db, rescanCodex, "codex", "11111111-0000-4000-8000-000000000002", "ended", rescanT0, &ended)

	moved := insertEvent(t, db, "e-sessionstart", rescanProject, rescanPhantom, "codex",
		"SessionStart", fixtureBytes(t, fixtures.ClaudeSessionStart), rescanT0)
	stayedClaude := insertEvent(t, db, "e-posttooluse", rescanProject, rescanReal, "claude-code",
		"PostToolUse", fixtureBytes(t, fixtures.ClaudePostToolUseObject), rescanT0+6_000)
	stayedCodex := insertEvent(t, db, "e-sessionend", rescanProject, rescanCodex, "codex",
		"SessionEnd", fixtureBytes(t, fixtures.CodexSessionEnd), rescanT0)

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate to 00006: %v", err)
	}

	if host, session := eventHostAndSession(t, db, moved); host != "claude-code" || session != rescanReal {
		t.Errorf("the misjudged event is (%s, %s), want (claude-code, %s)", host, session, rescanReal)
	}
	if host, session := eventHostAndSession(t, db, stayedClaude); host != "claude-code" || session != rescanReal {
		t.Errorf("a correctly judged Claude Code event became (%s, %s)", host, session)
	}
	if host, session := eventHostAndSession(t, db, stayedCodex); host != "codex" || session != rescanCodex {
		t.Errorf("a real Codex event became (%s, %s), so the correction runs in both directions "+
			"when it should only run where the two rules disagree", host, session)
	}

	if sessionExists(t, db, rescanPhantom) {
		t.Errorf("the invented session %s survived with nothing pointing at it", rescanPhantom)
	}
	if !sessionExists(t, db, rescanCodex) {
		t.Errorf("the untouched Codex session %s was deleted", rescanCodex)
	}

	// `first seen` is created_at, and the real session has just been handed
	// an event older than its own start. Leaving it would report a session
	// beginning after its first event.
	if got := sessionCreatedAt(t, db, rescanReal); got != rescanT0 {
		t.Errorf("the real session reports first seen %d, want %d - the event it just gained is older", got, rescanT0)
	}
}

func exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("exec %.40q: %v", query, err)
	}
}

func insertSession(t *testing.T, db *sql.DB, id, host, hsid, status string, createdAt int64, endedAt *int64) {
	t.Helper()
	exec(t, db, `INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at, ended_at)
		VALUES (?,?,?,?,?,?,?)`, id, rescanProject, host, hsid, status, createdAt, endedAt)
}

func insertEvent(t *testing.T, db *sql.DB, id, project, session, host, event string, payload []byte, at int64) string {
	t.Helper()
	exec(t, db, `INSERT INTO events (id, project_id, session_id, host, source, event_name,
		tool_name, tool_use_id, payload, privacy_class, redaction_version, received_at)
		VALUES (?,?,?,?,'pipe',?,NULL,NULL,?,?,?,?)`,
		id, project, session, host, event, string(payload), secret.Set{}.String(), int64(secret.Version), at)
	return id
}

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := fixtures.Fixture{File: name}.Bytes()
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func eventHostAndSession(t *testing.T, db *sql.DB, id string) (host, session string) {
	t.Helper()
	err := db.QueryRowContext(t.Context(),
		`SELECT host, session_id FROM events WHERE id = ?`, id).Scan(&host, &session)
	if err != nil {
		t.Fatalf("read event %s: %v", id, err)
	}
	return host, session
}

func sessionExists(t *testing.T, db *sql.DB, id string) bool {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM sessions WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count session %s: %v", id, err)
	}
	return n > 0
}

func sessionCreatedAt(t *testing.T, db *sql.DB, id string) int64 {
	t.Helper()
	var at int64
	if err := db.QueryRowContext(t.Context(),
		`SELECT created_at FROM sessions WHERE id = ?`, id).Scan(&at); err != nil {
		t.Fatalf("read session %s: %v", id, err)
	}
	return at
}
