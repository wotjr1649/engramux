package search_test

import (
	"database/sql"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// Gate M4 (memory spec 5): P1's three new classes, recall@10 and MRR with the
// derived-field boost on and off. **No improvement means the code is deleted.**
//
// That sentence is the whole point of this file and it is not a formality. What
// Step 4 built - three columns, a migration, a backfill and an ORDER BY term -
// exists on the hypothesis that a document carrying the query in a field beats
// one that merely mentions it. This measures the hypothesis over the corpus and
// is allowed to answer no.
//
// # Which three classes, and why these
//
// P1 names four literals: an error message, a stack frame, a command line, a
// path. M4 says three. The corpus is what reconciles them, and the reconciliation
// is a measurement rather than a reading. Over the 902 captures there is no
// structured error field to build a fourth class on - tool_response.stderr is
// present on 241 documents and non-empty on none, `success` appears on 3, and one
// key in the whole corpus matches /exit|return.?code|errno/ - so an error message
// and a stack frame are the same class here, both living in what a tool answered.
// The three are therefore a command line, a touched path, and an error message in
// a tool's output, and they are exactly the three columns [store.Derive] fills.
//
// # A class is not the tokenizer classes of spec 8's Phase 4
//
// "a path basename" already exists there and this is not it. That class asks
// whether the *tokenizer* reaches a basename that appears in any string leaf;
// this one asks whether the *ranking* prefers the document that actually touched
// the file over the documents that named it in passing. Same corpus, different
// question, and the populations differ: 174 documents carry a touched path
// against 900 that carry a path somewhere.
//
// # Nothing here logs a derived query, and that is deliberate
//
// TestPhase4Gate's corpus mode logs the queries it derives, and AGENTS.md carries
// the row about what that costs - one line of a corpus run carries a drive-letter
// path with the OS user name in it. This gate derives its queries from a command
// line and from a touched path, so *every* query it makes is that shape. It
// therefore logs counts and figures only. The output of this test is safe to
// paste; do not add a query to it.
func TestPhase4GateM4DerivedFieldsEarnTheirPlace(t *testing.T) {
	docs := corpusDocs(t) // skips when the local corpus is absent
	db := ingestAll(t, docs)

	var improved, regressed []string
	for _, c := range m4Classes {
		candidates := m4CandidatesFor(t, docs, c)
		if len(candidates) == 0 {
			t.Errorf("%s: no candidate document in %d; the class measures nothing", c.name, len(docs))
			continue
		}
		sampled := m4Sample(candidates)

		with := m4Measure(t, db, sampled, true)
		without := m4Measure(t, db, sampled, false)

		t.Logf("%s: %d candidates of %d documents, %d sampled", c.name, len(candidates), len(docs), len(sampled))
		t.Logf("%s: boost off  recall@%d %.3f (%d of %d), MRR %.3f",
			c.name, m4K, without.recall, without.found, len(sampled), without.mrr)
		t.Logf("%s: boost on   recall@%d %.3f (%d of %d), MRR %.3f",
			c.name, m4K, with.recall, with.found, len(sampled), with.mrr)

		if with.recall > without.recall || with.mrr > without.mrr {
			improved = append(improved, c.name)
		}
		// A ranking change that loses a document it used to find is a
		// defect whatever the averages say, so recall is gated on its
		// own and not traded against MRR.
		if with.recall < without.recall {
			regressed = append(regressed, c.name)
		}
	}

	if len(regressed) > 0 {
		t.Errorf("M4: the boost lost documents it used to find, in %v; recall@%d is not tradeable against MRR",
			regressed, m4K)
	}
	if len(improved) == 0 {
		t.Errorf("M4: the boost improved neither recall@%d nor MRR in any of the %d classes. "+
			"By this gate's own terms that is the delete condition, not a threshold to lower: "+
			"migration 00005, store.Derive, the three columns and search's ORDER BY term come out.",
			m4K, len(m4Classes))
	}
	t.Logf("M4: improved in %v of %d classes, regressed in %v", improved, len(m4Classes), regressed)
}

// m4K is the k of recall@k, and it is memory spec 5's number rather than a
// tuning knob.
const m4K = 10

// m4DocsPerClass bounds how many documents a class measures, for the reason
// [maxDocsPerClass] gives about the other gate: this one runs two searches per
// document rather than one, so the same 25 is 50 queries a class.
const m4DocsPerClass = 25

// m4Class is one of the three: a name, the column a candidate must carry, and
// the rule that turns that column's text into the one query that has to find the
// document it came from.
//
// derive returns "" for a column carrying nothing a query can be made of, which
// is what keeps the rules rules rather than judgements - the same discipline the
// five tokenizer classes are held to.
type m4Class struct {
	name   string
	column func(store.Derived) string
	derive func(field string) string
}

var m4Classes = []m4Class{
	{"a command line", func(d store.Derived) string { return d.Cmd }, m4DeriveFromCommand},
	{"a touched path", func(d store.Derived) string { return d.Paths }, m4DeriveFromPath},
	{"an error message", func(d store.Derived) string { return d.Output }, m4DeriveFromOutput},
}

// m4Token is what counts as a token a person would type: letters, digits and the
// punctuation that holds an identifier or a path component together, four
// characters or more.
//
// Four rather than two, because a query of one or two characters is the
// tokenizer's question and spec 8's Phase 4 classes already ask it. Here the
// question is about ranking, and a query that matches half the corpus measures
// the corpus rather than the boost.
var m4Token = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._+-]{3,}`)

// m4ErrorLine matches a line that reads like a failure. It is the same
// vocabulary the corpus measurement used, and it is a rule over prose because
// this corpus has no field to ask instead.
var m4ErrorLine = regexp.MustCompile(`(?i)\b(error|failed|failure|exception|traceback|panic|cannot|denied)\b`)

// m4DeriveFromCommand takes the longest token of a command line.
//
// The longest rather than the first, because the first is `go`, `git` or `bash`
// on most of this corpus, and a class whose every query is one of three words
// measures how common those words are.
func m4DeriveFromCommand(field string) string {
	return m4LongestToken(field)
}

// m4DeriveFromPath takes the basename of a touched path when it carries an
// extension, reusing [baseWithExt] so that this class and the tokenizer class of
// the same name agree about what a basename is.
func m4DeriveFromPath(field string) string {
	field = strings.TrimRight(field, `/\`)
	i := strings.LastIndexAny(field, `/\`)
	base := field[i+1:]
	if !baseWithExt.MatchString(base) {
		return ""
	}
	return base
}

// m4DeriveFromOutput takes the longest token of the first line of a tool's
// output that reads like a failure. A document whose output carries no such line
// is not a candidate for this class, which is what makes the class about error
// messages rather than about output.
func m4DeriveFromOutput(field string) string {
	for line := range strings.SplitSeq(field, "\n") {
		if m4ErrorLine.MatchString(line) {
			if tok := m4LongestToken(line); tok != "" {
				return tok
			}
		}
	}
	return ""
}

// m4LongestToken returns the longest [m4Token] in s, and "" when there is none.
// Ties go to the first, so the rule is a function of the text and not of the
// order a map happened to iterate in.
func m4LongestToken(s string) string {
	var best string
	for _, tok := range m4Token.FindAllString(s, -1) {
		if len(tok) > len(best) {
			best = tok
		}
	}
	return best
}

// m4Candidate is one document and the query its class derived from it.
type m4Candidate struct {
	id    string
	query string
}

// m4CandidatesFor returns every document whose class column carries text a query
// can be derived from.
//
// The derivation runs over [store.Derive] of the document's own payload rather
// than over the column read back from the database. That is the same value -
// TestIngestWritesTheDerivedColumns is what says so - and taking it from the
// payload keeps this gate independent of the write path it is measuring the read
// path of.
func m4CandidatesFor(t *testing.T, docs []doc, c m4Class) []m4Candidate {
	t.Helper()
	var out []m4Candidate
	for _, d := range docs {
		field := c.column(store.Derive(d.payload))
		if field == "" {
			continue
		}
		query := c.derive(field)
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

// m4Sample spreads [m4DocsPerClass] documents over the candidates rather than
// taking a prefix, so a class whose candidates cluster in one host's captures is
// not measured over one host. It is the same stride [candidatesFor] uses.
func m4Sample(candidates []m4Candidate) []m4Candidate {
	if len(candidates) <= m4DocsPerClass {
		return candidates
	}
	out := make([]m4Candidate, 0, m4DocsPerClass)
	stride := float64(len(candidates)) / float64(m4DocsPerClass)
	for i := range m4DocsPerClass {
		out = append(out, candidates[int(float64(i)*stride)])
	}
	return out
}

// m4Result is one arm's figures.
type m4Result struct {
	recall float64
	mrr    float64
	found  int
}

// m4Measure runs one query per candidate at limit [m4K] and returns recall@k and
// MRR@k.
//
// MRR@k and not MRR over the whole ranked list: a document past k contributes 0
// rather than a small reciprocal, which is the definition that matches the
// recall figure beside it. A single number over two different cut-offs would be
// two measurements reported as one.
func m4Measure(t *testing.T, db *sql.DB, sampled []m4Candidate, boost bool) m4Result {
	t.Helper()
	run := search.Search
	if !boost {
		run = search.SearchUnboosted
	}
	var found int
	var reciprocal float64
	for _, c := range sampled {
		hits, _, err := run(t.Context(), db, c.query, "", m4K)
		if err != nil {
			// A refusal is not a miss. queryTokens bounds what a
			// query may be, and a derived query that trips one of
			// those bounds means the derivation is wrong - which is
			// a broken gate rather than a bad recall number, and
			// must not be averaged into one.
			t.Fatalf("a derived query was refused: %v", err)
		}
		if rank := slices.IndexFunc(hits, func(h search.Hit) bool { return h.ID == c.id }); rank >= 0 {
			found++
			reciprocal += 1 / float64(rank+1)
		}
	}
	n := float64(len(sampled))
	return m4Result{
		recall: float64(found) / n,
		mrr:    reciprocal / n,
		found:  found,
	}
}
