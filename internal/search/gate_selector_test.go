package search_test

import (
	"database/sql"
	"slices"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
)

// selectorK is the k of recall@k, and it is the same 10 gate M4 uses so the two
// gates' figures can be read beside each other.
const selectorK = 10

// TestGateSelectorEarnsItsPlace measures [search.MatchAny] against
// [search.MatchAll] over one corpus in one run, on spec 8's five known-item
// classes, and fails when the OR loses a document the AND used to find.
//
// # Why this gate exists and the rank ceiling it replaced does not
//
// The first attempt asserted that every fixtures-mode answer sits at rank 1,
// which is what the five in-repository documents return. It was fake, and the
// break-it pass is what said so: reversing the whole ORDER BY left every class
// green, because in that mode a derived query matches exactly one document and a
// set of one has no order to get wrong. What survives a total inversion of the
// ranking is not measuring the ranking (AGENTS.md).
//
// The corpus has the ranks that mean something and cannot be pinned: `.capture/`
// grows whenever the owner uses the machine, and the upper medians had already
// moved from spec 7.1's 3 / 3 / 10 / 9 / 30 to 4 / 4 / 6 / 8 / 30 with no change
// to this package in between. So the number to assert on is not an absolute at
// all - it is the difference between two arms of the same run, which is gate
// M4's shape and is immune to the corpus by construction.
//
// # What it asserts, and what it only reports
//
// Recall is gated and MRR is reported. A wider match set is the whole point of
// [search.MatchAny] and it will move ranks; what it may not do is push a
// document out of the top ten that the AND had in it, because that is the
// precision loss recall@10 alone cannot see and is the reason spec 8's classes
// were not enough on their own (memory spec rev.11).
//
// MRR is reported rather than gated because the two arms do not rank the same
// population: the OR's ten come out of a match set an order of magnitude larger,
// so a lower MRR at equal recall is the expected shape of the trade rather than
// a regression. A number that always moves one way is a number to read, not a
// threshold to set.
//
// # Nothing here logs a derived query
//
// [TestPhase4Gate]'s corpus mode does, and AGENTS.md carries what that costs.
// This gate derives from the same corpus and therefore logs counts and figures
// only. Its output is safe to paste; do not add a query to it.
func TestGateSelectorEarnsItsPlace(t *testing.T) {
	docs := corpusDocs(t) // skips when the local corpus is absent
	db := ingestAll(t, docs)

	var lost, informative []string
	for _, c := range classes {
		cands, total := candidatesFor(c, docs)
		if total == 0 {
			t.Errorf("%s: no document carries a candidate, so this class measures nothing", c.name)
			continue
		}

		all := selectorMeasure(t, db, cands, search.MatchAll)
		any := selectorMeasure(t, db, cands, search.MatchAny)

		t.Logf("%s: %d candidates of %d documents, %d sampled", c.name, len(cands), len(docs), len(cands))
		t.Logf("%s: MatchAll recall@%d %.3f (%d of %d), MRR %.3f, match set mean %d",
			c.name, selectorK, all.recall, all.found, len(cands), all.mrr, all.meanSet)
		t.Logf("%s: MatchAny recall@%d %.3f (%d of %d), MRR %.3f, match set mean %d",
			c.name, selectorK, any.recall, any.found, len(cands), any.mrr, any.meanSet)

		// A class whose derived query is one token cannot tell the two
		// modes apart: there is nothing to join, so the expressions are
		// byte-identical and so is everything downstream of them. Four of
		// spec 8's five are that shape - only "two tokens" derives a pair -
		// and a run where none of them widened is a run that compared a
		// mode against itself.
		if any.meanSet != all.meanSet {
			informative = append(informative, c.name)
		}

		if any.found < all.found {
			lost = append(lost, c.name)
		}
	}

	if len(informative) == 0 {
		t.Errorf("no class produced a different match set under MatchAny than under MatchAll, so every "+
			"figure above compares a mode against itself and none of them could have failed. Either "+
			"the join stopped being applied, or every class derived a single-token query - four of "+
			"the five do, and %q is the one that does not", twoTokensClass)
	}
	t.Logf("the two modes were distinguishable in %v of %d classes", informative, len(classes))

	if len(lost) > 0 {
		t.Errorf("the selector lost documents the implicit AND used to find, in %v. recall@%d is "+
			"not tradeable against a wider match set: a known-item literal that fell out of the top "+
			"ten is the precision cost this gate exists to price, and memory spec rev.11's decision "+
			"was to introduce a rule and measure it at exactly this point rather than before it",
			lost, selectorK)
	}
}

// selectorResult is one arm's figures over one class.
type selectorResult struct {
	found   int
	recall  float64
	mrr     float64
	meanSet int
}

// selectorMeasure runs every candidate of one class through one match mode.
//
// The limit is [selectorK] and not the document count, which is the difference
// between this and [gateClass]: that one asks whether the index can reach the
// document at all, and this one asks whether the ranking still puts it where a
// reader looks.
func selectorMeasure(t *testing.T, db *sql.DB, cands []candidate, m search.Match) selectorResult {
	t.Helper()
	var out selectorResult
	var reciprocal float64
	var sets int64
	for _, cd := range cands {
		hits, total, err := search.Search(t.Context(), db, cd.query, "", selectorK, m)
		if err != nil {
			// A refusal is a broken derivation rather than a miss, and
			// averaging one into a recall figure hides it. Same rule as
			// gate M4's.
			t.Fatalf("a derived query was refused: %v", err)
		}
		sets += total
		if rank := slices.IndexFunc(hits, func(h search.Hit) bool { return h.ID == cd.id }); rank >= 0 {
			out.found++
			reciprocal += 1 / float64(rank+1)
		}
	}
	if len(cands) > 0 {
		out.recall = float64(out.found) / float64(len(cands))
		out.mrr = reciprocal / float64(len(cands))
		out.meanSet = int(sets / int64(len(cands)))
	}
	return out
}
