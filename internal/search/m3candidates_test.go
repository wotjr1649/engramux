package search_test

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/memory"
	"github.com/wotjr1649/engramux/internal/search"
)

// Gate M3's fixture is the owner's, and the reason is not privacy - it is
// circularity. M3 measures whether *a person's* natural query reaches an answer
// that exists in only one host, and a query written by anything that has just
// read the answer selects the answer's own words, which measures the tokenizer
// rather than retrieval. The memory spec's rev.3 records that a generated
// fixture was tried once and deleted for exactly this.
//
// Two of the fixture's three columns are not judgements, though, and this is
// what produces them. It writes candidate lines in the gate's own format with
// the **answer** column filled and verified, and the **query** column left as a
// marker for a person to replace. What is left to do by hand is the one column
// that has to be human.
//
// # What "verified" means here, and it is the gate's own test
//
// Every answer this emits has been checked the way
// [TestGateM3CrossHostRecall] checks a fixture line: it appears in the
// **masked** body of at least one item of its host and in **no** item of the
// other host. It is taken from the masked body rather than from the file, so it
// cannot carry anything internal/secret rewrites - which is the first of the two
// rules the gate would otherwise fail a line for. The second, that an answer is
// text rather than an id, holds by construction: these are runs of prose.
//
// # Nothing here prints a note
//
// The output goes to a file under `.capture/`, which is gitignored, and the test
// logs counts only. The corpus is the owner's private notes; a candidate line is
// a run of them, so a `-v` run that printed one would put it in a terminal, a
// transcript and possibly a commit message. Measured: 0 of this test's log lines
// carry a span.
const m3CandidatesEnv = "ENGRAMUX_WRITE_M3_CANDIDATES"

// m3CandidatesPerHost is how many candidate lines a host gets. Well over the
// spec's 25, because the owner is choosing which ones they can write a real
// query for and some spans will be prose nobody would ever search for.
const m3CandidatesPerHost = 60

// m3SpanBounds is what a usable answer looks like: long enough to be
// distinctive, short enough that a person can see all of it on one line.
const (
	m3MinSpan = 24
	m3MaxSpan = 110
)

// m3Pathish rejects a span that looks like a path or a URL even after masking.
// The gate's rule is about what the mask rewrites, and this is stricter on
// purpose: a path is a poor answer whether or not the mask caught it, because
// the query for it would be a path too and that is spec 8's Phase 4 class
// rather than this one.
var m3Pathish = regexp.MustCompile(`[\\/]{1,2}[A-Za-z0-9._-]+[\\/]|://|[A-Za-z]:[\\/]`)

// m3Wordy requires a span to carry at least three words of four letters or
// more, which is what separates prose from a run of punctuation, ids or numbers.
var m3Wordy = regexp.MustCompile(`\p{L}{4,}`)

func TestWriteM3Candidates(t *testing.T) {
	out := os.Getenv(m3CandidatesEnv)
	if out == "" {
		t.Skipf("set %s to a path to write gate M3's candidate lines there", m3CandidatesEnv)
	}

	db := m3Corpus(t) // the gate's own corpus, through the gate's own collector

	bodies := map[string][]m3Item{}
	for _, host := range []string{memory.HostClaude, memory.HostCodex} {
		bodies[host] = m3ItemsOf(t, db, host)
		t.Logf("%s: %d items", host, len(bodies[host]))
	}

	var lines []string
	total := map[string]int{}
	for _, host := range []string{memory.HostClaude, memory.HostCodex} {
		other := otherHost(host)
		picked := m3PickSpans(bodies[host], bodies[other], m3CandidatesPerHost)
		total[host] = len(picked)
		for _, p := range picked {
			// A comment carrying the item's title, so the owner has
			// the context without opening the file the span came
			// from. The gate skips a line starting with #.
			lines = append(lines,
				fmt.Sprintf("# %s | %s", host, p.title),
				fmt.Sprintf("%s\tTODO-WRITE-THE-QUERY\t%s", host, p.span),
				"")
		}
		t.Logf("%s: %d candidate spans over %d distinct items", host, len(picked), m3DistinctItems(picked))
	}

	//nolint:gosec // G703: the operator names the file to write, which is this harness's whole interface
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		t.Fatalf("make the output directory: %v", err)
	}
	f, err := os.Create(out) //nolint:gosec // G304: a path the operator passed in
	if err != nil {
		t.Fatalf("create %s: %v", out, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("close the output: %v", err)
		}
	}()
	w := bufio.NewWriter(f)
	header := []string{
		"# Gate M3 candidates. Replace TODO-WRITE-THE-QUERY with what you would actually type",
		"# to find the item, then save the lines you kept as .capture/m3/queries.en.tsv.",
		"#",
		"# Write the query in English. The corpus is 74% English and the gate measures the",
		"# documented usage, which is to search in English (memory spec rev.11, decisions 1 and 3).",
		"#",
		"# The answer column is already verified the way the gate verifies it: it is in the",
		"# masked body of at least one item of that host and in none of the other host's.",
		"# Do not edit it - if you change it, the gate re-checks it and will say so.",
		"#",
		"# Write the query from memory rather than from the answer beside it. A query that is",
		"# the answer's own words measures the tokenizer, which another gate already measures.",
		"#",
		"# This file is under .capture/ and is never committed.",
		"",
	}
	for _, l := range append(header, lines...) {
		if _, err := fmt.Fprintln(w, l); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	t.Logf("wrote %d claude-code and %d codex candidates", total[memory.HostClaude], total[memory.HostCodex])
	if total[memory.HostClaude] == 0 && total[memory.HostCodex] == 0 {
		t.Fatal("no candidate span survived on either host; the selection rules admit nothing")
	}
}

