package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/pipe"
)

// Two binaries, built once. Every exit-code assertion in this file runs the
// real executable as a subprocess, because that is the only thing that
// observes what I-03 actually promises: a host sees a process exit code, not
// an error value, and an in-process test of a function that returns an error
// would pass with os.Exit(1) on the next line.
var (
	relayBin string
	// panicBin is the same program with dial replaced by one that panics.
	// The replacement is a build-tagged file, not a flag: the shipped
	// binary contains no injection point at all, and the tag is a
	// dependency the test picks at build time.
	panicBin string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "engramux-relay-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}

	relayBin = filepath.Join(dir, "engramux.exe")
	panicBin = filepath.Join(dir, "engramux-panicinject.exe")
	if err := build(relayBin); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err := build(panicBin, "-tags", "engramux_panicinject"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// build compiles this package to out. No -H=windowsgui: the relay is a
// console-subsystem binary so it inherits the host's stdio (spec 5.1), and a
// GUI-subsystem build of it would have no stdin to read.
func build(out string, extra ...string) error {
	args := append([]string{"build", "-o", out}, extra...)
	args = append(args, ".")
	//nolint:gosec // G204: args is this function's own literals plus a path it built
	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go %v: %w\n%s", args, err, b)
	}
	return nil
}

// result is one relay run.
type result struct {
	exit     int
	stdout   []byte
	stderr   []byte
	elapsed  time.Duration
	spoolDir string
}

// run executes bin with stdin as its standard input and a LOCALAPPDATA of its
// own, so the spool it writes lands somewhere this test can read.
//
// Redirecting LOCALAPPDATA is the whole test seam and it costs production code
// nothing: spool.Dir calls os.UserCacheDir, which on Windows is %LocalAppData%
// and nothing else. The relay has no flag, no environment variable and no
// injection point for any of this.
func run(t *testing.T, bin string, stdin []byte) result {
	t.Helper()
	return runWith(t, bin, func(cmd *exec.Cmd) { cmd.Stdin = bytes.NewReader(stdin) })
}

func runWith(t *testing.T, bin string, setup func(*exec.Cmd)) result {
	t.Helper()

	local := t.TempDir()
	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(context.Background(), bin)
	cmd.Env = append(os.Environ(), "LOCALAPPDATA="+local)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	setup(cmd)

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("run %s: %v", bin, err)
	}
	if cmd.ProcessState == nil {
		t.Fatalf("run %s: no process state", bin)
	}

	res := result{
		exit:     cmd.ProcessState.ExitCode(),
		stdout:   stdout.Bytes(),
		stderr:   stderr.Bytes(),
		elapsed:  elapsed,
		spoolDir: filepath.Join(local, "engramux", "spool"),
	}
	t.Logf("exit=%d elapsed=%s stderr=%s", res.exit, res.elapsed, res.stderr)
	return res
}

// requireExitZeroAndSilentStdout is gate clauses 1 and 4 in one call, because
// they are one promise: a hook that neither fails nor speaks (I-03, spec 4.5).
func (r result) requireExitZeroAndSilentStdout(t *testing.T) {
	t.Helper()
	if r.exit != 0 {
		t.Errorf("exit code = %d, want 0 (stderr: %s)", r.exit, r.stderr)
	}
	if len(r.stdout) != 0 {
		t.Errorf("stdout = %q, want zero bytes", r.stdout)
	}
}

// spooled returns the one record in the spool, or fails. It returns the id -
// which is the file name, and therefore the id the relay minted - and the
// bytes.
//
// Exit 0 proves nothing on its own here: the relay exits 0 whether it
// delivered the event, spooled it, or lost it. This function is what tells
// those apart.
func (r result) spooled(t *testing.T) (id string, body []byte) {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(r.spoolDir, "*.json"))
	if err != nil {
		t.Fatalf("glob spool: %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("spool holds %d records, want exactly 1 (%q)", len(names), names)
	}
	body, err = os.ReadFile(names[0])
	if err != nil {
		t.Fatalf("read spool record: %v", err)
	}
	return filepath.Base(names[0][:len(names[0])-len(".json")]), body
}

