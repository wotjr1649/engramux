package search_test

import (
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
)

// Gate M11's non-vacuity arm (memory spec 5): how far the ranking puts a
// tool-plumbing document above the human-text document a query was cut from.
//
// # This arm is allowed to close backlog 48 rather than open it
//
// The row was filed on one observation of six hits - a first search on a fresh
// corpus returned four hook-plumbing documents and two carrying human text. That
// is not a measurement of the ranking, and down-weighting plumbing is a lever on
// every search every user runs. So the first thing measured is whether the
// effect is there at all, and if it is not, the number is recorded and the row
// closes with no lever built.
//
// # The bar was written down before this file existed
//
// The spec's M11 section carries it, committed before the arm was run: a weight
// is warranted only if **`buried` exceeds 0.10 in at least one class**. `buried`
// and not `displaced`, because the corpus is 82.2% plumbing and a corpus four
// fifths plumbing puts plumbing above the target by arithmetic rather than by
// defect. Reordering inside a top ten that already holds the answer is not the
// row's complaint; the answer not being in the ten is.
//
// # Nothing here logs a query, and that is stricter than M4's rule
//
// M4's queries are cut from command lines and paths and it logs none of them.
// Every query here is cut from a `prompt` or from a `last_assistant_message` -
// the most private text this corpus holds - so this file emits counts and
// figures only. `internal/search`'s own query errors keep the token out of the
// message by construction (see queryTokens), which is why a refusal below may
// be printed.
func TestGateM11PlumbingRarelyBuriesTheAnswer(t *testing.T) {
	docs := corpusDocs(t) // skips when the local corpus is absent
	db := ingestAll(t, docs)
	plumbing := m11Plumbing(docs)

	var machinery int
	for _, isPlumbing := range plumbing {
		if isPlumbing {
			machinery++
		}
	}
	// The arm asks how often machinery outranks human text. Over a corpus
	// that is all of one or all of the other there is no such question, and
	// a figure computed anyway would be a number about nothing.
	if machinery == 0 || machinery == len(docs) {
		t.Fatalf("M11: %d of %d documents are plumbing; the arm measures nothing at either extreme",
			machinery, len(docs))
	}
	// Pinned for the reason the class figures are: an inverted or misread
	// plumbing rule moves this before it moves anything else, and it moves
	// it whichever way the ranking happens to be behaving that day.
	const pinnedDocs, pinnedPlumbing = 901, 741
	if len(docs) != pinnedDocs || machinery != pinnedPlumbing {
		t.Errorf("M11: %d documents and %d plumbing, pinned %d and %d. Re-measure and correct memory "+
			"spec 5's M11 figures.", len(docs), machinery, pinnedDocs, pinnedPlumbing)
	}
	t.Logf("M11: %d documents, %d plumbing (%.1f%%) - a document is plumbing when it carries neither a "+
		"non-empty prompt nor a non-empty last_assistant_message",
		len(docs), machinery, 100*float64(machinery)/float64(len(docs)))

	var over []string
	for _, c := range m11Classes {
		candidates := m11CandidatesFor(t, docs, c)
		if len(candidates) == 0 {
			t.Errorf("%s: no candidate document in %d; the class measures nothing", c.name, len(docs))
			continue
		}
		r := m11Measure(t, db, candidates, plumbing)

		t.Logf("%s: %d candidates of %d documents, matching a median of %d documents each",
			c.name, r.n, len(docs), median(r.matched))
		t.Logf("%s: buried %.3f (%d of %d), of which %d under a top %d that is entirely plumbing",
			c.name, r.share(r.buried), r.buried, r.n, r.buriedAll, m11K)
		t.Logf("%s: displaced %.3f (%d of %d), displacement median %d of %d",
			c.name, r.share(r.displaced), r.displaced, r.n, median(r.displacement), m11K)
		t.Logf("%s: a perfect down-weight would rescue %.3f (%d of %d) into the top %d; %d targets sit "+
			"outside the top %d and are counted as not rescued",
			c.name, r.share(r.rescued), r.rescued, r.n, m11K, r.beyond, m11Deep)

		if got := r.counts(); got != c.pinned {
			t.Errorf("%s: measured %+v, pinned %+v. Re-measure and correct memory spec 5's M11 "+
				"figures rather than relaxing this comparison.", c.name, got, c.pinned)
		}
		if r.share(r.buried) > m11Bar {
			over = append(over, c.name)
		}
	}

	// The verdict is pinned rather than asserted fresh, which is gate M3's
	// shape and is here for M3's reason: what this arm answers is a fact
	// about one corpus at one weight, and what a committed test can hold is
	// that the fact has not moved. A failure here is not "the bar was
	// wrong" - it is the corpus or the ranking having changed, and it wants
	// the spec's figures re-measured rather than this line edited.
	if !slices.Equal(over, m11Warranted) {
		t.Errorf("M11: %v are over the %.2f bar for `buried`, pinned %v. Re-measure and correct memory "+
			"spec 5's M11 figures; do not move the bar, which was registered before the arm was run.",
			over, m11Bar, m11Warranted)
	}
	if len(over) == 0 {
		t.Logf("M11: no class is over the %.2f bar for `buried`. By this arm's own terms that closes "+
			"backlog 48 on the number rather than licensing a weight.", m11Bar)
	}
}

