// Package search reads events back out of the FTS5 index (spec 5.7).
//
// The index is an external-content FTS5 table over events, so an FTS row's
// rowid is the events rowid and the join below is the only way back to an
// event's id. That is also why the FTS column is named payload: external
// content requires the FTS column names to be the content table's.
//
// # What this package is not yet
//
// [Search] passes the caller's text to MATCH exactly as given. Spec 5.7 says a
// query expands per token with a trailing star - "token"*, the star outside the
// quotes, because a bare token* is a syntax error on a hyphenated identifier or
// a Windows path - and that expansion is what makes a two-character Korean
// query and a stem carrying a particle reachable at all. It is not here: T5
// adds it, and until then the classes that need it can only be measured, not
// passed.
//
// There is no excerpt either. [Hit] carries the event id and nothing else; T6
// adds the snippet and the pipe surface that returns it.
//
// The gate that measures all of this is TestPhase4Gate, in this package's
// external test package (spec 8, Phase 4).
package search

import (
	"context"
	"database/sql"
	"fmt"
)

// Hit is one matching event, in the order FTS5 ranked it: the caller's slice
// index is the rank.
type Hit struct {
	// ID is events.id - the relay-minted UUIDv7 the event was ingested under.
	ID string
}

// Search returns up to limit events whose indexed payload matches text, best
// first.
//
// text goes to MATCH unmodified, so it is FTS5 query syntax and not a literal:
// a caller handing it raw user input can get a syntax error back rather than an
// empty result. T5 is what puts a query builder in front of it. limit goes to
// LIMIT unmodified as well, and SQLite reads a negative LIMIT as "no limit".
func Search(ctx context.Context, db *sql.DB, text string, limit int) ([]Hit, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT events.id
		FROM events_fts
		JOIN events ON events.rowid = events_fts.rowid
		WHERE events_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, text, limit)
	if err != nil {
		return nil, fmt.Errorf("search: match: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID); err != nil {
			return nil, fmt.Errorf("search: scan a hit: %w", err)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: read the hits: %w", err)
	}
	return hits, nil
}