// requireSpoolEmpty is the other half: a delivered event must not also be
// spooled, or the drain replays every event the service already has.
func (r result) requireSpoolEmpty(t *testing.T) {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(r.spoolDir, "*"))
	if err != nil {
		t.Fatalf("glob spool: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("spool holds %q, want nothing", names)
	}
}

// requireSpooledAs asserts the event is in the spool under wantID with
// wantBody. wantID is the ingest id the test's own server read off the wire,
// so this is the assertion that catches a relay minting a second id on the
// spool path (I-05): the id that was sent and the id that was saved are
// compared against each other, not each against itself.
func (r result) requireSpooledAs(t *testing.T, wantID string, wantBody []byte) {
	t.Helper()
	id, body := r.spooled(t)
	if id != wantID {
		t.Errorf("spool record id = %q, want the id the relay sent, %q", id, wantID)
	}
	if !bytes.Equal(body, wantBody) {
		t.Errorf("spool record body\n got %q\nwant %q", body, wantBody)
	}
}

// ---------------------------------------------------------------------------
// Servers
// ---------------------------------------------------------------------------

// observed collects the envelopes a test server read off the wire.
type observed struct {
	mu   sync.Mutex
	envs []ipc.Envelope
}

func (o *observed) add(env ipc.Envelope) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.envs = append(o.envs, env)
}

// only returns the single envelope the server saw, or fails.
func (o *observed) only(t *testing.T) ipc.Envelope {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.envs) != 1 {
		t.Fatalf("server saw %d envelopes, want 1", len(o.envs))
	}
	return o.envs[0]
}

func currentSID(t testing.TB) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	return u.Uid
}

// relayPipeName is the name the relay dials. It is derived, not configurable -
// that is spec 5.2's fixed name - so a test server has to take the real one,
// and these tests cannot run beside a live service. -p 1 keeps them from
// colliding with each other.
func relayPipeName(t testing.TB) string {
	t.Helper()
	name, err := ipc.CurrentPipeName()
	if err != nil {
		t.Fatalf("ipc.CurrentPipeName: %v", err)
	}
	return name
}

func listenRelayPipe(t *testing.T) net.Listener {
	t.Helper()
	name := relayPipeName(t)
	l, err := pipe.Listen(name, currentSID(t))
	if err != nil {
		t.Fatalf("Listen(%s): %v\n"+
			"An access-denied here means something else already holds the relay's pipe - "+
			"a development engramux service, or another copy of this test binary. "+
			"Stop it and re-run with -p 1.", name, err)
	}
	return l
}

// serveReal stands up the production pipe server on the relay's real pipe
// name, backed by an IngestFunc that answers status. Every well-formed case
// goes through this rather than a hand-rolled reply, so the ACK the relay
// checks is the ACK the service actually produces.
func serveReal(t *testing.T, status ipc.AckStatus) *observed {
	t.Helper()
	l := listenRelayPipe(t)
	obs := &observed{}

	done := make(chan error, 1)
	go func() {
		done <- pipe.Serve(t.Context(), l, func(_ context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
			obs.add(env)
			return status, nil
		})
	}()
	t.Cleanup(func() {
		_ = l.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("pipe.Serve did not return within 10s of Close")
		}
	})
	return obs
}

// serveRaw stands up a listener that reads one frame and then hands the
// connection to reply, which decides what - if anything - goes back.
//
// It exists for the three replies a correct server cannot produce: a
// mismatched version, a mismatched ingest id, and a frame cut off in the
// middle. pipe.Serve is used for everything it can express.
//
// stop is closed before the listener, so a reply parked on it unblocks before
// the accept loop is torn down.
func serveRaw(t *testing.T, reply func(c net.Conn, env ipc.Envelope, stop <-chan struct{})) *observed {
	t.Helper()
	l := listenRelayPipe(t)
	obs := &observed{}
	stop := make(chan struct{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			serveRawConn(c, obs, reply, stop)
		}
	}()
	t.Cleanup(func() {
		close(stop)
		_ = l.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("raw server did not stop within 10s")
		}
	})
	return obs
}

