package search_test

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
)

// The derived-field boost is a ranking input and nothing else (memory spec M-3,
// and M-2's decision 7 for why it is not indexed text). What it is supposed to
// do is one sentence: a document that carries the query in a field a person was
// asking about - the command it ran, the file it touched, what it answered -
// beats a document that merely mentions the same words in prose.
//
// That sentence is what this file asserts, and it asserts it in both directions.
// A test that only showed the boosted order would pass against a Search that had
// always ranked that way for some unrelated reason, so every case here also runs
// the same query with the boost off and requires the order to be *different*
// there. Gate M4 measures whether the effect is worth keeping over the whole
// corpus; this measures that the effect exists and is the one that was built.
func TestTheDerivedBoostLiftsAFieldMatchOverAProseMention(t *testing.T) {
	for _, tc := range []struct {
		name string
		// query is what a person types.
		query string
		// field is the payload whose derived column carries the query.
		field string
		// prose is the payload that says the same words and derives
		// nothing, and which bm25 must rank first without the boost -
		// which is what makes the case a case.
		prose string
	}{
		{
			name:  "a command line beats a mention of the command",
			query: "gofmt",
			field: `{"tool_name":"Bash","tool_input":{"command":"gofmt -l ./internal"}}`,
			prose: `{"prompt":"gofmt gofmt gofmt gofmt gofmt gofmt gofmt gofmt"}`,
		},
		{
			name:  "a touched path beats a mention of the path",
			query: "excerpt.go",
			field: `{"tool_name":"Write","tool_input":{"file_path":"Z:/w/excerpt.go","content":"x"}}`,
			prose: `{"prompt":"excerpt.go excerpt.go excerpt.go excerpt.go excerpt.go excerpt.go"}`,
		},
		{
			name:  "what a tool answered beats a mention of the same text",
			query: "malformed",
			field: `{"hook_event_name":"PostToolUse","tool_response":{"stdout":"database disk image is malformed"}}`,
			prose: `{"prompt":"malformed malformed malformed malformed malformed malformed"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, name := boostDB(t, map[string]string{
				"the field document": tc.field,
				"the prose document": tc.prose,
			})

			boosted := rankOrder(t, name, func(limit int) ([]search.Hit, int64, error) {
				return search.Search(t.Context(), db, tc.query, "", limit)
			})
			unboosted := rankOrder(t, name, func(limit int) ([]search.Hit, int64, error) {
				return search.SearchUnboosted(t.Context(), db, tc.query, "", limit)
			})

			// Both arms must find both documents. A case where the
			// boost "won" because the unboosted arm returned one row
			// is measuring the MATCH, not the boost.
			if len(boosted) != 2 || len(unboosted) != 2 {
				t.Fatalf("both arms must return both documents; boosted %d, unboosted %d",
					len(boosted), len(unboosted))
			}
			if boosted[0] != "the field document" {
				t.Errorf("with the boost on, %q ranks first; want %q", boosted[0], "the field document")
			}
			if unboosted[0] != "the prose document" {
				t.Errorf("with the boost off, %q ranks first; want %q - this case is not measuring the boost",
					unboosted[0], "the prose document")
			}
			if slices.Equal(boosted, unboosted) {
				t.Errorf("the two arms produced the same order %v; the boost changed nothing", boosted)
			}
		})
	}
}

// TestTheDerivedBoostChangesNoResultSet is the boundary M-3 draws and M-1 rests
// on: the boost is allowed to reorder and is not allowed to admit or drop a
// document. A boost written into the WHERE clause instead of the ORDER BY would
// pass every ordering assertion above and quietly turn a ranking input into a
// filter - which is a different feature, and one nobody asked for.
func TestTheDerivedBoostChangesNoResultSet(t *testing.T) {
	db, name := boostDB(t, map[string]string{
		"a command":     `{"tool_input":{"command":"go test -p 1 ./..."}}`,
		"a path":        `{"tool_input":{"file_path":"Z:/w/search/query.go"}}`,
		"an answer":     `{"tool_response":{"stdout":"go test: no packages to test"}}`,
		"only prose":    `{"prompt":"go test is the command and query.go is the file"}`,
		"nothing of it": `{"prompt":"unrelated text about something else entirely"}`,
	})

	for _, query := range []string{"go", "test", "query.go", "go test", "packages", "unrelated"} {
		boosted := rankOrder(t, name, func(limit int) ([]search.Hit, int64, error) {
			return search.Search(t.Context(), db, query, "", limit)
		})
		unboosted := rankOrder(t, name, func(limit int) ([]search.Hit, int64, error) {
			return search.SearchUnboosted(t.Context(), db, query, "", limit)
		})
		// Sorted, because what is compared is the set and not the order
		// - the order is the whole point of the boost and is asserted
		// above.
		slices.Sort(boosted)
		slices.Sort(unboosted)
		if !slices.Equal(boosted, unboosted) {
			t.Errorf("query %q: the boost changed the result set\n  with:    %v\n  without: %v",
				query, boosted, unboosted)
		}
		if len(boosted) == 0 {
			t.Errorf("query %q matched nothing in either arm, so it compares nothing", query)
		}
	}
}

// boostDB builds a database holding one event per named payload, and returns it
// with the map from the event id [store.Ingest] minted to the caller's own name
// for the document. The name is what [rankOrder] reports, so a failure names the
// document rather than a UUID.
func boostDB(t *testing.T, byName map[string]string) (*sql.DB, map[string]string) {
	t.Helper()
	docs := make([]doc, 0, len(byName))
	for name, payload := range byName {
		docs = append(docs, doc{name: name, payload: []byte(payload)})
	}
	// Sorted, so the ingest order - and therefore the rowid order a tie in
	// rank falls back on - is the same on every run. Map iteration is not.
	slices.SortFunc(docs, func(a, b doc) int { return strings.Compare(a.name, b.name) })
	db := ingestAll(t, docs)
	name := make(map[string]string, len(docs))
	for _, d := range docs {
		name[d.id] = d.name
	}
	return db, name
}

// rankOrder runs one search over the whole result set and returns the document
// names in rank order.
//
// The limit is larger than any document set this file builds rather than a small
// k, because these tests compare two full orderings; recall@10 is gate M4's
// measurement and not this file's.
func rankOrder(t *testing.T, name map[string]string, run func(limit int) ([]search.Hit, int64, error)) []string {
	t.Helper()
	hits, _, err := run(boostProbeLimit)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	names := make([]string, len(hits))
	for i, h := range hits {
		n, ok := name[h.ID]
		if !ok {
			t.Fatalf("a hit carries an id no document was ingested under")
		}
		names[i] = n
	}
	return names
}

// boostProbeLimit is larger than any document set this file builds, so no
// ranking difference can hide behind a limit.
const boostProbeLimit = 100
