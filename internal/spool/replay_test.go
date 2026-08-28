package spool

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/store"
)

// The environment variables that turn a re-executed copy of this test binary
// into the process gate clause 3 kills. Re-executing os.Args[0] rather than
// building a second program keeps the child on the same code, the same
// production DSN and, under ./scripts/race.sh, the same race detector.
const (
	childDBEnv = "ENGRAMUX_SPOOL_CRASH_DB"
	childIDEnv = "ENGRAMUX_SPOOL_CRASH_ID"
)

// committedLine is what the child prints once its COMMIT has returned. Reading
// it is how the parent knows the kill lands after the commit and before
// anything else - which is the whole of clause 3.
const committedLine = "COMMITTED"

func TestMain(m *testing.M) {
	if path := os.Getenv(childDBEnv); path != "" {
		commitThenBlockForever(path, os.Getenv(childIDEnv))
		return
	}
	os.Exit(m.Run())
}

// commitThenBlockForever is the service, at the instant clause 3 describes: it
// opens the database with the production DSN, ingests one event, and stops
// dead between the COMMIT returning and any ACK being written.
//
// It blocks rather than exiting, so the parent's kill is a TerminateProcess
// against a live process still holding the exclusive lock. Nothing here defers
// a Close, a Sync or a checkpoint: a kill that lets cleanup run is a kill that
// tests nothing (docs/evidence/crash).
//
// It blocks on a read from stdin rather than on the select{} that
// docs/evidence/crash/main.go uses. Observed with select{} here: this process
// is then the only goroutine, the runtime's deadlock detector runs, and the
// child prints "fatal error: all goroutines are asleep - deadlock!" and starts
// dying on its own, racing the parent's kill. It did not reproduce on every
// run, which is worse rather than better - a kill under test that sometimes is
// not the thing that ends the process is a different experiment with the same
// row count. A read parked in a syscall keeps an M busy, so the detector never
// gets to run, and the parent holds the write end of the pipe open and never
// writes to it.
func commitThenBlockForever(path, id string) {
	ctx := context.Background()

	db, err := openWithPatience(ctx, path)
	if err != nil {
		fmt.Println("CHILD-OPEN-FAIL", err)
		os.Exit(2)
	}
	payload, err := fixtures.Fixture{File: fixtures.ClaudePostToolUseObject}.Bytes()
	if err != nil {
		fmt.Println("CHILD-FIXTURE-FAIL", err)
		os.Exit(2)
	}
	status, err := store.Ingest(ctx, db, ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: id,
		Payload:  payload,
	}, store.SourcePipe, time.Now())
	if err != nil || status != ipc.Committed {
		fmt.Println("CHILD-INGEST-FAIL", status, err)
		os.Exit(2)
	}

	// COMMIT has returned. Everything past this line is what the kill
	// destroys - including the ACK the relay is waiting for.
	fmt.Println(committedLine)
	if err := os.Stdout.Sync(); err != nil {
		fmt.Println("CHILD-SYNC-FAIL", err)
		os.Exit(2)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)

	// Only reachable if the parent let go of the pipe without killing this
	// process, which means the kill under test never happened.
	fmt.Println("CHILD-NOT-KILLED")
	os.Exit(3)
}