func serveRawConn(c net.Conn, obs *observed, reply func(net.Conn, ipc.Envelope, <-chan struct{}), stop <-chan struct{}) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))

	raw, err := ipc.ReadFrame(c)
	if err != nil {
		return
	}
	var env ipc.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	obs.add(env)
	reply(c, env, stop)
}

// ackBytes marshals an Ack. It is used only to build the deliberately wrong
// replies, so it takes every field rather than deriving any of them.
func ackBytes(t *testing.T, version string, status ipc.AckStatus, id string) []byte {
	t.Helper()
	b, err := json.Marshal(ipc.Ack{Version: version, Status: status, IngestID: id})
	if err != nil {
		t.Fatalf("marshal ack: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// The payload every test sends
// ---------------------------------------------------------------------------

// payload is a real fixture with the file's framing newline removed: the
// event's bytes, which is what trimFraming leaves and therefore what the relay
// carries down both of its paths.
//
// It is trimmed here with bytes.TrimRight rather than by calling trimFraming,
// so that every "what went in came out" assertion below keeps an expectation
// the implementation did not compute. That the relay's own trim is the right
// one is asserted by TestTrimFramingRemovesOnlyJSONFraming and, end to end, by
// the Phase 1 gate's clause 1.
func payload(t *testing.T) []byte {
	t.Helper()
	b, err := fixtures.Fixture{File: fixtures.CodexSessionEnd}.Bytes()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return bytes.TrimRight(b, "\r\n")
}

// ---------------------------------------------------------------------------
// The success path
// ---------------------------------------------------------------------------

// TestDeliveredEventIsNotSpooled is the baseline every failure test is read
// against: on the one path where delivery works, the payload arrives
// byte-identical, stdout is empty, and nothing is left in the spool.
func TestDeliveredEventIsNotSpooled(t *testing.T) {
	obs := serveReal(t, ipc.Committed)
	in := payload(t)

	res := run(t, relayBin, in)

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpoolEmpty(t)

	env := obs.only(t)
	if env.Version != ipc.Version {
		t.Errorf("envelope version = %q, want %q", env.Version, ipc.Version)
	}
	if env.Type != ipc.IngestEvent {
		t.Errorf("envelope type = %q, want %q", env.Type, ipc.IngestEvent)
	}
	if err := uuidV7(env.IngestID); err != nil {
		t.Errorf("ingest id %q: %v", env.IngestID, err)
	}
	// Byte-identical, not merely equivalent. encoding/json compacts a
	// json.RawMessage and HTML-escapes it on the way out, so a relay that
	// round-trips the payload through json.Marshal changes bytes the store
	// writes verbatim into a TEXT column.
	if !bytes.Equal(env.Payload, in) {
		t.Errorf("payload was rewritten in flight\n got %q\nwant %q", env.Payload, in)
	}
}

// uuidV7 checks id is a version 7 UUID in the canonical form. The test does
// its own check rather than importing google/uuid, so a relay that stopped
// minting v7 would be caught by an assertion that does not share the
// implementation's opinion of what v7 means.
func uuidV7(id string) error {
	if len(id) != 36 {
		return fmt.Errorf("length %d, want 36", len(id))
	}
	for i, r := range id {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return fmt.Errorf("byte %d is %q, want '-'", i, r)
			}
		default:
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				return fmt.Errorf("byte %d is %q, want a lowercase hex digit", i, r)
			}
		}
	}
	if id[14] != '7' {
		return fmt.Errorf("version nibble is %q, want '7'", id[14])
	}
	// RFC 4122 variant: the first nibble of octet 8 is 8, 9, a or b.
	switch id[19] {
	case '8', '9', 'a', 'b':
	default:
		return fmt.Errorf("variant nibble is %q, want one of 89ab", id[19])
	}
	return nil
}

