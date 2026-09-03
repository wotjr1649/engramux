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
	return searchWith(ctx, db, text, projectID, limit, false, m)
}