// m11K is the k of the top-k this arm looks inside, and it is memory spec 5's
// number - the same k gate M4 measures recall at.
const m11K = 10

// m11Deep is how far down the match set the arm looks to find a buried target's
// real rank, which is what the rescue figure needs: how far a target is, and how
// much of the distance is plumbing.
//
// One search per query at this limit rather than two at two limits. LIMIT
// truncates an ORDER BY rather than changing it, so the first m11K hits of a
// deep search are the same hits in the same order a shallow one returns, and the
// pre-registered top-k figures are unaffected by reading further.
//
// 200 is twice ipc.MaxSearchLimit, so no caller can ask for a window this arm
// did not look inside. A target past it is counted as not rescued, which keeps
// the ceiling a ceiling.
const m11Deep = 200

// m11Bar is the share of `buried` above which a weight is warranted. It is the
// spec's number and was committed before this file existed; a run that lands
// near it is reported as landing near it rather than rounded to the friendlier
// side.
const m11Bar = 0.10

// m11Warranted is the classes measured over [m11Bar], pinned. Empty would mean
// the corpus answered that there is nothing to fix; both classes are over it,
// and by a factor of four and six.
//
// # This arm measures the ranking with no down-weight in it, and must keep doing so
//
// It is the *motivation* measurement, so the moment a weight exists this arm
// becomes the `off` half of the full gate rather than a second opinion about the
// shipped ranking. Pointing it at a ranking that has the weight in it would make
// the numbers below fall, the pin fail, and the failure read as the corpus
// having moved - when what moved is the thing the row asked for.
var m11Warranted = []string{"a prompt's own words", "a reply's own words"}

// m11Human is the two payload keys that decide whether a document carries text a
// person wrote or a model wrote back: a prompt, and an assistant's last message.
//
// It is a field rule and not an event-name one. events.event_name carries no
// CHECK - it is whatever a payload said - so a closed set of names would be a
// guess about the host's next release, where backlog 48's own words are
// "documents carrying prompt or reply text" and these two keys say exactly that.
type m11Human struct {
	prompt string
	reply  string
}

// m11HumanOf reads those two keys out of a payload.
//
// Through a map and not through a struct tag, because a payload whose `prompt`
// is an object rather than a string would fail to decode into a typed field and
// the whole document would then be misread as plumbing - a silent
// reclassification, in the direction that makes the corpus look worse than it
// is. A type assertion answers "" for that document's field and leaves the other
// one alone.
func m11HumanOf(payload json.RawMessage) m11Human {
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return m11Human{}
	}
	str := func(key string) string {
		s, _ := m[key].(string)
		return strings.TrimSpace(s)
	}
	return m11Human{prompt: str("prompt"), reply: str("last_assistant_message")}
}