// ---------------------------------------------------------------------------
// Gate 1, 3 and 4: every failure mode exits 0, says nothing, and spools
// ---------------------------------------------------------------------------

// TestNoServiceListening. Nothing holds the pipe, so the dial fails
// immediately - winio.DialPipeContext only retries ERROR_PIPE_BUSY, and a
// missing pipe is not that, which is the behaviour the relay wants.
func TestNoServiceListening(t *testing.T) {
	requirePipeFree(t)
	in := payload(t)

	res := run(t, relayBin, in)

	res.requireExitZeroAndSilentStdout(t)
	id, body := res.spooled(t)
	if err := uuidV7(id); err != nil {
		t.Errorf("spool record id %q: %v", id, err)
	}
	if !bytes.Equal(body, in) {
		t.Errorf("spool record body\n got %q\nwant %q", body, in)
	}
}

func requirePipeFree(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, relayPipeName(t))
	if err == nil {
		_ = c.Close()
		t.Fatalf("something is listening on %s; stop the development engramux service", relayPipeName(t))
	}
}

// TestServiceClosesMidFrame: the service accepts, reads the request, and dies
// halfway through its ACK. The relay must not read a truncated frame as an
// ACK - ipc.ReadFrame surfaces the short read - and must spool.
func TestServiceClosesMidFrame(t *testing.T) {
	in := payload(t)
	obs := serveRaw(t, func(c net.Conn, env ipc.Envelope, _ <-chan struct{}) {
		// A real frame, cut in half on the way out: the length header
		// promises the whole ACK and the connection then closes partway
		// through the body.
		var framed bytes.Buffer
		if err := ipc.WriteFrame(&framed, ackBytes(t, ipc.Version, ipc.Committed, env.IngestID)); err != nil {
			return
		}
		_, _ = c.Write(framed.Bytes()[:framed.Len()/2])
	})

	res := run(t, relayBin, in)

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpooledAs(t, obs.only(t).IngestID, in)
}

// TestAckWithWrongVersion. A relay that only unmarshals the ACK sees a
// perfectly good document here.
func TestAckWithWrongVersion(t *testing.T) {
	in := payload(t)
	obs := serveRaw(t, func(c net.Conn, env ipc.Envelope, _ <-chan struct{}) {
		_ = ipc.WriteFrame(c, ackBytes(t, "999", ipc.Committed, env.IngestID))
	})

	res := run(t, relayBin, in)

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpooledAs(t, obs.only(t).IngestID, in)
}

// TestAckRejectedIsSpooled is the rev.2 bug's test. A rejected ACK is a
// delivery failure like any other (I-04): the relay exits 0 either way, so
// the exit code cannot catch a relay that treats rejected as success - the
// spool assertion is the only thing that can.
//
// The reply comes from the production server, with an IngestFunc that answers
// Rejected, so it is a real rejected ACK and not a hand-built one.
func TestAckRejectedIsSpooled(t *testing.T) {
	obs := serveReal(t, ipc.Rejected)
	in := payload(t)

	res := run(t, relayBin, in)

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpooledAs(t, obs.only(t).IngestID, in)
}

// TestAckWithDifferentIngestID: the ACK is well-formed and committed, but for
// some other event. Accepting it would mean the relay believes an event was
// stored on the strength of a reply about a different one.
func TestAckWithDifferentIngestID(t *testing.T) {
	in := payload(t)
	const other = "0192f0c0-0000-7000-8000-0000000000ff"
	obs := serveRaw(t, func(c net.Conn, _ ipc.Envelope, _ <-chan struct{}) {
		_ = ipc.WriteFrame(c, ackBytes(t, ipc.Version, ipc.Committed, other))
	})

	res := run(t, relayBin, in)

	res.requireExitZeroAndSilentStdout(t)
	sent := obs.only(t).IngestID
	if sent == other {
		t.Fatalf("the relay minted the id this test uses as the wrong one")
	}
	res.requireSpooledAs(t, sent, in)
}

