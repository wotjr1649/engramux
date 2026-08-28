package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wotjr1649/engramux/internal/project"
)

// Both functions below take a *sql.Tx rather than the pool: an event becomes a
// project row, a session row and an event row inside ONE transaction, and there
// is exactly one connection to run it on. They are the first two thirds of that
// transaction and neither opens one of its own.
//
// now is a parameter rather than a call to time.Now, so one ingest stamps all
// of its rows with one instant. Timestamps are milliseconds since the Unix
// epoch. They are not an ordering key - I-06 makes ordering partial, and
// Windows clock resolution is around 550 us against a busiest-session rate of
// 14.8 events/min, so a timestamp neither orders nor disambiguates.

// UpsertProject resolves cwd to a project (see [project.Identify]) and makes
// sure its row exists, returning the project id. Calling it again for the same
// project is not an error and does not touch the existing row: created_at means
// "when the service first saw this project".
//
// The conflict target is the id and not the whole row on purpose. projects.root
// is UNIQUE as well, and because the id is derived from root, a root conflict
// carrying a different id can only mean the derivation changed or collided -
// which is worth failing on rather than absorbing.
//
// The error names the id, never the root: a project root is an absolute path
// and carries the user's name, and errors reach logs.
func UpsertProject(ctx context.Context, tx *sql.Tx, cwd string, now time.Time) (string, error) {
	p := project.Identify(cwd)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO projects (id, root, name, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`,
		p.ID, p.Root, p.Name, now.UnixMilli())
	if err != nil {
		return "", fmt.Errorf("store: upsert project %s: %w", p.ID, err)
	}
	return p.ID, nil
}

// UpsertSession makes sure the row for one host session exists, records the
// status eventName leaves it in, and returns the session id.
//
// **Any** event creates the session, not just SessionStart. In the captured
// corpus only 9 of 19 sessions have a SessionStart at all and three Claude Code
// sessions have none, so waiting for one loses those three sessions entirely.
// A missing SessionStart is not an error and there is nothing here that reports
// one.
//
// host must be one of internal/host.Detect's three values; the column's CHECK
// is what enforces that, so a fourth value fails the write rather than being
// stored.
func UpsertSession(ctx context.Context, tx *sql.Tx, projectID, host, hostSessionID, eventName string, now time.Time) (string, error) {
	// Spec 6: "The id combines host and host session id." Neither of
	// Detect's three values contains a colon, so the join is unambiguous.
	id := host + ":" + hostSessionID

	status := sessionStatus(eventName)
	// ended_at is the only nullable column in the table, and only an ended
	// session has one. A nil any binds NULL. Nothing below can clear a
	// value once written, because nothing updates an ended row.
	var endedAt any
	if status == "ended" {
		endedAt = now.UnixMilli()
	}

	// The trailing WHERE is what makes "ended" terminal. Ordering is partial
	// (I-06) and the spool drains long after the fact, so a PostToolUse can
	// arrive after the SessionEnd that closed the session; without the
	// guard it would resurrect the row as active and clear its ended_at.
	//
	// Nothing else is guarded, and that is the point: active and stopped
	// flip back and forth for the life of the session.
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
		    status   = excluded.status,
		    ended_at = excluded.ended_at
		WHERE sessions.status <> 'ended'`,
		id, projectID, host, hostSessionID, status, now.UnixMilli(), endedAt)
	if err != nil {
		return "", fmt.Errorf("store: upsert session %s: %w", id, err)
	}
	return id, nil
}

// sessionStatus maps a hook event name to the status the session is in once
// that event has been seen. Spec 6: "Status advances on SessionEnd and Stop";
// every other event leaves the session active, including SubagentStop, which
// ends a subagent and not the session.
//
// "stopped" means idle between turns, not finished. Stop fires at the end of
// every turn: in the 902-capture corpus 11 sessions fire it, two of them 5 and
// 7 times, and 9 of the 11 produce further events afterwards - one of them 686.
// Only SessionEnd is terminal, which is why it is the only value the caller
// guards on.
func sessionStatus(eventName string) string {
	switch eventName {
	case "SessionEnd":
		return "ended"
	case "Stop":
		return "stopped"
	default:
		return "active"
	}
}
