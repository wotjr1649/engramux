package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/ipc/ipctest"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
	"github.com/wotjr1649/engramux/internal/store"
)

// The two shipped binaries, built once. Every assertion in this file is made
// against a real process: the service holds the database exclusively (I-07),
// so a test that ran the run loop in-process would be asserting against a
// handle no other process could ever have, which is not the thing a user runs.
var (
	serviceBin string
	relayBin   string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "engramux-service-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}
	serviceBin = filepath.Join(dir, "engramux-service.exe")
	relayBin = filepath.Join(dir, "engramux.exe")

	// Built exactly the way AGENTS.md builds them, -H=windowsgui included.
	// A console-subsystem build of the service would pass every assertion
	// below while being the wrong binary (spec 5.1).
	if err := build(serviceBin, ".", "-ldflags", "-s -w -H=windowsgui"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err := build(relayBin, filepath.Join("..", "engramux"), "-ldflags", "-s -w"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// build compiles pkg to out with CGO_ENABLED=0, which is the boundary every
// shipped binary holds to.
func build(out, pkg string, extra ...string) error {
	args := append([]string{"build", "-o", out}, extra...)
	args = append(args, pkg)
	//nolint:gosec // G204: args is this function's own literals plus paths it built
	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go %v: %w\n%s", args, err, b)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Running the service
// ---------------------------------------------------------------------------

// running is one engramux-service process under test, and the directory it was
// pointed at.
type running struct {
	cmd    *exec.Cmd
	wait   chan error
	out    *bytes.Buffer
	local  string
	exited bool
}

// childEnv is the environment every child this package spawns gets: this
// process's, plus the three redirections that keep a test off the machine's real
// files.
//
// LOCALAPPDATA is the service's own directory, and the two host homes are the
// native memory indexer's (memory spec rev.2, M-2). The homes matter for a
// reason the other seam does not have: the memory directories belong to the two
// hosts and hold the user's private notes, so a service started without these
// reads them into a test database. Measured: a service started without them and
// killed half a second later had already written 8 rows of the machine's own
// memory. They point at subdirectories of the test's own that do not exist,
// which internal/memory reads as "this host has no memory here" - not an error
// and not a warning, because a machine with one host installed is ordinary.
//
// Every exec in this package goes through here, and
// TestAServiceWithNoHostHomesIndexesNothing is what makes a site that forgets
// visible.
func childEnv(local string, extra ...string) []string {
	env := append(os.Environ(),
		"LOCALAPPDATA="+local,
		"CLAUDE_CONFIG_DIR="+filepath.Join(local, "no-claude-home"),
		"CODEX_HOME="+filepath.Join(local, "no-codex-home"),
	)
	return append(env, extra...)
}

// start runs the service with a LOCALAPPDATA of its own and waits until it is
// answering on the pipe.
//
// LOCALAPPDATA is the whole seam and it costs production code nothing: the
// service's directory comes from os.UserCacheDir, which on Windows is
// %LocalAppData% and nothing else (spec 5.6). The same seam the relay's tests
// steer the spool with.
func start(t *testing.T, local string) *running {
	t.Helper()
	claimAFreePipeName(t)

	// One buffer for both streams, read only after the process has exited:
	// os/exec copies into it on its own goroutine, so reading it while the
	// process runs is a data race the detector would catch.
	var out bytes.Buffer
	cmd := exec.CommandContext(t.Context(), serviceBin)
	cmd.Env = childEnv(local)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the service: %v", err)
	}

	s := &running{cmd: cmd, wait: make(chan error, 1), out: &out, local: local}
	go func() { s.wait <- cmd.Wait() }()
	t.Cleanup(func() { s.stop(t) })

	s.waitUntilServing(t)
	return s
}

// waitUntilServing polls until the service answers a whole Status request, and
// fails as soon as the process is gone rather than waiting out the deadline on
// a service that is never coming up.
//
// A dial is not the check. pipe.Listen creates the pipe instance before Serve
// is ever called, so a dial succeeds the moment ListenCurrent returns - while
// store.Open is still running, or wedged on a lock - and a test that started
// from there would be asserting against a service that is not up yet.
func (s *running) waitUntilServing(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case err := <-s.wait:
			s.wait <- err
			s.exited = true
			t.Fatalf("the service exited before it served: %v\n%s", err, s.out.Bytes())
		default:
		}

		if servingOK(t) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the service did not answer a Status request on %s within 30s", pipeName(t))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// servingOK reports whether a whole request completes: dial, send a Status
// frame, and get back a reply ipc.StatusReply.Verify accepts. Every failure is
// a "not yet" rather than a test failure, because this is a poll and the
// interesting failure is the caller's deadline.
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
	if err := ipc.WriteFrame(c, request(t, ipc.Version, ipc.Status, "", nil)); err != nil {
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

// stop kills the service and waits for it. TerminateProcess rather than a
// graceful stop, because a GUI-subsystem process started by Task Scheduler is
// stopped exactly this way and the exclusive lock does not survive process
// death either way - measured 20/20 in docs/evidence/crash. The claim that a
// *clean* shutdown releases the lock is asserted in internal/service, where
// the run loop's own teardown is what runs.
//
// Safe to call twice: t.Cleanup calls it after a test that already did.
func (s *running) stop(t *testing.T) {
	t.Helper()
	if s.exited {
		return
	}
	s.exited = true
	// Kill answers Access-denied for a process that has already gone, and
	// one may well have: t.Context() is cancelled just before Cleanup runs,
	// which is exec.CommandContext's own signal to kill it. The claim this
	// function makes is that the process is not running any more, and the
	// wait below is what establishes that - so the kill error only matters
	// when the wait does not finish.
	killErr := s.cmd.Process.Kill()
	select {
	case <-s.wait:
	case <-time.After(20 * time.Second):
		t.Fatalf("the service did not exit within 20s of being killed (kill: %v)", killErr)
	}
	if s.out.Len() != 0 {
		t.Logf("service stdio: %s", s.out.Bytes())
	}
}

// logFile returns the single file under the service's logs/ directory (spec
// 5.6). It fails when there is not exactly one, because "which file did I
// read" is the first thing a failing egress assertion has to answer.
func (s *running) logFile(t *testing.T) []byte {
	t.Helper()
	dir := filepath.Join(s.local, "engramux", "logs")
	names, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	if len(names) != 1 {
		t.Fatalf("%s holds %q, want exactly one log file", dir, names)
	}
	b, err := os.ReadFile(names[0]) //nolint:gosec // G304: a path this test's own temp dir produced
	if err != nil {
		t.Fatalf("read %s: %v", names[0], err)
	}
	if len(b) == 0 {
		t.Fatalf("%s is empty, so nothing below is asserting anything", names[0])
	}
	return b
}

// openDB opens the service's database. It is only valid once the service has
// exited: it holds the file exclusively while it runs (I-07).
func (s *running) openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(s.local, "engramux", "engramux.db"))
	if err != nil {
		t.Fatalf("open the service's database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	return db
}

// ---------------------------------------------------------------------------
// Talking to it
// ---------------------------------------------------------------------------

func pipeName(t *testing.T) string {
	t.Helper()
	name, err := ipc.CurrentPipeName()
	if err != nil {
		t.Fatalf("ipc.CurrentPipeName: %v", err)
	}
	return name
}

// useTestPipeName moves the derived name (spec 5.2) onto one unique to this
// test and this process, so that the development service holding the real name
// is not in the way. Every child inherits it - all of them are launched with
// os.Environ() or with the parent's environment untouched - so the service, the
// relay and the CLI all meet on the same name.
//
// It is called from every helper that launches a process - start through
// claimAFreePipeName, plus relay and cli - and never from pipeName: pipeName is
// also called inside subtests, whose t.Name() is not the parent's, and a
// subtest that re-derived the name would dial a pipe the service its parent
// started never listened on.
func useTestPipeName(t *testing.T) {
	t.Helper()
	ipctest.Use(t)
}

// claimAFreePipeName claims this test's pipe name and fails with the one
// diagnosis that matters when nothing answers there yet is wrong: something
// else owns the name and nothing here can. Since the name is the test's own,
// that something is a listener an earlier test leaked or a second copy of this
// binary sharing a process id - not the development service.
func claimAFreePipeName(t *testing.T) {
	t.Helper()
	useTestPipeName(t)
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, pipeName(t))
	if err == nil {
		_ = c.Close()
		t.Fatalf("something is already listening on %s - an earlier test leaked a listener; re-run with -p 1", pipeName(t))
	}
}

// request builds a request frame's payload by concatenation rather than with
// json.Marshal, so that a test asserting the payload survived unchanged is not
// comparing one encoder's output with its own.
func request(t *testing.T, version string, typ ipc.RequestType, id string, payload []byte) []byte {
	t.Helper()
	enc := func(s string) []byte {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal %q: %v", s, err)
		}
		return b
	}
	var b bytes.Buffer
	b.WriteString(`{"version":`)
	b.Write(enc(version))
	b.WriteString(`,"type":`)
	b.Write(enc(string(typ)))
	b.WriteString(`,"ingest_id":`)
	b.Write(enc(id))
	if payload == nil {
		payload = []byte("null")
	}
	b.WriteString(`,"payload":`)
	b.Write(payload)
	b.WriteByte('}')
	return b.Bytes()
}

// send writes one request frame and returns the reply frame's payload.
func send(t *testing.T, frame []byte) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, pipeName(t))
	if err != nil {
		t.Fatalf("dial %s: %v", pipeName(t), err)
	}
	defer func() { _ = c.Close() }()

	if err := c.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if err := ipc.WriteFrame(c, frame); err != nil {
		t.Fatalf("write the request: %v", err)
	}
	reply, err := ipc.ReadFrame(c)
	if err != nil {
		t.Fatalf("read the reply: %v", err)
	}
	return reply
}

