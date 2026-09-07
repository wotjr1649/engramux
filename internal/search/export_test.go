package search

import (
	"context"
	"database/sql"
	"errors"
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
	return searchWith(ctx, db, text, projectID, limit, false, 0, nil, m)
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
	return searchWith(ctx, db, text, projectID, limit, true, human, nil, m)
}

// SearchLiftingIDs is [SearchAtHumanWeight] with the same weight keyed on a set
// of event ids rather than on the event-name column, so that a gate can run one
// corpus under M11's rule and under a rule the index cannot express yet.
//
// ids is the per-query set of documents the candidate rule lifts, and what
// decides membership belongs to the gate rather than to this seam. Gate M12
// passes the documents whose match is not machine-only; gate M13 passes the
// documents the query is a large enough share of. It was called
// SearchAtHumanTextMatch while M12 was the only caller, and the name moved when
// the second one arrived - a set that is not about human text carried by a
// function that says it is would be one of the two going stale.
//
// It must be non-nil: nil is the shipped event-name rule, and passing it here by
// accident would measure M11 twice and report it as a difference.
//
// It ships nothing and cannot: [orderExpr]'s doc comment says why an id set is a
// measuring instrument rather than a candidate implementation.
func SearchLiftingIDs(ctx context.Context, db *sql.DB, text, projectID string, limit int, m Match, human float64, ids []string) ([]Hit, int64, error) {
	if ids == nil {
		return nil, 0, errNilLiftedIDs
	}
	return searchWith(ctx, db, text, projectID, limit, true, human, ids, m)
}

// errNilLiftedIDs is what [SearchLiftingIDs] refuses a nil set with, rather
// than silently measuring the rule it is there to be compared against.
var errNilLiftedIDs = errors.New("search: the lifted id set is nil, which is the event-name rule and not a candidate rule")

// HumanTextEvents is the closed set [orderExpr] tests `events.event_name`
// against, reachable from the gate so that it can assert the set agrees with the
// payload-key rule it classifies documents by. Without it the gate would be
// measuring one rule and the ranking obeying another, and nothing would say so.
var HumanTextEvents = humanTextEvents
