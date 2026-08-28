package search_test

import (
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/wotjr1649/engramux/internal/search"
)

// maxDocsPerClass bounds how many documents a class measures: N spread over the
// documents that carry a candidate, by [candidatesFor]. It exists so the corpus
// mode stays a gate rather than a benchmark - 900 documents times five classes
// is 4,500 queries - and it is small enough that one missing document is a
// visible fraction rather than a rounding error.
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

// precisionKey is the JSON key all but one document carries. It is a key and
// not a leaf, which is the whole point: under an index built over raw payload
// bytes it is a token in every document that has it and matches all of them,
// and under an index built over string leaves it matches only the documents
// that talk about it. T4 chooses between those two with this number in hand.
//
// The exception is [pairSharpener], which carries no `cwd` key, so in fixtures
// mode 4 of the 5 documents have it. That weakens nothing: the bound is a leaf
// count and is still 0, the precondition is still 0 < 5, and a raw-payload
// index would still match 4 against a bound of 0. The corpus has its own
// exception the other way - see [corpusDocs] on the capture probe.
const precisionKey = "cwd"

// hangulRun matches a run of precomposed Hangul syllables. Jamo and the
// compatibility block are deliberately out: the corpus is syllables, and a rule
// that also matched a stray jamo would derive a query no reader could check.
var hangulRun = regexp.MustCompile(`[\x{AC00}-\x{D7A3}]+`)

// particleStem matches a token that ends in one of spec 5.7's common Korean
// particles, capturing the stem in front of it.
//
// The stem may be Latin letters and digits or Hangul syllables, two or more of
// either, and that alternation is the point rather than a convenience.
// unicode61 does not split a Latin stem from an attached Korean particle -
// spec 5.7 measured Codex는 as one token - so an exact-word search for Codex
// misses it, and per-token prefix expansion is the only thing that reaches it.
// A rule that only looked inside a Hangul run would never gate that case.
//
// Both stem quantifiers are lazy, and that is load-bearing rather than style.
// Go's regexp is leftmost-first, so a greedy stem takes as much as it can and
// leaves the shortest particle that still fits: 의도적으로 split as 의도적으 +
// 로 instead of 의도적 + 으로. Lazy tries the shortest stem first, which leaves
// the longest particle, and the alternation being longest-first then decides
// between two particles of the same reach - 에서 before 에.
var particleStem = regexp.MustCompile(
	`([A-Za-z0-9]{2,}?|[\x{AC00}-\x{D7A3}]{2,}?)(에서|으로|는|은|이|가|을|를|의|에|로|와|과|도)$`)

// camelWord matches a camelCase identifier as a whole word. Go's \b is the
// ASCII word boundary, so PostToolUse does not match - there is no boundary
// between P and o - and neither does the tail of snake_caseName.
var camelWord = regexp.MustCompile(`\b[a-z]+[A-Z][a-zA-Z0-9]+\b`)

// camelMinHumps is how many humps an identifier needs to be a candidate.
//
// Three, not the two [camelWord] can match, because an identifier with exactly
// two humps is returned whole and a whole-identifier query is a different class
// of search from the partial-identifier path this one exists for. Measured: at
// two, the corpus derived bypassPermissions - permission_mode's value, which
// sorts before tool_input and is in nearly every Claude capture - for document
// after document, and a class that only ever asks for a whole common word
// cannot fail on the path it is gating.
const camelMinHumps = 3

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

// twoTokensClass is the one class [gateClass] runs an extra assertion for, and
// it is named here rather than spelled twice so that renaming the class cannot
// silently turn that assertion off.
const twoTokensClass = "two tokens"

