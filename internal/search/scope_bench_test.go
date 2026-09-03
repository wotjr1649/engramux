package search_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// BenchmarkProjectScope prices the project filter spec 5.9 adds to search.
//
// # The question, and why it could not be reasoned
//
// events_fts is an external-content table with one indexed column, because spec
// 5.7 measured that a `project_id UNINDEXED` column beside the leaves planned
// identically to the same MATCH unfiltered - the constraint never reached the
// virtual table's index. So the filter is a predicate on the joined `events`
// row: post-MATCH, pre-LIMIT.
//
// That has a shape worth measuring rather than assuming. SQLite walks the ranked
// list until the LIMIT is filled, and the filter discards rows on the way. For a
// project holding most of the matches the walk stops almost immediately; for one
// holding fewer matching documents than the limit it **cannot** stop early,
// because the rows to fill the limit do not exist - so it walks the whole ranked
// list. That is the worst case, it is reached without arranging any particular
// rank order, and it is what the third arm below measures.
//
// # The three arms
//
//   - unscoped: today's query, the control. The limit fills from the front.
//
//   - scoped, large: a project holding a large share of the matches.
//
//   - scoped, tiny: a project holding fewer matches than the limit, which is the
//     full walk.
//
//     go test -p 1 -run '^$' -bench BenchmarkProjectScope -benchtime 200x -v ./internal/search/
func BenchmarkProjectScope(b *testing.B) {
	const (
		projects = 40
		perLarge = 500
		perTiny  = 3
		limit    = 20
	)
	db, large, tiny, total := scopedCorpus(b, projects, perLarge, perTiny)
	b.Logf("%d events across %d projects: the large project holds %d, the tiny one %d, limit %d",
		total, projects, perLarge, perTiny, limit)
	b.Log(planOf(b, db, scopeTerm, large, limit))

	for _, term := range []struct {
		name string
		text string
	}{
		{"every-document", scopeTerm},
		{"one-in-a-hundred", scopeRareTerm},
	} {
		for _, arm := range []struct {
			name    string
			project string
		}{
			{"unscoped", ""},
			{"scoped-large", large},
			{"scoped-tiny", tiny},
		} {
			b.Run(term.name+"/"+arm.name, func(b *testing.B) {
				var hits int
				for b.Loop() {
					h, _, err := search.Search(b.Context(), db, term.text, arm.project, limit, search.MatchAll)
					if err != nil {
						b.Fatalf("search: %v", err)
					}
					hits = len(h)
				}
				// Reported rather than asserted: it is what
				// says which of the shapes above was actually
				// exercised, and a tiny arm that came back with
				// a full limit would mean the fixture and not
				// the filter was being measured.
				b.ReportMetric(float64(hits), "hits")
			})
		}
	}
}

// scopeTerm is in every document the benchmark inserts, so the MATCH selects the
// whole population and the only thing separating the arms is the filter.
//
// scopeRareTerm is in one document in a hundred, and it is there because the
// first run of this benchmark showed the term to be the thing that decides the
// cost. `ORDER BY rank` makes FTS5 score every matching document before it
// returns the first row, so a term in every document is a full scan whether or
// not anything is scoped. Measuring only that would price the MATCH and call it
// the filter.
const (
	scopeTerm     = "frobnicatorscope"
	scopeRareTerm = "frobnicatorrare"
	rareEvery     = 100
)

