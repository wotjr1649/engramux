package search_test

import (
	"bufio"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/memory"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// m3FixturePath is where gate M3's fixture lives, and it is deliberately not in
// the repository (memory spec rev.2, M-2 decision 4).
//
// The same shape `.capture/` and TestPhase4Gate's corpus mode already have: the
// file is the owner's own labelling of their own private notes, so promoting it
// would need redaction of both the queries and the answers, and that promotion
// is its own decision. What is given up is that this gate does not run on
// anyone else's machine, which the skip below says out loud rather than passing
// / quietly.
//
// The path is relative to *this package*, which is where a test runs, so the two
// leading steps up put it under the repository root beside the raw capture
// corpus - the same place corpusDir looks. A bare ".capture" here would resolve
// to internal/search/.capture, a directory nothing else uses and which a reader
// would put a fixture into exactly once before wondering why nothing ran.
var m3FixturePath = filepath.Join("..", "..", ".capture", "m3", "queries.tsv")

// m3Query is one labelled known-item query.
type m3Query struct {
	line   int
	host   string
	query  string
	answer string
}

// TestGateM3CrossHostRecall is the memory spec's gate M3: queries whose answer
// exists in only one host, recall@10, reported against each native memory's own
// ceiling.
//
// # The fixture, and how to write one
//
// A tab-separated file at [m3FixturePath], three columns and no header:
//
//	host    query    answer
//
// `host` is claude-code or codex. `query` is what a person would type. `answer`
// is a run of text that appears in the body of the item that should be found and
// nowhere in the other host's memory at all - that second half is what makes it
// a *cross-host* query, and this gate checks it rather than trusting it. Lines
// starting with # are comments and blank lines are skipped.
//
// Two rules for choosing an answer, both of which this gate will otherwise fail
// you for. It must not be a path, a credential or anything else internal/secret
// masks, because what is compared is the masked body. And it must be text, not
// an id: an id is derived from the file's path, so it moves when the file does
// and a fixture written against one rots silently.
//
// # What it asserts and what it only reports
//
// Recall is asserted at 100%: these are known-item queries whose answer is known
// to be in the corpus, so a miss is a retrieval failure and not a judgement
// call - the same shape TestPhase4Gate's five classes have.
//
// The ceiling is reported and not asserted. On the machine this was written for,
// Claude Code's whole population is 38 items over 3 projects and Codex's is 265,
// so "25 per host" is a number the corpus may not support; the spec's own
// wording is that the result is reported against each native memory's own
// ceiling, and printing the two numbers is what that means.
func TestGateM3CrossHostRecall(t *testing.T) {
	queries := readM3Fixture(t)
	if len(queries) == 0 {
		t.Skipf("no M3 fixture at %s; it is the owner's own labelling and lives outside the repository "+
			"(memory spec rev.2, M-2 decision 4). Format: host<TAB>query<TAB>answer, one per line.",
			filepath.Join(".capture", "m3", "queries.tsv"))
	}

	db := m3Corpus(t)
	population := map[string]int{}
	for _, host := range []string{memory.HostClaude, memory.HostCodex} {
		var n int
		if err := db.QueryRowContext(t.Context(),
			`SELECT count(*) FROM memory_items WHERE host = ?`, host).Scan(&n); err != nil {
			t.Fatalf("count the %s items: %v", host, err)
		}
		population[host] = n
	}

	asked := map[string]int{}
	found := map[string]int{}
	for _, q := range queries {
		asked[q.host]++
		// The cross-host half of the definition, checked rather than
		// trusted: an answer that also exists in the other host is not a
		// query about what only one host knows, and a fixture that drifts
		// into one would report a recall number about something else.
		if n := m3Carriers(t, db, otherHost(q.host), q.answer); n != 0 {
			t.Errorf("line %d: the answer is in %d of the other host's items, so this is not a "+
				"cross-host query", q.line, n)
			continue
		}
		if n := m3Carriers(t, db, q.host, q.answer); n == 0 {
			t.Errorf("line %d: no %s item carries the answer at all, so this query cannot be "+
				"satisfied - check that the text is not something the mask rewrites", q.line, q.host)
			continue
		}

		hits, _, err := search.SearchMemory(t.Context(), db, q.query, nil, 10)
		if err != nil {
			t.Errorf("line %d: SearchMemory: %v", q.line, err)
			continue
		}
		if m3Hit(t, db, hits, q.answer) {
			found[q.host]++
			continue
		}
		// The query and not the answer: the query is what the owner typed
		// and is the thing to change, and the answer is a run of their own
		// notes.
		t.Errorf("line %d: %s query %q found no item carrying its answer in the top %d",
			q.line, q.host, q.query, len(hits))
	}

	for _, host := range []string{memory.HostClaude, memory.HostCodex} {
		if asked[host] == 0 {
			t.Logf("M3: %s has no queries in the fixture", host)
			continue
		}
		t.Logf("M3: %s recall@10 = %d of %d, over a population of %d items",
			host, found[host], asked[host], population[host])
		if asked[host] < 25 {
			t.Logf("M3: %s has %d queries against the spec's 25; the ceiling above is what that "+
				"is reported against", host, asked[host])
		}
	}
}

// m3Hit reports whether any hit's whole item carries the answer.
//
// The item and not the excerpt, because an excerpt is a window around the match
// and the answer may sit outside it - a hit that found the right document and
// cut the window somewhere else is a hit.
func m3Hit(t *testing.T, db *sql.DB, hits []search.MemoryHit, answer string) bool {
	t.Helper()
	for _, h := range hits {
		it, err := search.GetMemoryItem(t.Context(), db, h.ID, nil)
		if err != nil {
			t.Fatalf("read a hit back: %v", err)
		}
		if it != nil && strings.Contains(it.Body, answer) {
			return true
		}
	}
	return false
}

// m3Carriers counts the items of one host whose masked body carries the answer.
// Masked, because that is what a reader gets and therefore what the fixture has
// to be written against.
func m3Carriers(t *testing.T, db *sql.DB, host, answer string) int {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT id FROM memory_items WHERE host = ?`, host)
	if err != nil {
		t.Fatalf("list a host's items: %v", err)
	}
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
	// Closed before the reads below, which take the same single connection.
	if err := rows.Close(); err != nil {
		t.Fatalf("close the ids: %v", err)
	}

	var n int
	for _, id := range ids {
		it, err := search.GetMemoryItem(t.Context(), db, id, nil)
		if err != nil {
			t.Fatalf("read an item: %v", err)
		}
		if it != nil && strings.Contains(it.Body, answer) {
			n++
		}
	}
	return n
}

func otherHost(h string) string {
	if h == memory.HostClaude {
		return memory.HostCodex
	}
	return memory.HostClaude
}

// m3Corpus indexes this machine's own native memory into a database of the
// test's own. It is the same corpus M1 walks, through the same collector, so
// what this gate measures is what the service would have written.
func m3Corpus(t *testing.T) *sql.DB {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "engramux.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	c := &memory.Collector{ClaudeHome: memory.ClaudeHome(), CodexHome: memory.CodexHome()}
	rep, err := c.Collect(ctx, db, time.Now())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if rep.Written == 0 {
		t.Skip("no native memory on this machine to measure recall over")
	}
	return db
}

// readM3Fixture parses the fixture, or returns nothing when there is none.
//
// A malformed line is a failure and not a skipped line: a fixture that silently
// dropped half of itself would report a recall number over a population nobody
// chose, which is the same defect a parser that skips quietly has.
func readM3Fixture(t *testing.T) []m3Query {
	t.Helper()
	//nolint:gosec // G304: m3FixturePath is this file's own package-level path, not a caller's
	f, err := os.Open(m3FixturePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("open the M3 fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	var out []m3Query
	s := bufio.NewScanner(f)
	for n := 1; s.Scan(); n++ {
		line := strings.TrimRight(s.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("line %d of the M3 fixture has %d tab-separated fields, want 3", n, len(parts))
		}
		host := strings.TrimSpace(parts[0])
		if host != memory.HostClaude && host != memory.HostCodex {
			t.Fatalf("line %d of the M3 fixture names the host %q, want %q or %q",
				n, host, memory.HostClaude, memory.HostCodex)
		}
		q := m3Query{line: n, host: host, query: strings.TrimSpace(parts[1]), answer: parts[2]}
		if q.query == "" || q.answer == "" {
			t.Fatalf("line %d of the M3 fixture has an empty query or answer", n)
		}
		out = append(out, q)
	}
	if err := s.Err(); err != nil {
		t.Fatalf("read the M3 fixture: %v", err)
	}
	return out
}
