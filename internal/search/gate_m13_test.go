package search_test

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// Gate M13 (memory spec 5): whether how much of a document the query accounts
// for is a signal worth putting in the index.
//
// # What this gate is not
//
// It is not a sweep over weights and it is not a proposal. M11 rejected a term
// keyed on the document; M12 found that a term keyed on where the match fell is
// a different instrument and is powerless on the class that vetoes. This is
// backlog 53's third and last candidate, and it is the one neither of the others
// has the shape of: coverage is a property of the *pair*. The same path is a
// large share of a two-line prompt and a vanishing share of a 40 KB tool output,
// and no property of either document alone says so.
//
// # The candidate may already be in the ranking, and that was measured first
//
// bm25 normalises by document length - that is its `b` parameter - so a second
// term could be a coefficient the index already applies for free. The spec's M13
// section carries the probe: with the derived-field boost off the ranking puts a
// 52 B document above a 43 KB one, so the length signal is there and working;
// with the boost on, both tool documents move above the 52 B prompt.
// [boostPerDerivedToken] is flat and blind to how long the column it matched is,
// and out-voting bm25's damping is what makes this candidate a different
// instrument rather than the same one twice.
//
// # What is swept is the threshold, and the weight is fixed
//
// The expression is M11's and M12's, so 5 is M12's weight for M12's reason and
// the six columns of each row are directly comparable to two tables already
// committed. The free parameter here is where "a large enough share" starts, and
// [m13Thresholds] is what stops it being tuned - the ladder was chosen from the
// population's own decile spread before any recall figure existed.
//
// # Nothing here logs a query
//
// Two of the five classes cut theirs from a prompt or from an assistant's
// message, which is M11's stricter rule, and it governs this whole file: class
// names, counts and figures only.
func TestGateM13TheQueryShareOfTheDocument(t *testing.T) {
	docs := corpusDocs(t) // skips when the local corpus is absent
	db := ingestAll(t, docs)
	text := m13IndexedText(docs)

	classes := m13Classes(t, docs)
	rows := make(map[string]m13Row, len(classes))
	for _, k := range classes {
		rows[k.name] = m13Measure(t, k, db, docs, text)
	}

	for _, k := range classes {
		r := rows[k.name]
		t.Logf("%-4s  %-22s  n %3d  recall@%d  weight 0 %3d  at %v ppm %v",
			k.arm(), k.name, len(k.candidates), m11K, r.base, m13Thresholds, r.at)
		t.Logf("%-4s  %-22s  of the %d top-%d places its queries fill at weight 0, %v hold a document "+
			"the rule does not lift at %v ppm",
			k.arm(), k.name, r.visible, m11K, r.demoted, m13Thresholds)

		// The anchor. Weight 0 is Search's own answer and M11's committed
		// baseline, so a run that misses it has a defect in this gate
		// rather than a finding about coverage.
		if r.base != k.anchor {
			t.Errorf("%s: recall@%d at weight 0 found %d of %d, and M11's own baseline is %d. The gate "+
				"has to reproduce the arm it is comparing against.",
				k.name, m11K, r.base, len(k.candidates), k.anchor)
		}
		if r != k.pinned {
			t.Errorf("%s: measured %+v, pinned %+v. Re-measure and correct memory spec 5's M13 figures "+
				"rather than relaxing this comparison.", k.name, r, k.pinned)
		}
	}

	// The condition, and it is M11's own, registered in memory spec 5's M13
	// section before this file existed and not moved afterwards.
	var licensed []int
	for i, tau := range m13Thresholds {
		var improved, regressed bool
		for _, k := range classes {
			r := rows[k.name]
			if r.at[i] < r.base {
				regressed = true
			}
			if k.gain && r.at[i] > r.base {
				improved = true
			}
		}
		if improved && !regressed {
			licensed = append(licensed, tau)
		}
	}
	t.Logf("M13: thresholds in ppm that improve recall@%d in a gain class and regress it in none of "+
		"the five: %v", m11K, licensed)
	if !slices.Equal(licensed, m13Licensed) {
		t.Errorf("M13: %v meet the condition, pinned %v. Re-measure and correct memory spec 5's M13 "+
			"section rather than editing this line; the condition was registered before the gate "+
			"was built.", licensed, m13Licensed)
	}
	if len(licensed) == 0 {
		t.Logf("M13: no threshold in the ladder meets the condition. By this gate's own terms that " +
			"closes backlog 53's last candidate rather than licensing a length column.")
	}
}

// m13Weight is the one weight this gate measures at. It is [m12Weight]'s value
// for [m12Weight]'s reason - 5 is where M11's gain was largest before the term
// begins to dominate bm25 - and sharing it is what makes this gate's columns
// comparable to M11's and M12's rather than a third scale to reconcile.
const m13Weight = m12Weight

