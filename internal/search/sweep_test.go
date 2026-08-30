package search_test

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/search"
)

// porterTable is the second tokenizer arm's index, and porterTokenize is the
// stemmed tokenizer clause spec 5.7 priced and dropped, spelled the way the
// migration spelled it before Phase 4 removed the word.
//
// porter is gone from the product. This is the only place in the tree that
// still builds an index with it, and it exists so that spec 5.7's Population B
// table has a reproduction rather than resting on a deleted throwaway harness -
// which is the state that section withdrew an inherited figure for.
//
// gosec's G101 fires on porterTokenize for the same reason it fires on
// [tokenizeClause]: an identifier containing "token" holding a string with an
// "=" in it. It is FTS5's tokenizer clause and not a credential.
//
//nolint:gosec // G101: false positive, see above
const (
	porterTable    = "events_fts_porter"
	porterTokenize = "tokenize = 'porter unicode61 remove_diacritics 2'"
)

// The two arms' labels, and how many misses a failing class prints before the
// message is cut. A class here has up to 900 candidates, so an unbounded miss
// list is a screen of output per class; five names the shapes without becoming
// the failure.
const (
	shippedArm       = "unicode61"
	porterArm        = "porter"
	maxPrintedMisses = 5
)

// populationB is spec 5.7's Population B table: per class, how many documents
// of the 901-document corpus carry a candidate, and how many of them the porter
// arm reaches.
//
// The shipped arm's column is not here. It is 1.0 for every class by
// definition - a query cut out of a document has to reach that document - so
// the sweep asserts the ratio, which holds over any corpus, rather than reading
// a count off a table.
//
// The porter column is a measurement of one population and is pinned only when
// the run's candidate counts match this table exactly. A contributor's
// .capture/ is a different corpus, and pinning another corpus's porter counts
// to these numbers would be asserting something nobody measured.
var populationB = map[string]struct{ candidates, porterFinds int }{
	"two-character Korean":               {196, 196},
	"a content word carrying a particle": {162, 160},
	"two tokens":                         {860, 860},
	"camelCase":                          {144, 141},
	"a path basename":                    {900, 900},
}

// classResult is one class's result under one tokenizer arm.
type classResult struct {
	candidates int
	found      int
	refused    int
	misses     []string
}

