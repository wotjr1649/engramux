package search_test

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
)

// Gate M12 (memory spec 5): whether a term keyed on *where the match fell* is a
// different instrument from the one M11 rejected, or the same one spelled twice.
//
// # What this gate is not
//
// It is not a sweep and it is not a proposal. M11 measured a down-weight keyed
// on the document - `events.event_name IN (…)`, the column form of "this
// document carries a prompt or a reply" - and no weight in its sweep earned its
// place, because lifting a prompt above the document that actually ran the
// command costs that command line its own place. Backlog 53 asks what signal is
// missing instead, and names a second FTS column over the human-authored leaves
// as a candidate. **That candidate may not be a different instrument at all**:
// 82.2% of this corpus carries no human text, so for four documents in five a
// column weight is a uniform multiplier on the whole score - which is exactly
// the term M11 already rejected.
//
// So this asks the one question that has to be answered before a migration is
// written: on the 160 documents that carry both halves, does it matter which
// half the query matched? The answer licenses a schema change or it closes the
// candidate, and it is the same five classes over the same populations either
// way.
//
// # The subset argument, which is why three columns are enough
//
// The location rule lifts a strict subset of what M11's rule lifts: the
// human-text documents whose match is *not* machine-only. Two things follow
// arithmetically, and this file asserts both rather than trusting them - a seam
// wired to the wrong predicate would break one of them before it broke anything
// a figure could show.
//
//   - A gain class's target is lifted by both rules, because its query is the
//     longest token cut out of its own human text. So its recall can only be
//     better under the location rule than under M11's, and can only be better
//     than at weight 0.
//   - A harm class's target is lifted by neither, provided no harm candidate is
//     itself a human-text document. So its recall sits between weight 0 and
//     M11's weight 5, inclusive.
//
// # Nothing here logs a query
//
// Two of the five classes cut theirs from a prompt or from an assistant's
// message, which is M11's stricter rule, and it governs this whole file: counts
// and figures only.
func TestGateM12TheSignalIsWhereTheMatchFell(t *testing.T) {
	docs := corpusDocs(t) // skips when the local corpus is absent
	db := ingestAll(t, docs)
	humanText := m12HumanText(docs)

	if len(humanText) != m12HumanDocs {
		t.Errorf("M12: %d documents carry human text, pinned %d. Re-measure and correct memory spec "+
			"5's M12 figures.", len(humanText), m12HumanDocs)
	}
	t.Logf("M12: %d of %d documents carry a non-empty prompt or last_assistant_message, and they are "+
		"the only ones the two rules can disagree about", len(humanText), len(docs))

	classes := m12Classes(t, docs, humanText)
	rows := make(map[string]m12Row, len(classes))
	for _, k := range classes {
		rows[k.name] = m12Measure(t, db, k, docs, humanText)
	}

	for _, k := range classes {
		r := rows[k.name]
		t.Logf("%-4s  %-22s  n %3d  recall@%d  weight 0 %2d  M11 at %.0f %2d  location at %.0f %2d",
			k.arm(), k.name, len(k.candidates), m11K, r.base, m12Weight, r.doc, m12Weight, r.loc)
		t.Logf("%-4s  %-22s  of the human-text documents in its top %d at M11's weight %.0f, %d of %d "+
			"are machine-only", k.arm(), k.name, m11K, m12Weight, r.machineOnly, r.humanHits)

		// The two anchors. A run that misses either has a defect in this
		// gate rather than a finding about the signal: weight 0 is
		// Search's own answer and M11's weight-5 column is a table
		// already committed.
		if r.base != k.anchor {
			t.Errorf("%s: recall@%d at weight 0 found %d of %d, and M11's own baseline is %d. The gate "+
				"has to reproduce the arm it is comparing against.",
				k.name, m11K, r.base, len(k.candidates), k.anchor)
		}
		if r.doc != k.atWeight {
			t.Errorf("%s: recall@%d under M11's rule at weight %.0f found %d of %d, and M11's table "+
				"records %d. The gate has to reproduce the row it is comparing against.",
				k.name, m11K, m12Weight, r.doc, len(k.candidates), k.atWeight)
		}

		// The subset argument, asserted. These hold whatever the corpus
		// says, so a failure here is the seam or the classification
		// being wrong and never a finding.
		if r.loc < r.doc {
			t.Errorf("%s: the location rule found %d and M11's rule found %d. The location rule lifts a "+
				"subset of what M11's lifts, so it cannot rank a target lower - this is the seam or "+
				"the machine-only classification being wrong, not a measurement.",
				k.name, r.loc, r.doc)
		}
		if k.gain && r.loc < r.base {
			t.Errorf("%s: the location rule found %d and weight 0 found %d. A gain target is lifted by "+
				"the location rule itself, so it cannot rank lower than unlifted.",
				k.name, r.loc, r.base)
		}
		if !k.gain && r.loc > r.base {
			t.Errorf("%s: the location rule found %d and weight 0 found %d. A harm target is lifted by "+
				"neither rule, so lifting anything above it cannot raise it.",
				k.name, r.loc, r.base)
		}

		if r != k.pinned {
			t.Errorf("%s: measured %+v, pinned %+v. Re-measure and correct memory spec 5's M12 figures "+
				"rather than relaxing this comparison.", k.name, r, k.pinned)
		}
	}

	// The condition, and it is M11's own: registered in memory spec 5's M12
	// section before this file existed, and not moved afterwards.
	var improved, regressed bool
	for _, k := range classes {
		r := rows[k.name]
		if r.loc < r.base {
			regressed = true
		}
		if k.gain && r.loc > r.base {
			improved = true
		}
	}
	licensed := improved && !regressed
	t.Logf("M12: at weight %.0f the location rule improves a gain class: %t; regresses one of the five: "+
		"%t; licensed: %t", m12Weight, improved, regressed, licensed)
	if licensed != m12Licensed {
		t.Errorf("M12: the location rule is licensed: %t, pinned %t. The condition was registered before "+
			"this gate was built - re-measure and correct memory spec 5's M12 section rather than "+
			"editing this line.", licensed, m12Licensed)
	}
	if !licensed {
		t.Logf("M12: by this gate's own terms the location signal is not licensed at weight %.0f, and "+
			"what that closes is the candidate rather than backlog 53.", m12Weight)
	}

	m12Supplementary(t, db, docs, humanText)
}

