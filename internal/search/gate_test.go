package search_test

import (
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wotjr1649/engramux/internal/search"
)

// maxDocsPerClass bounds how many documents a class measures: the first N in
// the mode's own order that carry a candidate. It exists so the corpus mode
// stays a gate rather than a benchmark - 900 documents times five classes is
// 4,500 queries - and it is small enough that one missing document is a visible
// fraction rather than a rounding error.
const maxDocsPerClass = 25

// Per-class verdicts, one constant each so that lowering one later is a visible
// diff on one line and not an average quietly absorbing a class.
//
// All five are 1.0: known-item retrieval means the query was derived from the
// document it has to find, so anything below "all of them" is the index failing
// to return a document it was built from. Spec 5.7 withdrew the micro-averaged
// number that used to stand here, and its shape - one figure over everything -
// is exactly what these five constants replace.
const (
	wantKoreanTwoChar = 1.0
	wantParticle      = 1.0
	wantTwoTokens     = 1.0
	wantCamelCase     = 1.0
	wantPathBasename  = 1.0
)

// eyeballQueries is how many derived queries per class are logged under -v.
// The derivations are rules rather than judgements, so the only way to check
// one is to read what it produced; three is enough to see a rule that fired on
// the wrong thing. Nothing is written to a file: 900 of 902 captures carry the
// user's directory (I-10, spec 7.5).
const eyeballQueries = 3

// precisionKey is the JSON key present in every document. It is a key and not a
// leaf, which is the whole point: under an index built over raw payload bytes
// it is a token in every document and matches all of them, and under an index
// built over string leaves it matches only the documents that talk about it.
// T4 chooses between those two with this number in hand.
const precisionKey = "cwd"

// hangulRun matches a run of precomposed Hangul syllables. Jamo and the
// compatibility block are deliberately out: the corpus is syllables, and a rule
// that also matched a stray jamo would derive a query no reader could check.
var hangulRun = regexp.MustCompile(`[\x{AC00}-\x{D7A3}]+`)

// particles are spec 5.7's common Korean particles, longest first so that a run
// ending in 으로 yields the stem before 으로 rather than the one before 로.
var particles = []string{"에서", "으로", "는", "은", "이", "가", "을", "를", "의", "에", "로", "와", "과", "도"}

// camelWord matches an identifier with at least two humps, as a whole word.
// Go's \b is the ASCII word boundary, so PostToolUse does not match - there is
// no boundary between P and o - and neither does the tail of snake_caseName.
var camelWord = regexp.MustCompile(`\b[a-z]+[A-Z][a-zA-Z0-9]+\b`)

// baseWithExt matches a path's last component when it carries an extension.
// The leading class must match at least one character before the final dot, so
// a dotfile with no extension - .gitignore, .codex - is not a basename.
var baseWithExt = regexp.MustCompile(`^[A-Za-z0-9._+-]+\.[A-Za-z0-9]{1,10}$`)

// class is one of spec 8's five known-item retrieval classes: a name, its own
// verdict, and the rule that turns a document into the one query that has to
// find it. derive returns "" for a document that carries no candidate.
type class struct {
	name   string
	want   float64
	derive func(d doc) string
}

// classes is spec 8's list, in spec 8's order.
var classes = []class{
	{"two-character Korean", wantKoreanTwoChar, deriveKoreanTwoChar},
	{"a content word carrying a particle", wantParticle, deriveParticle},
	{"two tokens", wantTwoTokens, deriveTwoTokens},
	{"camelCase", wantCamelCase, deriveCamelCase},
	{"a path basename", wantPathBasename, derivePathBasename},
}

// deriveKoreanTwoChar takes the first Hangul run of three or more syllables and
// returns its first two. Three is the floor because two syllables of a
// two-syllable run is the run itself, which is a whole-word search and not this
// class; the class exists for the query that only a trailing star can reach.
func deriveKoreanTwoChar(d doc) string {
	for _, leaf := range d.leaves {
		for _, run := range hangulRun.FindAllString(leaf, -1) {
			if r := []rune(run); len(r) >= 3 {
				return string(r[:2])
			}
		}
	}
	return ""
}