// TestEveryCandidateDocumentIsReachable is spec 5.7's Population B table, built
// rather than quoted: every candidate document of every one of spec 8's five
// classes, under both tokenizers, asking of each whether the query derived from
// it reaches it.
//
// # What it closes
//
// [TestPhase4Gate] samples [maxDocsPerClass] documents per class, so a class can
// read 1.0 while being below 1.0 over its whole population. That is not
// hypothetical: before the particle derivation was anchored at a token start
// that class was 160 of 162 at full scale and the gate still reported 1.0,
// because the sample happened to miss both bad documents. This is the run that
// would have seen it.
//
// The porter arm is the second half. Nothing else in the tree builds a stemmed
// index, so without it spec 5.7's `porter finds` column rests entirely on a
// harness that was deleted.
//
// # What it does not cover, deliberately
//
// [reaches] asks the index directly for the candidate's own rowid. It does not
// go through [search.Search], so nothing here exercises the ranked read path,
// the limit, the mask or the excerpt - since Phase 4's egress work every hit
// that returns from Search is masked and given an excerpt, and one gate query
// over this corpus builds 901 of them. At 2,262 queries per arm that is not a
// test anyone keeps. What this measures is reachability, which is what the
// table it defends is about; [TestPhase4Gate] remains the test that runs the
// real read path end to end, and this is not a replacement for it.
//
// Reachability is monotone under widening, and that is the sharper limitation:
// a query with a term dropped can only match more documents, so this sweep
// cannot tell a conjunction from a disjunction and would stay green against a
// builder that joined the tokens with OR. Measured, with the leading token
// dropped from the builder: the two-token class fell to 853 of 860, and those
// seven are pairs whose surviving term the tokenizer reduces to nothing - an
// empty query, not a set relation this test can see.
// [escapesTheIntersection] is the only check for that, and it runs over the
// gate's 25 documents.
//
// The expression handed to MATCH is the product's own, through
// [search.QueryTokens] and [search.MatchExpression]. A sweep that quoted its
// tokens itself would be measuring a different system.
//
// It skips without a raw corpus. Cost is logged per arm.
//
//	go test -p 1 -count=1 -run TestEveryCandidateDocumentIsReachable -v ./internal/search/
func TestEveryCandidateDocumentIsReachable(t *testing.T) {
	docs := corpusDocs(t) // skips when .capture/ is absent
	db := ingestAll(t, docs)

	// One database, two indexes over the same rows, one word apart. The
	// shipped arm is the index the migration built and the one
	// [search.Search] reads.
	arms := []struct{ table, label string }{
		{ftsTable, shippedArm},
		{porterIndex(t, db), porterArm},
	}

	results := make([][]classResult, len(arms))
	for a, arm := range arms {
		results[a] = make([]classResult, len(classes))
		start := time.Now()
		var candidates, found int
		for i, c := range classes {
			r := sweepClass(t, db, arm.table, allCandidates(c, docs))
			if r.candidates == 0 {
				t.Fatalf("%s / %s: no document carries a candidate, so this class sweeps nothing",
					arm.label, c.name)
			}
			results[a][i] = r
			candidates, found = candidates+r.candidates, found+r.found
			t.Logf("%s / %s: %d of %d candidate documents reached, %d refused by the builder",
				arm.label, c.name, r.found, r.candidates, r.refused)
		}
		t.Logf("%s: %d of %d over %d documents and %d classes in %s",
			arm.label, found, candidates, len(docs), len(classes), time.Since(start).Round(time.Millisecond))
	}
	shipped, stemmed := results[0], results[1]

	// The shipped arm, per class, over everything. This is the assertion
	// the gate's sample cannot make.
	for i, c := range classes {
		r := shipped[i]
		if r.found == r.candidates {
			continue
		}
		t.Errorf("%s / %s: reached %d of %d candidate documents (%.2f%%), want all of them - the query "+
			"was cut out of the document it has to find, so a miss is the index having lost it%s",
			shippedArm, c.name, r.found, r.candidates, 100*float64(r.found)/float64(r.candidates),
			printMisses(r.misses))
	}

	// The porter arm carries spec 5.7's decision: it is dropped because it
	// loses known-item documents the shipped tokenizer finds and wins none
	// back. A corpus where that is not so is one this section is not
	// entitled to cite.
	var candidateTotal, shippedTotal, stemmedTotal int
	for i := range classes {
		candidateTotal += shipped[i].candidates
		shippedTotal, stemmedTotal = shippedTotal+shipped[i].found, stemmedTotal+stemmed[i].found
	}
	if stemmedTotal >= shippedTotal {
		t.Errorf("the porter arm reached %d documents and %s reached %d, so over this corpus the stemmer "+
			"loses nothing. Spec 5.7 drops porter on the strength of exactly that loss; if this is the "+
			"corpus the section cites, the section is wrong and moves",
			stemmedTotal, shippedArm, shippedTotal)
	}
	t.Logf("porter reached %d of %d and %s reached %d: the stemmer loses %d documents and wins none back",
		stemmedTotal, candidateTotal, shippedArm, shippedTotal, shippedTotal-stemmedTotal)

	requirePopulationB(t, shipped, stemmed)
}

// requirePopulationB pins spec 5.7's porter column, but only over the
// population that table was measured on: every class's candidate count has to
// match the table before any of its porter counts mean anything.
//
// A class missing from [populationB] is fatal rather than skipped. A class
// renamed here and not there would otherwise quietly stop defending the table.
func requirePopulationB(t *testing.T, shipped, stemmed []classResult) {
	t.Helper()
	var drifted []string
	for i, c := range classes {
		want, ok := populationB[c.name]
		if !ok {
			t.Fatalf("class %q is not in spec 5.7's Population B table; a class renamed here and not "+
				"there stops defending that table without saying so", c.name)
		}
		if shipped[i].candidates != want.candidates {
			drifted = append(drifted, fmt.Sprintf("%s %d against %d",
				c.name, shipped[i].candidates, want.candidates))
		}
	}
	if len(drifted) > 0 {
		t.Logf("this corpus is not spec 5.7's Population B - candidates differ: %s - so that table's "+
			"porter column is reported by this run and not defended by it", strings.Join(drifted, ", "))
		return
	}
	for i, c := range classes {
		if got, want := stemmed[i].found, populationB[c.name].porterFinds; got != want {
			t.Errorf("spec 5.7 Population B, %s: the porter arm reached %d of %d and the table says %d. "+
				"The measurement is what stands - correct 5.7 rather than this constant",
				c.name, got, stemmed[i].candidates, want)
		}
	}
}

