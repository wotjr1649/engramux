package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/spool"
	"github.com/wotjr1649/engramux/internal/store"
)

// These tests run the real run loop in this process, which is the only way to
// assert what its *own* shutdown released: a service that leaks the pool or the
// listener and is then killed looks identical to one that cleaned up, because
// Windows closes every handle a dead process held (docs/evidence/crash). The
// process-level gates live in cmd/engramux-service.
//
// The pipe name is fixed (spec 5.2), so these cannot run beside a development
// service or beside each other - the same constraint every pipe test in this
// repository has, and the reason for -p 1.

// running starts Run in dir on its own goroutine and returns a function that
// stops it and hands back Run's error.
//
// Run installs a default logger, so the previous one is put back afterwards:
// otherwise every test that runs after this one logs into a temp directory
// that has been deleted.
func running(t *testing.T, dir string) (stop func() error) {
	t.Helper()
	requirePipeFree(t)

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

func requirePipeFree(t *testing.T) {
	t.Helper()
	if dialOK(t) {
		t.Fatal("something is already listening on the service's pipe - stop the development engramux service and re-run with -p 1")
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

	requirePipeFree(t)
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