// m13Thresholds is the ladder, in parts per million of the document's indexed
// text, and memory spec 5's M13 section is where the five values come from: the
// five classes' own target coverages run from a median of 1,741 ppm to one of
// 23,026 ppm, and the ladder brackets that range at both ends.
//
// Parts per million and integer arithmetic rather than a float share, so that a
// threshold is exact, a row stays comparable with ==, and a log line carries no
// rounding to argue about.
//
// The length is fixed because [m13Row] pins one count per threshold and an array
// is what keeps that row comparable. A sixth value is a spec change and then a
// re-measurement, which is the order this gate is under.
var m13Thresholds = [5]int{1_000, 2_000, 5_000, 10_000, 20_000}

// m13Licensed is the measured answer, pinned: the thresholds meeting the
// condition. Empty would mean no threshold in the ladder earns a length column
// and backlog 53's last candidate closing on the table. Three of the five meet
// it, which is the first time any of row 53's candidates has.
//
// **Read [m14Licensed] before this number is quoted anywhere.** The condition is
// over M11's populations, which for the three harm classes is a sample of 25 -
// and over the full harm populations no threshold in this ladder or in M14's
// wider one clears it. The condition is not moved to account for that: it was
// registered before the gate was built, the supplementary run was registered as
// reported and unable to change the verdict, and what the pair says is that the
// licensed schema change has to be swept over the full populations before a term
// ships, not that this gate answered wrongly.
//
// The supplementary run that first said so is not in this file any more. It was
// retired into gate M14, which measures the same three populations at the same
// five rungs and four more, on the condition memory spec 5 registered for the
// move: M14's first run reproduced every one of its figures, recall and demoted
// alike, before the arm came out.
var m13Licensed = []int{2_000, 5_000, 20_000}

// m13Row is one class's measurement, and every count in it is pinned rather than
// only the verdict - M11's rule and M12's, for their reason. The verdict is a
// list that is usually empty, and an empty list survives a coverage rule
// inverted, a threshold compared the wrong way round, or a set that never
// reached the statement.
type m13Row struct {
	// base is recall@[m11K] as a count at weight 0, and at is the same at
	// [m13Weight] for each of [m13Thresholds] in order.
	base int
	at   [5]int
	// visible is how many top-[m11K] places this class's queries fill at
	// weight 0, and demoted how many of those hold a document the rule does
	// not lift at each threshold. Their ratio is the exact size of what a
	// coverage term would move, which is what M12 left for this candidate to
	// say: `machine-only` was 85 of 519 sampled and 330 of 760 over every
	// harm candidate.
	visible int
	demoted [5]int
}

// m13Class is one of the five, with M11's own weight-0 count carried alongside
// as the anchor this gate has to reproduce.
type m13Class struct {
	name       string
	gain       bool
	candidates []m4Candidate
	anchor     int
	pinned     m13Row
}

func (k m13Class) arm() string {
	if k.gain {
		return "gain"
	}
	return "harm"
}

// m13Classes builds all five from M11's, so that the populations are M11's and
// not a third sampling that would have to be explained. The anchors are read out
// of [m11GateClasses]'s own rows rather than written again here, which is
// [m12Classes]'s reason: two spellings of one table is one of them going stale.
func m13Classes(t *testing.T, docs []doc) []m13Class {
	t.Helper()
	rows := map[string]m13Row{
		"a prompt's own words": {base: 6, at: [5]int{6, 6, 6, 6, 6},
			visible: 153, demoted: [5]int{0, 4, 23, 69, 89}},
		"a reply's own words": {base: 76, at: [5]int{83, 86, 92, 92, 82},
			visible: 1136, demoted: [5]int{157, 208, 427, 579, 910}},
		"a command line": {base: 17, at: [5]int{17, 17, 18, 15, 17},
			visible: 210, demoted: [5]int{18, 42, 81, 111, 147}},
		"a touched path": {base: 13, at: [5]int{12, 13, 13, 14, 13},
			visible: 227, demoted: [5]int{17, 42, 82, 138, 189}},
		"an error message": {base: 19, at: [5]int{19, 19, 19, 18, 19},
			visible: 173, demoted: [5]int{39, 75, 123, 157, 167}},
	}
	var out []m13Class
	for _, k := range m11GateClasses(t, docs) {
		pinned, ok := rows[k.name]
		if !ok {
			t.Fatalf("%s: M11 has a class this gate has no pinned row for", k.name)
		}
		out = append(out, m13Class{
			name: k.name, gain: k.gain, candidates: k.candidates,
			anchor: k.anchor, pinned: pinned,
		})
	}
	if len(out) != len(rows) {
		t.Fatalf("M13: %d classes and %d pinned rows", len(out), len(rows))
	}
	return out
}

// m13IndexedText is every document's indexed text by id, folded to lower case
// once so that the per-query containment test below is a substring test and not
// a fold per query per document.
//
// [store.Leaves] and not the document's own leaves slice, because the length in
// the denominator has to be the length of the text the index was built over -
// which is the column `events_fts` reads and which a shipped term would have to
// take the length of. Joining the slice a second way here would measure a
// denominator no column holds.
func m13IndexedText(docs []doc) map[string]string {
	out := make(map[string]string, len(docs))
	for _, d := range docs {
		out[d.id] = strings.ToLower(store.Leaves(d.payload))
	}
	return out
}

