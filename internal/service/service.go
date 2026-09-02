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
	"github.com/wotjr1649/engramux/internal/mcpserver"
	"github.com/wotjr1649/engramux/internal/pipe"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/search"
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

// Spec 5.4's checkpoint policy, as the three numbers [store.Checkpointer]
// takes. Vars for the same reason drainInterval is one.
//
// walThreshold is the spec's, and it is the same 64 MiB as the DSN's
// journal_size_limit. §7.4 measured a cold TRUNCATE there at 32.5 ms, which is
// the bound on how long one checkpoint can hold the single connection.
//
// checkpointInterval and checkpointPoll are judgements the spec leaves open,
// read against §7.2's 12,439 B of WAL per event and the busiest rate that was
// actually measured, 2 events/s across eight sessions.
//
// One thing has to be said about that growth rate before the numbers below make
// sense: §7.2 measured it with wal_autocheckpoint(0), and this DSN does not
// turn wal_autocheckpoint off. Left at SQLite's default of 1,000 pages, it
// PASSIVE-checkpoints on its own at about 4.1 MB - 1,000 pages of 4 KiB is
// 4,096,000 B, which is 3.9 MiB - and the WAL settles there
// rather than growing - measured, and it is what the live installation's WAL
// was doing. So the threshold below is not what stops the WAL running away;
// SQLite already does. What these two numbers buy is the file being *given
// back*, which only TRUNCATE does, and a bound on what a crash leaves to
// recover.
//
//   - Five minutes is 7.5 MiB of writes at that busiest rate, so a checkpoint
//     costs about 4 ms (§7.4's 0.54 ms/MiB) and reclaims roughly the 4.1 MB
//     the automatic checkpoint would otherwise leave allocated. 288 a day.
//   - Five seconds is how far past the threshold the WAL can get before
//     anything notices: 124 KiB at that same rate, 0.2% of the threshold. One
//     os.Stat, and deliberately far shorter than the interval - if it were not,
//     the timer would always fire first and the threshold would never do
//     anything.
var (
	walThreshold       = int64(64 << 20)
	checkpointInterval = 5 * time.Minute
	checkpointPoll     = 5 * time.Second
)

// shutdownCheckpointTimeout bounds the checkpoint on the way out. §7.4 measured
// 140.5 ms at 258 MiB of WAL, against a WAL the loop above keeps at 64 MiB, so
// this is not a budget - it is a limit on how long a wedged checkpoint may hold
// up the process exit.
const shutdownCheckpointTimeout = 5 * time.Second

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

	// The drain and the checkpointer share the shutdown but get their own
	// cancel, so that the accept loop returning for any reason stops them
	// too. Both hold the single connection (spec 5.4), so both have to be
	// stopped and waited for before the deferred Close above runs.
	bgCtx, stopBackground := context.WithCancel(ctx)
	defer stopBackground()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c := &store.Checkpointer{
			DB:        db,
			Path:      dbPath,
			Threshold: walThreshold,
			Interval:  checkpointInterval,
			Poll:      checkpointPoll,
		}
		c.Run(bgCtx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		drain(bgCtx, &spool.Drainer{
			Dir: spoolPath,
			Log: slog.Default(),
			Ingest: func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
				return store.Ingest(ctx, db, env, store.SourceSpool, time.Now())
			},
		})
	}()

	// One Handler for both surfaces. The MCP tools call these closures
	// directly rather than dialing the pipe, so a tool call takes the same
	// read gate a CLI read takes - see internal/mcpserver for why that is
	// the point and not a shortcut.
	h := handlers(db, dbPath, spoolPath, started, newReadGate())
	serveMCP(bgCtx, &wg, dir, h)

	serveErr := pipe.Serve(ctx, l, h)

	// Serve has returned, so no handler is using the pool any more. Stop the
	// drain and the checkpointer and wait for them before the deferred Close
	// runs, or the last replay would be handed a closed database.
	stopBackground()
	wg.Wait()
	checkpointOnTheWayOut(ctx, db)

	// Serve always returns an error, so "was this the shutdown we asked for"
	// is the caller's question and this is where it is answered: the context
	// was cancelled and the listener closed underneath it.
	if ctx.Err() != nil && errors.Is(serveErr, net.ErrClosed) {
		slog.Info("engramux-service: stopped")
		return nil
	}
	return serveErr
}