// scopedCorpus builds one database of synthetic events spread over several
// projects, and returns the ids of a large one and a tiny one.
//
// The rows are inserted directly rather than through store.Ingest, in one
// transaction. That is not a shortcut around the production path: what is being
// measured is a SELECT, the FTS triggers that maintain the index are the
// migration's own and still fire, and ingesting tens of thousands of events one
// transaction at a time would spend minutes on the half that is not the
// measurement.
//
// events.leaves is written explicitly because the AFTER INSERT trigger indexes
// that column, and nothing here calls store.Leaves to derive it. It holds the
// same text the payload does, which is what the walk in Go would have produced
// for a single-leaf payload.
func scopedCorpus(b *testing.B, projects, perLarge, perTiny int) (db *sql.DB, large, tiny string, total int) {
	b.Helper()
	db, err := store.Open(b.Context(), filepath.Join(b.TempDir(), "engramux.db"))
	if err != nil {
		b.Fatalf("store.Open: %v\nA \"database is locked\" here is a development service holding "+
			"its own file, not this one - see AGENTS.md", err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Errorf("close the database: %v", err)
		}
	})
	if err := store.Migrate(b.Context(), db); err != nil {
		b.Fatalf("store.Migrate: %v", err)
	}

	tx, err := db.BeginTx(b.Context(), nil)
	if err != nil {
		b.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	exec := func(query string, args ...any) {
		b.Helper()
		if _, err := tx.ExecContext(b.Context(), query, args...); err != nil {
			b.Fatalf("seed: %v", err)
		}
	}
	for p := range projects {
		id := fmt.Sprintf("p%03d", p)
		exec(`INSERT INTO projects (id, root, name, created_at) VALUES (?, ?, ?, 0)`,
			id, fmt.Sprintf(`z:\scope\%03d`, p), id)
		exec(`INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at)
		      VALUES (?, ?, 'codex', ?, 'active', 0)`, "codex:"+id, id, id)

		// The last project is the tiny one. Everything before it holds
		// perLarge events, so the ranked list is long whichever project
		// is scoped to.
		n := perLarge
		if p == projects-1 {
			n = perTiny
		}
		for i := range n {
			// The filler differs per event so the index is not one
			// term repeated, and its length is the same in every
			// project so no arm is measuring a different document
			// size.
			leaves := scopeTerm + " " + strings.Repeat("filler ", 20) + fmt.Sprintf("doc%03d%05d", p, i)
			// Every hundredth document, counted across the whole
			// corpus rather than per project, so the tiny project
			// carries some of them too.
			if total%rareEvery == 0 {
				leaves += " " + scopeRareTerm
			}
			exec(`INSERT INTO events (id, project_id, session_id, host, source, event_name,
			                          payload, leaves, privacy_class, redaction_version, received_at)
			      VALUES (?, ?, ?, 'codex', 'pipe', 'PostToolUse', ?, ?, '', 1, ?)`,
				fmt.Sprintf("%03d-%05d", p, i), id, "codex:"+id,
				`{"text":"`+leaves+`"}`, leaves, int64(i))
			total++
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("commit: %v", err)
	}
	return db, "p000", fmt.Sprintf("p%03d", projects-1), total
}

// planOf returns the query plan of the scoped statement, which is the half of
// this measurement that survives a machine change. Spec 5.7 records plan strings
// for the same reason.
func planOf(b *testing.B, db *sql.DB, text, projectID string, limit int) string {
	b.Helper()
	rows, err := db.QueryContext(b.Context(), `
		EXPLAIN QUERY PLAN
		SELECT events.id, events.host, events.event_name, events.received_at, events.payload
		FROM events_fts
		JOIN events ON events.rowid = events_fts.rowid
		WHERE events_fts MATCH ?
		  AND events.project_id = ?
		ORDER BY rank
		LIMIT ?`, text+"*", projectID, limit)
	if err != nil {
		b.Fatalf("explain: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			b.Fatalf("scan the plan: %v", err)
		}
		out = append(out, detail)
	}
	if err := rows.Err(); err != nil {
		b.Fatalf("read the plan: %v", err)
	}
	return "scoped plan: " + strings.Join(out, " | ")
}

// BenchmarkSelectorMode prices [search.MatchAny] against [search.MatchAll] on
// one two-token query over the same synthetic corpus.
//
// It is a separate benchmark rather than a fourth arm of BenchmarkProjectScope
// because that one's numbers are in spec 7.1 and its terms are single tokens,
// where the two modes produce a byte-identical expression and there is nothing
// to compare.
//
// The pair is chosen so the two modes land on opposite ends of the corpus:
// [scopeRareTerm] is in one document in a hundred and [scopeTerm] is in all of
// them, so the AND selects the rare set and the OR selects the population. That
// is the worst case for the OR rather than a typical one - a real query's terms
// are not one-in-a-hundred and everywhere - and the worst case is what a budget
// is written against.
//
//	go test -p 1 -run '^$' -bench BenchmarkSelectorMode -benchtime 200x -v ./internal/search/
func BenchmarkSelectorMode(b *testing.B) {
	const (
		projects = 40
		perLarge = 500
		perTiny  = 3
		limit    = 20
	)
	db, _, _, total := scopedCorpus(b, projects, perLarge, perTiny)
	query := scopeRareTerm + " " + scopeTerm
	b.Logf("%d events, limit %d, two tokens: one in %d documents and one in all of them",
		total, limit, rareEvery)

	for _, mode := range []struct {
		name string
		m    search.Match
	}{
		{"MatchAll", search.MatchAll},
		{"MatchAny", search.MatchAny},
	} {
		b.Run(mode.name, func(b *testing.B) {
			var matched int64
			for b.Loop() {
				_, t, err := search.Search(b.Context(), db, query, "", limit, mode.m)
				if err != nil {
					b.Fatalf("search: %v", err)
				}
				matched = t
			}
			// The match set, reported beside the time, because it is
			// the thing that decides the time and a run where the two
			// modes matched the same number priced nothing.
			b.ReportMetric(float64(matched), "matched")
		})
	}
}
