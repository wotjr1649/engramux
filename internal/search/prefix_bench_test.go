package search_test

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// BenchmarkPrefixIndex prices spec 5.7's `prefix='2 3 4'` clause.
//
// The clause buys no recall - a bare two-character query matches nothing with
// or without it, and only a trailing star matches at all - so the only thing
// left to decide it on is the latency it buys against the index it costs.
// Nobody had measured either. This is that measurement.
//
// Two indexes over one set of rows: the one the migration created, and one this
// benchmark creates with the opposite prefix setting. Same events, same
// tokenizer, same queries, so the only difference the numbers can carry is the
// clause.
//
// One thing does differ and is inert here: the migration sets `secure-delete`
// on its table and this benchmark does not set it on the alternate. That option
// only changes what a delete leaves behind, nothing is deleted from either
// index inside the timed window, and both are rebuilt at every scale - so it
// cannot reach either the latency or the size figures.
//
// The queries are two-character Korean prefixes derived from the corpus by the
// gate's own rule, so they are the shapes real captures actually carry.
//
// It skips without a raw corpus, and the scale factor takes 901 documents to
// about 18,000 - a few days of the live installation's capture rate, where 901
// is a few hours of it.
//
//	go test -p 1 -run '^$' -bench BenchmarkPrefixIndex -benchtime 50x -timeout 25m -v ./internal/search/
func BenchmarkPrefixIndex(b *testing.B) {
	docs := corpusDocs(b) // skips when .capture/ is absent
	db := ingestAll(b, docs)

	alt, prodLabel, altLabel := alternateIndex(b, db)
	queries := koreanPrefixQueries(b, docs)
	b.Logf("%d two-character Korean prefix queries derived from %d documents", len(queries), len(docs))

	targets := []struct{ table, label string }{
		{ftsTable, prodLabel},
		{alt, altLabel},
	}

	for _, scale := range []int{1, prefixScaleFactor} {
		for range scale - 1 {
			ingestInto(b, db, slices.Clone(docs))
		}
		// Both, at every scale. The migration's index is maintained by
		// triggers from here on and the alternate one is not, and an
		// incrementally maintained index is fragmented differently from
		// a rebuilt one - which is a real difference and not the one
		// being measured. Rebuilding both makes the size figures
		// reproducible too.
		for _, tg := range targets {
			rebuild(b, db, tg.table)
		}
		events := countEvents(b, db)

		// Both indexes must return the same rows for every query, or
		// the two latencies are not measuring the same work. This is
		// the check that would catch a rebuild that silently did
		// nothing - the failure mode an external-content index has.
		requireSameHits(b, db, targets[0].table, targets[1].table, queries)

		for _, tg := range targets {
			bytes := indexBytes(b, db, tg.table)
			b.Run(fmt.Sprintf("events=%d/%s", events, tg.label), func(b *testing.B) {
				p50, mean := timeQueries(b, db, tg.table, queries)
				// ns/op is one sample, which is
				// sampleSweeps * len(queries) queries. These
				// two are per query.
				b.ReportMetric(float64(p50.Nanoseconds()), "p50-ns/query")
				b.ReportMetric(float64(mean.Nanoseconds()), "mean-ns/query")
				b.ReportMetric(float64(bytes), "index-B")
			})
		}
	}
}

// ftsTable is the index the migration creates, and prefixScaleFactor is how
// many times the corpus is ingested for the larger of the two scales.
const (
	ftsTable          = "events_fts"
	altTable          = "events_fts_alt"
	prefixScaleFactor = 20
)

// searchLimit is the LIMIT the timed query carries. Ranking still visits the
// whole match set, so this bounds the rows fetched and not the work done - it
// is here so the measurement is a search result page rather than an export.
const searchLimit = 20

// prefixClause and tokenizeClause are spec 5.7's two tokenizer settings as the
// migration spells them. tokenizeClause is asserted against the migration's own
// DDL below, so a tokenizer that changes there and not here fails loudly rather
// than producing two indexes that are not comparable.
// gosec's G101 fires on tokenizeClause: an identifier containing "token" that holds a string with
// an "=" in it. It is FTS5's tokenizer clause and not a credential.
//
//nolint:gosec // G101: false positive, see above
const (
	prefixClause   = "prefix = '2 3 4'"
	tokenizeClause = "tokenize = 'unicode61 remove_diacritics 2'"
)