// classes is spec 8's list, in spec 8's order.
var classes = []class{
	{"two-character Korean", wantKoreanTwoChar, deriveKoreanTwoChar},
	{"a content word carrying a particle", wantParticle, deriveParticle},
	{twoTokensClass, wantTwoTokens, deriveTwoTokens},
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

// deriveParticle takes the first whitespace-delimited token that ends in a
// particle over a stem of at least two characters, and returns the stem.
//
// Trailing ASCII punctuation is trimmed first, because real text writes
// Codex는, and 서비스가. and the particle is then not the last character. Only
// trailing punctuation: anything inside the token is part of it, and unicode61
// would split there anyway.
func deriveParticle(d doc) string {
	for _, leaf := range d.leaves {
		for _, tok := range strings.Fields(leaf) {
			tok = strings.TrimRightFunc(tok, func(r rune) bool {
				return r < utf8.RuneSelf && !unicode.IsLetter(r) && !unicode.IsDigit(r)
			})
			if m := particleStem.FindStringSubmatch(tok); m != nil {
				return m[1]
			}
		}
	}
	return ""
}

// particleStemShapes counts how many documents derive a particle stem of each
// shape. Both shapes are spec 5.7's, and the Latin one is the measured case
// that makes per-token expansion load-bearing, so a run where it contributes
// nothing is a run where that case went ungated.
func particleStemShapes(docs []doc) (latin, hangul int) {
	for _, d := range docs {
		stem := deriveParticle(d)
		if stem == "" {
			continue
		}
		if r := []rune(stem)[0]; r < utf8.RuneSelf {
			latin++
		} else {
			hangul++
		}
	}
	return latin, hangul
}

// TestDeriveParticleStems pins [deriveParticle] to exact stems, because the
// gate cannot: before T5's expansion every particle query misses, so a
// derivation that produced the wrong stem and one that produced the right one
// look identical there.
//
// Every input is invented. The 의도적으로 row is the one that matters most: a
// greedy stem quantifier splits it as 의도적으 + 로, which is a stem no reader
// would ask for, and nothing else in this package notices.
func TestDeriveParticleStems(t *testing.T) {
	for _, tc := range []struct{ leaf, want string }{
		{"Codex는 하나의 토큰이다", "Codex"},       // spec 5.7's measured Latin case
		{"fixturebot이 응답했다", "fixturebot"}, // the fixtures' own shape
		{"서비스가 시작", "서비스"},                 // Hangul stem, one-syllable particle
		{"의도적으로 끊었다", "의도적"},               // two-syllable particle, not 으 + 로
		{"프로젝트에서 찾는다", "프로젝트"},             // 에서 before 에
		{"Codex는, 그리고", "Codex"},           // trailing ASCII punctuation
		{"relay-v2를 재시작", "v2"},            // a digit-carrying Latin run
		{"종이 한 장", ""},                     // a one-syllable stem is not a candidate
		{"nothing to strip here", ""},      // no particle anywhere
	} {
		t.Run(tc.leaf, func(t *testing.T) {
			if got := deriveParticle(doc{leaves: []string{tc.leaf}}); got != tc.want {
				t.Errorf("deriveParticle(%q) = %q, want %q", tc.leaf, got, tc.want)
			}
		})
	}
}

// deriveTwoTokens takes two adjacent whitespace-separated tokens, each at least
// two characters, from one leaf. A pair containing Hangul wins over an earlier
// pair that does not, which is what "prefer a pair containing Korean when the
// document has any" means: the mixed-script pair is the one that exercises
// implicit AND across the tokenizer's two behaviours at once.
//
// Splitting on whitespace only, so a leading quote or a --- rule stays attached
// to its token, is deliberate and not a gap: that is what a token looks like in
// real captured text, and T5's per-token expansion has to survive it. A query
// this produces would be an FTS5 syntax error today (spec 5.7), which is a
// louder failure here than a miss and is the point.
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

// deriveCamelCase takes the document's identifier with the most humps - the
// first occurrence on a tie - and cuts it after the second: waitUntilServing
// becomes waitUntil. A document whose best identifier is under [camelMinHumps]
// carries no candidate and is not measured.
//
// Most humps rather than first, because "first" is whichever leaf sorts
// earliest, and that is a structural field far more often than it is something
// a person wrote.
func deriveCamelCase(d doc) string {
	var best string
	bestHumps, bestCut := 0, 0
	for _, leaf := range d.leaves {
		for _, id := range camelWord.FindAllString(leaf, -1) {
			if h, cut := humpsOf(id); h > bestHumps {
				best, bestHumps, bestCut = id, h, cut
			}
		}
	}
	if bestHumps < camelMinHumps {
		return ""
	}
	return best[:bestCut]
}

// humpsBeforeCut is how many humps the query keeps: "cut after its second
// hump", so two. It is deliberately a separate constant from [camelMinHumps],
// which happens to be one more today. The floor decides which identifiers
// qualify and the cut decides where a query ends; moving one must not silently
// move the other.
const humpsBeforeCut = 2

// humpsOf counts an identifier's humps and returns the byte offset the query is
// cut at - where the hump after the last kept one begins. [camelWord]
// guarantees the match starts lower case, so the identifier's own start is hump
// one and every uppercase letter opens another.
//
// cut is len(id) for an identifier with too few humps to cut, so a caller that
// ignores the count still gets the whole identifier rather than a panic.
func humpsOf(id string) (humps, cut int) {
	humps, cut = 1, len(id)
	for i, r := range id {
		if r >= 'A' && r <= 'Z' {
			humps++
			if humps == humpsBeforeCut+1 {
				cut = i
			}
		}
	}
	return humps, cut
}

// derivePathBasename takes the last component of the first path-shaped token
// that has one with an extension. Windows and POSIX separators both count, and
// only the basename leaves this function - a corpus path is an absolute path
// under the user's directory in 900 of 902 captures (I-10, spec 7.5).
//
// A basename keeps whatever punctuation [baseWithExt] admits, and under
// unicode61 a dot and a hyphen both split, so this class reaches MATCH as
// several tokens rather than one. That is the phrase path and it is why the
// class exists; before T5 quotes each token it is also how a basename becomes
// an FTS5 syntax error rather than a miss, which is the louder failure.
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

// candidatesFor returns up to [maxDocsPerClass] of the documents that carry a
// candidate for c, and how many carried one in total.
//
// The sample is spread over the candidates - index i*M/N for i in 0..N-1, or
// all of them when there are no more than N - rather than taken from the front,
// and this is the one place every class gets it. A corpus file name begins with
// the host, the event and the capture timestamp, so sorted order groups a
// session together: the first 25 of 901 were 25 captures of one session, with
// one transcript_path basename between them.
//
// i*M/N rather than a floor(M/N) step, which is what this did first: with
// M = 49 and N = 25 the step is 1 and the sample is indices 0 to 24, half the
// list, with the whole tail unreachable. Multiplying first reaches index 47.
// Both are exact integer arithmetic, so the same corpus samples the same
// documents every run.
func candidatesFor(c class, docs []doc) (sample []candidate, total int) {
	var all []candidate
	for _, d := range docs {
		if q := c.derive(d); q != "" {
			all = append(all, candidate{query: q, name: d.name, id: d.id})
		}
	}
	if len(all) <= maxDocsPerClass {
		return all, len(all)
	}
	// The largest index is (N-1)*M/N, which is below M for every M > N, so
	// this cannot walk off the end.
	for i := range maxDocsPerClass {
		sample = append(sample, all[i*len(all)/maxDocsPerClass])
	}
	return sample, len(all)
}

// TestCandidateSampleSpansTheList pins [candidatesFor]'s spread, which the gate
// itself cannot show: over this corpus a floor(M/N) step and an i*M/N index
// pick the same 25 documents, and they diverge only when M is not a multiple of
// N. 49 is the smallest awkward case worth naming - a step of 1 stops at index
// 24 and leaves the whole second half unreachable.
func TestCandidateSampleSpansTheList(t *testing.T) {
	name := class{name: "name", want: 1, derive: func(d doc) string { return d.name }}
	docs := make([]doc, 49)
	for i := range docs {
		docs[i] = doc{name: strconv.Itoa(i)}
	}

	sample, total := candidatesFor(name, docs)
	if total != len(docs) {
		t.Errorf("total = %d, want %d", total, len(docs))
	}
	if len(sample) != maxDocsPerClass {
		t.Fatalf("sampled %d documents, want %d", len(sample), maxDocsPerClass)
	}
	// 24*49/25 = 47, two short of the end and 23 past where a step of 1 stops.
	if got := []string{sample[0].name, sample[len(sample)-1].name}; !slices.Equal(got, []string{"0", "47"}) {
		t.Errorf("sample runs %v, want from %q to %q", got, "0", "47")
	}

	if sample, total := candidatesFor(name, docs[:maxDocsPerClass]); len(sample) != total || total != maxDocsPerClass {
		t.Errorf("with M == N, sampled %d of %d, want all %d", len(sample), total, maxDocsPerClass)
	}
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
// It was written before internal/store created events_fts, when every subtest
// failed with "no such table" behind its own class name. A gate written after
// the thing it gates is a gate shaped to pass.
//
//	go test -p 1 -count=1 -run TestPhase4Gate -v ./internal/search/
func TestPhase4Gate(t *testing.T) {
	for _, mode := range []struct {
		name string
		load func(t testing.TB) []doc
	}{
		{"fixtures", fixtureDocs},
		{"corpus", corpusDocs},
	} {
		t.Run(mode.name, func(t *testing.T) {
			docs := mode.load(t) // corpus skips when absent
			db := ingestAll(t, docs)
			latin, hangul := particleStemShapes(docs)
			t.Logf("%s: %d documents ingested; particle stems: %d Latin, %d Hangul",
				mode.name, len(docs), latin, hangul)

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
	cands, total := candidatesFor(c, docs)
	if total == 0 {
		t.Fatalf("%s / %s: no document carries a candidate, so this class gates nothing", mode, c.name)
	}
	for i, cd := range cands[:min(eyeballQueries, len(cands))] {
		t.Logf("%s / %s: derived query %d of %d sampled from %d candidate documents: %q",
			mode, c.name, i+1, len(cands), total, cd.query)
	}

	var found int
	var ranks []int
	var misses []string
	var escaped []string
	var skipped, sharp int
	var leadingCatchers, trailingCatchers int
	for _, cd := range cands {
		hits, err := search.Search(t.Context(), db, cd.query, len(docs))
		if err != nil {
			t.Fatalf("%s / %s: %d of %d candidate documents sampled; Search(%q), derived from %s: %v",
				mode, c.name, len(cands), total, cd.query, cd.name, err)
		}
		if c.name == twoTokensClass {
			e, s, d, lead, trail := escapesTheIntersection(t, db, cd, hits, len(docs))
			escaped, skipped, sharp = append(escaped, e...), skipped+s, sharp+d
			if lead {
				leadingCatchers++
			}
			if trail {
				trailingCatchers++
			}
		}
		rank := slices.IndexFunc(hits, func(h search.Hit) bool { return h.ID == cd.id })
		if rank < 0 {
			misses = append(misses, fmt.Sprintf("%q (%s)", cd.query, cd.name))
			continue
		}
		found++
		ranks = append(ranks, rank+1)
	}
	if c.name == twoTokensClass {
		// Logged on the passing path too, and both numbers are needed.
		// The first says how many comparisons ran; the second says how
		// many of them could have failed - see [escapesTheIntersection].
		t.Logf("%s / %s: the intersection holds over %d of %d terms; %d carry no token to match on, "+
			"and %d match something the pair does not, which is where an OR would show",
			mode, c.name, 2*len(cands)-skipped-len(escaped), 2*len(cands), skipped, sharp)
		if sharp == 0 {
			t.Errorf("%s / %s: no term of the %d compared matched a document the pair does not, so every "+
				"comparison this class ran was one where an AND and an OR return the same set and the "+
				"class did not test the join - the containment check above cannot fail on a run like "+
				"this. Either no document repeats one word of a derived pair, or the pair is already "+
				"returning at least as much as each term alone, which is the OR that check names",
				mode, c.name, 2*len(cands)-skipped)
		}
		// Also on the passing path, and this pair of numbers is what says
		// the class covers both dropped-term regressions rather than one.
		t.Logf("%s / %s: of %d pairs, %d could catch a dropped leading token and %d a dropped "+
			"trailing one", mode, c.name, len(cands), leadingCatchers, trailingCatchers)
		if leadingCatchers == 0 {
			t.Errorf("%s / %s: no pair had its second term matching a document its first does not, so a "+
				"builder that dropped the first token would return the second term's set, which sits "+
				"inside both single-term sets, and would pass this class. Nothing here would fail. The "+
				"pairs this mode derives need one whose second term is the wider of the two - see "+
				"[escapesTheIntersection] for what the fixtures use", mode, c.name)
		}
		if trailingCatchers == 0 {
			t.Errorf("%s / %s: no pair had its first term matching a document its second does not, so a "+
				"builder that dropped the second token would return the first term's set, which sits "+
				"inside both single-term sets, and would pass this class. Nothing here would fail. The "+
				"pairs this mode derives need one whose first term is the wider of the two - see "+
				"[escapesTheIntersection] for what the fixtures use", mode, c.name)
		}
	}
	if len(escaped) > 0 {
		t.Errorf("%s / %s: %d term comparisons of %d sampled queries returned an event that term did not. "+
			"An implicit AND returns the intersection, so its result set sits inside both single-term "+
			"sets; a superset is an OR and a one-sided set is a dropped term:\n%s",
			mode, c.name, len(escaped), len(cands), strings.Join(escaped, "\n"))
	}

	slices.Sort(ranks)
	if len(ranks) > 0 {
		// Upper median on an even count - ranks[n/2], not the mean of the
		// two middle values. Named rather than averaged because a rank is
		// a position and half a position is not one.
		t.Logf("%s / %s: rank upper median %d, worst %d over %d found (not gated)",
			mode, c.name, ranks[len(ranks)/2], ranks[len(ranks)-1], len(ranks))
	}
	if got := float64(found) / float64(len(cands)); got < c.want {
		t.Errorf("%s / %s: found %d of %d sampled (%.1f%%), want %.1f%% - %d documents carried a candidate\nmissed: %s",
			mode, c.name, found, len(cands), got*100, c.want*100, total, strings.Join(misses, ", "))
	}
}

// escapesTheIntersection runs each of a two-token query's terms alone and
// returns one line per term whose result set does not contain the whole
// two-token result set. Three queries per candidate document: the pair, and
// each term.
//
// The class's found check cannot do this. It asks only whether the source
// document came back, and that is true of an implicit AND, of an OR, and of a
// builder that dropped one term entirely - the document carries both tokens, so
// any of the three returns it. The set relation tells them apart: an AND
// returns the intersection, so its result set sits inside both single-term sets;
// an OR returns the union, which is a superset of at least one; a dropped term
// returns exactly one side, which is a superset of the intersection whenever the
// two terms select different documents.
//
// Containment and not equality, because rank order is not what this is
// measuring, and because an intersection is a subset by definition - a proper
// one whenever either term matches a document the other does not. Not because
// of [maxDocsPerClass]: the pair and both of its terms are searched with the
// limit at the document count, so no sampling cap is in play on either side.
//
// A term the tokenizer reduces to nothing constrains nothing and is skipped,
// which is measurement and not leniency. The corpus derives them - `"===`
// beside 커밋 - and FTS5 drops such a term from the conjunction rather than
// taking the whole query to nothing, so the pair returns what the surviving
// term returns and the empty side returns nothing at all. Requiring containment
// there would assert a relation that is false whenever the query works.
// [constrains] is the test, and skipped is returned rather than assumed to be
// zero: it is how much of the sample this assertion did not cover.
//
// sharp counts the comparisons that could have failed - the terms matching
// something the pair does not - and it is returned so that [gateClass] can
// require it to be positive. A term whose result set is exactly the pair's
// cannot tell an AND from an OR: the union and the intersection are then the
// same set, and a run of nothing but those reports a pass for a check that
// gated nothing. The four fixtures alone were exactly such a run - all six
// comparisons of that kind - which is what [pairSharpener] is in the document
// set to fix. Measured with it, over the fixtures: 8 comparisons over 4 pairs,
// none skipped, sharp 2. Those two are different mechanisms and not one
// repeated: `fixture-two` against the pair it was taken from, where it now
// selects two documents against that pair's one, and `turns` against the
// sharpener's own pair, where the stemmer's turn/turns fold puts it in three
// documents against that pair's two. Over the corpus: 50 comparisons, 1
// skipped, sharp 45 of the 49 that ran.
//
// catchesLeadingDrop and catchesTrailingDrop are the pair's two sides, and they
// are what makes this check's coverage measurable rather than assumed. A
// dropped term is caught only from the side that survives: a builder that drops
// the leading token returns the second term's set, which escapes containment
// only if the second term matched a document the first did not, and a dropped
// trailing token is the mirror of that. So a class whose every pair has one
// term's set inside the other's covers one of those two regressions, misses the
// other, and reports a pass either way - sharp does not notice, because it
// compares each term against the pair rather than against the other term.
// [gateClass] requires both directions to have occurred at least once.
// Measured: over the fixtures exactly one pair of 4 on each side, and the
// leading side is the sharpener's, which has it only because of the stemmer
// collision [pairSharpener] documents. Over the corpus, 23 and 22 of 25.
func escapesTheIntersection(
	t *testing.T, db *sql.DB, cd candidate, both []search.Hit, limit int,
) (escaped []string, skipped, sharp int, catchesLeadingDrop, catchesTrailingDrop bool) {
	t.Helper()
	terms := strings.Fields(cd.query)
	if len(terms) != 2 {
		t.Fatalf("%s: derived %q, which is %d tokens rather than 2; this class no longer measures an AND",
			cd.name, cd.query, len(terms))
	}

	var sets [2]map[string]bool
	for i, term := range terms {
		if !constrains(term) {
			skipped++
			continue
		}
		alone, err := search.Search(t.Context(), db, term, limit)
		if err != nil {
			t.Fatalf("Search(%q), one term of %q derived from %s: %v", term, cd.query, cd.name, err)
		}
		if len(alone) > len(both) {
			sharp++
		}
		ids := make(map[string]bool, len(alone))
		for _, h := range alone {
			ids[h.ID] = true
		}
		sets[i] = ids
		var strangers int
		for _, h := range both {
			if !ids[h.ID] {
				strangers++
			}
		}
		if strangers > 0 {
			escaped = append(escaped, fmt.Sprintf(
				"  %q returned %d events, %d of them absent from the %d that %q returns alone",
				cd.query, len(both), strangers, len(alone), term))
		}
	}
	// A term that constrains nothing was never searched, so there is no
	// second set to compare against and this pair settles neither
	// direction. It is not evidence against either one.
	if sets[0] == nil || sets[1] == nil {
		return escaped, skipped, sharp, false, false
	}
	return escaped, skipped, sharp, holdsAStranger(sets[1], sets[0]), holdsAStranger(sets[0], sets[1])
}

// holdsAStranger reports whether a matched a document b did not.
func holdsAStranger(a, b map[string]bool) bool {
	for id := range a {
		if !b[id] {
			return true
		}
	}
	return false
}

// constrains reports whether a term can narrow a conjunction at all: whether
// unicode61 will find a token in it.
//
// Letters and digits, which is unicode61's own rule - it takes the Unicode
// letter and number categories as token characters and everything else as a
// separator, and remove_diacritics strips marks rather than tokenizing them.
// A term of nothing but punctuation, `"===` or ---, produces no token and FTS5
// drops it from the query.
func constrains(term string) bool {
	return strings.ContainsFunc(term, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}

// gatePrecision is the one precision assertion, and it carries no threshold:
// the bound is counted from the same documents the search runs over.
//
// [precisionKey] is a JSON key all but one document carries - see its own doc
// for the exception and why it costs this assertion nothing. Its leaf count -
// the documents whose string leaves contain it as a substring - is the most an
// index that stores content rather than structure can honestly match. An index
// over the raw payload bytes matches every document that has the key instead,
// because there the key is text like any other.
//
// The precondition is asserted first and is not decoration: when the leaf count
// reaches the document count the bound is the whole corpus, and an index that
// matched everything would pass.
//
// Two assertions follow it, and the second is not implied by the first. A count
// bound alone is satisfied by an index that returns the right number of the
// wrong documents - swap one hit that carries cwd for one that does not and the
// count is unchanged - so every hit is also required to be a document whose
// leaves carry the key.
func gatePrecision(t *testing.T, db *sql.DB, mode string, docs []doc) {
	t.Helper()
	carriesKey := make(map[string]bool, len(docs))
	for _, d := range docs {
		if d.hasLeaf(precisionKey) {
			carriesKey[d.id] = true
		}
	}
	inLeaves := len(carriesKey)
	t.Logf("%s / precision: %q is in a string leaf of %d of %d documents", mode, precisionKey, inLeaves, len(docs))
	if inLeaves >= len(docs) {
		t.Fatalf("%s / precision: %q is in a leaf of %d of %d documents; the bound is the whole set and "+
			"the check cannot fail", mode, precisionKey, inLeaves, len(docs))
	}

	hits, err := search.Search(t.Context(), db, precisionKey, len(docs))
	if err != nil {
		t.Fatalf("%s / precision: Search(%q): %v", mode, precisionKey, err)
	}
	// Logged on the passing path as well as the failing one, so the number
	// spec 5.7 cites comes out of this harness rather than out of a probe
	// nobody kept. It is below the bound rather than equal to it because the
	// bound counts a substring of a leaf and the index matches a token.
	t.Logf("%s / precision: MATCH %q returned %d documents, bound %d",
		mode, precisionKey, len(hits), inLeaves)
	if len(hits) > inLeaves {
		t.Errorf("%s / precision: %q matched %d documents, want at most %d - the documents that carry it in a "+
			"string leaf. A key matching more than that is the index storing structure as content",
			mode, precisionKey, len(hits), inLeaves)
	}
	var strangers int
	for _, h := range hits {
		if !carriesKey[h.ID] {
			strangers++
		}
	}
	if strangers > 0 {
		t.Errorf("%s / precision: %d of the %d events %q matched carry it in no string leaf; the index matched "+
			"them on structure, not on content", mode, strangers, len(hits), precisionKey)
	}
}