// m13CoveragePPM is the rule: the query's byte length over the document's
// indexed byte length, in parts per million.
//
// Occurrences are not counted, and memory spec 5's M13 section says why that is
// the deliberate choice rather than the accurate one: FTS5 exposes no per-row
// term frequency to SQL, so the stronger form could not be shipped, and
// measuring it would overstate a candidate this gate exists to be sceptical of.
//
// A document with no indexed text covers nothing and is never lifted, which is
// the same answer the division would want to give and is not a special case
// worth a threshold of its own.
func m13CoveragePPM(query, indexed string) int {
	if len(indexed) == 0 {
		return 0
	}
	return len(query) * 1_000_000 / len(indexed)
}

// m13Lifted is the per-query set the rule keys on at one threshold: every
// document whose indexed text contains the query and which the query is at least
// tau parts per million of.
//
// The containment half is not the rule and does not narrow it. An FTS5 prefix
// match implies containment, so every document the query can reach is already in
// the set the coverage test then filters; what containment buys is an
// `events.id IN (…)` list the size of the match set rather than the size of the
// corpus, which at 901 documents and six searches a candidate is the difference
// between this gate fitting scripts/race.sh's budget and not.
//
// The result is never nil, because nil is the shipped event-name rule and
// [search.SearchLiftingIDs] refuses it. It is built in document order, so a run
// is reproducible.
func m13Lifted(docs []doc, text map[string]string, query string, tau int) []string {
	lowered := strings.ToLower(query)
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		indexed, ok := text[d.id]
		if !ok || !strings.Contains(indexed, lowered) {
			continue
		}
		if m13CoveragePPM(lowered, indexed) >= tau {
			out = append(out, d.id)
		}
	}
	return out
}

// m13Measure runs one class six ways: weight 0, and [m13Weight] at each of
// [m13Thresholds].
//
// [search.MatchAll] is M4's matcher and M11's, and every class's query is a
// single token, so MatchAll and MatchAny build the identical FTS5 expression and
// nothing here turns on the choice.
func m13Measure(t *testing.T, k m13Class, db *sql.DB, docs []doc, text map[string]string) m13Row {
	t.Helper()
	var r m13Row
	for _, c := range k.candidates {
		// Every class's query is cut out of one of its own document's
		// string leaves - a prompt, a reply, a command line, a path's
		// basename, a line of tool output - so a candidate that does not
		// contain its own query means the derivation and the indexed
		// text disagree, and every count below would then be about a
		// corpus this gate is misreading.
		own, ok := text[c.id]
		if !ok {
			t.Fatalf("%s: a candidate has no indexed text; ingestAll did not fill in its id", k.name)
		}
		if !strings.Contains(own, strings.ToLower(c.query)) {
			t.Errorf("%s: a candidate's own indexed text does not contain its own query. The "+
				"derivation and the text the index is built over disagree.", k.name)
			continue
		}

		base := m13Hits(t, db, c, 0, nil)
		if m13Holds(base, c.id) {
			r.base++
		}
		top := base[:min(len(base), m11K)]
		r.visible += len(top)

		for i, tau := range m13Thresholds {
			lifted := m13Lifted(docs, text, c.query, tau)
			// The candidate is in its own lifted set exactly when its
			// own coverage reaches the threshold - containment is
			// already established above, so this is the numeric half
			// of the rule asserted against the set it produced.
			wantMember := m13CoveragePPM(strings.ToLower(c.query), own) >= tau
			if slices.Contains(lifted, c.id) != wantMember {
				t.Errorf("%s: at %d ppm a candidate's membership of its own lifted set and its own "+
					"coverage disagree. The set and the rule are not the same rule.", k.name, tau)
			}
			if m13Holds(m13Hits(t, db, c, m13Weight, lifted), c.id) {
				r.at[i]++
			}
			for _, h := range top {
				if !slices.Contains(lifted, h.ID) {
					r.demoted[i]++
				}
			}
		}
	}
	return r
}

// m13Holds reports whether the target is in the top [m11K], which is what
// recall@k counts.
func m13Holds(hits []search.Hit, id string) bool {
	return slices.ContainsFunc(hits, func(h search.Hit) bool { return h.ID == id })
}

// m13Hits is one search at the top [m11K], at weight 0 when lifted is nil and
// under the coverage rule when it is not.
//
// A refusal is a broken gate and not a result, which is M4's rule, M11's and
// M12's. The token is not in the message - internal/search's own query errors
// keep it out deliberately - so a refusal here may be printed.
func m13Hits(t *testing.T, db *sql.DB, c m4Candidate, weight float64, lifted []string) []search.Hit {
	t.Helper()
	var hits []search.Hit
	var err error
	if lifted == nil {
		hits, _, err = search.SearchAtHumanWeight(t.Context(), db, c.query, "", m11K, search.MatchAll, weight)
	} else {
		hits, _, err = search.SearchLiftingIDs(t.Context(), db, c.query, "", m11K, search.MatchAll, weight, lifted)
	}
	if err != nil {
		t.Fatalf("a derived query was refused: %v", err)
	}
	return hits
}
