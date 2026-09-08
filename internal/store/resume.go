package store

import (
	"context"
	"database/sql"
	"fmt"
)

// LatestSessionEvents reads the last ingested prompt and main reply in an exact
// project/host/session scope. Received time is ingestion time, not host time;
// rowid breaks millisecond ties. Oversize payloads are omitted, never sliced.
// Ranking reads metadata only; the payload join sees at most two rows. One
// statement gives both records the same SQLite read snapshot.
func LatestSessionEvents(ctx context.Context, db *sql.DB, projectID, host, sessionID string, payloadCap, idCap int) ([]ResumeEvent, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT CASE WHEN length(CAST(e.id AS BLOB)) <= ? THEN e.id ELSE '' END,
		       e.event_name,
		       e.received_at, length(CAST(e.payload AS BLOB)) > ?,
		       CASE WHEN length(CAST(e.payload AS BLOB)) <= ? THEN e.payload ELSE '' END
		FROM (
		    SELECT rowid AS rid, row_number() OVER (
		        PARTITION BY event_name ORDER BY received_at DESC, rowid DESC) AS n
		    FROM events
		    WHERE project_id = ? AND host = ? AND event_name IN ('UserPromptSubmit', 'Stop')
		      AND session_id = (SELECT id FROM sessions WHERE host = ? AND host_session_id = ?)
		) AS k JOIN events AS e ON e.rowid = k.rid
		WHERE k.n = 1`, idCap, payloadCap, payloadCap, projectID, host, host, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: read session resume events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ResumeEvent
	for rows.Next() {
		var e ResumeEvent
		if err := rows.Scan(&e.ID, &e.EventName, &e.ReceivedAtMS, &e.Omitted, &e.Payload); err != nil {
			return nil, fmt.Errorf("store: scan resume event: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: resume rows: %w", err)
	}
	return out, nil
}

type ResumeEvent struct {
	Event
	Omitted bool
}
