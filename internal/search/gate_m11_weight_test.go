package search_test

import (
	"database/sql"
	"encoding/json"
	"slices"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
)

// Gate M11 proper (memory spec 5): the plumbing down-weight, both arms of one
// run over one corpus at every weight of the sweep.
//
// The non-vacuity arm beside this one says there is something large to win -
// 41% of queries could move from invisible to visible. This says whether winning
// it costs more than it is worth, and it is allowed to answer that no weight
// ships, in which case the seam comes out and backlog 48's ranking half closes
// on the table.
//
// # The harm arm is the point, not a formality
//
// Its three classes are M4's, unchanged, and their targets are exactly the
// documents the weight pushes down. A weight that lifts a prompt by burying the
// document that actually ran the command has moved the defect rather than fixed
// it, and a gate with only the gain arm cannot see that.
//
// # Two anchors, and a run that misses either has a defect rather than a finding
//
// At weight 0 the gain arm must reproduce the non-vacuity arm's own figures -
// recall@10 is one minus its `buried`, so 6 of 17 and 76 of 139 - and the harm
// arm must reproduce M4's recorded recall@10 with the boost on, 17, 13 and 19 of
// 25. Both are pinned as counts and not as shares, because a share that rounds
// to the right number is a weaker claim than the count that produced it. They
// are what makes weight 0 [Search]'s own answer rather than a second ranking
// that happens to agree: [search.SearchAtHumanWeight] leaves the derived-field
// boost on for exactly that reason.
//
// # Nothing here logs a query
//
// Two of the five classes cut theirs from a prompt or an assistant message and
// three from a command line or a path. Counts and figures only, which is the
// stricter of the two rules those arms are under.
func TestGateM11TheWeightEarnsItsPlace(t *testing.T) {
	docs := corpusDocs(t) // skips when the local corpus is absent
	db := ingestAll(t, docs)
	plumbing := m11Plumbing(docs)

	// The ranking keys on events.event_name, which is a column with an
	// index; the arms classify on two payload keys, which are not. Memory
	// spec 5's M11 says the two agree, and this asserts it rather than
	// repeating it - the day a host adds an event or renames one, this line
	// is what says so.
	var disagree int
	for _, d := range docs {
		byName := !slices.Contains(search.HumanTextEvents, m11EventNameOf(d.payload))
		if byName != plumbing[d.id] {
			disagree++
		}
	}
	if disagree != 0 {
		t.Errorf("the ranking's event-name set and the arms' payload-key rule disagree on %d of %d "+
			"documents; memory spec 5's M11 has them agreeing on all of them, and the shortcut from "+
			"the keys to the column is only safe while they do", disagree, len(docs))
	}

	classes := m11GateClasses(t, docs)
	found := map[m11Cell]int{}
	mrr := map[m11Cell]float64{}
	for _, w := range m11Weights {
		for _, k := range classes {
			f, m := m11ScoreAt(t, db, k.candidates, w)
			cell := m11Cell{k.name, w}
			found[cell], mrr[cell] = f, m
		}
	}

	for _, k := range classes {
		for i, w := range m11Weights {
			got := found[m11Cell{k.name, w}]
			if got == k.pinned[i] {
				continue
			}
			if w == 0 {
				t.Errorf("%s: recall@%d at weight 0 found %d of %d, pinned %d. The baseline has to "+
					"reproduce the arm it came from, so a difference here is this gate being wrong "+
					"rather than a finding about the weight.",
					k.name, m11K, got, len(k.candidates), k.anchor)
				continue
			}
			t.Errorf("%s: recall@%d at weight %.0f found %d of %d, pinned %d. Re-measure and correct "+
				"memory spec 5's M11 table rather than relaxing this comparison.",
				k.name, m11K, w, got, len(k.candidates), k.pinned[i])
		}
	}

	for _, k := range classes {
		for _, w := range m11Weights {
			cell := m11Cell{k.name, w}
			t.Logf("%-4s  %-22s  weight %5.0f  recall@%d %.3f (%2d of %3d)  MRR %.3f",
				k.arm(), k.name, w, m11K,
				float64(found[cell])/float64(len(k.candidates)),
				found[cell], len(k.candidates), mrr[cell])
		}
	}

	var ships []float64
	for _, w := range m11Weights {
		if w == 0 {
			continue
		}
		var improved, regressed bool
		for _, k := range classes {
			base, at := found[m11Cell{k.name, 0}], found[m11Cell{k.name, w}]
			// Recall is gated on its own and never traded against MRR,
			// which is M4's rule and is here for M4's reason: a ranking
			// change that loses a document it used to find is a defect
			// whatever the averages say.
			if at < base {
				regressed = true
			}
			if k.gain && at > base {
				improved = true
			}
		}
		if improved && !regressed {
			ships = append(ships, w)
		}
	}
	t.Logf("M11: weights that improve recall@%d in a gain class and regress it in none of the five: %v",
		m11K, ships)

	if !slices.Equal(ships, m11ShippableWeights) {
		t.Errorf("M11: %v meet the condition, pinned %v. Re-measure and correct memory spec 5's M11 "+
			"table rather than relaxing this comparison; the condition was registered before the "+
			"gate was built.", ships, m11ShippableWeights)
	}
	if len(ships) == 0 {
		t.Logf("M11: no weight in the sweep meets the condition. By this gate's own terms that closes " +
			"backlog 48's ranking half on the table and takes the seam out rather than shipping one.")
	}
}