// m12Weight is the one weight this gate measures at, and memory spec 5's M12
// section says why it is one rather than M11's eight: 5 is where M11's gain was
// largest before the term begins to dominate bm25, and a sweep is what the real
// gate runs once a column exists to sweep over.
const m12Weight = 5.0

// m12HumanDocs is how many of the corpus's documents carry human text, pinned.
// It is the population the two rules can disagree about at all: over a corpus
// where every document carried human text, or none did, the location rule and
// M11's rule are the same expression.
const m12HumanDocs = 160

// m12Licensed is the measured verdict, pinned. False means the location rule
// does not clear M11's condition and the schema change is not licensed.
const m12Licensed = false

// m12Row is one class's measurement.
//
// Every field is pinned and not only the verdict, for the reason M11's two arms
// pin theirs: the verdict is one boolean, and a seam keyed on the wrong column,
// a machine-only rule inverted, or a weight that never reached the statement all
// leave it where it was while changing every number behind it.
type m12Row struct {
	// base, doc and loc are recall@[m11K] as a count: at weight 0, under
	// M11's document rule at [m12Weight], and under the location rule at the
	// same weight.
	base, doc, loc int
	// humanHits is how many human-text documents appear in this class's top
	// tens under M11's rule, and machineOnly how many of those matched
	// somewhere other than their human text. Their ratio is the mechanism
	// behind whatever the verdict is: near zero says the two rules are the
	// same instrument on this corpus, near one says the signal is real and
	// this weight is not where it pays.
	humanHits, machineOnly int
}

// m12Class is one of the five, and it is M11's class with M11's two figures
// carried alongside as the anchors this gate has to reproduce.
type m12Class struct {
	name       string
	gain       bool
	candidates []m4Candidate
	// anchor is recall@[m11K] at weight 0 and atWeight is the same under
	// M11's rule at [m12Weight], both from M11's own committed table.
	anchor   int
	atWeight int
	pinned   m12Row
}