// productDDL is the statement the migration created [ftsTable] with, as SQLite
// kept it, having first asserted that it still spells the tokenizer the way
// [tokenizeClause] does.
//
// Both harnesses that build a second index start here rather than writing their
// own CREATE: [alternateIndex] varies the prefix clause and [porterIndex]
// varies the tokenizer, and each needs everything it did not vary to be exactly
// what the migration wrote. A tokenizer that moves in the migration and not
// here fails loudly instead of producing two indexes that are not comparable.
func productDDL(t testing.TB, db *sql.DB) string {
	t.Helper()
	var ddl string
	if err := db.QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_schema WHERE name = ?`, ftsTable).Scan(&ddl); err != nil {
		t.Fatalf("read the DDL of %s: %v", ftsTable, err)
	}
	if !strings.Contains(ddl, tokenizeClause) {
		t.Fatalf("the migration no longer spells the tokenizer %q, so two indexes built from this would "+
			"not be comparable:\n%s", tokenizeClause, ddl)
	}
	return ddl
}

// swapInDDL replaces the first occurrence of old in a copy of the migration's
// statement, and fails when there was nothing to replace.
//
// The failure is the point. A substitution that silently matched nothing
// produces a second index that differs from the shipped one in some way its
// harness did not intend and does not report, which is exactly how
// [alternateIndex] came to index a different column from the one production
// indexes and to go on reporting numbers for it.
func swapInDDL(t testing.TB, ddl, old, replacement string) string {
	t.Helper()
	out := strings.Replace(ddl, old, replacement, 1)
	if out == ddl {
		t.Fatalf("the migration's statement holds no %q to replace, so the index built from it would "+
			"not be the one this harness means to build:\n%s", old, ddl)
	}
	return out
}

// alternateIndex creates a second external-content index over events carrying
// the opposite prefix setting to the migration's, rebuilds it, and returns its
// name along with a label for each of the two.
//
// It is the migration's own statement with the table name changed and the
// prefix clause added or removed - never a CREATE this file spells out. It used
// to spell one out, over a column named `payload`, which was right until the
// index moved to `leaves` underneath it: after that the two indexes read two
// different columns of `events` and held different text, so the benchmark timed
// two things that were not comparable. Deriving from the DDL is what makes the
// next schema change fail loudly here instead of silently.
//
// Measured against this corpus, at 901 events: with the `payload` column the
// benchmark aborts at [requireSameHits] on the first query, before any timing -
// the two indexes return the same number of hits and not the same rows. With
// the column taken from the migration, all ten queries agree.
func alternateIndex(b *testing.B, db *sql.DB) (table, prodLabel, altLabel string) {
	b.Helper()
	ddl := productDDL(b, db)

	// The exact clause, not the word: the migration's own comment explains
	// why the prefix index is absent, and sqlite_schema keeps whatever
	// comments fall inside the statement text.
	prodHasPrefix := strings.Contains(ddl, prefixClause)
	if !prodHasPrefix && strings.Contains(ddl, "prefix") {
		b.Fatalf("the migration mentions a prefix index but not as %q, so this harness cannot tell "+
			"which of the two settings it carries:\n%s", prefixClause, ddl)
	}
	alt := swapInDDL(b, ddl, ftsTable, altTable)
	prodLabel, altLabel = "prefix", "no-prefix"
	if prodHasPrefix {
		// Removing it takes the comma and the indentation in front of
		// the clause with it. This branch is unreached today and
		// [swapInDDL] is what says so if the migration ever writes the
		// clause some other way.
		alt = swapInDDL(b, alt, ",\n    "+prefixClause, "")
	} else {
		prodLabel, altLabel = "no-prefix", "prefix"
		alt = swapInDDL(b, alt, tokenizeClause, tokenizeClause+",\n    "+prefixClause)
	}

	if _, err := db.ExecContext(b.Context(), alt); err != nil {
		b.Fatalf("create the %s index: %v", altLabel, err)
	}
	rebuild(b, db, altTable)
	return altTable, prodLabel, altLabel
}

// rebuild reindexes an external-content table from its content table.
//
// table is interpolated because a table name cannot be a bind parameter. Every
// caller passes one of the three index-name constants this package's tests
// declare, and no value from the corpus reaches it.
func rebuild(t testing.TB, db *sql.DB, table string) {
	t.Helper()
	//nolint:gosec // G202: table is a constant, never a value
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO `+table+`(`+table+`) VALUES('rebuild')`); err != nil {
		t.Fatalf("rebuild %s: %v", table, err)
	}
}

// indexBytes is the on-disk size of an FTS5 index: the sum of every block in
// its %_data shadow table.
func indexBytes(b *testing.B, db *sql.DB, table string) int64 {
	b.Helper()
	var n sql.NullInt64
	if err := db.QueryRowContext(b.Context(),
		`SELECT sum(length(block)) FROM `+table+`_data`).Scan(&n); err != nil {
		b.Fatalf("size of %s: %v", table, err)
	}
	return n.Int64
}

