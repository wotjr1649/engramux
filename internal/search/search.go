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
// # The rows are read first and masked afterwards, and that is not a style
//
// The two phases below look like one loop that was pulled apart for no reason.
// They were pulled apart because the service has exactly one database
// connection (spec 5.4, and internal/store sets SetMaxOpenConns(1)), and an
// open *sql.Rows holds it. [excerpt] is the most expensive thing in this
// package - eight RE2 patterns over every string leaf, a JSON decode and
// re-encode when anything matched, then a second walk of the result - and
// running it inside the cursor would hold that one connection for the whole of
// it, per row.
//
// The service hands the same pool to the ingest handler, and internal/pipe
// serves a goroutine per connection, so what would wait is every concurrent
// store.Ingest: at the cap of 100 hits over spec 7.4's largest observed payload
// that is plausibly hundreds of milliseconds against the relay's 800 ms
// post-dial budget (spec 5.3). Nothing is lost when a relay blows its budget -
// it spools, and I-04 holds - but a search that quietly slows ingest is not a
// trade this package gets to make silently.
//
// So: scan, check Err, close, and only then mask. Anyone tidying this back into
// one loop is reintroducing that.
//
// limit goes to LIMIT unmodified, and SQLite reads a negative LIMIT as "no
// limit". [github.com/wotjr1649/engramux/internal/ipc.SearchRequest.EffectiveLimit]
// is what the pipe surface bounds it with before it gets here.
//
// total is how many rows the MATCH and the filter admitted before the limit
// (backlog 33). It is a window count in the same statement rather than a
// second query, so it counts exactly the rows the hits were ranked from, and
// it costs nothing the ORDER BY was not already paying: ranking needs every
// matching row in hand before the first one can be returned.
func Search(ctx context.Context, db *sql.DB, text, projectID string, limit int) (hits []Hit, total int64, err error) {
	tokens, err := queryTokens(text)
	if err != nil {
		return nil, 0, err
	}

	query, args := matchQuery(tokens, projectID, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search: match: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Phase one: read the rows and nothing else.
	//
	// The stored payload is read and never returned. It goes to [excerpt]
	// below, which masks it whole before cutting anything out of it, and the
	// window is all that leaves this function (I-10).
	type row struct {
		hit     Hit
		payload []byte
	}
	var scanned []row
	for rows.Next() {
		var r row
		// The window count is the same value on every row; scanning it
		// each time is what keeps the column list and the scan list one
		// to one.
		if err := rows.Scan(&r.hit.ID, &r.hit.Host, &r.hit.EventName, &r.hit.ReceivedAtMS, &r.payload, &total); err != nil {
			return nil, 0, fmt.Errorf("search: scan a hit: %w", err)
		}
		scanned = append(scanned, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search: read the hits: %w", err)
	}
	// Closed here rather than only by the defer, because the cursor must be
	// gone before the masking starts - see this function's doc comment for
	// what holding it costs. Close is idempotent, so the defer above stays
	// as the path every error return takes.
	if err := rows.Close(); err != nil {
		return nil, 0, fmt.Errorf("search: close the hits: %w", err)
	}

	// Phase two: the connection is free, so this can take as long as it
	// takes.
	hits = make([]Hit, len(scanned))
	for i, r := range scanned {
		hits[i] = r.hit
		hits[i].Excerpt = excerpt(r.payload, tokens)
	}
	return hits, total, nil
}

// matchQuery builds the statement and its arguments: the MATCH, an optional
// project filter, and the limit.
//
// # The filter is a predicate on events and it cannot be anything else
//
// events_fts is an external-content table with one indexed column, and spec 5.7
// measured why: a `project_id UNINDEXED` column beside the leaves planned
// identically to the same MATCH unfiltered, so the constraint never reached the
// virtual table's index and the column bought one byte. The filter is therefore
// applied to the joined `events` row, after the MATCH has produced it and before
// the LIMIT counts it. What that costs when a small project's limit makes SQLite
// walk a long ranked list is measured in BenchmarkProjectScope, and spec 5.7
// carries the number.
//
// # An empty project is every project, and the statement is then the old one
//
// The unscoped statement is byte-identical to the one this package sent before
// scoping existed, rather than being the scoped one with a predicate that
// happens to be true. A disjunction like `(? = ” OR project_id = ?)` would read
// as tidier and would put an unmeasured expression in front of every existing
// invocation for the sake of one fewer string.
func matchQuery(tokens []string, projectID string, limit int) (string, []any) {
	const (
		head = `
		SELECT events.id, events.host, events.event_name, events.received_at, events.payload,
		       count(*) OVER ()
		FROM events_fts
		JOIN events ON events.rowid = events_fts.rowid
		WHERE events_fts MATCH ?`
		tail = `
		ORDER BY rank
		LIMIT ?`
	)
	if projectID == "" {
		return head + tail, []any{matchExpression(tokens), limit}
	}
	return head + `
		  AND events.project_id = ?` + tail,
		[]any{matchExpression(tokens), projectID, limit}
}