// m11Weights is the sweep, and it is M4's at M4's numbers because the two
// questions have the same shape: 0 is the baseline the rest are read against,
// and 20 and 100 are there to show where the term stops buying anything.
var m11Weights = []float64{0, 1, 2, 3, 4, 5, 20, 100}

// m11ShippableWeights is the measured answer, pinned. Empty would mean no weight
// in the sweep earns its place and the seam comes out.
var m11ShippableWeights []float64

// m11Cell is one class at one weight.
type m11Cell struct {
	class  string
	weight float64
}

// m11GateClass is one of the five: the two gain classes are M11's and the three
// harm classes are M4's, unchanged and over M4's own sample.
type m11GateClass struct {
	name       string
	gain       bool
	candidates []m4Candidate
	// anchor is recall@[m11K] at weight 0 as a count, from the arm this
	// class comes from rather than from this gate.
	anchor int
	// pinned is this gate's own row: recall@[m11K] as a count at each of
	// [m11Weights], measured 2026-09-06.
	//
	// The whole row and not only the verdict, for the reason the non-vacuity
	// arm pins all of its counts: the verdict is "no weight ships", and a pin
	// on that alone sits still through a term with the wrong sign, the wrong
	// set, or no effect at all - three mutations that leave the answer empty
	// while changing every number that produced it. pinned[0] has to equal
	// anchor, which is what catches a table transcribed from a run it did not
	// come from.
	//
	// MRR is logged and not pinned. Recall is the gated statistic - M4's
	// rule, that a ranking change losing a document it used to find is a
	// defect whatever the averages say - and forty more floats would be forty
	// more things to re-measure for no extra assertion.
	pinned []int
}

func (k m11GateClass) arm() string {
	if k.gain {
		return "gain"
	}
	return "harm"
}

// m11GateClasses builds all five, with the anchors the baseline has to
// reproduce.
//
// The harm arm goes through [m4Sample] and the gain arm does not, and that is
// the two arms inheriting their own gates' decisions rather than a new one: M4
// samples 25 because it runs two searches per document, and M11's arm measures
// every candidate because it runs one. Changing either here would make the
// anchors unreachable, which is the point of having them.
func m11GateClasses(t *testing.T, docs []doc) []m11GateClass {
	t.Helper()
	anchors := map[string]int{
		"a prompt's own words": 6, "a reply's own words": 76,
		"a command line": 17, "a touched path": 13, "an error message": 19,
	}
	// One row per class, in [m11Weights] order.
	rows := map[string][]int{
		"a prompt's own words": {6, 6, 6, 6, 6, 6, 13, 13},
		"a reply's own words":  {76, 82, 87, 88, 96, 107, 135, 135},
		"a command line":       {17, 17, 17, 17, 17, 17, 13, 13},
		"a touched path":       {13, 12, 12, 12, 12, 12, 10, 10},
		"an error message":     {19, 19, 19, 19, 19, 19, 17, 17},
	}
	var out []m11GateClass
	add := func(name string, gain bool, candidates []m4Candidate) {
		if len(candidates) == 0 {
			t.Fatalf("%s: no candidate document in %d; the class measures nothing", name, len(docs))
		}
		anchor, ok := anchors[name]
		if !ok {
			t.Fatalf("%s: no anchor for this class; a class without one is measured against nothing", name)
		}
		row, ok := rows[name]
		if !ok {
			t.Fatalf("%s: no pinned row for this class", name)
		}
		if len(row) != len(m11Weights) {
			t.Fatalf("%s: the pinned row has %d entries and the sweep has %d weights",
				name, len(row), len(m11Weights))
		}
		if row[0] != anchor {
			t.Fatalf("%s: the pinned row starts at %d and the anchor is %d; the table was transcribed "+
				"from a run it did not come from", name, row[0], anchor)
		}
		out = append(out, m11GateClass{name: name, gain: gain, candidates: candidates,
			anchor: anchor, pinned: row})
	}
	for _, c := range m11Classes {
		add(c.name, true, m11CandidatesFor(t, docs, c))
	}
	for _, c := range m4Classes {
		add(c.name, false, m4Sample(m4CandidatesFor(t, docs, c)))
	}
	return out
}

// m11ScoreAt is recall@[m11K] as a count, and MRR, for one class at one weight.
//
// [search.MatchAll] is M4's matcher, and the gain arm is unaffected by the
// choice: [m4LongestToken] returns one token, and one token builds the same FTS5
// expression either way.
func m11ScoreAt(t *testing.T, db *sql.DB, candidates []m4Candidate, human float64) (int, float64) {
	t.Helper()
	var found int
	var reciprocal float64
	for _, c := range candidates {
		hits, _, err := search.SearchAtHumanWeight(t.Context(), db, c.query, "", m11K, search.MatchAll, human)
		if err != nil {
			// A refusal is a broken gate and not a result, which is
			// [m4Measure]'s rule. The token is not in the message -
			// queryTokens keeps it out deliberately.
			t.Fatalf("a derived query was refused: %v", err)
		}
		if rank := slices.IndexFunc(hits, func(h search.Hit) bool { return h.ID == c.id }); rank >= 0 {
			found++
			reciprocal += 1 / float64(rank+1)
		}
	}
	return found, reciprocal / float64(len(candidates))
}

// m11EventNameOf reads hook_event_name out of a payload, which is what
// internal/store files an event under and therefore what the ordering expression
// tests. A payload that is not an object, or carries no name, answers "" - and
// "" is in no set, so such a document is plumbing to both rules and cannot make
// them disagree by accident.
func m11EventNameOf(payload json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ""
	}
	name, _ := m["hook_event_name"].(string)
	return name
}
