package search_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// payloadRatioCeiling is how much slower a search over fat documents may be than
// the same search over thin ones, with everything else held equal.
//
// 3 and not 1. Some scaling is real and is not the defect: the twenty rows that
// *are* returned carry the payload either way, and [search.Search]'s second
// phase masks and excerpts each of them, which is work proportional to their
// size. What the gate is shaped against is the other 19,980 - a cost that grows
// with documents nobody asked for.
//
// The margin is wide on purpose. This is a timing ratio and the machine under it
// is not quiet; the defect it caught measured **13.5x** on the shape that
// shipped, so a ceiling of 3 has an order of magnitude of room before it starts
// reporting load instead of scaling.
const payloadRatioCeiling = 3.0

// TestGateTheSearchDoesNotReadPayloadsItDoesNotReturn asserts that a search's
// cost does not scale with the size of the documents it leaves behind.
//
// # The defect this was written for
//
// `count(*) OVER ()` is a window over the whole result set, so SQLite
// materialises every matching row before it can return the first - and
// `events.payload` was in the same SELECT list. A query matching 15,000
// documents therefore read 15,000 payloads off disk to return 20 of them.
//
// Nothing caught it. Spec 7.1 measured the shape at **24.2 ms** over 19,503
// synthetic events, and the synthetic events are tiny; on the owner's real
// 227 MB database the same query took **4 s** and hit the service's 4 s read
// deadline. Measured 2026-09-04, immediately after Step 7 was installed: **5 of
// 10** ordinary questions came back as `context deadline exceeded`, and a single
// common token was enough to do it - `bash` and `the` both timed out, which is
// what says the defect predates Step 7 and was only made reachable by it.
//
// # Why a ratio and not a duration
//
// A duration is a statement about the machine. Two corpora identical in every
// way except payload size, in one run, is a statement about the query: the
// machine, the cache and the load are the same for both arms and cancel.
//
// # Nothing here logs a document
//
// The corpora are generated and carry no capture, but the rule is the file's
// either way: times and counts only.
func TestGateTheSearchDoesNotReadPayloadsItDoesNotReturn(t *testing.T) {
	const (
		events = 20000
		thin   = 64
		fat    = 8192
		limit  = 20
	)
	// Built once each and reused across the timed runs, because building
	// them is far more expensive than the thing being measured.
	thinDB := payloadCorpus(t, events, thin)
	fatDB := payloadCorpus(t, events, fat)

	thinAt := searchBest(t, thinDB, limit)
	fatAt := searchBest(t, fatDB, limit)
	ratio := float64(fatAt) / float64(thinAt)

	t.Logf("%d events, one term in all of them, limit %d", events, limit)
	t.Logf("payload %5d B: %v", thin, thinAt.Round(time.Microsecond))
	t.Logf("payload %5d B: %v", fat, fatAt.Round(time.Microsecond))
	t.Logf("ratio %.2f, ceiling %.2f", ratio, payloadRatioCeiling)

	if ratio > payloadRatioCeiling {
		t.Errorf("a search over %d B documents is %.2fx one over %d B documents, ceiling %.2f. "+
			"The result is %d rows either way, so the difference is payload the query read and did "+
			"not return - check that count(*) OVER () is not sharing a SELECT list with a large "+
			"column, because a window function materialises every matching row",
			fat, ratio, thin, payloadRatioCeiling, limit)
	}
}

// searchBest runs the shipped search path four times and returns the best of the
// last three, the first being a warm-up. Best rather than mean, because the
// question is what the statement costs and not what the machine was doing.
func searchBest(t *testing.T, db *sql.DB, limit int) time.Duration {
	t.Helper()
	var best time.Duration
	for i := range 4 {
		start := time.Now()
		hits, total, err := search.Search(t.Context(), db, payloadProbeTerm, "", limit, search.MatchAll)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(hits) != limit {
			t.Fatalf("search returned %d hits, want %d", len(hits), limit)
		}
		// The match set is the point: an arm that matched a handful
		// would price nothing, and a corpus that stopped carrying the
		// term in every document would look like a fix.
		if total != 20000 {
			t.Fatalf("the term matched %d documents, want every one of them", total)
		}
		if d := time.Since(start); i > 0 && (best == 0 || d < best) {
			best = d
		}
	}
	return best
}

// payloadProbeTerm is in every document of a [payloadCorpus].
const payloadProbeTerm = "engramuxpayloadprobe"

// payloadCorpus builds one database whose events all carry
// [payloadProbeTerm] and whose payloads are a chosen size.
//
// The rows go in directly and in one transaction, on the same reasoning
// [scopedCorpus] is written with: what is being measured is a SELECT, the FTS
// triggers that maintain the index are the migration's own and still fire, and
// ingesting twenty thousand events through the production path would make this
// a benchmark of the writer.
func payloadCorpus(t *testing.T, events, payloadBytes int) *sql.DB {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "engramux.db"))
	if err != nil {
		t.Fatalf("store.Open: %v\nA \"database is locked\" here is a development service holding "+
			"its own file, not this one - see AGENTS.md", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	exec(`INSERT INTO projects (id, root, name, created_at) VALUES ('p', 'z:\payload', 'p', 0)`)
	exec(`INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at)
	      VALUES ('codex:p', 'p', 'codex', 'p', 'active', 0)`)

	for i := range events {
		// The filler differs per event so the index is not one term
		// repeated, which would make every document's bm25 identical and
		// the ranking meaningless.
		var b strings.Builder
		b.WriteString(payloadProbeTerm)
		for b.Len() < payloadBytes {
			fmt.Fprintf(&b, " w%d", i)
		}
		leaves := b.String()[:payloadBytes]
		p, err := json.Marshal(map[string]any{"text": leaves})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		exec(`INSERT INTO events (id, project_id, session_id, host, source, event_name,
		                          payload, leaves, privacy_class, redaction_version, received_at)
		      VALUES (?, 'p', 'codex:p', 'codex', 'pipe', 'PostToolUse', ?, ?, '', 1, ?)`,
			fmt.Sprintf("e%06d", i), string(p), leaves, int64(i))
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return db
}