// m11Plumbing is every ingested document by id, true when it carries neither
// half of [m11Human].
func m11Plumbing(docs []doc) map[string]bool {
	out := make(map[string]bool, len(docs))
	for _, d := range docs {
		h := m11HumanOf(d.payload)
		out[d.id] = h.prompt == "" && h.reply == ""
	}
	return out
}

// m11Class is one of the two human-text classes: a name, which half of
// [m11Human] its query is cut from, and every figure the class measured when it
// was measured.
//
// # Why the figures are pinned and not only logged
//
// The bar is over `buried`, which is a fact about where the target ranks and is
// the same number whatever this file decides is plumbing. So a pin on the bar's
// verdict alone would sit still through an inverted plumbing rule, a rescue
// ceiling counted the wrong way round, or a displacement that counted the whole
// window instead of the top ten - the three things this arm exists to compute.
// Pinning every count is what makes a mutation in any of them fail something.
//
// This is gate M3's shape and it inherits M3's obligation: a corpus that grows
// makes these wrong, and the answer is to re-measure and correct the spec's
// figures, never to relax the comparison.
type m11Class struct {
	name   string
	field  func(m11Human) string
	pinned m11Counts
}

// m11Counts is one class's measurement as a comparable value. Measured
// 2026-09-06 over the 901-document corpus.
type m11Counts struct {
	n, buried, buriedAll, displaced, rescued, beyond int
	matchedMedian, displacementMedian                int
}

var m11Classes = []m11Class{
	{
		"a prompt's own words",
		func(h m11Human) string { return h.prompt },
		m11Counts{n: 17, buried: 11, buriedAll: 11, displaced: 16, rescued: 7, beyond: 2,
			matchedMedian: 51, displacementMedian: 10},
	},
	{
		"a reply's own words",
		func(h m11Human) string { return h.reply },
		m11Counts{n: 139, buried: 63, buriedAll: 58, displaced: 111, rescued: 59, beyond: 0,
			matchedMedian: 15, displacementMedian: 6},
	},
}

// m11CandidatesFor returns every document whose class field carries text a query
// can be cut from.
//
// The cut is [m4LongestToken] and not a rule of its own, so that this arm and
// M4's command-line class are held to one discipline rather than two. A
// single-token query also makes MatchAll and MatchAny the identical FTS5
// expression, so nothing here turns on which matching the arm chose - and one
// token is what backlog 48's own observation was.
func m11CandidatesFor(t *testing.T, docs []doc, c m11Class) []m4Candidate {
	t.Helper()
	var out []m4Candidate
	for _, d := range docs {
		field := c.field(m11HumanOf(d.payload))
		if field == "" {
			continue
		}
		query := m4LongestToken(field)
		if query == "" {
			continue
		}
		if d.id == "" {
			t.Fatalf("%s: the document has no ingested id; ingestAll did not fill it in", d.name)
		}
		out = append(out, m4Candidate{id: d.id, query: query})
	}
	return out
}

// m11Result is one class's measurement: the pre-registered top-k figures, and
// beside them what the whole match set says a weight could actually win.
type m11Result struct {
	n         int
	displaced int
	buried    int
	buriedAll int
	// displacement is one entry per query, so the median is over every
	// query and not over the ones that happened to be displaced.
	displacement []int
	// matched is how many documents each query matched before the limit,
	// which is what tells a burial caused by ranking from one caused by the
	// query naming a third of the corpus.
	matched []int
	// rescued is the queries whose target is outside the top k now and
	// would be inside it if every plumbing document above it were moved
	// below it. It is the exact benefit ceiling of any down-weight, computed
	// without choosing one.
	rescued int
	// beyond is the queries whose target is not in the top [m11Deep] at all,
	// so no rank was available for it and it cannot be counted as rescued.
	beyond int
}

