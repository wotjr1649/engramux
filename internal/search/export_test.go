package search

import (
	"context"
	"database/sql"
)

// The query builder, reachable from this package's external test package.
//
// TestEveryCandidateDocumentIsReachable derives one query per candidate
// document per class - 2,262 of them over the corpus, per tokenizer arm - and
// has to turn each one into the expression [Search] would hand to MATCH. A
// sweep that spelled the quoting and the trailing star itself would be
// measuring a second builder, and the first time the two disagreed the
// difference would arrive looking like a recall result.
//
// These are aliases and not copies: they are the functions [Search] calls.
//
// The sweep cannot simply live in this package instead. Everything else it
// needs - the doc type, corpusDocs, ingestAll, classes and the five derivations
// - is in search_test beside the gate that owns them, and moving that gate in
// here to reach two unexported functions is a much larger change than the
// test-only export this is.
//
// This file compiles into this package's test binary and into nothing else, so
// no shipped surface gains an exported query builder.
var (
	QueryTokens     = queryTokens
	MatchExpression = matchExpression
)

// SearchUnboosted is [Search] with the derived-field boost off.
//
// Gate M4 measures the derived fields by running one corpus both ways and
// comparing recall@10 and MRR (memory spec 5), and there is no honest way to do
// that from outside the package without this. It is deliberately not a flag on
// the exported surface: nothing a caller of this package can do turns the boost
// off, so no reply anybody receives was ranked by a path the gate did not
// measure.
func SearchUnboosted(ctx context.Context, db *sql.DB, text, projectID string, limit int, m Match) ([]Hit, int64, error) {
	return searchWith(ctx, db, text, projectID, limit, false, 0, m)
}

// SearchAtHumanWeight is [Search] with gate M11's human-text weight made
// explicit, so that the gate can run one corpus at every weight of its sweep.
//
// The derived-field boost is left on, which is what makes the sweep's weight-0
// column [Search]'s own answer rather than a second ranking that happens to
// agree with it - and it is why the gate's baseline can be pinned against
// figures M4 and M11's non-vacuity arm measured through [Search] itself.
//
// It is deliberately not a flag on the exported surface. Nothing a caller of
// this package can do turns this term on, so no reply anybody receives was
// ranked by a weight the gate has not licensed.
func SearchAtHumanWeight(ctx context.Context, db *sql.DB, text, projectID string, limit int, m Match, human float64) ([]Hit, int64, error) {
	return searchWith(ctx, db, text, projectID, limit, true, human, m)
}

// HumanTextEvents is the closed set [orderExpr] tests `events.event_name`
// against, reachable from the gate so that it can assert the set agrees with the
// payload-key rule it classifies documents by. Without it the gate would be
// measuring one rule and the ranking obeying another, and nothing would say so.
var HumanTextEvents = humanTextEvents
