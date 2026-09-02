package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CellsQuery is the statement behind a status reply's per-cell breakdown: one
// row per distinct (host, event_name) with the count and the span of
// received_at, in an order that makes two runs over an unchanged database
// diff cleanly (SQLite's BINARY collation puts `unknown` last by construction).
//
// It lives here rather than beside its caller in internal/service because
// migration 00003's index exists for this statement and nothing else, and the
// test that holds the plan to that index has to explain the statement the
// service actually runs - one literal, not a copy that drifts.
const CellsQuery = `
		SELECT host, event_name, count(*), min(received_at), max(received_at)
		FROM events
		GROUP BY host, event_name
		ORDER BY host, event_name`

// Event is one events row as it is stored: unmasked, payload included.
//
// It is this package's type and not the wire's, the same way internal/search's
// Hit is. What crosses the boundary is internal/service's business, and that is
// where I-10's masking is applied - the database keeps what it captured
// (spec 6.1), so a reader of this package gets the original.
type Event struct {
	ID           string
	Host         string
	EventName    string
	SessionID    string
	ReceivedAtMS int64
	PrivacyClass string
	// Payload is events.payload, byte for byte as the host wrote it.
	Payload []byte
}

// GetEvent returns the event with this id in this project, or nil when there is
// none.
//
// # Both, in one WHERE
//
// events.id is the relay-minted UUIDv7 and is the primary key of the whole
// table (I-05), so it is unique across every project. Reading by id alone would
// therefore answer about any project, and a caller that learned an id from one
// project's search could read an event out of another. The project is part of
// the predicate rather than checked after the row comes back, so there is no
// window and no second branch to get wrong.
//
// A row that exists under a different project is nil here, exactly as a row that
// does not exist is. That is deliberate: distinguishing them would confirm the
// id exists somewhere, which is the one bit the pair is meant to withhold.
//
// This is a filtering-correctness property and not an authorization check - spec
// 2 puts the whole SID inside the trust boundary and the caller names its own
// project (spec 5.9).
func GetEvent(ctx context.Context, db *sql.DB, id, projectID string) (*Event, error) {
	var e Event
	err := db.QueryRowContext(ctx, `
		SELECT id, host, event_name, session_id, received_at, privacy_class, payload
		FROM events
		WHERE id = ? AND project_id = ?`,
		id, projectID).Scan(&e.ID, &e.Host, &e.EventName, &e.SessionID,
		&e.ReceivedAtMS, &e.PrivacyClass, &e.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		// Not an error. "No such event in that project" is an answer,
		// and the caller has a reply shape for it.
		return nil, nil
	}
	if err != nil {
		// The id is not in the message. It came off the wire and is
		// bounded only by ipc.MaxEventIDBytes, and this error reaches a
		// log.
		return nil, fmt.Errorf("store: read one event: %w", err)
	}
	return &e, nil
}

// Session is one sessions row, less the project every row in a listing shares.
type Session struct {
	ID            string
	Host          string
	HostSessionID string
	Status        string
	CreatedAtMS   int64
	// EndedAtMS is 0 for a session that has not ended. sessions.ended_at is
	// the one nullable column in the table, and 0 is how its NULL is
	// carried: an epoch millisecond of 0 is 1970, which no row holds.
	EndedAtMS int64
}

// Sessions returns up to limit of a project's sessions, newest first.
//
// The order is created_at descending, then id descending. The tiebreak is not
// decoration: created_at is milliseconds and the service stamps every row of one
// ingest with one instant, so two sessions first seen in the same millisecond
// are not hypothetical. Without it their order is whatever the query plan
// happens to produce, and two runs over an unchanged database could disagree.
//
// It orders on created_at while I-06 makes event ordering partial, and the two
// do not conflict: this is a listing for a reader to look at, not an ordering
// anything depends on. Nothing downstream treats the position of a session in
// this slice as a fact about when it ran relative to another.
//
// limit reaches LIMIT unmodified, and SQLite reads a negative LIMIT as "no
// limit". [github.com/wotjr1649/engramux/internal/ipc.ListSessionsRequest.EffectiveLimit]
// is what bounds it before it gets here.
func Sessions(ctx context.Context, db *sql.DB, projectID string, limit int) ([]Session, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, host, host_session_id, status, created_at, coalesce(ended_at, 0)
		FROM sessions
		WHERE project_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Host, &s.HostSessionID, &s.Status,
			&s.CreatedAtMS, &s.EndedAtMS); err != nil {
			return nil, fmt.Errorf("store: scan a session: %w", err)
		}
		out = append(out, s)
	}
	// Checked rather than assumed: rows.Next returns false both for "that
	// was the last row" and for "the read failed", and without this a
	// truncated listing would be served as a complete one.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read the session list: %w", err)
	}
	return out, nil
}