// deriveParticle takes the first Hangul run that ends in one of [particles]
// over a stem of at least two syllables, and returns the stem.
//
// unicode61 keeps the whole run as one token, so an exact-word search for the
// stem cannot match it and only prefix expansion can (spec 5.7).
func deriveParticle(d doc) string {
	for _, leaf := range d.leaves {
		for _, run := range hangulRun.FindAllString(leaf, -1) {
			r := []rune(run)
			for _, p := range particles {
				pr := []rune(p)
				if len(r) >= len(pr)+2 && string(r[len(r)-len(pr):]) == p {
					return string(r[:len(r)-len(pr)])
				}
			}
		}
	}
	return ""
}

// deriveTwoTokens takes two adjacent whitespace-separated tokens, each at least
// two characters, from one leaf. A pair containing Hangul wins over an earlier
// pair that does not, which is what "prefer a pair containing Korean when the
// document has any" means: the mixed-script pair is the one that exercises
// implicit AND across the tokenizer's two behaviours at once.
func deriveTwoTokens(d doc) string {
	var first string
	for _, leaf := range d.leaves {
		f := strings.Fields(leaf)
		for i := 0; i+1 < len(f); i++ {
			if utf8.RuneCountInString(f[i]) < 2 || utf8.RuneCountInString(f[i+1]) < 2 {
				continue
			}
			pair := f[i] + " " + f[i+1]
			if hangulRun.MatchString(pair) {
				return pair
			}
			if first == "" {
				first = pair
			}
		}
	}
	return first
}

// deriveCamelCase takes the first camelCase identifier and cuts it after its
// second hump: waitUntilServing becomes waitUntil, and waitUntil - which has
// exactly two humps - stays whole. The cut is at the second uppercase letter,
// and [camelWord] guarantees the first character is lowercase, so the first
// hump is never the identifier's own start.
func deriveCamelCase(d doc) string {
	for _, leaf := range d.leaves {
		if id := camelWord.FindString(leaf); id != "" {
			humps := 0
			for i, r := range id {
				if r >= 'A' && r <= 'Z' {
					humps++
					if humps == 2 {
						return id[:i]
					}
				}
			}
			return id
		}
	}
	return ""
}

// derivePathBasename takes the last component of the first path-shaped token
// that has one with an extension. Windows and POSIX separators both count, and
// only the basename leaves this function - a corpus path is an absolute path
// under the user's directory in 900 of 902 captures (I-10, spec 7.5).
func derivePathBasename(d doc) string {
	for _, leaf := range d.leaves {
		for _, tok := range strings.Fields(leaf) {
			i := strings.LastIndexAny(tok, `\/`)
			if i < 0 {
				continue
			}
			if base := tok[i+1:]; baseWithExt.MatchString(base) {
				return base
			}
		}
	}
	return ""
}

// candidate is one document a class measures, and the query derived from it.
type candidate struct {
	query string
	name  string
	id    string
}

// candidatesFor is the first [maxDocsPerClass] documents, in the mode's own
// order, that carry a candidate for c.
func candidatesFor(c class, docs []doc) []candidate {
	var out []candidate
	for _, d := range docs {
		q := c.derive(d)
		if q == "" {
			continue
		}
		out = append(out, candidate{query: q, name: d.name, id: d.id})
		if len(out) == maxDocsPerClass {
			break
		}
	}
	return out
}

// TestPhase4Gate is spec 8's Phase 4 gate: known-item retrieval, gated per
// class, over the fixtures always and over the raw corpus when it is present.
//
// The query is derived from the document it has to find, so there is no
// relevance judgement to trust and no assessor to disagree with: a class that
// does not find its own source document has lost it. Each class is a subtest
// with its own verdict constant, because an average over five classes hides
// exactly the failure that matters - spec 5.7 withdrew one for doing that.
//
// The precision assertion runs beside them and is data-derived rather than a
// threshold: a recall-only gate cannot see what indexing structure destroys,
// and the number it produces is what T4 uses to choose between indexing the raw
// payload and indexing the string leaves.
//
// Written before internal/store creates events_fts, so today every subtest
// fails with "no such table: events_fts" behind its own class name. A gate
// written after the thing it gates is a gate shaped to pass.
//
//	go test -p 1 -count=1 -run TestPhase4Gate -v ./internal/search/
func TestPhase4Gate(t *testing.T) {
	for _, mode := range []struct {
		name string
		load func(t *testing.T) []doc
	}{
		{"fixtures", fixtureDocs},
		{"corpus", corpusDocs},
	} {
		t.Run(mode.name, func(t *testing.T) {
			docs := mode.load(t) // corpus skips when absent
			db := ingestAll(t, docs)
			t.Logf("%s: %d documents ingested", mode.name, len(docs))

			for _, c := range classes {
				t.Run(c.name, func(t *testing.T) {
					gateClass(t, db, mode.name, c, docs)
				})
			}
			t.Run("precision", func(t *testing.T) {
				gatePrecision(t, db, mode.name, docs)
			})
		})
	}
}

