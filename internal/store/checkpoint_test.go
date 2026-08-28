package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// exists reports whether path is there, which for the -shm file is the whole
// question (spec 5.4).
func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	t.Fatalf("stat %s: %v", path, err)
	return false
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

// copyDB copies the database at from and its WAL to to, and nothing else.
//
// Taken while a writer still holds the database, that is byte for byte the
// state a hard kill leaves: a main file, a hot WAL, and no -shm. Both §7.2's
// crash rounds and the live installation measured the -shm absent immediately
// after a TerminateProcess, so the copy is checked for one before it is used.
func copyDB(t *testing.T, from, to string) {
	t.Helper()
	for _, suffix := range []string{"", "-wal"} {
		//nolint:gosec // G304: from is a path this test built under
		// t.TempDir, and suffix is one of two literals.
		b, err := os.ReadFile(from + suffix)
		if err != nil {
			t.Fatalf("read %s%s: %v", from, suffix, err)
		}
		//nolint:gosec // G703: to is the same, built by the caller from
		// t.TempDir and a literal name.
		if err := os.WriteFile(to+suffix, b, 0o600); err != nil {
			t.Fatalf("write %s%s: %v", to, suffix, err)
		}
	}
	if exists(t, to+"-shm") {
		t.Fatalf("%s-shm exists before anything opened the copy", to)
	}
}

// lockingModeFirstDSN is the production DSN with journal_mode moved out of the
// _pragma list and into the driver's own _journal_mode key.
//
// That is not cosmetic. modernc.org/sqlite sorts _pragma values
// lexicographically before applying them, and applies its shorthand keys after
// the whole list. So the production DSN runs journal_mode(wal) before
// locking_mode(exclusive) - j before l - and this one runs it after. See
// [TestTheWalIndexIsCreatedOnEveryReopen] for what that changes.
func lockingModeFirstDSN(t *testing.T, path string) string {
	t.Helper()
	const journal = "_pragma=journal_mode(wal)&"
	out := strings.Replace(dsn(path), journal, "", 1)
	if out == dsn(path) {
		t.Fatalf("test setup: the production DSN no longer contains %q: %s", journal, dsn(path))
	}
	return out + "&_journal_mode=WAL"
}

// openRaw opens uri the way [open] does but without the DSN name check, so a
// test can open with a DSN that spells a pragma somewhere else.
func openRaw(t *testing.T, uri string) *sql.DB {
	t.Helper()
	db, err := sql.Open(driverName, uri)
	if err != nil {
		t.Fatalf("sql.Open %s: %v", uri, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close %s: %v", uri, err)
		}
	})
	if err := verifyPragmas(t.Context(), db); err != nil {
		t.Fatalf("verifyPragmas %s: %v", uri, err)
	}
	if err := takeExclusiveLock(t.Context(), db); err != nil {
		t.Fatalf("takeExclusiveLock %s: %v", uri, err)
	}
	return db
}

// TestTheWalIndexIsCreatedOnEveryReopen pins what the -shm file actually does,
// against spec 5.4's claim that locking_mode=EXCLUSIVE means it is never
// created at all.
//
// That claim holds for the first open of a database this process created, and
// for nothing else. Every step is a measurement, and steps 3 and 4 are the ones
// that contradict the spec:
//
//  1. a database created by this Open has no -shm, and does not grow one;
//  2. a checkpoint and a clean Close leave no -wal and no -shm on disk;
//  3. reopening that database - fully checkpointed, cleanly closed, with no WAL
//     file at all - creates a 32,768-byte -shm immediately;
//  4. so does reopening the hot WAL a kill leaves, which is the case measured
//     against the live installation;
//  5. and the cause is neither the WAL nor the crash. It is that
//     modernc.org/sqlite applies journal_mode(wal) before
//     locking_mode(exclusive), so the pager opens the wal-index while it is
//     still in normal locking mode and can never move it to heap memory
//     afterwards. Applying locking_mode first suppresses the -shm on the very
//     same file, hot WAL and all.
//
// Step 3 is why no amount of checkpointing prevents this, and why the shutdown
// checkpoint cannot either: there is no WAL left to checkpoint and the -shm
// appears anyway. If step 5's ordering is ever adopted in the production DSN,
// steps 3 and 4 go red - which is the signal to come back here and to spec 5.4.
func TestTheWalIndexIsCreatedOnEveryReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "engramux.db")
	payload := ckptFixture(t)

	// 1. Created by this Open.
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if exists(t, path+"-shm") {
		t.Error("a -shm exists on a database this Open created")
	}
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for i := range 10 {
		ingestOne(t, db, i, payload)
	}
	if exists(t, path+"-shm") {
		t.Error("a -shm appeared after ingest on a database this Open created")
	}

	// Both hot copies are taken here, while the writer still holds the WAL.
	// Copying one from the other later would not work: opening a hot copy
	// and closing it cleanly is what consumes the hot WAL.
	crashed := filepath.Join(dir, "crashed.db")
	copyDB(t, path, crashed)
	fixed := filepath.Join(dir, "fixed.db")
	copyDB(t, path, fixed)

	// 2. Checkpoint, then close. This is the shutdown path.
	if err := Checkpoint(t.Context(), db); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := fileSize(t, path+"-wal"); got != 0 {
		t.Errorf("the -wal is %d bytes after a checkpoint and a clean Close, want it gone", got)
	}
	if exists(t, path+"-shm") {
		t.Error("a -shm survives a clean Close")
	}

	// 3. Reopen it. Nothing is hot and nothing needs recovering.
	reopened, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	switch shm := fileSize(t, path+"-shm"); shm {
	case 0:
		t.Error("no -shm after reopening a checkpointed, cleanly closed database - spec 5.4 is " +
			"right after all, and this test and §5.4 both need revisiting")
	case 32768:
	default:
		t.Errorf("the -shm is %d bytes, want the 32,768 measured against the live installation", shm)
	}
	requireCount(t, reopened, "events", 10)
	if err := reopened.Close(); err != nil {
		t.Fatalf("close the reopened database: %v", err)
	}

	// 4. The hot WAL a kill leaves, which is the case measured live.
	hot, err := Open(t.Context(), crashed)
	if err != nil {
		t.Fatalf("open the hot copy: %v", err)
	}
	if got := fileSize(t, crashed+"-shm"); got != 32768 {
		t.Errorf("the hot copy's -shm is %d bytes, want 32,768", got)
	}
	requireCount(t, hot, "events", 10)
	if err := hot.Close(); err != nil {
		t.Fatalf("close the hot copy: %v", err)
	}

	// 5. The cause, and what removes it, on the same hot WAL.
	fixedDB := openRaw(t, lockingModeFirstDSN(t, fixed))
	if exists(t, fixed+"-shm") {
		t.Error("a -shm exists with locking_mode applied before journal_mode - the pragma order " +
			"is not what creates it, and the S4 report's finding is wrong")
	}
	requireCount(t, fixedDB, "events", 10)
	if exists(t, fixed+"-shm") {
		t.Error("a -shm appeared while recovering the hot WAL with locking_mode applied first")
	}
}