// sweepClass runs every candidate of one class against one index.
//
// A query the builder refuses is counted as a miss and not passed over: the
// product would refuse it too, so the document is not reachable by the query
// this class derives from it, which is the same fact a miss records. It is
// counted separately as well, because a refusal is the query bounds firing and
// a miss is the index, and those are repaired in different places.
func sweepClass(t *testing.T, db *sql.DB, table string, cands []candidate) classResult {
	t.Helper()
	r := classResult{candidates: len(cands)}
	for _, cd := range cands {
		tokens, err := search.QueryTokens(cd.query)
		if err != nil {
			r.refused++
			r.misses = append(r.misses, fmt.Sprintf("  %q (%s): the builder refused it: %v",
				cd.query, cd.name, err))
			continue
		}
		if reaches(t, db, table, search.MatchExpression(tokens), cd.id) {
			r.found++
			continue
		}
		r.misses = append(r.misses, fmt.Sprintf("  %q (%s)", cd.query, cd.name))
	}
	return r
}

// reaches asks one index whether one document's own row matches one expression.
//
// The rowid constraint beside the MATCH is what makes this cheap enough to run
// over a whole population: FTS5 seeks to that row instead of returning and
// ranking every document the expression matches, and the answer is the one bit
// this sweep needs. An error is fatal rather than a miss - the expression comes
// from the product's builder, so a syntax error here is a builder defect and
// not a recall result.
//
// table is interpolated because a table name cannot be a bind parameter; it is
// one of this package's index-name constants and no corpus value reaches it.
func reaches(t *testing.T, db *sql.DB, table, expr, id string) bool {
	t.Helper()
	var n int
	//nolint:gosec // G202: table is a constant, never a value
	if err := db.QueryRowContext(t.Context(), `
		SELECT count(*)
		FROM `+table+`
		WHERE rowid = (SELECT rowid FROM events WHERE id = ?)
		  AND `+table+` MATCH ?`, id, expr).Scan(&n); err != nil {
		t.Fatalf("%s MATCH the expression built for one candidate: %v", table, err)
	}
	if n == 1 {
		return true
	}
	// An id that is not in events makes the subquery NULL, the count 0, and
	// the candidate a recall miss - which would report the index having
	// lost a document when what happened is that the document was never
	// ingested. Only reachable if store.Ingest dropped one silently, and
	// that is precisely the failure worth not misattributing.
	var ingested bool
	if err := db.QueryRowContext(t.Context(),
		`SELECT EXISTS(SELECT 1 FROM events WHERE id = ?)`, id).Scan(&ingested); err != nil {
		t.Fatalf("look up the candidate's own row: %v", err)
	}
	if !ingested {
		t.Fatalf("the candidate's id is not in events, so this sweep would count it as a miss against " +
			"the index; the document was never ingested")
	}
	return false
}

// printMisses renders up to [maxPrintedMisses] of a class's misses, and says
// how many it left out.
func printMisses(misses []string) string {
	if len(misses) == 0 {
		return ""
	}
	shown := misses[:min(maxPrintedMisses, len(misses))]
	out := "\nmissed:\n" + strings.Join(shown, "\n")
	if len(shown) < len(misses) {
		out += fmt.Sprintf("\n  ... and %d more", len(misses)-len(shown))
	}
	return out
}

