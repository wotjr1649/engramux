package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// ErrCheckpointBusy means the checkpoint could not run to completion because
// something else held a read or write lock on the WAL.
//
// Under spec 5.4's locking there is no second connection to hold one, and §7.4
// measured busy=0 at every WAL size it tried. So this is not a retry condition,
// it is a report that the single-connection assumption no longer holds -
// compare it with [errors.Is] and treat it as the surprise it is.
var ErrCheckpointBusy = errors.New("store: the checkpoint could not run to completion")

// Checkpoint moves every page in the WAL into the database and truncates the
// WAL to nothing. Spec 5.4's policy is this and nothing else: a straight
// TRUNCATE, never a PASSIVE probe with conditional escalation.
//
// rev.3 specified the escalation on the theory that a blind TRUNCATE could hold
// the connection for the full 10 s busy_timeout. §7.4 re-measured it: the
// theory came from a design with several connections, and cold TRUNCATE on an
// exclusive single connection is linear and cheap - about 0.54 ms per MiB of
// WAL, 32.5 ms at the 64 MiB threshold, with busy=0 every time. There is no
// other connection to block on.
//
// It shares the one connection with live ingest and the drain, so it runs
// outside any transaction and returns it immediately. What keeps a single call
// short is the WAL size, which is what [Checkpointer] is for.
func Checkpoint(ctx context.Context, db *sql.DB) error {
	busy, err := checkpoint(ctx, db)
	if err != nil {
		return err
	}
	if busy != 0 {
		return fmt.Errorf("%w: wal_checkpoint reported busy=%d", ErrCheckpointBusy, busy)
	}
	return nil
}

// checkpoint runs the TRUNCATE and returns the busy flag, so a test can assert
// on the value [Checkpoint] turns into an error.
//
// PRAGMA wal_checkpoint returns one row of three columns - busy, the size of
// the WAL in pages, and how many of them were moved - and all three are scanned
// because the driver will not run the statement without the row being read.
func checkpoint(ctx context.Context, db *sql.DB) (busy int, err error) {
	var pages, moved int
	if err := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &pages, &moved); err != nil {
		return 0, fmt.Errorf("store: checkpoint the WAL: %w", err)
	}
	return busy, nil
}

// walSize is the size of the WAL beside the database at path, and 0 when there
// is no WAL file.
//
// A stat rather than a PRAGMA, and that is the point: the size has to be
// readable often enough that the threshold means something, and every way of
// asking SQLite costs the one connection that live ingest is waiting for.
// PRAGMA wal_checkpoint(PASSIVE) would report the page count and do the work as
// a side effect, which is the policy §7.4 removed.
func walSize(path string) (int64, error) {
	fi, err := os.Stat(path + "-wal")
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: measure the WAL beside %s: %w", path, err)
	}
	return fi.Size(), nil
}

// Checkpointer keeps the WAL bounded while the service runs. Spec 5.4 gives it
// two triggers and they are independent: a size threshold and a timer.
//
// Every field is required; there are no defaults, because the values are spec
// 5.4's and belong with the rest of the run loop's numbers rather than here.
//
// # What it adds to what SQLite already does
//
// Not "the WAL would otherwise grow without bound". Spec 5.4's DSN leaves
// wal_autocheckpoint at SQLite's default of 1,000 pages, so SQLite
// PASSIVE-checkpoints on its own and the WAL settles at about 4.1 MiB whatever
// the run length - measured over 800 ingests, and the size the live
// installation's WAL had reached. What a PASSIVE checkpoint does not do is give
// the file back: it moves pages into the database and reuses the WAL in place,
// so the high-water mark stands until something truncates. TRUNCATE is what
// reclaims it, and a small WAL is also less for the next start to recover after
// a hard kill.
//
// # Why it does not need a batch
//
// It competes with live ingest and the drain for the single connection (spec
// 5.4), so the same rule applies as to internal/spool's Drainer: do not hold
// the connection for an unbounded stretch. The drain's answer is a bounded batch
// and a pause, because it has an unbounded number of records to replay. A
// checkpoint is one statement and cannot be split, so the equivalent bound is
// the WAL itself: Threshold caps how much work any single TRUNCATE can have to
// do, and §7.4's 0.54 ms per MiB turns that cap into a time. Nothing here adds
// a second throttle on top of it.
type Checkpointer struct {
	// DB is the pool [Open] returned.
	DB *sql.DB

	// Path is the database path, the same one Open was given. The WAL is
	// Path with "-wal" appended.
	Path string

	// Threshold is the WAL size in bytes at or above which the next poll
	// checkpoints.
	Threshold int64

	// Interval is how long the WAL may go unchecked while it stays under
	// Threshold.
	Interval time.Duration

	// Poll is how often the WAL's size is measured. It bounds how far past
	// Threshold the WAL can get before anything notices, so it has to be a
	// good deal shorter than Interval or the threshold trigger never fires
	// first.
	Poll time.Duration
}

// Run checkpoints until ctx is cancelled.
//
// It does not checkpoint on entry. The service starts this alongside the drain
// and the accept loop, and a checkpoint there would be doing the timer's first
// pass early - which also makes the two triggers indistinguishable, since a
// test could no longer tell an initial checkpoint from a timer that fired.
//
// A failed checkpoint is logged and the loop continues: the WAL is still valid
// and the next poll tries again. It logs through [slog.Default], which the
// service points at spec 5.6's file behind I-10's filter before anything else
// runs; nothing but the service links this package.
func (c *Checkpointer) Run(ctx context.Context) {
	t := time.NewTicker(c.Poll)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			size, err := walSize(c.Path)
			if err != nil {
				slog.Error("engramux-service: measure the WAL", "error", err)
				continue
			}
			over := size >= c.Threshold
			if !over && time.Since(last) < c.Interval {
				continue
			}
			// Set before the attempt, not after it: a checkpoint that
			// keeps failing on the timer path must not turn into a
			// retry every Poll.
			last = time.Now()
			if over {
				// The timer path is silent - at spec 5.4's
				// interval it is a line every few minutes into a
				// log that does not rotate. Crossing the
				// threshold is the event worth a record: it means
				// the WAL filled faster than the timer.
				slog.Info("engramux-service: the WAL reached the checkpoint threshold",
					"wal_bytes", size, "threshold_bytes", c.Threshold)
			}
			if err := Checkpoint(ctx, c.DB); err != nil && ctx.Err() == nil {
				slog.Error("engramux-service: checkpoint the WAL", "error", err, "wal_bytes", size)
			}
		}
	}
}