// openWithPatience opens the database at path, retrying for up to 3 seconds at
// 20 ms intervals.
//
// The patience is not decoration. Windows releases a killed process's handles
// asynchronously, so the first open after a TerminateProcess can still find the
// exclusive lock held; docs/evidence/crash measured the reopen 20/20 with
// exactly this retry, and a single immediate open is a flaky test.
func openWithPatience(ctx context.Context, path string) (*sql.DB, error) {
	deadline := time.Now().Add(3 * time.Second)
	for {
		db, err := store.Open(ctx, path)
		if err == nil {
			return db, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// openMigrated opens path and migrates it up, closing the pool when the test
// ends. On Windows an open handle makes t.TempDir()'s cleanup fail, and the WAL
// sidecar counts.
func openMigrated(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := openWithPatience(t.Context(), path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	})
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatalf("Migrate %s: %v", path, err)
	}
	return db
}

// killAfterCommit is gate clause 3's steps 1 to 3, and the assertions that
// make "killed" more than a hope.
//
// It re-executes this test binary against the database at dbPath, waits for the
// child to say its COMMIT returned, and kills it with TerminateProcess before
// any ACK can be written. It returns once the child is gone; Windows releases a
// killed process's handles asynchronously, so the caller reopens the database
// with openWithPatience rather than immediately.
//
// Two assertions decide whether the kill is what ended the child, and without
// them the test passes just as happily when the child ended itself first -
// which is a different experiment with the same row count, and the measured way
// it happens is the runtime's deadlock detector rather than anything a reader
// would look for.
//
// TerminateProcess against a process that has already exited fails with
// ERROR_ACCESS_DENIED, so a nil error from Kill means there was something alive
// to kill. And Go's Kill terminates with an exit code of 1, where every way this
// child can end itself uses another: 2 for a reported failure or a Go fatal
// error, 3 for reaching the end of its own function. Both were confirmed by
// making the child exit 9 on its own: Kill became "TerminateProcess: Access is
// denied".
func killAfterCommit(t *testing.T, dbPath, id string) {
	t.Helper()

	//nolint:gosec // G204: os.Args[0] is this test binary, and the two
	// environment values are this test's own literals.
	cmd := exec.CommandContext(t.Context(), os.Args[0])
	cmd.Env = append(os.Environ(), childDBEnv+"="+dbPath, childIDEnv+"="+id)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("child stdout: %v", err)
	}
	// Opened and never written to. It is what the child parks in a read
	// syscall on, so that it is alive and holding the exclusive lock when
	// the kill arrives.
	if _, err := cmd.StdinPipe(); err != nil {
		t.Fatalf("child stdin: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the child: %v", err)
	}
	sc := bufio.NewScanner(stdout)
	line := ""
	if sc.Scan() {
		line = sc.Text()
	}

	// os/exec's Kill is TerminateProcess on Windows: no defers run, nothing
	// is closed, nothing is flushed, and no ACK is written.
	killErr := cmd.Process.Kill()
	waitErr := cmd.Wait()
	if line != committedLine {
		t.Fatalf("child said %q, want %q (scan %v, wait %v)", line, committedLine, sc.Err(), waitErr)
	}
	if killErr != nil {
		t.Fatalf("kill the child: %v", killErr)
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("child wait = %v, want an *exec.ExitError from the kill", waitErr)
	}
	if code := exitErr.ExitCode(); code != 1 {
		t.Fatalf("child exit code = %d, want 1 - it did not die of TerminateProcess", code)
	}
}

// countEvents returns the number of rows in events.
func countEvents(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM events`).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// eventRow returns the source and received_at of one event, failing the test
// when there is no such row.
func eventRow(t *testing.T, db *sql.DB, id string) (source string, receivedAt int64) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(),
		`SELECT source, received_at FROM events WHERE id = ?`, id).Scan(&source, &receivedAt); err != nil {
		t.Fatalf("read event %s: %v", id, err)
	}
	return source, receivedAt
}

// countBySource returns how many events carry each source value.
func countBySource(t *testing.T, db *sql.DB, src store.Source) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM events WHERE source = ?`, string(src)).Scan(&n); err != nil {
		t.Fatalf("count events with source %q: %v", src, err)
	}
	return n
}

