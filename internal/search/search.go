// Package search reads events back out of the FTS5 index (spec 5.7).
//
// The index is an external-content FTS5 table over events, so an FTS row's
// rowid is the events rowid and the join below is the only way back to an
// event's id. That is also why the FTS column is named leaves: external content
// requires the FTS column names to be the content table's, and events.leaves is
// where [github.com/wotjr1649/engramux/internal/store.Leaves] puts the payload's
// string leaves. The raw JSON is not indexed - its keys are tokens of every
// document, which costs precision and buys nothing (spec 5.7).
//
// What a person types never reaches MATCH as query syntax: [queryTokens] and
// [matchExpression] turn it into one quoted prefix phrase per token, which is
// what makes a hyphenated identifier, a Windows path and a two-character Korean
// query work at all (spec 5.7).
//
// # This is the second egress
//
// A hit carries an excerpt, and I-10 says the secret never leaves the machine.
// The database keeps what it captured, secrets and all - that is what makes an
// event re-readable - so the masking has to happen on the way out, here. See
// [excerpt] for how, and for why FTS5's own snippet() and highlight() are not
// available to do it.
//
// The gates that measure all of this are TestPhase4Gate and
// TestPhase4GateTheSearchEgressMasks, in this package's external test package
// (spec 8, Phase 4).
package search

import (
	"context"
	"database/sql"
	"fmt"
)

// Hit is one matching event, in the order FTS5 ranked it: the caller's slice
// index is the rank.
//
// It is this package's own type and not the wire's. internal/ipc owns what
// travels the pipe; what is here is what a reader of the index gets, and the
// service is the seam that turns one into the other.
type Hit struct {
	// ID is events.id - the relay-minted UUIDv7 the event was ingested under.
	ID string
	// Host is events.host and EventName is events.event_name: between them,
	// the cell the event landed in.
	Host      string
	EventName string
	// ReceivedAtMS is events.received_at, milliseconds since the Unix epoch.
	ReceivedAtMS int64
	// Excerpt is a window of the event's masked text around the match - see
	// [excerpt] for why it is built here rather than by snippet(), and why
	// it carries no offset into anything.
	Excerpt string
}

// Search returns up to limit events whose indexed text matches text, best
// first. What is indexed is the payload's string leaves, so a match is on
// something the event said and never on the shape it said it in.
//
// text is what a person typed and not FTS5 query syntax: it is split on
// whitespace and each token is quoted, so no syntax error can reach the caller
// and no operator a person happened to type is obeyed ([matchExpression]). A
// query that carries no token, too many, or one too long is refused with
// [ErrEmptyQuery], [ErrTooManyTokens] or [ErrTokenTooLong] before the database
// is touched - all three are errors and not empty results.
//
// Every hit carries an excerpt cut from the event's masked payload, which is
// the only text about the event this function returns - see [excerpt].
//
// limit goes to LIMIT unmodified, and SQLite reads a negative LIMIT as "no
// limit". [github.com/wotjr1649/engramux/internal/ipc.SearchRequest.EffectiveLimit]
// is what the pipe surface bounds it with before it gets here.
func Search(ctx context.Context, db *sql.DB, text string, limit int) ([]Hit, error) {
	tokens, err := queryTokens(text)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT events.id, events.host, events.event_name, events.received_at, events.payload
		FROM events_fts
		JOIN events ON events.rowid = events_fts.rowid
		WHERE events_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, matchExpression(tokens), limit)
	if err != nil {
		return nil, fmt.Errorf("search: match: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hits []Hit
	for rows.Next() {
		var h Hit
		// The stored payload is read and never returned. It goes to
		// [excerpt], which masks it whole before cutting anything out of
		// it, and the window is all that leaves this function (I-10).
		var payload []byte
		if err := rows.Scan(&h.ID, &h.Host, &h.EventName, &h.ReceivedAtMS, &payload); err != nil {
			return nil, fmt.Errorf("search: scan a hit: %w", err)
		}
		h.Excerpt = excerpt(payload, tokens)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: read the hits: %w", err)
	}
	return hits, nil
}