// TestMalformedStdin. The bytes are not JSON, so they can never travel inside
// an envelope and can never be ingested. They are still an event the host
// handed us, and I-04 says an event is never silently dropped.
//
// The input's trailing space is the second thing this asserts: trimFraming runs
// at the stdin boundary, before anything decides these bytes are not a
// document, so the record holds the event and not the sender's framing. want is
// written out rather than computed from in, so a broken trim cannot satisfy it.
func TestMalformedStdin(t *testing.T) {
	serveReal(t, ipc.Committed)
	in := []byte(`{"hook_event_name": "SessionEnd", `)
	want := []byte(`{"hook_event_name": "SessionEnd",`)

	res := run(t, relayBin, in)

	res.requireExitZeroAndSilentStdout(t)
	id, body := res.spooled(t)
	if err := uuidV7(id); err != nil {
		t.Errorf("spool record id %q: %v", id, err)
	}
	if !bytes.Equal(body, want) {
		t.Errorf("spool record body\n got %q\nwant %q", body, want)
	}
}

// TestEmptyStdin. Zero bytes is not a JSON document either, and the record is
// the empty file - which is exactly the diagnostic a human wants when a host
// starts invoking the hook with no payload.
func TestEmptyStdin(t *testing.T) {
	serveReal(t, ipc.Committed)

	res := run(t, relayBin, nil)

	res.requireExitZeroAndSilentStdout(t)
	id, body := res.spooled(t)
	if err := uuidV7(id); err != nil {
		t.Errorf("spool record id %q: %v", id, err)
	}
	if len(body) != 0 {
		t.Errorf("spool record body = %q, want zero bytes", body)
	}
}

// ---------------------------------------------------------------------------
// Gate 2: a panic exits 0, and the event survives it
// ---------------------------------------------------------------------------

// TestPanicExitsZeroAndSpools runs the build whose dial panics. Nothing in the
// shipped binary can reach this: the panicking dial lives behind a build tag
// and the production build compiles the other file.
//
// Both halves matter. Exit 0 alone would also be true of a relay that
// recovered and forgot the event; the spool record is what says the recover
// happened somewhere the bytes were still in hand.
func TestPanicExitsZeroAndSpools(t *testing.T) {
	in := payload(t)

	res := run(t, panicBin, in)

	res.requireExitZeroAndSilentStdout(t)
	if !bytes.Contains(res.stderr, []byte("panic")) {
		t.Errorf("stderr = %q, want it to report the panic", res.stderr)
	}
	id, body := res.spooled(t)
	if err := uuidV7(id); err != nil {
		t.Errorf("spool record id %q: %v", id, err)
	}
	if !bytes.Equal(body, in) {
		t.Errorf("spool record body\n got %q\nwant %q", body, in)
	}
}

// ---------------------------------------------------------------------------
// Gate 5: the budgets
// ---------------------------------------------------------------------------

// budgetSlack is what a subprocess costs around the work it does -
// CreateProcess, the Go runtime coming up, the exit - where the assertion
// cannot subtract it. Measured on this machine at roughly 10 ms warm and
// 130 ms cold; 200 ms is that with room.
const budgetSlack = 200 * time.Millisecond

// spawnOverhead measures what running the relay costs when the relay itself
// does no I/O at all: empty stdin never reaches the dial, so the elapsed time
// is process start and teardown and nothing else.
//
// It is measured rather than assumed because it is subtracted from a real
// assertion below, and a padded constant there would turn the upper bound into
// decoration. The first run is thrown away: a cold binary pages in from disk
// and costs an order of magnitude more than the warm one the real run gets.
func spawnOverhead(t *testing.T) time.Duration {
	t.Helper()
	run(t, relayBin, nil)
	d := run(t, relayBin, nil).elapsed
	t.Logf("subprocess overhead = %s", d)
	return d
}