// countEvents is how many rows both indexes are built over.
func countEvents(b *testing.B, db *sql.DB) int64 {
	b.Helper()
	var n int64
	if err := db.QueryRowContext(b.Context(), `SELECT count(*) FROM events`).Scan(&n); err != nil {
		b.Fatalf("count events: %v", err)
	}
	return n
}

// koreanPrefixQueries derives distinct two-character Korean prefix queries from
// the corpus, using the gate's own rule so they are the shapes real captures
// carry, and wraps each as `"XX"*` - the quoted form spec 5.7 measured, since a
// bare token* is a syntax error on half of what this corpus holds.
func koreanPrefixQueries(b *testing.B, docs []doc) []string {
	b.Helper()
	var out []string
	for _, d := range docs {
		p := deriveKoreanTwoChar(d)
		if p == "" {
			continue
		}
		q := `"` + p + `"*`
		if !slices.Contains(out, q) {
			out = append(out, q)
		}
		if len(out) == prefixQueryCount {
			break
		}
	}
	if len(out) < prefixQueryCount {
		b.Fatalf("the corpus yielded %d distinct two-character Korean prefixes, want %d",
			len(out), prefixQueryCount)
	}
	return out
}

// prefixQueryCount is how many distinct prefixes are timed. Enough that one
// unusually cheap or unusually common prefix cannot be the whole number, and
// small enough that the sample stays a rotation rather than a corpus sweep.
const prefixQueryCount = 10

// hitsFor runs one prefix query against one index in the shape [search.Search]
// uses, and returns the ids it matched. The query text is bound; only the table
// name is interpolated, and that is a constant from this file.
func hitsFor(b *testing.B, db *sql.DB, table, query string) []string {
	b.Helper()
	//nolint:gosec // G202: table is a constant from this file, never a value
	rows, err := db.QueryContext(b.Context(), `
		SELECT events.id
		FROM `+table+`
		JOIN events ON events.rowid = `+table+`.rowid
		WHERE `+table+` MATCH ?
		ORDER BY rank
		LIMIT ?`, query, searchLimit)
	if err != nil {
		b.Fatalf("%s MATCH %s: %v", table, query, err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			b.Fatalf("%s MATCH %s: scan: %v", table, query, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		b.Fatalf("%s MATCH %s: %v", table, query, err)
	}
	return ids
}

// requireSameHits asserts the two indexes answer every query identically. A
// prefix index changes how a term is found and not which rows carry it, so any
// difference here means one of the two indexes is not built over these rows.
func requireSameHits(b *testing.B, db *sql.DB, left, right string, queries []string) {
	b.Helper()
	for _, q := range queries {
		l, r := hitsFor(b, db, left, q), hitsFor(b, db, right, q)
		if len(l) == 0 {
			b.Fatalf("%s MATCH %s returned nothing, so timing it measures nothing", left, q)
		}
		if !slices.Equal(l, r) {
			b.Fatalf("%s and %s disagree on %s: %d hits against %d", left, right, q, len(l), len(r))
		}
	}
}

// sampleSweeps is how many times one timed sample runs the whole query set.
//
// It is not a smoothing knob, it is what makes the sample measurable at all.
// Go's monotonic clock on Windows steps in about a millisecond here, and a
// single query at the smaller scale is a few hundred microseconds: timed one at
// a time, more than half the samples read exactly zero and the median is zero.
// Eight sweeps of ten queries puts every sample twenty milliseconds or more
// above that floor, and the per-query figure is the sample divided by the
// queries in it. Repeat that measurement before trusting a smaller number here.
const sampleSweeps = 8

// timeQueries returns the median and the mean per-query latency, both derived
// from samples of [sampleSweeps] full sweeps of the query set.
//
// The median as well as the mean testing already reports: one prefix that
// happens to be in most documents is an outlier a mean carries into the
// decision and a p50 does not.
//
// Every sample but the first runs against a warm page cache, which is the state
// a service that stays running is in.
func timeQueries(b *testing.B, db *sql.DB, table string, queries []string) (p50, mean time.Duration) {
	b.Helper()
	perSample := time.Duration(sampleSweeps * len(queries))
	took := make([]time.Duration, 0, b.N)
	for b.Loop() {
		start := time.Now()
		for range sampleSweeps {
			for _, q := range queries {
				hitsFor(b, db, table, q)
			}
		}
		took = append(took, time.Since(start))
	}
	var total time.Duration
	for _, d := range took {
		total += d
	}
	slices.Sort(took)
	return took[len(took)/2] / perSample, total / (time.Duration(len(took)) * perSample)
}
