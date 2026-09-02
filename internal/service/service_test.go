package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Microsoft/go-winio"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/ipc/ipctest"
	"github.com/wotjr1649/engramux/internal/spool"
	"github.com/wotjr1649/engramux/internal/store"
)

// These tests run the real run loop in this process, which is the only way to
// assert what its *own* shutdown released: a service that leaks the pool or the
// listener and is then killed looks identical to one that cleaned up, because
// Windows closes every handle a dead process held (docs/evidence/crash). The
// process-level gates live in cmd/engramux-service.
//
// The pipe name is fixed (spec 5.2), and these tests move it with
// ipc.TestPipeSIDEnv so that a development service holding the real one is not
// in the way. They still cannot run beside each other: the override is a
// process-wide environment variable, which is why the value carries the test's
// name and the process id.

// running starts Run in dir on its own goroutine and returns a function that
// stops it and hands back Run's error.
//
// Run installs a default logger, so the previous one is put back afterwards:
// otherwise every test that runs after this one logs into a temp directory
// that has been deleted.
func running(t *testing.T, dir string) (stop func() error) {
	t.Helper()
	claimAFreePipeName(t)

	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, dir) }()

	// Wait for a served request rather than for a duration, and rather than
	// for a dial: pipe.Listen creates the pipe instance before Serve is ever
	// called, so a dial succeeds the moment ListenCurrent returns - while
	// store.Open is still running, or wedged. A Status reply is the first
	// thing that cannot be true until the whole run loop is up, and waiting
	// for the wrong one turned a startup that never finished into a shutdown
	// that appeared to hang.
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case err := <-done:
			cancel()
			t.Fatalf("Run returned before it served: %v", err)
		default:
		}
		if servingOK(t) {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("Run did not answer a Status request within 30s of being started")
		}
		time.Sleep(20 * time.Millisecond)
	}

	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(20 * time.Second):
			t.Fatal("Run did not return within 20s of the context being cancelled")
			return nil
		}
	}
}

// dialOK reports whether the pipe exists and accepts a connection. It says
// nothing about whether anything is serving on it - see [servingOK].
func dialOK(t *testing.T) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, pipeName(t))
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// servingOK reports whether a whole request completes: dial, send a Status
// frame, and get a reply that [ipc.StatusReply.Verify] accepts.
//
// Every failure is a "not yet" rather than a test failure, because this is a
// poll and the interesting failure is the caller's deadline.
func servingOK(t *testing.T) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, pipeName(t))
	if err != nil {
		return false
	}
	defer func() { _ = c.Close() }()

	if err := c.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return false
	}
	req, err := json.Marshal(ipc.Envelope{Version: ipc.Version, Type: ipc.Status})
	if err != nil {
		t.Fatalf("marshal the status request: %v", err)
	}
	if err := ipc.WriteFrame(c, req); err != nil {
		return false
	}
	raw, err := ipc.ReadFrame(c)
	if err != nil {
		return false
	}
	var reply ipc.StatusReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return false
	}
	return reply.Verify() == nil
}

func pipeName(t *testing.T) string {
	t.Helper()
	name, err := ipc.CurrentPipeName()
	if err != nil {
		t.Fatalf("ipc.CurrentPipeName: %v", err)
	}
	return name
}

// useTestPipeName moves Run's listener and this test's dials onto a name
// unique to the test and the process, so that a development service holding
// the real one is no longer in the way. It sets nothing else: the DACL is
// still the real user's.
//
// It is called from claimAFreePipeName, which is the single gate every test here
// that touches the pipe already goes through, and not from pipeName: pipeName
// is called again after the listener exists, and re-deriving there would put a
// second rule in the same place as the first.
func useTestPipeName(t *testing.T) {
	t.Helper()
	ipctest.Use(t)
}

// claimAFreePipeName claims the test's own pipe name and checks nothing answers
// on it yet. After the override it is no longer about the development
// service: what it catches is a listener an earlier test in this binary
// leaked, or two copies of this binary sharing a process id, neither of which
// any other assertion here would name.
func claimAFreePipeName(t *testing.T) {
	t.Helper()
	useTestPipeName(t)
	if dialOK(t) {
		t.Fatalf("something is already listening on %s - an earlier test leaked a listener", pipeName(t))
	}
}

