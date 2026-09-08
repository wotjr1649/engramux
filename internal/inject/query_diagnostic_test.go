package inject_test

import (
	"database/sql"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/search"
)

// This measures the existing selector without choosing replacements or exposing
// prompt text. The already-labelled sample is development evidence, not a holdout.
func TestDiagnoseSelectedQueryTerms(t *testing.T) {
	dir := os.Getenv("ENGRAMUX_QUERY_DIAGNOSTIC_DIR")
	if dir == "" {
		t.Skip("query diagnostic is opt-in")
	}
	uri, err := temporalSourceURI(m7Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	source, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	source.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})
	prompts := m7LabelledPrompts(t, source, dir, m7Agent)
	stamps := map[string]int64{}
	for _, p := range prompts {
		var stamp int64
		if err := source.QueryRowContext(t.Context(), "SELECT received_at FROM events WHERE id=?", p.id).Scan(&stamp); err != nil {
			t.Fatal("read timestamp")
		}
		stamps[p.id] = stamp
		if m7Drifted(p) {
			t.Fatal("project identity drift")
		}
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	prefix, err := newTemporalReplay(t, m7Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(prompts, func(i, j int) bool { return stamps[prompts[i].id] < stamps[prompts[j].id] })
	type counts struct{ prompts, terms, zeroTerms, broadTerms, anyAbsent, allAbsent, combinedEmpty, intersectionEmpty int }
	groups := map[string]*counts{}
	for _, p := range prompts {
		if err := prefix.advance(t, stamps[p.id]); err != nil {
			t.Fatal(err)
		}
		key := p.wanted + "/" + p.script
		c := groups[key]
		if c == nil {
			c = &counts{}
			groups[key] = c
		}
		c.prompts++
		terms := inject.QueryFor(p.prompt)
		if len(terms) == 0 {
			continue
		}
		absent := 0
		for _, term := range terms {
			_, total, err := search.Search(t.Context(), prefix.db, term, p.project, 1, search.MatchAll)
			if err != nil {
				t.Fatal("single-term diagnostic refused")
			}
			c.terms++
			if total == 0 {
				c.zeroTerms++
				absent++
			}
			if total > 200 {
				c.broadTerms++
			}
		}
		if absent > 0 {
			c.anyAbsent++
		}
		if absent == len(terms) {
			c.allAbsent++
		}
		_, total, err := search.Search(t.Context(), prefix.db, strings.Join(terms, " "), p.project, 1, search.MatchAll)
		if err != nil {
			t.Fatal("combined diagnostic refused")
		}
		if total == 0 {
			c.combinedEmpty++
			if absent == 0 {
				c.intersectionEmpty++
			}
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		c := groups[key]
		t.Logf("%s: prompts=%d terms=%d absent_terms=%d broad_terms=%d prompts_any_absent=%d prompts_all_absent=%d empty_AND=%d empty_intersection_with_all_terms_present=%d", key, c.prompts, c.terms, c.zeroTerms, c.broadTerms, c.anyAbsent, c.allAbsent, c.combinedEmpty, c.intersectionEmpty)
	}
}
