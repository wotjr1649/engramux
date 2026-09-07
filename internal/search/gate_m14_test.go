package search_test

import (
	"database/sql"
	"slices"
	"strings"
	"testing"
)

// Gate M14 (memory spec 5): the threshold M13 left open, asked over the
// populations M13's sample could not see.
//
// # The question, and it is one question
//
// M13 licensed a length column and no shipped term. What it left open is
// whether a threshold *between* 5,000 and 20,000 ppm keeps the reply gain
// without the `an error message` loss - its own five rungs were spaced to
// bracket the population and were never spaced to answer that. This gate asks
// it and asks nothing else: same rule, same weight, one ladder and one
// condition.
//
// # Why this is not a grid chosen from an answer already seen
//
// Adding rungs to [m13Thresholds] would be exactly that. Two of its five
// cleared M13's condition and the one between them did not, so a value inserted
// there is picked from that shape and would move a verdict already recorded.
// Nothing of M13's moves. What moves is the population the condition is taken
// over: [m4Sample]'s 25 becomes every candidate, which for the three harm
// classes is 21, 5 and 4 times as many documents, and that makes M11's sentence
// strictly *harder* to pass over the arm where M13's own supplementary run
// already says the harm is. A rule tuned to an answer is a rule that was made
// easier.
//
// # Three anchors, and only one of them tests the rule
//
// Weight 0 over the sampled 25 is M11's committed baseline - 6, 76, 17, 13, 19.
// Weight 0 over every candidate is M11's on the gain classes and M12's
// supplementary table on the harm ones - 336, 56, 79. Neither says anything
// about coverage, because at weight 0 no id set is passed at all. The third is
// [m14Class.m13], every column M13 measured at the five rungs both ladders
// carry: without it a first run would be pinning whatever came out of it, and
// with it this gate's own set builder has to agree with M13's over 906
// documents before any new rung is believed.
//
// # Nothing here logs a query
//
// Two of the five classes cut theirs from a prompt or from an assistant's
// message, which is M11's stricter rule and M13's, and it governs this whole
// file: class names, counts and figures only.
func TestGateM14TheThresholdM13LeftOpen(t *testing.T) {
	docs := corpusDocs(t) // skips when the local corpus is absent
	db := ingestAll(t, docs)
	text := m13IndexedText(docs)

	classes := m14Classes(t, docs)
	rows := make(map[string]m14Row, len(classes))
	for _, k := range classes {
		r, sampled := m14Measure(t, k, db, docs, text)
		rows[k.name] = r

		t.Logf("%-4s  %-22s  n %3d  recall@%d  weight 0 %3d  at %v ppm %v",
			k.arm(), k.name, len(k.candidates), m11K, r.base, m14Thresholds, r.at)
		t.Logf("%-4s  %-22s  of the %d top-%d places its queries fill at weight 0, %v hold a document "+
			"the rule does not lift at %v ppm",
			k.arm(), k.name, r.visible, m11K, r.demoted, m14Thresholds)

		// The two weight-0 anchors. A run that misses either is measuring
		// a different corpus, and every count below would then be about
		// a corpus this gate is misreading rather than about coverage.
		if sampled != k.sampledAnchor {
			t.Errorf("%s: recall@%d at weight 0 over the sampled %d found %d, and M11's own baseline "+
				"is %d. The gate has to reproduce the arm it is comparing against.",
				k.name, m11K, len(k.sampled), sampled, k.sampledAnchor)
		}
		if r.base != k.fullAnchor {
			t.Errorf("%s: recall@%d at weight 0 over all %d found %d, and the committed figure is %d.",
				k.name, m11K, len(k.candidates), r.base, k.fullAnchor)
		}

		// The anchor that tests the rule. Every column M13 measured over
		// this same population has to come back unchanged, recall and
		// demoted alike, before a rung M13 did not measure means
		// anything.
		for j, tau := range m13Thresholds {
			i := m14IndexOf(t, tau)
			if r.at[i] != k.m13.at[j] {
				t.Errorf("%s: recall@%d at %d ppm found %d of %d, and M13 measured %d over the same "+
					"population. This gate's set builder and M13's are not the same rule.",
					k.name, m11K, tau, r.at[i], len(k.candidates), k.m13.at[j])
			}
			if r.demoted[i] != k.m13.demoted[j] {
				t.Errorf("%s: at %d ppm %d of the top-%d places hold a document the rule does not "+
					"lift, and M13 measured %d.", k.name, tau, r.demoted[i], m11K, k.m13.demoted[j])
			}
		}
		if r.visible != k.m13.visible {
			t.Errorf("%s: its queries fill %d top-%d places at weight 0 and M13 measured %d over the "+
				"same population.", k.name, r.visible, m11K, k.m13.visible)
		}

		if r != k.pinned {
			t.Errorf("%s: measured %+v, pinned %+v. Re-measure and correct memory spec 5's M14 figures "+
				"rather than relaxing this comparison.", k.name, r, k.pinned)
		}
	}

	// The condition, registered in memory spec 5's M14 section before this
	// file existed: M11's own sentence, over every candidate of every class.
	var licensed []int
	for i, tau := range m14Thresholds {
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
	t.Logf("M14: thresholds in ppm that improve recall@%d in a gain class and regress it in none of "+
		"the five, over every candidate: %v", m11K, licensed)
	if !slices.Equal(licensed, m14Licensed) {
		t.Errorf("M14: %v meet the condition, pinned %v. Re-measure and correct memory spec 5's M14 "+
			"section rather than editing this line; the condition was registered before the gate "+
			"was built.", licensed, m14Licensed)
	}

	// Registered in advance rather than found: M13's supplementary table
	// already says all five of its rungs regress a harm class over these
	// populations, so only the four new rungs can license anything here.
	for _, tau := range m13Thresholds {
		if slices.Contains(licensed, tau) {
			t.Errorf("M14: %d ppm is licensed here and M13's supplementary run has it regressing a "+
				"harm class over the same population. The two gates disagree about one corpus, "+
				"which is a defect in one of them rather than a finding.", tau)
		}
	}
	if len(licensed) == 0 {
		t.Logf("M14: no threshold from %d to %d ppm clears M11's condition over the full "+
			"populations at weight %.0f. By this gate's own terms backlog 53's third candidate is "+
			"measured out at that weight, and what remains is a sweep over the weight.",
			m14Thresholds[0], m14Thresholds[len(m14Thresholds)-1], m14Weight)
	}
}

// m14Weight is [m13Weight], which is [m12Weight], which is 5 for M11's reason.
// This gate does not sweep the weight and therefore cannot license one; what it
// sweeps is where "a large enough share" starts.
const m14Weight = m13Weight

// m14Thresholds is the ladder, in parts per million of the document's indexed
// text: [m13Thresholds]'s five, and the 2,500 ppm grid filling the interval
// M13's spacing left open between 5,000 and 20,000.
//
// Uniform spacing rather than values chosen to sit anywhere in particular,
// which is the only defence a free parameter has. 1,000 and 2,000 are here for
// a cost reason and memory spec 5 states it rather than implying it: M13's
// supplementary arm is retired into this gate, and those two rungs are two of
// its columns.
var m14Thresholds = [9]int{1_000, 2_000, 5_000, 7_500, 10_000, 12_500, 15_000, 17_500, 20_000}

// m14Licensed is the measured answer, pinned: the thresholds meeting the
// condition over every candidate of every class.
//
// **It is empty**, so no threshold anywhere from 1,000 to 20,000 ppm clears
// M11's condition at weight [m14Weight] over the full populations, and by this
// gate's own terms backlog 53's third candidate is measured out at that weight.
//
// The interval M13 left open does hold a better rung and it does not hold a
// clean one. At 7,500 ppm `a reply's own words` reaches 93 of 139, which is
// higher than anything M13's ladder could see, `a command line` reaches 346 of
// 534 and `a touched path` gains its only document anywhere - 57 of 120 - so
// four of the five classes are at or above their baseline and the fifth is the
// veto. `an error message` is 74 of 96 there and is short of 79 at every rung
// in the ladder, which is the sentence M13's supplementary run wrote and this
// one closes: the class that vetoes is not traded against, it is simply lost.
var m14Licensed []int

// m14Row is one class's measurement over its full population, and every count
// in it is pinned rather than only the verdict - M11's rule, M12's and M13's.
// The verdict is a list that may well be empty, and an empty list survives a
// coverage rule inverted, a threshold compared the wrong way round, or a set
// that never reached the statement.
type m14Row struct {
	// base is recall@[m11K] as a count at weight 0, and at is the same at
	// [m14Weight] for each of [m14Thresholds] in order.
	base int
	at   [9]int
	// visible is how many top-[m11K] places this class's queries fill at
	// weight 0, and demoted how many of those hold a document the rule does
	// not lift at each threshold.
	visible int
	demoted [9]int
}

// m14Class is one of the five over its whole population, with the sampled 25
// carried alongside as the subset M11's baseline was taken over and M13's
// committed row as the anchor that tests the rule.
type m14Class struct {
	name       string
	gain       bool
	candidates []m4Candidate
	// sampled is the id set of [m4Sample]'s 25 for a harm class, and every
	// candidate for a gain class, whose weight-0 recall is sampledAnchor.
	// It costs no search: the sample is a subset of the population this
	// gate already measures document by document.
	sampled       map[string]bool
	sampledAnchor int
	fullAnchor    int
	m13           m13Row
	pinned        m14Row
}

func (k m14Class) arm() string {
	if k.gain {
		return "gain"
	}
	return "harm"
}

// m14Classes builds all five over their full populations.
//
// The gain classes and their M13 rows come out of [m13Classes] rather than
// being written again here, because M11's arm already measures those two over
// every candidate and two spellings of one table is one of them going stale.
// The harm classes are the same three widened from [m4Sample]'s 25 to every
// candidate, and their M13 rows are M13's supplementary table transcribed once
// - which is the arm memory spec 5 retires into this gate on the condition that
// these numbers come back.
func m14Classes(t *testing.T, docs []doc) []m14Class {
	t.Helper()
	// M13's supplementary table, over the same three populations this gate
	// measures. Its weight-0 column is M12's, which is the second anchor.
	supp := map[string]m13Row{
		"a command line": {base: 336, at: [5]int{336, 336, 342, 345, 349},
			visible: 4627, demoted: [5]int{258, 438, 1108, 1771, 2803}},
		"a touched path": {base: 56, at: [5]int{54, 54, 56, 56, 55},
			visible: 1046, demoted: [5]int{78, 176, 350, 610, 854}},
		"an error message": {base: 79, at: [5]int{76, 73, 74, 75, 78},
			visible: 776, demoted: [5]int{215, 333, 555, 674, 740}},
	}
	// Weight-0 recall over the full population: M11's own figures on the
	// two gain classes, whose arm is already every candidate, and M12's
	// supplementary table on the three harm ones. Written out rather than
	// read off the rows above, so that a digit dropped out of that table
	// fails as a transcription error here instead of arriving as a
	// disagreement about the rule.
	full := map[string]int{
		"a prompt's own words": 6, "a reply's own words": 76,
		"a command line": 336, "a touched path": 56, "an error message": 79,
	}
	pins := map[string]m14Row{
		"a prompt's own words": {base: 6, at: [9]int{6, 6, 6, 6, 6, 6, 6, 6, 6},
			visible: 153, demoted: [9]int{0, 4, 23, 58, 69, 85, 85, 88, 89}},
		"a reply's own words": {base: 76, at: [9]int{83, 86, 92, 93, 92, 90, 86, 79, 82},
			visible: 1136, demoted: [9]int{157, 208, 427, 511, 579, 710, 769, 883, 910}},
		"a command line": {base: 336, at: [9]int{336, 336, 342, 346, 345, 347, 347, 348, 349},
			visible: 4627, demoted: [9]int{258, 438, 1108, 1423, 1771, 2079, 2287, 2623, 2803}},
		"a touched path": {base: 56, at: [9]int{54, 54, 56, 57, 56, 55, 55, 55, 55},
			visible: 1046, demoted: [9]int{78, 176, 350, 460, 610, 644, 776, 848, 854}},
		"an error message": {base: 79, at: [9]int{76, 73, 74, 74, 75, 76, 78, 78, 78},
			visible: 776, demoted: [9]int{215, 333, 555, 627, 674, 689, 718, 729, 740}},
	}

	harm := make(map[string]m4Class, len(m4Classes))
	for _, c := range m4Classes {
		harm[c.name] = c
	}

	var out []m14Class
	for _, k := range m13Classes(t, docs) {
		candidates, m13 := k.candidates, k.pinned
		if c, ok := harm[k.name]; ok {
			candidates = m4CandidatesFor(t, docs, c)
			row, ok := supp[k.name]
			if !ok {
				t.Fatalf("%s: no M13 supplementary row for a harm class", k.name)
			}
			m13 = row
			if len(candidates) <= len(k.candidates) {
				t.Fatalf("%s: %d candidates over the full population and %d over the sample; this "+
					"gate is measuring the arm it exists to widen",
					k.name, len(candidates), len(k.candidates))
			}
		}
		anchor, ok := full[k.name]
		if !ok {
			t.Fatalf("%s: no weight-0 anchor over the full population for this class", k.name)
		}
		if m13.base != anchor {
			t.Fatalf("%s: M13's row over this population starts at %d and the committed weight-0 "+
				"figure is %d; the table was transcribed from a run it did not come from",
				k.name, m13.base, anchor)
		}
		pinned, ok := pins[k.name]
		if !ok {
			t.Fatalf("%s: M13 has a class this gate has no pinned row for", k.name)
		}
		sampled := make(map[string]bool, len(k.candidates))
		for _, c := range k.candidates {
			sampled[c.id] = true
		}
		out = append(out, m14Class{
			name: k.name, gain: k.gain, candidates: candidates,
			sampled: sampled, sampledAnchor: k.anchor, fullAnchor: anchor,
			m13: m13, pinned: pinned,
		})
	}
	if len(out) != len(pins) {
		t.Fatalf("M14: %d classes and %d pinned rows", len(out), len(pins))
	}
	return out
}

// m14IndexOf is where one of [m13Thresholds]'s rungs sits in [m14Thresholds].
//
// It fails rather than returning -1: the five shared rungs are what the third
// anchor is made of, and a ladder edited so that one of them is gone would
// otherwise stop checking it silently.
func m14IndexOf(t *testing.T, tau int) int {
	t.Helper()
	i := slices.Index(m14Thresholds[:], tau)
	if i < 0 {
		t.Fatalf("M14: %d ppm is one of M13's rungs and is not in this gate's ladder; the anchor "+
			"that tests the rule cannot be taken", tau)
	}
	return i
}

// m14Lifted is the per-query set the rule keys on, at every threshold in one
// scan of the corpus.
//
// The rule is [m13CoveragePPM] itself and not a second spelling of it. What
// differs from [m13Lifted] is only the shape: coverage does not depend on the
// threshold, so one scan answers nine rungs where M13 scanned once per rung -
// which is what keeps a ladder nearly twice as long from costing nearly twice
// as much.
//
// No set is ever nil, because nil is the shipped event-name rule and
// [search.SearchLiftingIDs] refuses it. Each is built in document order, so a
// run is reproducible.
func m14Lifted(docs []doc, text map[string]string, query string) [9][]string {
	lowered := strings.ToLower(query)
	var out [9][]string
	for i := range out {
		out[i] = []string{}
	}
	for _, d := range docs {
		indexed, ok := text[d.id]
		if !ok || !strings.Contains(indexed, lowered) {
			continue
		}
		ppm := m13CoveragePPM(lowered, indexed)
		for i, tau := range m14Thresholds {
			if ppm >= tau {
				out[i] = append(out[i], d.id)
			}
		}
	}
	return out
}

// m14Measure runs one class ten ways - weight 0, and [m14Weight] at each of
// [m14Thresholds] - over every candidate, and returns the row alongside the
// weight-0 recall of the sampled subset.
//
// The sampled count is the same searches counted twice rather than a second
// arm: [m4Sample]'s 25 are a subset of the population being measured here, so
// M11's baseline is an anchor this gate gets for nothing.
func m14Measure(t *testing.T, k m14Class, db *sql.DB, docs []doc, text map[string]string) (m14Row, int) {
	t.Helper()
	var r m14Row
	var sampled int
	for _, c := range k.candidates {
		// Every class's query is cut out of one of its own document's
		// string leaves, so a candidate that does not contain its own
		// query means the derivation and the indexed text disagree.
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
			if k.sampled[c.id] {
				sampled++
			}
		}
		top := base[:min(len(base), m11K)]
		r.visible += len(top)

		lifted := m14Lifted(docs, text, c.query)
		for i, tau := range m14Thresholds {
			// The candidate is in its own lifted set exactly when its
			// own coverage reaches the threshold - containment is
			// already established above, so this is the numeric half
			// of the rule asserted against the set it produced.
			wantMember := m13CoveragePPM(strings.ToLower(c.query), own) >= tau
			if slices.Contains(lifted[i], c.id) != wantMember {
				t.Errorf("%s: at %d ppm a candidate's membership of its own lifted set and its own "+
					"coverage disagree. The set and the rule are not the same rule.", k.name, tau)
			}
			if m13Holds(m13Hits(t, db, c, m14Weight, lifted[i]), c.id) {
				r.at[i]++
			}
			for _, h := range top {
				if !slices.Contains(lifted[i], h.ID) {
					r.demoted[i]++
				}
			}
		}
	}
	return r, sampled
}