// handlers is everything internal/pipe answers a request with, wired to one
// database and one read gate.
//
// It is a function rather than a literal inside [run] so that a test can hold
// the *production* wiring rather than a copy of it. The order reads and ingest
// take on the single connection is a property of these five closures and of
// nothing else, so a test that built its own would measure its own wiring. The
// gate is a parameter for the same reason: a test needs to look at the one these
// closures use, and [run] is the only caller that mints one.
//
// # Reads are gated and bounded; ingest is neither
//
// Every read goes through [boundedRead]: a query deadline, then a gate that
// allows one read at a time and lets a pending ingest go first. Ingest only
// marks itself pending and runs - it waits for the connection and for nothing
// here. I-04 is why: a captured event is never silently dropped, and a relay
// that blows spec 5.3's 800 ms post-dial budget spools and costs latency, while
// a read that waits costs nobody anything.
//
// The seam internal/pipe exists for is the same as it was: ipc cannot import
// store, so the database reaches the accept loop as a closure and nothing else
// (spec 5.4's one connection is what these close over).
func handlers(db *sql.DB, dbPath, spoolPath string, started time.Time, gate *readGate) pipe.Handler {
	return pipe.Handler{
		Ingest: func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
			gate.enterIngest()
			defer gate.leaveIngest()
			return store.Ingest(ctx, db, env, store.SourcePipe, time.Now())
		},
		Status: func(ctx context.Context) (ipc.StatusReply, error) {
			return boundedRead(ctx, gate, func(ctx context.Context) (ipc.StatusReply, error) {
				return status(ctx, db, dbPath, spoolPath, started)
			})
		},
		Doctor: func(ctx context.Context) (ipc.DoctorReply, error) {
			return boundedRead(ctx, gate, func(ctx context.Context) (ipc.DoctorReply, error) {
				return doctorReport(ctx, db, dbPath, spoolPath, started)
			})
		},
		Search: func(ctx context.Context, req ipc.SearchRequest) (ipc.SearchReply, error) {
			return boundedRead(ctx, gate, func(ctx context.Context) (ipc.SearchReply, error) {
				return searchEvents(ctx, db, req)
			})
		},
		GetEvent: func(ctx context.Context, req ipc.GetEventRequest) (ipc.GetEventReply, error) {
			return boundedRead(ctx, gate, func(ctx context.Context) (ipc.GetEventReply, error) {
				return getEvent(ctx, db, req)
			})
		},
		ListSessions: func(ctx context.Context, req ipc.ListSessionsRequest) (ipc.ListSessionsReply, error) {
			return boundedRead(ctx, gate, func(ctx context.Context) (ipc.ListSessionsReply, error) {
				return listSessions(ctx, db, req)
			})
		},
	}
}

// serveMCP starts spec 5.9's MCP endpoint on wg, and does not stop the service
// when it cannot start.
//
// # A failure here is logged, not returned
//
// I-04 is why this product exists: the thing that must keep working is capture.
// The MCP endpoint is a reader, and every failure it can have on the way up -
// no port free on the loopback interface, a data directory that will not take
// mcp.json - leaves ingest, the pipe and the drain entirely able to run. A
// service that refused to start over a reader would turn a port conflict into
// lost events.
//
// What makes the failure visible rather than silent is `doctor`: with no
// mcp.json, or one naming a URL nothing answers, it reports the endpoint as
// unreachable. The log line here is the other half.
//
// It takes bgCtx rather than ctx, so the shutdown order holds: the endpoint
// stops when the accept loop does, and [Server.Serve] does not return until its
// in-flight tool calls have finished - which has to be before the deferred
// db.Close, because those calls are reading through it.
func serveMCP(ctx context.Context, wg *sync.WaitGroup, dir string, h pipe.Handler) {
	m, err := mcpserver.Listen(ctx, dir, h)
	if err != nil {
		slog.Error("engramux-service: the MCP endpoint did not start", "error", err)
		return
	}
	// The endpoint, never the token (spec 6.1). mcp.json holds both and is
	// the only place the token is.
	slog.Info("engramux-service: MCP serving", "endpoint", m.Endpoint())

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := m.Serve(ctx); err != nil {
			slog.Error("engramux-service: the MCP endpoint stopped", "error", err)
		}
	}()
}