func (r m11Result) share(k int) float64 {
	if r.n == 0 {
		return 0
	}
	return float64(k) / float64(r.n)
}

// counts is r as the comparable value a class pins.
func (r m11Result) counts() m11Counts {
	return m11Counts{
		n: r.n, buried: r.buried, buriedAll: r.buriedAll, displaced: r.displaced,
		rescued: r.rescued, beyond: r.beyond,
		matchedMedian: median(r.matched), displacementMedian: median(r.displacement),
	}
}

func median(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	sorted := slices.Clone(xs)
	slices.Sort(sorted)
	return sorted[len(sorted)/2]
}

// m11Measure runs one search per candidate at the current weight and counts what
// sits above the document the query was cut from - inside the top [m11K], which
// is what the pre-registered bar is about, and then down the whole match window,
// which is what the rescue figure is about.
//
// # Why a target outside the top k still has a displacement
//
// Dropping those queries would bias the answer towards "nothing to fix" by
// dropping exactly the worst cases, so a buried target counts every plumbing
// document in the top k instead. That is a lower bound on how far it actually
// is, and a lower bound is the safe direction for a figure that decides whether
// to build nothing.
//
// # What "rescued" is, and what it is not
//
// It is the exact ceiling of any down-weight: the queries whose target is
// outside the top k now and whose distance from it is *entirely* plumbing, so
// that moving every plumbing document below it puts it inside. No weight is
// chosen and none is needed - a weight can do no better than move all of them,
// and this counts the queries that would still not be rescued if it did.
// It says nothing about whether the trade is worth making; that is the harm
// arm's question, over the classes whose targets are the plumbing.
func m11Measure(t *testing.T, db *sql.DB, candidates []m4Candidate, plumbing map[string]bool) m11Result {
	t.Helper()
	r := m11Result{n: len(candidates)}
	for _, c := range candidates {
		// MatchAny is what the CLI and the MCP tools run for a person's
		// question. For a one-token query it is the same expression
		// MatchAll builds, so this names the production path rather than
		// choosing a side.
		hits, total, err := search.Search(t.Context(), db, c.query, "", m11Deep, search.MatchAny)
		if err != nil {
			// A refusal is a broken arm and not a result: queryTokens
			// bounds what a query may be, and a cut that trips one of
			// those bounds means the cut is wrong. The token is not in
			// the message - that package keeps it out deliberately.
			t.Fatalf("a derived query was refused: %v", err)
		}
		r.matched = append(r.matched, int(total))
		rank := slices.IndexFunc(hits, func(h search.Hit) bool { return h.ID == c.id })

		// The pre-registered half, over the top k and nothing else.
		top := hits[:min(len(hits), m11K)]
		above := top
		if rank >= 0 && rank < len(top) {
			above = top[:rank]
		} else {
			r.buried++
			// A burial with a human-text document above it is not this
			// row's defect, so the attribution is counted separately.
			if len(top) > 0 && !slices.ContainsFunc(top, func(h search.Hit) bool { return !plumbing[h.ID] }) {
				r.buriedAll++
			}
		}
		var displacement int
		for _, h := range above {
			if plumbing[h.ID] {
				displacement++
			}
		}
		if displacement > 0 {
			r.displaced++
		}
		r.displacement = append(r.displacement, displacement)

		// The ceiling half, over the whole window. A target already in
		// the top k needs no rescuing and is not counted as one.
		if rank < 0 {
			r.beyond++
			continue
		}
		if rank < m11K {
			continue
		}
		var human int
		for _, h := range hits[:rank] {
			if !plumbing[h.ID] {
				human++
			}
		}
		if human < m11K {
			r.rescued++
		}
	}
	return r
}
