package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/spool"
)

// sidecarSize returns the size of the database sidecar with the given suffix,
// and -1 when there is no such file. A missing -wal and an empty one are the
// same thing to a reader; a missing -shm and an empty one are not (spec 5.4).
func sidecarSize(t *testing.T, dir, suffix string) int64 {
	t.Helper()
	fi, err := os.Stat(filepath.Join(dir, dbName) + suffix)
	if errors.Is(err, os.ErrNotExist) {
		return -1
	}
	if err != nil {
		t.Fatalf("stat %s%s: %v", dbName, suffix, err)
	}
	return fi.Size()
}

// TestTheRunLoopCheckpointsWhileItServes is the wiring, which is the part
// store's own tests cannot see. [store.Checkpointer] can be correct and never
// started - which is exactly the state this task found the service in, with
// spec 5.4's checkpointing specified and nothing calling it.
//
// The threshold is left at production, so the timer is the only trigger that
// can fire and a WAL of a few tens of kilobytes cannot reach it. Events arrive
// through the spool rather than the pipe because the drain is already wired and
// a relay is a second process; what matters here is that rows are committed,
// not how they got in.
//
// The WAL reaching 0 is the assertion. Nothing but a checkpoint empties it, and
// the spool being empty first means the rows were committed before the wait
// began - so a WAL of 0 bytes after that is a checkpoint that ran inside the
// running service.
func TestTheRunLoopCheckpointsWhileItServes(t *testing.T) {
	restoreInterval, restorePoll := checkpointInterval, checkpointPoll
	checkpointInterval, checkpointPoll = 200*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { checkpointInterval, checkpointPoll = restoreInterval, restorePoll })

	dir := t.TempDir()
	spoolPath := filepath.Join(dir, spoolDir)
	const (
		first  = "0192f0c0-0000-7000-8000-0000000ac001"
		second = "0192f0c0-0000-7000-8000-0000000ac002"
	)
	payload := []byte(`{"hook_event_name":"Stop","session_id":"s","cwd":"Z:\\ckpt\\project","model":"m"}`)
	for _, id := range []string{first, second} {
		if err := spool.Write(spoolPath, id, payload, nil); err != nil {
			t.Fatalf("spool %s: %v", id, err)
		}
	}

	stop := running(t, dir)
	waitForEmptySpool(t, spoolPath, "the pass the run loop makes at startup")

	deadline := time.Now().Add(20 * time.Second)
	for wal := sidecarSize(t, dir, "-wal"); wal > 0; wal = sidecarSize(t, dir, "-wal") {
		if time.Now().After(deadline) {
			t.Fatalf("the WAL is still %d bytes 20s after the drain committed - "+
				"the run loop is not checkpointing", wal)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// And the shutdown leaves nothing behind. This is the state spec 5.4's
	// checkpoint on the way out produces - and, measured, the state a clean
	// db.Close produces on its own; see checkpointOnTheWayOut.
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := sidecarSize(t, dir, "-wal"); got != -1 {
		t.Errorf("the -wal is %d bytes after a clean shutdown, want it gone", got)
	}
	// The DSN keeps this one away rather than the shutdown doing so: with
	// locking_mode set before the first WAL access there is no wal-index to
	// remove. internal/store owns that measurement in all three of its
	// states; this is the service's own end state, asserted where it runs.
	if got := sidecarSize(t, dir, "-shm"); got != -1 {
		t.Errorf("the -shm is %d bytes after a clean shutdown, want it gone", got)
	}

	db := openMigrated(t, filepath.Join(dir, dbName))
	for _, id := range []string{first, second} {
		requireStoredFromSpool(t, db, id)
	}
}

// TestCheckpointOnTheWayOutRunsUnderACancelledContext is the one thing about
// spec 5.4's checkpoint on shutdown that a test can decide.
//
// Whether the call exists at all is not observable: a clean db.Close
// checkpoints the WAL and removes it anyway, so the on-disk state after a
// shutdown is the same with the call and without it - deleting it
// from run() leaves the whole suite green, which was confirmed rather than
// assumed. What is observable is that it runs at all. It is reached only after
// the context that started the shutdown has been cancelled, and a checkpoint
// handed that context is refused by the driver before it touches the database.
// So the context is what this asserts, with a WAL that is not empty and a
// context that is already dead.
func TestCheckpointOnTheWayOutRunsUnderACancelledContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, dbName)
	db := openMigrated(t, path)

	seedEvent(t, db, "0192f0c0-0000-7000-8000-0000000ac003", "codex", "Stop", 1)
	if got := sidecarSize(t, dir, "-wal"); got <= 0 {
		t.Fatalf("the WAL is %d bytes before the checkpoint, so truncating it would prove nothing", got)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	checkpointOnTheWayOut(ctx, db)

	if got := sidecarSize(t, dir, "-wal"); got != 0 {
		t.Errorf("the WAL is %d bytes after the checkpoint on the way out, want 0 - "+
			"the cancelled shutdown context reached the driver", got)
	}
}