// checkpointOnTheWayOut is spec 5.4's checkpoint on shutdown. It runs after the
// accept loop, the drain and the checkpoint loop have all stopped, so it is the
// last thing to touch the connection before the deferred Close.
//
// It gets a context of its own because ctx is already cancelled by the time it
// is called - that is what started the shutdown - and the driver would refuse
// the statement without running it.
//
// What it is worth, measured rather than assumed: nothing, on this path. A
// clean db.Close checkpoints the WAL and deletes it anyway, and
// TestTheRunLoopCheckpointsWhileItServes measures the same on-disk state either
// way. Deleting this line changes no observable outcome as long as Close is
// reached. It is here because it is the only part of the shutdown state this
// file states rather than inherits from the driver, and because there is one
// path where it is not redundant: a Close that fails.
//
// It is emphatically not what keeps the -shm away. The DSN is, by setting
// locking_mode before the first WAL access - see store's package documentation
// and spec 5.4. Checkpointing never had anything to do with it, on any
// schedule.
func checkpointOnTheWayOut(ctx context.Context, db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownCheckpointTimeout)
	defer cancel()
	if err := store.Checkpoint(ctx, db); err != nil {
		slog.Error("engramux-service: checkpoint the WAL on the way out", "error", err)
	}
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
//
// # There is one status shape and it is masked (spec 5.9)
//
// dbPath is under the user's local application data directory (spec 5.6), so it
// carries a Windows user name. internal/ipc used to justify sending it verbatim
// on three grounds - one SID inside the trust boundary, the pipe's DACL admitting
// only that SID, and the CLI printing it on the same machine. All three are void
// when the reader is a model, which may repeat what it read into a transcript
// that leaves the machine.
//
// The CLI sees the masked path too, rather than there being a second shape for
// it. Two shapes would mean the field that reaches MCP is the one nobody looks
// at, which is how a masked path quietly becomes an unmasked one. `doctor` is
// where the real path belongs and is where it is: a local diagnostic, printed to
// the terminal of the SID that owns the file.
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
		DatabasePath: secret.MaskString(dbPath),
	}, nil
}

// searchEvents answers a Search request (spec 5.2, I-08) and is the second
// place I-10 has to hold.
//
// This process is the only one that can read the database (I-07), which is why
// a search travels the pipe at all - and it is also why the masking belongs
// here rather than in the CLI. What crosses this boundary is what left the
// trust the service holds: internal/search cuts every excerpt from a payload
// internal/secret has masked whole, and the stored row is never on the wire.
//
// The bound on the limit is applied before internal/search sees it, because a
// negative one means "no limit" to SQLite and an unbounded result set does not
// fit a frame (see [ipc.SearchRequest.EffectiveLimit]).
//
// The hits are copied field by field rather than shared. internal/search's Hit
// is a reader's type and ipc.SearchHit is the wire's, and this function is the
// seam between them - the same seam the pipe's Handler is for the database.
//
// # EventName is masked here, and the order is masking before the bound
//
// The excerpt arrives already masked - internal/search cuts it from a payload
// internal/secret masked whole - and events.event_name did not, which is the
// I-10 perimeter spec 5.7 recorded and spec 5.9 closes. It is whatever a
// payload's hook_event_name said, so a captured path lands in it as readily as
// `PostToolUse` does.
//
// Masking runs first because the bound is about what fits a frame and masking is
// about what may leave the machine, and a bound applied to unmasked text is a
// bound on a value that should not have existed. The remaining interaction is
// harmless in both directions: masking expands, so a name near the bound can be
// cut mid-placeholder, and internal/secret decides a placeholder on its prefix.
func searchEvents(ctx context.Context, db *sql.DB, req ipc.SearchRequest) (ipc.SearchReply, error) {
	limit, err := req.EffectiveLimit()
	if err != nil {
		return ipc.SearchReply{}, err
	}
	// An empty project is every project (spec 5.9), so it never reaches
	// project.FromArgument - which refuses "" as not absolute, and is right
	// to for the two request types where a project is required.
	var projectID string
	if req.Project != "" {
		p, err := project.FromArgument(req.Project)
		if err != nil {
			return ipc.SearchReply{}, err
		}
		projectID = p.ID
	}
	hits, total, err := search.Search(ctx, db, req.Query, projectID, limit)
	if err != nil {
		return ipc.SearchReply{}, err
	}
	out := make([]ipc.SearchHit, len(hits))
	for i, h := range hits {
		name, cut := truncateRunes(secret.MaskString(h.EventName), maxEventNameRunes)
		out[i] = ipc.SearchHit{
			// The same untrusted column [getEvent] masks, reaching the
			// same wire by a different reply (backlog 29). A real id is
			// unchanged by it, so the id a model hands back to get_event
			// is still the one that was stored.
			ID:                 secret.MaskString(h.ID),
			Host:               h.Host,
			EventName:          name,
			EventNameTruncated: cut,
			ReceivedAtMS:       h.ReceivedAtMS,
			Excerpt:            h.Excerpt,
		}
	}
	return ipc.SearchReply{Hits: out, Total: total}, nil
}