// TestPostDialBudgetStops is gate clause 5. The server accepts and reads the
// request and then says nothing at all, which is the shape of a service that
// is wedged rather than absent.
//
// The upper bound is the assertion that does work. A relay with no post-dial
// budget would still spool and still exit 0 - the total 1 s ceiling would fire
// instead - so the only thing that distinguishes "the post-dial limit stopped
// it" from "the total limit stopped it" is that it happened at 800 ms and not
// at 1 s. That is a 200 ms gap, which is why the subprocess overhead is
// measured and subtracted rather than added to the limit as slack.
func TestPostDialBudgetStops(t *testing.T) {
	in := payload(t)
	obs := serveRaw(t, func(_ net.Conn, _ ipc.Envelope, stop <-chan struct{}) {
		// Hold the connection open, silently, past the relay's budget.
		select {
		case <-stop:
		case <-time.After(5 * time.Second):
		}
	})

	// Measured against the same stalled server, and unaffected by it: an
	// empty stdin is refused before the dial, so these runs never connect.
	overhead := spawnOverhead(t)

	res := run(t, relayBin, in)

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpooledAs(t, obs.only(t).IngestID, in)

	if res.elapsed < postDialBudget {
		t.Errorf("elapsed %s < post-dial budget %s: the relay gave up early, so this test did not exercise the budget",
			res.elapsed, postDialBudget)
	}
	if inRelay := res.elapsed - overhead; inRelay >= totalBudget {
		t.Errorf("the relay's own wall clock was %s (%s measured less %s of subprocess overhead), "+
			"which is the %s total ceiling rather than the %s post-dial budget",
			inRelay, res.elapsed, overhead, totalBudget, postDialBudget)
	}
}

// TestStdinThatNeverClosesStopsAtTheTotalBudget. A host that opens the hook's
// stdin and never closes it would hang the relay forever, and a hung relay is
// a hung host - the one thing I-03 forbids. There is nothing complete to
// spool, so this is the single path that saves nothing, and it says so on
// stderr rather than silently.
func TestStdinThatNeverClosesStopsAtTheTotalBudget(t *testing.T) {
	serveReal(t, ipc.Committed)

	res := runWith(t, relayBin, func(cmd *exec.Cmd) {
		if _, err := cmd.StdinPipe(); err != nil {
			t.Fatalf("stdin pipe: %v", err)
		}
	})

	res.requireExitZeroAndSilentStdout(t)
	if res.elapsed < totalBudget {
		t.Errorf("elapsed %s < total budget %s: it gave up before the ceiling", res.elapsed, totalBudget)
	}
	if limit := totalBudget + budgetSlack; res.elapsed >= limit {
		t.Errorf("elapsed %s >= %s: the total ceiling did not stop it", res.elapsed, limit)
	}
	res.requireSpoolEmpty(t)
}

// ---------------------------------------------------------------------------
// Envelope encoding, in process
// ---------------------------------------------------------------------------

// TestEncodeEnvelopeDoesNotTouchThePayload pins the reason the envelope is
// built by concatenation. json.Marshal compacts a json.RawMessage and escapes
// '<', '>' and '&' inside it; the store writes the payload bytes verbatim into
// a TEXT column, and Phase 1 gates on a byte-for-byte round trip.
func TestEncodeEnvelopeDoesNotTouchThePayload(t *testing.T) {
	for name, in := range map[string]string{
		"whitespace":  "{ \"a\" : 1,\n\t\"b\": 2 }",
		"html":        `{"a":"<b>&</b>"}`,
		"escapes":     `{"a":"é\\x"}`,
		"big number":  `{"a":1700000000000000000}`,
		"not object":  `"just a string"`,
		"unicode raw": `{"a":"한글"}`,
	} {
		t.Run(name, func(t *testing.T) {
			const id = "0192f0c0-0000-7000-8000-000000000001"
			b, err := encodeEnvelope(id, []byte(in))
			if err != nil {
				t.Fatalf("encodeEnvelope: %v", err)
			}

			var env ipc.Envelope
			if err := json.Unmarshal(b, &env); err != nil {
				t.Fatalf("the envelope is not valid JSON: %v\n%s", err, b)
			}
			if env.Version != ipc.Version {
				t.Errorf("version = %q, want %q", env.Version, ipc.Version)
			}
			if env.Type != ipc.IngestEvent {
				t.Errorf("type = %q, want %q", env.Type, ipc.IngestEvent)
			}
			if env.IngestID != id {
				t.Errorf("ingest id = %q, want %q", env.IngestID, id)
			}
			if string(env.Payload) != in {
				t.Errorf("payload\n got %q\nwant %q", env.Payload, in)
			}
		})
	}
}