func (k m12Class) arm() string {
	if k.gain {
		return "gain"
	}
	return "harm"
}

// m12Classes builds all five from M11's, so that the populations are M11's
// populations and not a second sampling that would have to be explained.
//
// The anchors are read out of [m11GateClasses]'s own pinned rows rather than
// written again here: two spellings of one table is one of them going stale, and
// this gate's whole claim is that it is measuring the same five classes M11 did.
func m12Classes(t *testing.T, docs []doc, humanText map[string]string) []m12Class {
	t.Helper()
	rows := map[string]m12Row{
		"a prompt's own words": {base: 6, doc: 6, loc: 6, humanHits: 41, machineOnly: 17},
		"a reply's own words":  {base: 76, doc: 107, loc: 109, humanHits: 409, machineOnly: 53},
		"a command line":       {base: 17, doc: 17, loc: 17, humanHits: 21, machineOnly: 11},
		"a touched path":       {base: 13, doc: 12, loc: 12, humanHits: 31, machineOnly: 0},
		"an error message":     {base: 19, doc: 19, loc: 19, humanHits: 17, machineOnly: 4},
	}
	at := slices.Index(m11Weights, m12Weight)
	if at < 0 {
		t.Fatalf("M12 measures at weight %.0f and M11's sweep does not include it, so there is no row "+
			"to reproduce", m12Weight)
	}
	var out []m12Class
	for _, k := range m11GateClasses(t, docs) {
		pinned, ok := rows[k.name]
		if !ok {
			t.Fatalf("%s: M11 has a class this gate has no pinned row for", k.name)
		}
		out = append(out, m12Class{
			name: k.name, gain: k.gain, candidates: k.candidates,
			anchor: k.anchor, atWeight: k.pinned[at], pinned: pinned,
		})
	}
	if len(out) != len(rows) {
		t.Fatalf("M12: %d classes and %d pinned rows", len(out), len(rows))
	}
	// The precondition the harm half of the subset argument rests on. It
	// holds because store.Derive reads only tool_input and tool_response,
	// which the three human-text events do not carry - but that is a
	// property of a host's payload shape and not a law, so it is asserted
	// on every run.
	for _, k := range out {
		if k.gain {
			continue
		}
		for _, c := range k.candidates {
			if humanText[c.id] != "" {
				t.Errorf("%s: a harm candidate is itself a human-text document, so the location rule "+
					"lifts a target M11's rule also lifts and this gate's arithmetic no longer "+
					"holds. Memory spec 5's M12 section is what has to change.", k.name)
				break
			}
		}
	}
	return out
}

// m12HumanText is every document that carries human text, by id, folded to lower
// case once so that the per-query classification below is a substring test and
// not a fold per query per document.
//
// The two halves are joined rather than kept apart: a document carrying both a
// prompt and a reply is one document to the ranking, and the question here is
// whether the match fell in either of them.
func m12HumanText(docs []doc) map[string]string {
	out := make(map[string]string, len(docs))
	for _, d := range docs {
		h := m11HumanOf(d.payload)
		if h.prompt == "" && h.reply == "" {
			continue
		}
		out[d.id] = strings.ToLower(h.prompt + "\n" + h.reply)
	}
	return out
}

// m12Lifted is the per-query set the location rule keys on: every document whose
// human text contains the query.
//
// # A substring and not a token start, which is the conservative choice
//
// FTS5 anchors a prefix query at a token start, so this counts a document as
// human-matched that the index would not have reached - which lifts *more*
// documents, which is the direction that makes this candidate harder to
// license. It also makes a gain target a member of its own set by construction
// rather than by measurement, since its query is the longest token cut out of
// this very text; a token-start rule would drop a handful of targets out of
// their own set for a reason that is about the derivation and not about the
// signal.
//
// The result is never nil, because nil is the shipped event-name rule and
// [search.SearchAtHumanTextMatch] refuses it. It is built in document order, so
// a run is reproducible.
func m12Lifted(docs []doc, humanText map[string]string, query string) []string {
	lowered := strings.ToLower(query)
	out := make([]string, 0, len(humanText))
	for _, d := range docs {
		if text, ok := humanText[d.id]; ok && strings.Contains(text, lowered) {
			out = append(out, d.id)
		}
	}
	return out
}