// TestACleanShutdownReleasesWhatTheNextStartNeeds is gate clause 6.
//
// The assertion is a second Run over the same directory, because that is what
// an upgrade does (spec 5.5: drain, stop, replace, start) and because it names
// both failures precisely: a leaked listener refuses it with an access-denied
// on the pipe, and a leaked pool refuses it with "database is locked". Asserting
// only that the first Run returned nil would catch neither - a Run that never
// closed anything returns nil just as happily.
func TestACleanShutdownReleasesWhatTheNextStartNeeds(t *testing.T) {
	dir := t.TempDir()

	if err := running(t, dir)(); err != nil {
		t.Fatalf("the first run: %v", err)
	}
	if err := running(t, dir)(); err != nil {
		t.Fatalf("the second run over the same directory: %v\n"+
			"An access-denied names the listener, a locked database names the pool; either way the first run kept it.", err)
	}

	// And the exclusive lock is gone in the sense I-07 means it: another
	// process - this one - can now open the file.
	db, err := store.Open(t.Context(), filepath.Join(dir, dbName))
	if err != nil {
		t.Fatalf("open the database after a clean shutdown: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}

// TestAFailedMigrationStopsStartup. A service that serves a database it could
// not bring up to date is worse than one that refuses to start: every ingest is
// answered Rejected, the relay spools each one, the drain retries them until it
// quarantines them, and the only place any of it is visible is a log nobody is
// reading. The startup order exists so that this is a startup failure.
//
// The failure is arranged rather than simulated. A table the first migration
// creates is created by hand first, so goose's own CREATE TABLE is what fails -
// there is no seam here, and a test that injected one would be asserting
// against the seam.
func TestAFailedMigrationStopsStartup(t *testing.T) {
	dir := t.TempDir()
	seed, err := store.Open(t.Context(), filepath.Join(dir, dbName))
	if err != nil {
		t.Fatalf("seed the database: %v", err)
	}
	if _, err := seed.ExecContext(t.Context(), `CREATE TABLE projects (x INTEGER)`); err != nil {
		t.Fatalf("create the conflicting table: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close the seed: %v", err)
	}

	claimAFreePipeName(t)
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	// On its own goroutine, because the failure being ruled out is a Run
	// that carries on and serves - and that one never returns at all. A
	// synchronous call here would hang the whole suite instead of failing,
	// which is a worse way to be told the same thing.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, dir) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil against a database it could not migrate")
		}
		t.Logf("Run refused to start: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		<-done
		t.Fatal("Run is still running 10s after a migration it could not apply: it is serving a database with no schema")
	}

	// And it let go of the pipe on the way out, so the next start is not
	// refused by the corpse of this one.
	if dialOK(t) {
		t.Error("the listener outlived the failed startup")
	}
}

// TestTheDrainRunsAtStartupAndThenKeepsRunning covers both halves of the loop,
// and it has to cover them separately because one test cannot tell them apart
// by accident.
//
// The first record is written before the service starts, so only the immediate
// pass can take it. The second is written after that pass has provably finished
// - the first record is gone - so only a later tick can. A loop that drained
// once at startup and never again passes the first half and fails the second.
func TestTheDrainRunsAtStartupAndThenKeepsRunning(t *testing.T) {
	restore := drainInterval
	drainInterval = 50 * time.Millisecond
	t.Cleanup(func() { drainInterval = restore })

	dir := t.TempDir()
	spoolPath := filepath.Join(dir, spoolDir)
	const (
		atStartup = "0192f0c0-0000-7000-8000-00000000d001"
		later     = "0192f0c0-0000-7000-8000-00000000d002"
	)
	payload := []byte(`{"hook_event_name":"Stop","session_id":"s","cwd":"Z:\\drain\\project","model":"m"}`)

	if err := spool.Write(spoolPath, atStartup, payload, nil); err != nil {
		t.Fatalf("spool the first record: %v", err)
	}

	stop := running(t, dir)
	waitForEmptySpool(t, spoolPath, "the pass the run loop makes at startup")

	if err := spool.Write(spoolPath, later, payload, nil); err != nil {
		t.Fatalf("spool the second record: %v", err)
	}
	waitForEmptySpool(t, spoolPath, "a later tick of the drain loop")

	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	db, err := store.Open(t.Context(), filepath.Join(dir, dbName))
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	for _, id := range []string{atStartup, later} {
		requireStoredFromSpool(t, db, id)
	}
}

// waitForEmptySpool waits until the drain has consumed every record, and says
// which pass was supposed to do it when it has not.
func waitForEmptySpool(t *testing.T, dir, what string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		n, err := spool.Depth(dir)
		if err != nil {
			t.Fatalf("spool.Depth: %v", err)
		}
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still holds %d records after 20s: %s did not replay them", dir, n, what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// requireStoredFromSpool asserts the row exists and came in by the drain. The
// source column is what separates "the drain replayed it" from "something else
// stored it": both leave one row.
func requireStoredFromSpool(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	var source string
	if err := db.QueryRowContext(t.Context(), `SELECT source FROM events WHERE id = ?`, id).Scan(&source); err != nil {
		t.Fatalf("read event %s: %v", id, err)
	}
	if source != string(store.SourceSpool) {
		t.Errorf("event %s has source %q, want %q", id, source, store.SourceSpool)
	}
}

// TestTheServiceDrainsTheDirectoryTheRelayWritesTo pins the one thing two
// independent derivations of the same path can get wrong.
//
// [Dir] and [spool.Dir] both build on os.UserCacheDir, in two packages, for two
// binaries. If they ever disagree the relay spools into one directory and the
// service drains another: every event still lands in a file, nothing errors,
// nothing is logged, and the events are simply never stored. This is the
// assertion that would fail instead.
func TestTheServiceDrainsTheDirectoryTheRelayWritesTo(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	relaySpool, err := spool.Dir()
	if err != nil {
		t.Fatalf("spool.Dir: %v", err)
	}
	if want := filepath.Join(dir, spoolDir); relaySpool != want {
		t.Errorf("the relay spools into %q and the service drains %q", relaySpool, want)
	}
}

// TestTheCellBreakdownIsWhatTheDatabaseHolds is the per-cell half of a Status
// reply (spec 8's Phase 2 counts cells, and I-07 leaves the service as the only
// process that can).
//
// Every expected number is one this test wrote in by hand, with received_at
// values it chose, so nothing here is compared against another run of the query
// under test. The rows are inserted with SQL rather than through store.Ingest
// for the same reason: a seed that classified the host itself would be asserting
// that two code paths agree, not that the breakdown matches the table.
//
// It does not need the pipe at all - it calls [status] directly - so unlike
// everything above it in this file it needs no pipe name of its own either.
func TestTheCellBreakdownIsWhatTheDatabaseHolds(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))

	// Three of one cell with the smallest and largest received_at neither
	// first nor last in insertion order, so min/max cannot be an accident of
	// which row went in when.
	seedEvent(t, db, "e1", "claude-code", "PostToolUse", 1000)
	seedEvent(t, db, "e2", "claude-code", "PostToolUse", 3000)
	seedEvent(t, db, "e3", "claude-code", "PostToolUse", 2000)
	seedEvent(t, db, "e4", "codex", "SessionEnd", 5000)
	// host `unknown` is reachable and is not an error (I-04), and an event
	// whose payload carried no hook_event_name is stored with event_name ""
	// rather than dropped. Both are real rows and both must appear.
	seedEvent(t, db, "e5", "unknown", "", 7000)

	// A row in another table the breakdown must not count. Without it, a
	// query that grouped over the wrong table would still have to be caught
	// by the numbers alone.
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO observations (id, project_id, event_id, kind, body, created_at)
		 VALUES ('o1', 'p', 'e1', 'note', 'not an event', 0)`); err != nil {
		t.Fatalf("seed an observation: %v", err)
	}

	reply := statusOf(t, db, dir)

	want := []ipc.Cell{
		{Host: "claude-code", EventName: "PostToolUse", Count: 3, FirstSeenMS: 1000, LastSeenMS: 3000},
		{Host: "codex", EventName: "SessionEnd", Count: 1, FirstSeenMS: 5000, LastSeenMS: 5000},
		{Host: "unknown", EventName: "", Count: 1, FirstSeenMS: 7000, LastSeenMS: 7000},
	}
	if !slices.Equal(reply.Cells, want) {
		t.Fatalf("the breakdown is not what the table holds\n got %+v\nwant %+v", reply.Cells, want)
	}
	// The counts are a decomposition of the total, not a second opinion
	// about it. A query that grouped the wrong rows can match one and not
	// both.
	var sum int64
	for _, c := range reply.Cells {
		sum += c.Count
	}
	if sum != reply.Events {
		t.Errorf("the cells sum to %d and events = %d", sum, reply.Events)
	}

	// A cell nothing was captured for is absent, never a row whose count is
	// zero: ("codex", "PostToolUse") is exactly such a cell above, and the
	// equality already proved it is missing. This says the other half - that
	// no cell on the wire is a zero - which is what makes "absent" readable
	// as "zero" at the other end.
	for _, c := range reply.Cells {
		if c.Count == 0 {
			t.Errorf("cell %s/%q has count 0; an empty cell is absent, not zero", c.Host, c.EventName)
		}
	}
}

// TestTheCellBreakdownIsReadAtEveryRequest. The breakdown is a decomposition of
// a number the status command exists to be able to trust, so a cached one would
// go stale exactly when someone is checking whether a cell has been captured
// yet - which is the question spec 8's Phase 2 gates on.
//
// Two calls with a write in between, because one call cannot tell a value that
// was read from a value that was computed once and kept.
func TestTheCellBreakdownIsReadAtEveryRequest(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))

	seedEvent(t, db, "e1", "codex", "PreToolUse", 1000)
	before := statusOf(t, db, dir)
	if len(before.Cells) != 1 || before.Cells[0].Count != 1 {
		t.Fatalf("the first breakdown is already wrong: %+v", before.Cells)
	}

	seedEvent(t, db, "e2", "codex", "PreToolUse", 4000)
	seedEvent(t, db, "e3", "claude-code", "Stop", 9000)

	after := statusOf(t, db, dir)
	want := []ipc.Cell{
		{Host: "claude-code", EventName: "Stop", Count: 1, FirstSeenMS: 9000, LastSeenMS: 9000},
		{Host: "codex", EventName: "PreToolUse", Count: 2, FirstSeenMS: 1000, LastSeenMS: 4000},
	}
	if !slices.Equal(after.Cells, want) {
		t.Fatalf("the second breakdown did not see the two new rows\n got %+v\nwant %+v", after.Cells, want)
	}
}

// statusOf calls [status] the way the pipe handler does.
func statusOf(t *testing.T, db *sql.DB, dir string) ipc.StatusReply {
	t.Helper()
	reply, err := status(t.Context(), db, filepath.Join(dir, dbName), filepath.Join(dir, spoolDir), time.Now(), newHealth())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	return reply
}

// openMigrated opens a database at path with the schema applied, and closes it
// when the test ends - including its WAL sidecars, or t.TempDir cannot clean up.
func openMigrated(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatalf("migrate %s: %v", path, err)
	}
	return db
}

// seedEvent inserts one events row with the host, event name and received_at
// the caller chose, plus the project and session rows the foreign keys need
// (spec 5.4 turns foreign_keys on).
func seedEvent(t *testing.T, db *sql.DB, id, host, eventName string, receivedAt int64) {
	t.Helper()
	// Spec 6's session id: the host joined to the host session id.
	sessionID := host + ":s"
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	exec(`INSERT INTO projects (id, root, name, created_at)
	      VALUES ('p', 'Z:\cells', 'cells', 0) ON CONFLICT DO NOTHING`)
	exec(`INSERT INTO sessions (id, project_id, host, host_session_id, status, created_at)
	      VALUES (?, 'p', ?, 's', 'active', 0) ON CONFLICT DO NOTHING`, sessionID, host)
	exec(`INSERT INTO events (id, project_id, session_id, host, source, event_name,
	                          payload, privacy_class, redaction_version, received_at)
	      VALUES (?, 'p', ?, ?, 'pipe', ?, '{}', '', 1, ?)`,
		id, sessionID, host, eventName, receivedAt)
}

// TestTruncateRunesCutsOnRuneBoundaries pins the bound a search reply puts on
// events.event_name, which is the one hit field nothing else bounds.
//
// The over-length case is three-byte runes, so a cut made on a byte offset
// would land inside one: the assertion is the exact string and the exact rune
// count, not that something was shortened.
func TestTruncateRunesCutsOnRuneBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		n    int
		want string
		cut  bool
	}{
		{name: "over the bound is cut", in: "PostToolUse", n: 5, want: "PostT", cut: true},
		{name: "under the bound is untouched", in: "abc", n: 5, want: "abc"},
		{name: "exactly the bound is untouched", in: "abcde", n: 5, want: "abcde"},
		{name: "empty is empty", in: "", n: 5, want: ""},
		{name: "multibyte cuts on a rune", in: "가나다라마바", n: 3, want: "가나다", cut: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, cut := truncateRunes(tc.in, tc.n)
			if got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
			// The flag is backlog 17: a cut that left no mark made a
			// shortened name indistinguishable from a real one of
			// exactly the bound's length.
			if cut != tc.cut {
				t.Errorf("truncateRunes(%q, %d) reported cut = %v, want %v", tc.in, tc.n, cut, tc.cut)
			}
			if n := utf8.RuneCountInString(got); n > tc.n {
				t.Errorf("the result is %d runes, over the bound of %d", n, tc.n)
			}
		})
	}
}