// TestAKillBetweenCommitAndTheAckReplaysExactlyOnce is Phase 1 gate clause 3,
// step for step:
//
//  1. a relay sends an event under a minted id - here, a child process ingests
//     it with store.SourcePipe;
//  2. the ingest transaction's COMMIT returns, and the child says so;
//  3. the child is killed with TerminateProcess before any ACK is written;
//  4. the relay's post-dial budget expires with no valid ACK, so it spools the
//     event under the same id;
//  5. a fresh process opens the same database, which it can because the
//     exclusive lock does not survive process death;
//  6. the drain replays the spooled record;
//  7. exactly one row exists.
//
// Three assertions have to hold together, because two of them pass on their
// own for the wrong reason. One row after the drain is also what a lost commit
// looks like, so step 2's row is asserted before the drain runs. One row after
// the drain is also what a drain that replayed nothing looks like, so the
// record is asserted consumed and the ingest is asserted to have been called
// with the original id. And the surviving row's source is still "pipe": the
// replay was a no-op on the row the child committed, not a delete and reinsert
// of it.
func TestAKillBetweenCommitAndTheAckReplaysExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "engramux.db")
	spoolDir := filepath.Join(dir, "spool")
	id := idN(1)

	payload, err := fixtures.Fixture{File: fixtures.ClaudePostToolUseObject}.Bytes()
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}

	// Seed the schema and let go of the exclusive lock, so the child can
	// take it. A migration is not part of what clause 3 measures.
	seed, err := openWithPatience(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if err := store.Migrate(t.Context(), seed); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("seed close: %v", err)
	}

	// Steps 1-3.
	killAfterCommit(t, dbPath, id)

	// Step 4. This is what the relay does when its post-dial budget expires
	// with no valid ACK: the same id, not a fresh one (I-05).
	if err := Write(spoolDir, id, payload); err != nil {
		t.Fatalf("spool the undelivered event: %v", err)
	}
	requireNames(t, spoolDir, "after the relay spooled the event", id+ext)

	// Step 5.
	db := openMigrated(t, dbPath)

	// Step 2's row survived the kill. Asserted before the drain, because
	// "exactly one row" afterwards is also what a lost commit looks like.
	if n := countEvents(t, db); n != 1 {
		t.Fatalf("events after the kill = %d, want 1 - the committed row did not survive", n)
	}
	source, receivedAt := eventRow(t, db, id)
	if source != string(store.SourcePipe) {
		t.Fatalf("the committed row's source = %q, want %q", source, store.SourcePipe)
	}

	// Step 6.
	c := &collector{}
	d := &Drainer{Dir: spoolDir, Ingest: func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
		if _, err := c.ingest(ctx, env); err != nil {
			return ipc.Rejected, err
		}
		return store.Ingest(ctx, db, env, store.SourceSpool, time.Now())
	}}
	n, err := d.Drain(t.Context())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// Step 7, and the two ways one row can be the wrong answer.
	if n != 1 {
		t.Fatalf("Drain replayed %d records, want 1 - nothing was replayed, so nothing was tested", n)
	}
	if got := c.ids(); !slices.Equal(got, []string{id}) {
		t.Fatalf("the drain replayed %q, want the id the relay minted %q - a re-minted id is a second row", got, id)
	}
	requireNames(t, spoolDir, "after the drain consumed the record")
	requireAbsent(t, spoolDir, quarantineDir, "the quarantine directory")

	if total := countEvents(t, db); total != 1 {
		t.Fatalf("events after the replay = %d, want exactly 1", total)
	}
	gotSource, gotReceived := eventRow(t, db, id)
	if gotSource != string(store.SourcePipe) || gotReceived != receivedAt {
		t.Fatalf("after the replay the row is (source %q, received_at %d), want the committed row (%q, %d) untouched",
			gotSource, gotReceived, source, receivedAt)
	}
}

// TestDrainingWhileTheServiceIngestsKeepsEveryEvent is the drain competing with
// live ingest for the single connection (spec 5.4), which is what
// ./scripts/race.sh exists to look at. Every event has to land exactly once,
// whichever path it came in on.
func TestDrainingWhileTheServiceIngestsKeepsEveryEvent(t *testing.T) {
	const (
		writers      = 4
		perWriter    = 8
		liveEvents   = writers * perWriter
		spoolRecords = 32
	)
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, "engramux.db"))
	spoolDir := filepath.Join(dir, "spool")

	payload, err := fixtures.Fixture{File: fixtures.CodexSessionEnd}.Bytes()
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	for i := 1; i <= spoolRecords; i++ {
		if err := Write(spoolDir, idN(i), payload); err != nil {
			t.Fatalf("Write %s: %v", idN(i), err)
		}
	}

	errs := make(chan error, liveEvents)
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range perWriter {
				id := idN(1000 + w*perWriter + e)
				status, err := store.Ingest(t.Context(), db, ipc.Envelope{
					Version:  ipc.Version,
					Type:     ipc.IngestEvent,
					IngestID: id,
					Payload:  payload,
				}, store.SourcePipe, time.Now())
				if err != nil || status != ipc.Committed {
					errs <- fmt.Errorf("live ingest %s answered %q: %w", id, status, err)
				}
			}
		}()
	}

	d := &Drainer{Dir: spoolDir, Ingest: func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
		return store.Ingest(ctx, db, env, store.SourceSpool, time.Now())
	}}
	n, err := d.Drain(t.Context())
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if n != spoolRecords {
		t.Fatalf("Drain replayed %d records, want %d", n, spoolRecords)
	}
	if total := countEvents(t, db); total != liveEvents+spoolRecords {
		t.Fatalf("events = %d, want %d", total, liveEvents+spoolRecords)
	}
	if got := countBySource(t, db, store.SourcePipe); got != liveEvents {
		t.Fatalf("events with source pipe = %d, want %d", got, liveEvents)
	}
	if got := countBySource(t, db, store.SourceSpool); got != spoolRecords {
		t.Fatalf("events with source spool = %d, want %d", got, spoolRecords)
	}
	requireNames(t, spoolDir, "after the drain")
}