// m12Measure runs one class three ways: weight 0, M11's document rule at
// [m12Weight], and the location rule at the same weight.
//
// [search.MatchAll] is M11's gate's matcher, and every class's query is a single
// token, so MatchAll and MatchAny build the identical FTS5 expression and
// nothing here turns on the choice.
func m12Measure(t *testing.T, db *sql.DB, k m12Class, docs []doc, humanText map[string]string) m12Row {
	t.Helper()
	var r m12Row
	for _, c := range k.candidates {
		lifted := m12Lifted(docs, humanText, c.query)
		if k.gain && !slices.Contains(lifted, c.id) {
			t.Errorf("%s: a gain target is not in its own lifted set, and its query is the longest "+
				"token of its own human text. The derivation and the classification disagree.", k.name)
		}

		base := m12Rank(t, db, c, 0, nil)
		doc := m12Rank(t, db, c, m12Weight, nil)
		loc := m12Rank(t, db, c, m12Weight, lifted)
		if base >= 0 {
			r.base++
		}
		if doc >= 0 {
			r.doc++
		}
		if loc >= 0 {
			r.loc++
		}

		// The mechanism, over M11's own ranking rather than over the
		// location rule's: what M11's weight lifted into this class's
		// visible list, and how much of it matched somewhere other than
		// the human text it was lifted for.
		for _, h := range m12Hits(t, db, c, m12Weight, nil) {
			if humanText[h.ID] == "" {
				continue
			}
			r.humanHits++
			if !slices.Contains(lifted, h.ID) {
				r.machineOnly++
			}
		}
	}
	return r
}

// m12Rank is the target's rank in the top [m11K], or -1 when it is not there.
func m12Rank(t *testing.T, db *sql.DB, c m4Candidate, human float64, lifted []string) int {
	t.Helper()
	hits := m12Hits(t, db, c, human, lifted)
	return slices.IndexFunc(hits, func(h search.Hit) bool { return h.ID == c.id })
}

// m12Hits is one search at the top [m11K], under M11's rule when lifted is nil
// and under the location rule when it is not.
//
// A refusal is a broken gate and not a result, which is M4's and M11's rule. The
// token is not in the message - internal/search's own query errors keep it out
// deliberately - so a refusal here may be printed.
func m12Hits(t *testing.T, db *sql.DB, c m4Candidate, human float64, lifted []string) []search.Hit {
	t.Helper()
	var hits []search.Hit
	var err error
	if lifted == nil {
		hits, _, err = search.SearchAtHumanWeight(t.Context(), db, c.query, "", m11K, search.MatchAll, human)
	} else {
		hits, _, err = search.SearchAtHumanTextMatch(t.Context(), db, c.query, "", m11K, search.MatchAll, human, lifted)
	}
	if err != nil {
		t.Fatalf("a derived query was refused: %v", err)
	}
	return hits
}

