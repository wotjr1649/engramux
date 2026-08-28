// Package service is the run loop of engramux-service: the one persistent
// process per Windows user (I-01), and the only process that ever opens the
// database (I-07).
//
// Everything it composes was built and tested a piece at a time. This package
// is where the pieces become a program, and the only decisions it makes are the
// ones that only exist once they are assembled: what order to start in, where
// the log goes, and what a Status request answers.
//
// # The startup order is load-bearing
//
// Listen, then open the database, then serve.
//
//   - [pipe.ListenCurrent] first, because I-09 makes the pipe the singleton. A
//     second instance then fails with an Access-denied naming the pipe, which
//     says "one is already running". Opening the database first would fail it
//     with "database is locked" instead - true, and a confusing way to say the
//     same thing (spec 5.4's exclusive lock is what produces it).
//   - [store.Open] and [store.Migrate] next. The exclusive lock cannot be held
//     by a dead predecessor - measured 20/20 in docs/evidence/crash - so a
//     failure here is a real failure and not a stale one.
//   - [pipe.Serve] last, so no connection is accepted before the schema exists.
//
// # The log is a file
//
// Under Task Scheduler there is no console and stderr goes nowhere (spec 5.5),
// so the only durable record of what this process did is a file under spec
// 5.6's logs/. It is installed before anything else can log, wrapped in
// [secret.NewLogHandler]: I-10's egress half is in force in no binary until
// something calls slog.SetDefault, and this is that call.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/pipe"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/spool"
	"github.com/wotjr1649/engramux/internal/store"
)

// The names spec 5.6 gives the service's files, all of them under [Dir].
const (
	dbName   = "engramux.db"
	spoolDir = "spool"
	logsDir  = "logs"
	logName  = "engramux-service.log"
)

// drainInterval is how often the spool is replayed while the service is up.
//
// It is a judgement, not a measurement, and the reason it can be one is that
// the spool exists for the time the service is *down*: a relay that reaches a
// running service does not spool at all (spec 5.3), so records appear here only
// after a restart, an upgrade gap (spec 5.5), or a relay that hit its budget
// against a busy service. Half a minute of extra latency on an event that has
// already missed its live delivery is not a cost anything measures, and a pass
// over an empty directory is one os.ReadDir.
//
// A var so a test can shrink it; nothing else writes to it.
var drainInterval = 30 * time.Second

// Dir is Engramux's directory: "engramux" under the user's local application
// data directory (spec 5.6). os.UserCacheDir returns %LocalAppData% on Windows,
// which is the same derivation [spool.Dir] makes - a test pins the two against
// each other, because a service draining a different directory from the one the
// relay writes to would lose every spooled event without failing anything.
func Dir() (string, error) {
	local, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("service: locate the local application data directory: %w", err)
	}
	return filepath.Join(local, "engramux"), nil
}

// Run starts the service in dir and blocks until ctx is cancelled or something
// fails.
//
// A clean shutdown returns nil, and by the time it does the accept loop has
// finished, every connection goroutine with it, the drain has stopped, and the
// database is closed - which is what releases the exclusive lock (I-07) so that
// the next start can take it.
func Run(ctx context.Context, dir string) error {
	closeLog, err := installLogger(dir)
	if err != nil {
		return err
	}
	defer closeLog()

	err = run(ctx, dir)
	if err != nil {
		// Logged here rather than left to the caller: under Task
		// Scheduler the caller's stderr goes nowhere, and a service that
		// failed to start with no record of why is the failure mode this
		// file is written to avoid.
		slog.Error("engramux-service: stopped", "error", err)
	}
	return err
}

// installLogger points the default logger at spec 5.6's log file, through
// I-10's egress filter, and returns the function that closes it.
//
// It is the first thing [Run] does, because a handler installed second cannot
// filter what was logged first.
//
// The file is written unbuffered. A bufio.Writer would lose the tail of the log
// exactly when it matters most - the process is stopped by TerminateProcess,
// which is how Task Scheduler ends a task and how a crash ends anything, and
// what was in the buffer at that moment is the part that says why.
//
// ponytail: one file, appended to forever. Spec 5.6 says "rotating logs" and
// this does not rotate, so the ceiling is that a long-lived service grows one
// file without bound. The upgrade path is a size check at startup and a rename,
// which needs a retention number nobody has picked; nothing in Phase 1 depends
// on it.
func installLogger(dir string) (func(), error) {
	path := filepath.Join(dir, logsDir, logName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("service: create %s: %w", filepath.Dir(path), err)
	}
	//nolint:gosec // G304: path is dir joined with two constants of this
	// package, and dir comes from os.UserCacheDir. No part of it is input.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("service: open %s: %w", path, err)
	}
	slog.SetDefault(slog.New(secret.NewLogHandler(slog.NewJSONHandler(f, nil))))
	return func() { _ = f.Close() }, nil
}