// maxEventNameRunes bounds events.event_name on the way onto the wire.
//
// It is the one field of a hit with no bound anywhere else: the column has no
// CHECK, and internal/store takes hook_event_name from the payload verbatim, so
// one event with a multi-megabyte name would make every search that matched it
// fail at ipc.WriteFrame - which the CLI can only report as a failed read - and
// would put the same megabytes in an MCP response, which has no frame to refuse
// it (see [cells]). A shortened name is a worse answer than the real one and a
// much better one than no answer at all, and since backlog 17 the reply says
// which it is.
//
// 256 is a bound on the reply and not on what one client prints, which is what
// backlog 16 asked for: the old value was the CLI's display width. Measured
// over the 902 captures, the longest name either host has ever emitted is 17
// runes (`PermissionRequest`), and both hosts draw their names from a fixed
// list - so at fifteen times that, a real name is never cut and this exists
// only for a payload that lies. At 4 bytes a rune worst case, 256 runes is 1
// KiB per hit and 100 KiB across ipc.MaxSearchLimit hits, a fortieth of the 4
// MiB frame and small beside the excerpts. `engramux search` and `event` cut
// again for the terminal, to 64, with their own mark.
const maxEventNameRunes = 256

// truncateRunes cuts s to at most n runes and says whether it did. Runes and
// not bytes, so the cut cannot land inside one and produce U+FFFD.
func truncateRunes(s string, n int) (string, bool) {
	r := []rune(s)
	if len(r) <= n {
		return s, false
	}
	return string(r[:n]), true
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
// caps it. Over the pipe the ceiling is ipc.MaxFrameLen, at which point
// WriteFrame refuses and the CLI reports a failed read rather than a short
// answer. **Over MCP there is no ceiling at all** - that surface marshals
// straight to an HTTP response and never touches WriteFrame - so the two
// surfaces do not fail alike, and this is the one place that says so. Real
// traffic is spec 4.1's 11 event names across two hosts; the upgrade path is a
// LIMIT and a truncation flag, which needs a number nothing has measured, and
// an unmeasured cap is what AGENTS.md forbids.
func cells(ctx context.Context, db *sql.DB) ([]ipc.Cell, error) {
	rows, err := db.QueryContext(ctx, store.CellsQuery)
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
		// The same untrusted column [searchEvents] masks, reaching the
		// same wire by a different reply. Masking one and not the other
		// would close the perimeter on the surface somebody happened to
		// look at.
		//
		// It is masked after the GROUP BY rather than before it, so two
		// names that mask to one placeholder stay two cells. Grouping on
		// the masked value would silently merge them and report a count
		// that is not any cell's.
		c.EventName = secret.MaskString(c.EventName)
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
