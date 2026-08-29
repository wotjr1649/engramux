package search

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
