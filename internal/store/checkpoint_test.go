package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/fixtures"
)

// ckptPayload is the fixture every checkpoint test ingests. Which one does not
// matter here - what an event costs the WAL is page granularity, not payload
// size (§7.2 measured 12,439 B of WAL per event against a p50 payload of
// 1,182 B) - so this is the largest of the four and nothing more.
const ckptPayload = fixtures.ClaudePostToolUseObject

// ckptDB opens and migrates a database in the test's own temp directory, and
// returns the pool and the path, since the WAL is a file beside it.
func ckptDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := dbPath(t)
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db, path
}

// fileSize returns the size of path, or 0 when it does not exist. A missing
// -wal is the same thing as an empty one to everything below.
func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size()
}

// ckptID is the event id the nth ingest of these tests uses.
func ckptID(n int) string { return fmt.Sprintf("ckpt-%06d", n) }

// ingestOne stores one event under a distinct id and fails the test unless it
// committed.
func ingestOne(t *testing.T, db *sql.DB, n int, payload []byte) {
	t.Helper()
	status, err := Ingest(t.Context(), db, ingestEnv(ckptID(n), payload), SourcePipe, time.Now())
	requireCommitted(t, status, err, "ingest "+ckptID(n))
}

// ckptFixture reads the fixture the checkpoint tests ingest.
func ckptFixture(t *testing.T) []byte {
	t.Helper()
	b, err := fixtures.Fixture{File: ckptPayload}.Bytes()
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	return b
}

// start runs c for the duration of the test and returns the function that
// stops it and waits for it, so no loop outlives the database it holds.
// Calling the returned function more than once is safe: every test stops the
// loop before it reads a final size, and t.Cleanup stops it again.
func start(t *testing.T, c *Checkpointer) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx)
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
	t.Cleanup(stop)
	return stop
}

// waitForWAL waits until the WAL beside path is at most want bytes, and fails
// the test when it never gets there.
func waitForWAL(t *testing.T, path string, want int64, within time.Duration, what string) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		got := fileSize(t, path+"-wal")
		if got <= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: the WAL is %d bytes after %v, want at most %d - no checkpoint ran",
				what, got, within, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestCheckpointTruncatesTheWALAndKeepsEveryRow is the checkpoint itself, with
// no loop around it: spec 5.4's straight TRUNCATE.
//
// The row count is the assertion gate clause 3 names. The other two are what
// make it more than "the call returned": the WAL is 0 bytes afterwards, which
// is what TRUNCATE means and PASSIVE does not do, and the main database file
// grew. In WAL mode nothing but a checkpoint writes the main file, so its size
// is direct evidence that pages moved rather than that a function was called.
func TestCheckpointTruncatesTheWALAndKeepsEveryRow(t *testing.T) {
	db, path := ckptDB(t)
	payload := ckptFixture(t)

	const events = 20
	mainBefore := fileSize(t, path)
	for i := range events {
		ingestOne(t, db, i, payload)
	}
	walBefore := fileSize(t, path+"-wal")
	if walBefore == 0 {
		t.Fatalf("the WAL is empty after %d ingests, so there is nothing to checkpoint", events)
	}

	if err := Checkpoint(t.Context(), db); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	if got := fileSize(t, path+"-wal"); got != 0 {
		t.Errorf("the WAL is %d bytes after a TRUNCATE checkpoint, want 0 (it was %d)", got, walBefore)
	}
	if got := fileSize(t, path); got <= mainBefore {
		t.Errorf("the main database is %d bytes, was %d - no pages moved out of the WAL", got, mainBefore)
	}
	requireCount(t, db, "events", events)

	// Read back, not merely counted: a checkpoint that lost a page would
	// still count.
	for i := range events {
		requirePayload(t, readEvent(t, db, ckptID(i)).payload, payload,
			fmt.Sprintf("event %d after the checkpoint", i))
	}
	t.Logf("%d events grew the WAL to %d B; the checkpoint moved it into a main file of %d B",
		events, walBefore, fileSize(t, path))
}

