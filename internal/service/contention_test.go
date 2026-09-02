package service

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// The load. It is sized so that the unguarded arm is far over the budget and the
// guarded one is far under, because a test that discriminates by a few percent
// would be a flake on a loaded machine rather than a gate.
//
// contentionEvents is the corpus one search has to rank. `ORDER BY rank` makes
// FTS5 score every matching document before the first row comes back (spec 5.7),
// and every document here carries the search term, so one search scans all of
// them - which is the expensive read this is about.
//
// contentionReaders is how many searches are in flight while the ingests run.
// Without the gate that is how many read statements can queue in front of one
// ingest, because database/sql serves waiting acquisitions in arrival order and
// spec 5.4 leaves one connection.
const (
	contentionEvents  = 4000
	contentionReaders = 96
	contentionIngests = 20
)

// TestPhase5GateAReaderDoesNotPushIngestPastItsBudget is spec 8's Phase 5
// contention clause, and it is the composed property the three mechanism tests
// take apart.
//
// # The number is spec 5.3's
//
// A relay has 800 ms after the dial to write its event and read the ACK. Blowing
// it costs nothing that is lost - it spools and I-04 holds - but a reader that
// quietly pushes every relay into the spool is a reader that has taken the
// product's one job away, and Phase 5 is the first time the reader is not a
// person typing a command.
//
// # What it measures, and against what
//
// It uses [handlers], which is the production wiring: the same gate, the same
// deadline, the same order. The ingest path is timed because that is the side
// with the budget; the searches are only load.
//
// The budget is measured around the handler and not around a whole relay round
// trip. The pipe, the frame and the dial are measured elsewhere at p50 0.515 ms
// (spec 7.1), so what is left of the 800 ms is what the handler may take, and
// this asserts the whole of it against the handler alone - a deliberately
// stricter reading than the clause needs.
//
// Deliberately broken by taking the gate out of [handlers], the reads queue
// ahead of each ingest and the slowest one goes far over.
func TestPhase5GateAReaderDoesNotPushIngestPastItsBudget(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))
	seedForContention(t, db, contentionEvents)

	h := handlers(db, filepath.Join(dir, dbName), filepath.Join(dir, spoolDir), time.Now(), newReadGate(), newHealth())

	// The readers run until the ingests are done. They are started first and
	// given a moment to fill the queue, so the ingests below arrive into
	// contention rather than into an idle service.
	readersDone := make(chan struct{})
	var readers sync.WaitGroup
	for range contentionReaders {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-readersDone:
					return
				default:
				}
				// A failure is not asserted here: a read that
				// hits its deadline under this much load is the
				// designed behaviour, not a fault. What is
				// asserted is the ingest side.
				_, _ = h.Search(t.Context(), ipc.SearchRequest{Query: contentionTerm})
			}
		}()
	}
	t.Cleanup(func() {
		close(readersDone)
		readers.Wait()
	})
	time.Sleep(50 * time.Millisecond)

	took := make([]time.Duration, contentionIngests)
	for i := range took {
		id := fmt.Sprintf("8f1c2a10-0000-7000-8000-0000000%05d", i)
		// A different worktree from the seeded one: the seed writes its
		// project row by hand with an id of its own, and the ingest path
		// derives an id from the cwd. One root under two ids collides on
		// projects.root, which is UNIQUE - and the collision is the
		// constraint doing its job, not a fixture to work around.
		payload := fmt.Appendf(nil, `{"hook_event_name":"Stop","session_id":"contention","cwd":%q}`,
			`Z:\contention-live`)
		start := time.Now()
		status, err := h.Ingest(t.Context(), ipc.Envelope{
			Version: ipc.Version, Type: ipc.IngestEvent, IngestID: id, Payload: payload,
		})
		took[i] = time.Since(start)
		if err != nil || status != ipc.Committed {
			t.Fatalf("ingest %d: status %q, err %v", i, status, err)
		}
	}

	slices.Sort(took)
	worst := took[len(took)-1]
	t.Logf("%d ingests against %d readers over %d events: median %s, worst %s",
		contentionIngests, contentionReaders, contentionEvents, took[len(took)/2], worst)
	if worst > postDialBudget {
		t.Errorf("the slowest ingest took %s, over spec 5.3's %s post-dial budget: "+
			"a reader is holding the one connection in front of it", worst, postDialBudget)
	}
}

// postDialBudget is spec 5.3's, restated here because internal/service does not
// import the relay. A change to the relay's budget has to reach this line by
// hand, which is why the number is written out rather than derived.
const postDialBudget = 800 * time.Millisecond

// contentionTerm is in every seeded document, so one search ranks the whole
// corpus.
const contentionTerm = "frobnicatorcontention"

// seedForContention inserts n events in one transaction, all matching
// [contentionTerm].
//
// Direct SQL rather than store.Ingest, and one transaction rather than n: what
// is being measured is the contention between a read and a write, not the cost
// of building the fixture. The AFTER INSERT trigger the migration owns is what
// indexes them, so the index this fills is the production one.
func seedForContention(t *testing.T, db *sql.DB, n int) {
	t.Helper()
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
	exec(`INSERT INTO projects (id, root, name, created_at) VALUES ('pc', 'z:\contention', 'c', 0)`)
	exec(`INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at)
	      VALUES ('codex:pc', 'pc', 'codex', 'pc', 'active', 0)`)
	for i := range n {
		leaves := contentionTerm + " " + strings.Repeat("filler ", 20) + fmt.Sprintf("doc%06d", i)
		exec(`INSERT INTO events (id, project_id, session_id, host, source, event_name,
		                          payload, leaves, privacy_class, redaction_version, received_at)
		      VALUES (?, 'pc', 'codex:pc', 'codex', 'pipe', 'PostToolUse', ?, ?, '', 1, ?)`,
			fmt.Sprintf("seed-%06d", i), `{"text":"`+leaves+`"}`, leaves, int64(i))
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}