// porterIndex builds the stemmed arm: the migration's own CREATE statement with
// two words changed, the table name and the tokenizer.
//
// Taken from the migration rather than written out, because everything this
// measurement does not vary - the column, the content table, the prefix
// setting - has to be exactly what the shipped index carries or the two arms
// are not comparable. [productDDL] is where the tokenizer clause is checked
// against the migration first.
//
// The migration's triggers maintain the shipped index and not this one. Nothing
// is written after the rebuild below, so that costs this sweep nothing.
func porterIndex(t testing.TB, db *sql.DB) string {
	t.Helper()
	stemmed := swapInDDL(t, productDDL(t, db), ftsTable, porterTable)
	stemmed = swapInDDL(t, stemmed, tokenizeClause, porterTokenize)
	if _, err := db.ExecContext(t.Context(), stemmed); err != nil {
		t.Fatalf("create the porter index: %v", err)
	}
	rebuild(t, db, porterTable)
	return porterTable
}

// gluedHangul matches a Hangul run with a digit or Latin letter immediately in
// front of it, which is the shape [atTokenStart] passes over: unicode61 opens
// no token at the Hangul, so no prefix query reaches it however the index is
// built.
var gluedHangul = regexp.MustCompile(`[0-9A-Za-z][\x{AC00}-\x{D7A3}]{2,}`)

// hangulRunAnywhere matches a Hangul run wherever it stands.
var hangulRunAnywhere = regexp.MustCompile(`[\x{AC00}-\x{D7A3}]{2,}`)

// TestHowMuchKoreanIsOutOfReach reports, over the real corpus, how much Hangul
// no prefix query can reach - and it reports rather than gates, deliberately.
//
// # Why this is not a sixth class
//
// [TestEveryCandidateDocumentIsReachable] answers 2,262 of 2,262: every
// candidate document of every class is already found. So nothing that widens
// the index can improve those five classes, and a proposal to segment the
// indexed text at script boundaries - `Codex는` indexed also as `Codex 는`,
// `2단계를` also as `2 단계를` - cannot be justified by them. It is a NEW
// capability, not a repair, and this test is what sizes it.
//
// A sixth gate class would be the wrong shape twice over. Its want would have
// to be 1.0 like the others, and it would fail on every run until the index
// changed, which is a red suite rather than a measurement; and [atTokenStart]'s
// own comment already rules the shape out - "a class that cannot pass however
// the index is built gates nothing". Reporting is the honest form, and it is
// the form the rank figures beside it already take.
//
// # What the numbers mean
//
// Two are reported and only the second bounds anything. `documents carrying a
// glued run` is how many documents hold at least one such run anywhere. `no
// free Hangul at all` is how many of those hold no unglued Hangul run either,
// and that is the bound: a document in the first group but not the second is
// still reachable through its other Korean text, so segmenting would change
// which query finds it and not whether it can be found.
//
// Measured 2026-08-30 over the 901-document corpus: 84 documents carry a glued
// run, and 0 of them are without free Hangul. Spec 7.1 carries the reading.
//
//	go test -p 1 -count=1 -run TestHowMuchKoreanIsOutOfReach -v ./internal/search/
func TestHowMuchKoreanIsOutOfReach(t *testing.T) {
	docs := corpusDocs(t) // skips when .capture/ is absent

	var glued, gluedAndNoFree int
	for _, d := range docs {
		var hasGlued, hasFree bool
		for _, leaf := range d.leaves {
			if gluedHangul.MatchString(leaf) {
				hasGlued = true
			}
			for _, tok := range strings.Fields(leaf) {
				tok = strings.TrimRightFunc(tok, func(r rune) bool { return !tokenChar(r) })
				if m := hangulRunAnywhere.FindStringIndex(tok); m != nil && atTokenStart(tok, m[0]) {
					hasFree = true
				}
			}
		}
		if hasGlued {
			glued++
			if !hasFree {
				gluedAndNoFree++
			}
		}
	}

	t.Logf("of %d documents: %d carry a Hangul run no prefix query reaches; %d of those carry no reachable Hangul at all",
		len(docs), glued, gluedAndNoFree)

	// The premise, not the result: a corpus with no glued run at all would
	// make this test report zero and mean nothing, and that is exactly how
	// spec 5.7's Population B table lost its evidence once already.
	if glued == 0 {
		t.Fatal("no document carries a glued Hangul run, so this measures nothing on this corpus")
	}
	if gluedAndNoFree > glued {
		t.Fatalf("%d of %d is not a subset", gluedAndNoFree, glued)
	}
}