// ingest sends one well-formed IngestEvent and asserts the service ACKed it
// the way the relay requires - version, status and ingest id, all three (spec
// 5.3).
func ingest(t *testing.T, id string, payload []byte) {
	t.Helper()
	var ack ipc.Ack
	if err := json.Unmarshal(send(t, request(t, ipc.Version, ipc.IngestEvent, id, payload)), &ack); err != nil {
		t.Fatalf("decode the ack: %v", err)
	}
	if err := ack.Verify(id); err != nil {
		t.Fatalf("the service did not commit event %s: %v", id, err)
	}
}

// storedPayload reads one event's payload and privacy class back.
func storedPayload(t *testing.T, db *sql.DB, id string) (payload, privacyClass string) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(),
		`SELECT payload, privacy_class FROM events WHERE id = ?`, id).Scan(&payload, &privacyClass); err != nil {
		t.Fatalf("read event %s: %v", id, err)
	}
	return payload, privacyClass
}

// ---------------------------------------------------------------------------
// Gate 2: the log egress, at the process level (I-10)
// ---------------------------------------------------------------------------

// TestTheLogFileNeverCarriesASecret is I-10's egress half, asserted against
// the file a running service actually wrote rather than against a handler a
// test constructed. Nothing installs slog.SetDefault today, so the filter is
// in force in no binary at all, and an in-process test of the handler cannot
// see that.
//
// It needs a log line that carries bytes off the wire, or it asserts nothing:
// a service that logs no payload-derived value passes an "the secret is not in
// the log" assertion by never having had the chance to leak. The malformed
// request below is that line. pipe.validate refuses an envelope whose version
// this build does not speak and puts up to 64 characters of it in the error -
// wire bytes, unvalidated, on their way to a log - which is exactly the shape
// the filter exists for.
//
// So there are three assertions and each one alone is satisfied by a design
// the other two forbid:
//
//   - the placeholder is in the log, so the line happened and went through the
//     filter. Without it, absence proves nothing;
//   - the secret's own bytes are not in the log;
//   - the database row still holds them (I-10: tagged, not destroyed).
func TestTheLogFileNeverCarriesASecret(t *testing.T) {
	sample := secrettest.Of(secret.ClassAPIKey)
	const (
		storedID    = "0192f0c0-0000-7000-8000-00000000a001"
		refusedID   = "0192f0c0-0000-7000-8000-00000000a002"
		sessionID   = "0192f0c0-0000-7000-8000-00000000e5e5"
		placeholder = "[redacted-api-key]"
	)
	// cwd is on a volume that does not exist and holds no Windows user
	// directory: a path under one would add secret.ClassUserPath to the
	// payload and make the privacy_class assertion untrue for a reason that
	// has nothing to do with this test.
	payload := []byte(`{"hook_event_name":"PreToolUse","session_id":"` + sessionID +
		`","cwd":"Z:\\service\\workspace\\service-project","tool_name":"Bash",` +
		`"tool_input":{"command":"echo ` + sample.Value + `"}}`)
	if !bytes.Contains(payload, []byte(sample.Secret)) || !json.Valid(payload) {
		t.Fatalf("the payload is not a JSON document carrying the generated secret:\n%s", payload)
	}

	svc := start(t, t.TempDir())

	// Stored, tagged, and still holding the secret.
	ingest(t, storedID, payload)

	// Logged. The version field is the secret, so the service writes wire
	// bytes it never validated into its own log file.
	var refused ipc.Ack
	if err := json.Unmarshal(send(t, request(t, sample.Value, ipc.IngestEvent, refusedID, payload)), &refused); err != nil {
		t.Fatalf("decode the reply to the malformed request: %v", err)
	}
	if refused.Status != ipc.Rejected {
		t.Fatalf("a request whose version is %q was answered %q, want %q",
			"the generated secret", refused.Status, ipc.Rejected)
	}

	svc.stop(t)

	logged := svc.logFile(t)
	if bytes.Contains(logged, []byte(sample.Secret)) {
		t.Fatalf("the secret reached the log file:\n%s", logged)
	}
	if !bytes.Contains(logged, []byte(placeholder)) {
		t.Fatalf("the log file carries no %s, so the line that would have leaked never happened "+
			"and the absence assertion above is vacuous:\n%s", placeholder, logged)
	}
	// Every line still parses. A token pattern widened to \S+ swallows the
	// closing quote and produces a log nothing can read.
	for i, line := range bytes.Split(bytes.TrimRight(logged, "\r\n"), []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("log line %d does not parse as JSON: %v\n%s", i+1, err, line)
		}
	}

	db := svc.openDB(t)
	stored, privacyClass := storedPayload(t, db, storedID)
	if privacyClass != "api-key" {
		t.Errorf("privacy_class = %q, want %q - the row was not tagged", privacyClass, "api-key")
	}
	if !bytes.Contains([]byte(stored), []byte(sample.Secret)) {
		t.Errorf("the stored row no longer holds the secret - it was erased rather than tagged:\n%s", stored)
	}
	if !bytes.Equal([]byte(stored), payload) {
		t.Errorf("the stored payload is not the bytes that were ingested\n got: %q\nwant: %q", stored, payload)
	}
	// The row the malformed request carried must not exist: it never
	// reached the database, and a service that ingested it anyway would
	// satisfy every assertion above.
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM events WHERE id = ?`, refusedID).Scan(&n); err != nil {
		t.Fatalf("count the refused event: %v", err)
	}
	if n != 0 {
		t.Errorf("the refused request left %d rows, want 0", n)
	}
}