// m12Supplementary runs the three harm classes over every candidate rather than
// over [m4Sample]'s 25, and it is reported rather than gated on.
//
// It is here for the reason M11's own supplementary run was: M11 failed on a
// single document of 25, and a sample that small cannot say whether one document
// is a boundary artefact. This one cannot change the verdict above and does not
// try to - it says whether the sampled arm's answer is the shape of the whole
// population's.
//
// The counts are pinned for the reason every other count in this file is: a
// figure that only ever gets logged is a figure nothing re-runs.
func m12Supplementary(t *testing.T, db *sql.DB, docs []doc, humanText map[string]string) {
	t.Helper()
	pinned := map[string]m12Row{
		"a command line":   {base: 336, doc: 320, loc: 331, humanHits: 530, machineOnly: 310},
		"a touched path":   {base: 56, doc: 53, loc: 53, humanHits: 130, machineOnly: 0},
		"an error message": {base: 79, doc: 77, loc: 77, humanHits: 100, machineOnly: 20},
	}
	for _, c := range m4Classes {
		k := m12Class{name: c.name, candidates: m4CandidatesFor(t, docs, c)}
		r := m12Measure(t, db, k, docs, humanText)
		t.Logf("supp  %-22s  n %3d  recall@%d  weight 0 %3d  M11 at %.0f %3d  location at %.0f %3d",
			k.name, len(k.candidates), m11K, r.base, m12Weight, r.doc, m12Weight, r.loc)
		if want := pinned[k.name]; r != want {
			t.Errorf("supplementary %s: measured %+v over %d candidates, pinned %+v. Re-measure and "+
				"correct memory spec 5's M12 figures.", k.name, r, len(k.candidates), want)
		}
	}
}

// TestTheHumanIDSetReachesTheStatement pins the two things gate M12's seam rests
// on, neither of which any figure it reports could distinguish from a term that
// never arrived at all.
//
// An **empty** set must lift nothing, and it is not the same value as nil: nil
// is the shipped event-name rule, so a gate that let one become the other would
// measure M11 twice and report the difference as a finding. And a **non-empty**
// set must actually reorder, which is what says the `events.id IN (…)` predicate
// reached the statement rather than being built into a string nobody bound.
//
// It runs over the fixtures rather than the corpus, so it is not one of the
// things that go quiet when `.capture/` is absent.
func TestTheHumanIDSetReachesTheStatement(t *testing.T) {
	docs := fixtureDocs(t)
	db := ingestAll(t, docs)

	var query string
	var baseline []search.Hit
	for _, d := range docs {
		tok := m4LongestToken(strings.Join(d.leaves, " "))
		if tok == "" {
			continue
		}
		hits, _, err := search.Search(t.Context(), db, tok, "", m11K, search.MatchAll)
		if err != nil {
			t.Fatalf("a derived query was refused: %v", err)
		}
		if len(hits) >= 2 {
			query, baseline = tok, hits
			break
		}
	}
	if query == "" {
		t.Fatalf("no fixture query matches two documents; there is nothing here to reorder")
	}

	ids := func(hits []search.Hit) []string {
		out := make([]string, len(hits))
		for i, h := range hits {
			out[i] = h.ID
		}
		return out
	}

	// Empty, and not nil. The weight is [m12Weight]'s own so that a term
	// that ignored the set would show up as M11's rule instead of as
	// nothing.
	empty, _, err := search.SearchAtHumanTextMatch(t.Context(), db, query, "", m11K, search.MatchAll, m12Weight, []string{})
	if err != nil {
		t.Fatalf("an empty human-text id set was refused: %v", err)
	}
	if !slices.Equal(ids(empty), ids(baseline)) {
		t.Errorf("an empty human-text id set reordered %d hits; it lifts nothing and must leave the "+
			"ranking exactly as weight 0 left it", len(baseline))
	}

	// Non-empty, and it has to move something. The weight is 100 because
	// M11 measured that as the point where the term dominates bm25 within
	// the matched set, so the lifted document is first or the predicate
	// never reached the statement.
	want := baseline[1].ID
	lifted, _, err := search.SearchAtHumanTextMatch(t.Context(), db, query, "", m11K, search.MatchAll, 100, []string{want})
	if err != nil {
		t.Fatalf("a human-text id set was refused: %v", err)
	}
	if len(lifted) == 0 || lifted[0].ID != want {
		t.Errorf("lifting the second of %d hits at weight 100 did not put it first; the id predicate is "+
			"not reaching the statement", len(baseline))
	}

	// nil is the shipped rule and this seam must refuse it rather than
	// quietly measuring it.
	if _, _, err := search.SearchAtHumanTextMatch(t.Context(), db, query, "", m11K, search.MatchAll, m12Weight, nil); err == nil {
		t.Errorf("a nil human-text id set was accepted; nil is the event-name rule and this seam exists " +
			"to be compared against it")
	}
}