// m3Item is one item's masked body and the title a reader would recognise it by.
type m3Item struct {
	id    string
	title string
	body  string
}

// m3ItemsOf reads every item of one host through [search.GetMemoryItem], which
// is what masks the body - the same call the gate compares against.
func m3ItemsOf(t *testing.T, db *sql.DB, host string) []m3Item {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT id FROM memory_items WHERE host = ?`, host)
	if err != nil {
		t.Fatalf("list %s items: %v", host, err)
	}
	// The defer as well as the explicit Close below, which is the shape
	// m3Carriers in gate_m3_test.go already uses: the explicit one frees the
	// single connection before the per-item reads take it, and the defer is
	// what covers the paths that never reach it.
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan an id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the ids: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close the ids: %v", err)
	}

	out := make([]m3Item, 0, len(ids))
	for _, id := range ids {
		it, err := search.GetMemoryItem(t.Context(), db, id, nil)
		if err != nil {
			t.Fatalf("read an item: %v", err)
		}
		if it == nil {
			continue
		}
		out = append(out, m3Item{id: it.ID, title: it.Title, body: it.Body})
	}
	return out
}

// m3Span is one candidate answer and the item it came from.
type m3Span struct {
	id    string
	title string
	span  string
}

// m3PickSpans walks the host's items and returns spans that appear in no item of
// the other host, at most one per item until every item has had one.
//
// One per item first, because a fixture of 25 spans out of three files measures
// three files. Only after every item has contributed does it take a second from
// any of them.
func m3PickSpans(mine, theirs []m3Item, want int) []m3Span {
	var picked []m3Span
	seen := map[string]bool{}
	for round := 0; round < 4 && len(picked) < want; round++ {
		for _, it := range mine {
			if len(picked) >= want {
				break
			}
			n := 0
			for _, span := range m3Candidates(it.body) {
				if seen[span] {
					continue
				}
				if n < round { // this item already gave one this round
					n++
					continue
				}
				// The gate compares with strings.Contains against
				// the *raw* body, and [m3Candidates] normalises
				// whitespace and trims markdown furniture - so a
				// span it produced need not be in the body it came
				// from. Without this the generator hands over lines
				// that fail the gate's own precondition, which is
				// the worst kind of candidate: it looks labelled and
				// is not. This is the gate's check, made here.
				if !strings.Contains(it.body, span) {
					continue
				}
				if m3InAny(theirs, span) {
					continue
				}
				seen[span] = true
				picked = append(picked, m3Span{id: it.id, title: it.title, span: span})
				break
			}
		}
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].title < picked[j].title })
	return picked
}

// m3Candidates turns one masked body into the spans that could be answers, best
// first: whole lines that read like prose, trimmed to one line and bounded.
func m3Candidates(body string) []string {
	var out []string
	for _, raw := range strings.Split(body, "\n") {
		span := strings.Join(strings.Fields(raw), " ")
		// Leading markdown furniture is not part of a sentence and would
		// make a fixture line hard to read.
		span = strings.TrimLeft(span, "#-*>| \t")
		if len(span) < m3MinSpan {
			continue
		}
		if len(span) > m3MaxSpan {
			span = m3TrimToWord(span, m3MaxSpan)
			if len(span) < m3MinSpan {
				continue
			}
		}
		if m3Pathish.MatchString(span) {
			continue
		}
		if len(m3Wordy.FindAllString(span, -1)) < 3 {
			continue
		}
		out = append(out, span)
	}
	return out
}

// m3TrimToWord cuts a span at the last word boundary at or before max, so a
// candidate never ends mid-word - a half word is a substring match that reads
// like a typo in the fixture.
func m3TrimToWord(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := strings.LastIndex(s[:max], " ")
	if cut <= 0 {
		return ""
	}
	return s[:cut]
}

// m3InAny reports whether span appears in any of the items, which is the
// cross-host half of the gate's own check applied before the line is written
// rather than after.
func m3InAny(items []m3Item, span string) bool {
	for _, it := range items {
		if strings.Contains(it.body, span) {
			return true
		}
	}
	return false
}

// m3DistinctItems counts how many different items the picks came from, which is
// the number that says whether a fixture would measure a corpus or a file.
func m3DistinctItems(picked []m3Span) int {
	seen := map[string]bool{}
	for _, p := range picked {
		seen[p.id] = true
	}
	return len(seen)
}