// TestCheckpointReportsBusy pins that the busy column is read and compared
// rather than discarded. Under exclusive locking there is no second connection
// to block on, and §7.4 measured busy=0 at every WAL size it tried.
func TestCheckpointReportsBusy(t *testing.T) {
	db, _ := ckptDB(t)
	payload := ckptFixture(t)
	for i := range 5 {
		ingestOne(t, db, i, payload)
	}
	busy, err := checkpoint(t.Context(), db)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if busy != 0 {
		t.Errorf("wal_checkpoint(TRUNCATE) reported busy=%d, want 0", busy)
	}
}

// TestTheSizeThresholdCheckpoints is half of gate clause 2. The interval is set
// out of reach - an hour against a test that gives up after five seconds - so
// the size threshold is the only trigger that can fire.
func TestTheSizeThresholdCheckpoints(t *testing.T) {
	const threshold = 128 << 10
	db, path := ckptDB(t)
	payload := ckptFixture(t)
	start(t, &Checkpointer{DB: db, Path: path, Threshold: threshold, Interval: time.Hour, Poll: time.Millisecond})

	for i := 0; fileSize(t, path+"-wal") < threshold; i++ {
		if i > 10000 {
			t.Fatalf("the WAL never reached %d bytes in %d ingests", int64(threshold), i)
		}
		ingestOne(t, db, i, payload)
	}
	waitForWAL(t, path, 0, 5*time.Second, "the size threshold")
}

// TestTheTimerCheckpoints is the other half of gate clause 2. The threshold is
// set out of reach - 1 GiB against a WAL of a few tens of kilobytes - so the
// timer is the only trigger that can fire.
//
// The elapsed time is measured from before the loop starts, and compared
// against the interval rather than against a slack figure. A loop whose timer
// fired could not have truncated the WAL sooner than that, and one whose
// threshold fired instead would be much faster - so this is what separates the
// two triggers, and it cannot pass by accident of timing.
func TestTheTimerCheckpoints(t *testing.T) {
	const (
		unreachable = 1 << 30
		interval    = 200 * time.Millisecond
	)
	db, path := ckptDB(t)
	payload := ckptFixture(t)

	began := time.Now()
	start(t, &Checkpointer{DB: db, Path: path, Threshold: unreachable, Interval: interval, Poll: time.Millisecond})

	for i := range 3 {
		ingestOne(t, db, i, payload)
	}
	wal := fileSize(t, path+"-wal")
	if wal == 0 {
		t.Fatal("the WAL is empty, so truncating it would prove nothing")
	}
	if wal >= unreachable {
		t.Fatalf("the WAL is %d bytes, over the threshold this test set out of reach", wal)
	}

	waitForWAL(t, path, 0, 5*time.Second, "the timer")
	if took := time.Since(began); took < interval {
		t.Errorf("the WAL was truncated %v after the loop started, under its %v interval - "+
			"something other than the timer fired", took, interval)
	}
}