// TestTrimFramingRemovesOnlyJSONFraming pins the two halves of the set
// trimFraming trims: RFC 8259's four whitespace bytes come off either end, and
// nothing else does.
//
// The last case is the one that rules out bytes.TrimSpace. U+00A0 is
// unicode.IsSpace but not JSON whitespace, so a parser rejects a document
// wrapped in it - and trimming it here would turn bytes the relay has to refuse
// into bytes it sends.
func TestTrimFramingRemovesOnlyJSONFraming(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"trailing newline":            {"{\"a\":1}\n", `{"a":1}`},
		"crlf":                        {"{\"a\":1}\r\n", `{"a":1}`},
		"both ends":                   {" \t\r\n{\"a\":1}\n\t ", `{"a":1}`},
		"nothing to trim":             {`{"a":1}`, `{"a":1}`},
		"inside is untouched":         {"{ \"a\" : 1,\n\t\"b\": 2 }\n", "{ \"a\" : 1,\n\t\"b\": 2 }"},
		"whitespace only":             {"\n\t ", ""},
		"empty":                       {"", ""},
		"nbsp is not json whitespace": {"\u00a0{\"a\":1}\u00a0", "\u00a0{\"a\":1}\u00a0"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := trimFraming([]byte(c.in)); string(got) != c.want {
				t.Errorf("trimFraming(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
	// The premise of the last case, so it cannot quietly stop testing it.
	if json.Valid([]byte("\u00a0{\"a\":1}\u00a0")) {
		t.Error("encoding/json accepts a document wrapped in U+00A0; the reason for the narrow set is gone")
	}
}

// TestEncodeEnvelopeRefusesAPayloadThatIsNotJSON. There is no envelope that
// can carry it, so the send is not attempted and the caller spools instead.
func TestEncodeEnvelopeRefusesAPayloadThatIsNotJSON(t *testing.T) {
	for name, in := range map[string][]byte{
		"empty":      {},
		"truncated":  []byte(`{"a":`),
		"text":       []byte("not json at all"),
		"two values": []byte(`{"a":1}{"b":2}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := encodeEnvelope("0192f0c0-0000-7000-8000-000000000001", in)
			if !errors.Is(err, errPayloadJSON) {
				t.Fatalf("encodeEnvelope error = %v, want errPayloadJSON", err)
			}
		})
	}
}

// TestEveryFixtureEncodesByteIdentically runs the same assertion over the four
// Phase 1 fixtures, which are the real shapes a host writes.
func TestEveryFixtureEncodesByteIdentically(t *testing.T) {
	for _, f := range fixtures.All() {
		t.Run(f.File, func(t *testing.T) {
			raw, err := f.Bytes()
			if err != nil {
				t.Fatalf("fixture bytes: %v", err)
			}
			raw = bytes.TrimRight(raw, "\r\n")

			b, err := encodeEnvelope("0192f0c0-0000-7000-8000-000000000001", raw)
			if err != nil {
				t.Fatalf("encodeEnvelope: %v", err)
			}
			var env ipc.Envelope
			if err := json.Unmarshal(b, &env); err != nil {
				t.Fatalf("envelope is not valid JSON: %v", err)
			}
			if !bytes.Equal(env.Payload, raw) {
				t.Errorf("payload changed\n got %q\nwant %q", env.Payload, raw)
			}
		})
	}
}