// gateClass runs one class: derive one query per candidate document, search for
// it with the limit set to the document count so the whole match set comes
// back, and require the source document in it.
//
// A class with no candidate document fails rather than passing empty. Zero of
// zero is 100% by arithmetic and nothing by measurement, and the fixtures are
// edited by hand - a fixture that lost the value carrying a class would
// otherwise turn that class off silently.
func gateClass(t *testing.T, db *sql.DB, mode string, c class, docs []doc) {
	t.Helper()
	cands := candidatesFor(c, docs)
	if len(cands) == 0 {
		t.Fatalf("%s / %s: no document carries a candidate, so this class gates nothing", mode, c.name)
	}
	for i, cd := range cands[:min(eyeballQueries, len(cands))] {
		t.Logf("%s / %s: derived query %d of %d: %q", mode, c.name, i+1, len(cands), cd.query)
	}

	var found int
	var ranks []int
	var misses []string
	for _, cd := range cands {
		hits, err := search.Search(t.Context(), db, cd.query, len(docs))
		if err != nil {
			t.Fatalf("%s / %s: Search(%q), derived from %s: %v", mode, c.name, cd.query, cd.name, err)
		}
		rank := slices.IndexFunc(hits, func(h search.Hit) bool { return h.ID == cd.id })
		if rank < 0 {
			misses = append(misses, fmt.Sprintf("%q (%s)", cd.query, cd.name))
			continue
		}
		found++
		ranks = append(ranks, rank+1)
	}

	slices.Sort(ranks)
	if len(ranks) > 0 {
		t.Logf("%s / %s: rank median %d, worst %d over %d found (not gated)",
			mode, c.name, ranks[len(ranks)/2], ranks[len(ranks)-1], len(ranks))
	}
	if got := float64(found) / float64(len(cands)); got < c.want {
		t.Errorf("%s / %s: found %d of %d (%.1f%%), want %.1f%%\nmissed: %s",
			mode, c.name, found, len(cands), got*100, c.want*100, strings.Join(misses, ", "))
	}
}

// gatePrecision is the one precision assertion, and it carries no threshold:
// the bound is counted from the same documents the search runs over.
//
// [precisionKey] is a JSON key every document has. Its leaf count - the
// documents whose string leaves contain it as a substring - is the most an
// index that stores content rather than structure can honestly match. An index
// over the raw payload bytes matches every document instead, because there the
// key is text like any other.
//
// The precondition is asserted first and is not decoration: when the leaf count
// reaches the document count the bound is the whole corpus, and an index that
// matched everything would pass.
func gatePrecision(t *testing.T, db *sql.DB, mode string, docs []doc) {
	t.Helper()
	var inLeaves int
	for _, d := range docs {
		if d.hasLeaf(precisionKey) {
			inLeaves++
		}
	}
	t.Logf("%s / precision: %q is in a string leaf of %d of %d documents", mode, precisionKey, inLeaves, len(docs))
	if inLeaves >= len(docs) {
		t.Fatalf("%s / precision: %q is in a leaf of %d of %d documents; the bound is the whole set and "+
			"the check cannot fail", mode, precisionKey, inLeaves, len(docs))
	}

	hits, err := search.Search(t.Context(), db, precisionKey, len(docs))
	if err != nil {
		t.Fatalf("%s / precision: Search(%q): %v", mode, precisionKey, err)
	}
	if len(hits) > inLeaves {
		t.Errorf("%s / precision: %q matched %d documents, want at most %d - the documents that carry it in a "+
			"string leaf. A key matching more than that is the index storing structure as content",
			mode, precisionKey, len(hits), inLeaves)
	}
}