// run is [Run] with the logger already installed, so that everything below can
// report through it - including its own failures.
func run(ctx context.Context, dir string) error {
	l, err := pipe.ListenCurrent()
	if err != nil {
		// The error names the pipe: winio wraps ERROR_ACCESS_DENIED in
		// an *os.PathError carrying the path. That is the whole reason
		// this call comes first (I-09).
		return err
	}
	defer func() { _ = l.Close() }()

	dbPath := filepath.Join(dir, dbName)
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	// Deferred, and reached only after the accept loop and the drain have
	// both stopped below - both of them use this pool.
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("engramux-service: close the database", "error", err)
		}
	}()

	if err := store.Migrate(ctx, db); err != nil {
		return err
	}

	spoolPath := filepath.Join(dir, spoolDir)
	started := time.Now()
	slog.Info("engramux-service: serving",
		"pipe", l.Addr().String(), "database", dbPath, "spool", spoolPath)

	// The drain shares the shutdown but gets its own cancel, so that the
	// accept loop returning for any reason stops it too.
	drainCtx, stopDrain := context.WithCancel(ctx)
	defer stopDrain()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		drain(drainCtx, &spool.Drainer{
			Dir: spoolPath,
			Log: slog.Default(),
			Ingest: func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
				return store.Ingest(ctx, db, env, store.SourceSpool, time.Now())
			},
		})
	}()

	serveErr := pipe.Serve(ctx, l, pipe.Handler{
		// The seam internal/pipe exists for: ipc cannot import store, so
		// the database reaches the accept loop as a closure and nothing
		// else (spec 5.4's one connection is what this is closing over).
		Ingest: func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
			return store.Ingest(ctx, db, env, store.SourcePipe, time.Now())
		},
		Status: func(ctx context.Context) (ipc.StatusReply, error) {
			return status(ctx, db, dbPath, spoolPath, started)
		},
	})

	// Serve has returned, so no handler is using the pool any more. Stop the
	// drain and wait for it before the deferred Close runs, or the last
	// replay would be handed a closed database.
	stopDrain()
	wg.Wait()

	// Serve always returns an error, so "was this the shutdown we asked for"
	// is the caller's question and this is where it is answered: the context
	// was cancelled and the listener closed underneath it.
	if ctx.Err() != nil && errors.Is(serveErr, net.ErrClosed) {
		slog.Info("engramux-service: stopped")
		return nil
	}
	return serveErr
}

// drain replays the spool now and then every drainInterval, until ctx is
// cancelled.
//
// Now first, and that is the half that matters: every event captured while the
// service was down is sitting in the spool the moment it starts, and waiting
// out an interval before looking is latency for nothing.
//
// It competes with live ingest for the single connection (spec 5.4), which
// [spool.Drainer] already accounts for with its own batch and pause. Nothing
// here adds a second throttle on top of that one.
func drain(ctx context.Context, d *spool.Drainer) {
	t := time.NewTicker(drainInterval)
	defer t.Stop()
	for {
		n, err := d.Drain(ctx)
		if n > 0 {
			slog.Info("engramux-service: replayed spooled events", "records", n)
		}
		// A cancelled drain is the shutdown, not a failure. Every other
		// error is one pass's problem: the records it did not reach are
		// still on disk and the next pass tries again.
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("engramux-service: drain the spool", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// status answers a Status request (spec 5.2, I-08).
//
// Every number is read when it is asked for rather than kept as a counter. A
// counter would drift from what the database and the directory actually hold -
// exactly the failure a status command exists to rule out - and the cost is one
// COUNT(*) and one os.ReadDir per request, on a request type a human types.
//
// A failure to read either one returns an error, and internal/pipe turns that
// into a rejected reply. Half a status is worse than none: a caller cannot tell
// a zero that was read from a zero that was never filled in.
func status(ctx context.Context, db *sql.DB, dbPath, spoolPath string, started time.Time) (ipc.StatusReply, error) {
	var events int64
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events`).Scan(&events); err != nil {
		return ipc.StatusReply{}, fmt.Errorf("service: count events: %w", err)
	}
	byCell, err := cells(ctx, db)
	if err != nil {
		return ipc.StatusReply{}, err
	}
	depth, err := spool.Depth(spoolPath)
	if err != nil {
		return ipc.StatusReply{}, err
	}
	return ipc.StatusReply{
		SpoolDepth:   depth,
		Events:       events,
		Cells:        byCell,
		UptimeMS:     time.Since(started).Milliseconds(),
		DatabasePath: dbPath,
	}, nil
}

// cells is the per-cell breakdown [ipc.Cell] documents: one row per distinct
// (host, event_name) pair in the events table, with the count and the span of
// received_at.
//
// It is one aggregate query and no join. The counts have to come from events
// and not from sessions: a session row is one per session and would count how
// many sessions touched a cell rather than how many events landed in it, which
// is a different number that looks like the right one.
//
// The ORDER BY is not decoration - it is what makes the CLI's table stable
// between two runs over an unchanged database, so a reader can diff them.
// SQLite's default collation is BINARY, so `unknown` sorts last of the three
// hosts by construction rather than by luck.
//
// A cell with no events produces no row, because that is what GROUP BY does.
// [ipc.Cell] says why that is the answer rather than a zero-filled grid.
//
// ponytail: the reply grows with the number of distinct cells, and nothing
// caps it. The ceiling is ipc.MaxFrameLen, at which point WriteFrame refuses
// and the CLI reports a failed read rather than a short answer. Real traffic
// is spec 4.1's 11 event names across two hosts; the upgrade path is a LIMIT
// and a truncation flag, which needs a number nothing has needed yet.
func cells(ctx context.Context, db *sql.DB) ([]ipc.Cell, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT host, event_name, count(*), min(received_at), max(received_at)
		FROM events
		GROUP BY host, event_name
		ORDER BY host, event_name`)
	if err != nil {
		return nil, fmt.Errorf("service: group events by cell: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ipc.Cell
	for rows.Next() {
		var c ipc.Cell
		if err := rows.Scan(&c.Host, &c.EventName, &c.Count, &c.FirstSeenMS, &c.LastSeenMS); err != nil {
			return nil, fmt.Errorf("service: scan a cell: %w", err)
		}
		out = append(out, c)
	}
	// Checked rather than assumed: rows.Next returns false both for "that
	// was the last row" and for "the read failed", and without this a
	// truncated breakdown would be served as a complete one.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("service: read the cell breakdown: %w", err)
	}
	return out, nil
}