// TestTheWALStaysBoundedAcrossALongRun is gate clause 1. It runs the same
// ingests twice, once with a checkpointer and once without, and asserts on the
// two WAL sizes.
//
// The second run is what stops this from being a test that cannot fail. A bound
// on the checkpointed WAL says nothing unless the same workload would have gone
// through it, and 400 ingests of a small fixture is not obviously enough - so
// the size without a checkpointer is measured here rather than assumed from
// §7.2's per-event rate.
//
// That second number is not unbounded growth, and it is worth being exact about
// what it is. SQLite's own wal_autocheckpoint is 1,000 pages by default, spec
// 5.4's DSN does not turn it off, and 4 KiB pages make that about 4.1 MiB - so
// the WAL settles there whatever the run length, and 800 ingests reach the same
// figure as 400. What that automatic checkpoint does is PASSIVE: it moves pages
// into the main file and reuses the WAL in place, so the file keeps its
// high-water mark. Only a TRUNCATE gives the space back, which is what the
// difference between the two numbers below actually measures.
func TestTheWALStaysBoundedAcrossALongRun(t *testing.T) {
	const (
		events    = 400
		threshold = 128 << 10
	)
	payload := ckptFixture(t)

	db, path := ckptDB(t)
	stop := start(t, &Checkpointer{DB: db, Path: path, Threshold: threshold, Interval: time.Hour, Poll: time.Millisecond})
	var peak int64
	for i := range events {
		ingestOne(t, db, i, payload)
		if got := fileSize(t, path+"-wal"); got > peak {
			peak = got
		}
	}
	waitForWAL(t, path, threshold, 5*time.Second, "after the last ingest")
	settled := fileSize(t, path+"-wal")
	stop()

	// The same run with no checkpointer, which is what the service did
	// before this existed: SQLite's automatic PASSIVE checkpoint and nothing
	// else.
	loose, loosePath := ckptDB(t)
	for i := range events {
		ingestOne(t, loose, i, payload)
	}
	automatic := fileSize(t, loosePath+"-wal")

	t.Logf("%d ingests: the WAL peaked at %d B and settled at %d B against a %d B threshold, "+
		"and reached %d B with only SQLite's own wal_autocheckpoint",
		events, peak, settled, int64(threshold), automatic)

	if automatic <= 4*threshold {
		t.Fatalf("with no checkpointer the WAL only reached %d bytes against a %d byte threshold - "+
			"this run is too short for the bound to be doing anything", automatic, int64(threshold))
	}
	// The loop is asynchronous, so the WAL crosses the threshold before the
	// next poll sees it. What it may not do is follow the uncheckpointed
	// curve.
	if peak > 4*threshold {
		t.Errorf("the WAL peaked at %d bytes against a %d byte threshold", peak, int64(threshold))
	}
}

// TestCheckpointingDoesNotBlockIngest is the rest of gate clause 3: the
// checkpoint shares the one connection with live ingest (spec 5.4), so it has
// to give it back.
//
// The bound asserted is spec 5.3's, because that is the requirement: a relay
// has 1 s for its whole round trip. rev.3's fear was that a blind TRUNCATE
// could hold the connection for the full 10 s busy_timeout, and that is what
// this catches. §7.4 measured the checkpoint itself at 32.5 ms at the 64 MiB
// threshold, so the margin is wide on purpose - a tighter bound would fail on a
// loaded machine and say nothing about the design.
//
// The WAL is waited on before the timing is asserted, because otherwise a loop
// that never fired would pass. It has to be the WAL shrinking and not the main
// file growing: SQLite's own wal_autocheckpoint moves pages into the main file
// on its own every 1,000 pages, and 400 ingests cross that. Growth in the main
// file is therefore evidence of *a* checkpoint and not of this one. A WAL file
// that gets smaller is only ever a TRUNCATE, which the automatic checkpoint
// never does - this assertion was written the wrong way round first, and it
// stayed green with the size threshold deliberately removed.
func TestCheckpointingDoesNotBlockIngest(t *testing.T) {
	const (
		writers   = 4
		perWriter = 100
		threshold = 32 << 10
	)
	db, path := ckptDB(t)
	payload := ckptFixture(t)
	stop := start(t, &Checkpointer{DB: db, Path: path, Threshold: threshold, Interval: time.Hour, Poll: time.Millisecond})

	var mu sync.Mutex
	var slowest time.Duration
	errs := make(chan error, writers*perWriter)
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range perWriter {
				id := ckptID(w*perWriter + e)
				began := time.Now()
				status, err := Ingest(t.Context(), db, ingestEnv(id, payload), SourcePipe, time.Now())
				took := time.Since(began)
				switch {
				case err != nil:
					errs <- fmt.Errorf("ingest %s: %w", id, err)
				case status != "committed":
					errs <- fmt.Errorf("ingest %s answered %q", id, status)
				}
				mu.Lock()
				if took > slowest {
					slowest = took
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	waitForWAL(t, path, threshold, 5*time.Second, "after the last ingest")
	stop()

	t.Logf("%d ingests against a %d B threshold: the slowest took %v", writers*perWriter, int64(threshold), slowest)
	requireCount(t, db, "events", writers*perWriter)
	if slowest > time.Second {
		t.Errorf("the slowest ingest took %v, over spec 5.3's whole 1 s relay budget", slowest)
	}
}
