package pipe

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/ipc"
)

// recorder is the IngestFunc seam under test. internal/ipc cannot import
// internal/store, so the accept loop is handed a function rather than a
// database; this is what the service will hand it, minus the database.
type recorder struct {
	mu     sync.Mutex
	calls  []ipc.Envelope
	status ipc.AckStatus
	err    error
}

func (r *recorder) ingest(_ context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, env)
	return r.status, r.err
}

func (r *recorder) seen() []ipc.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ipc.Envelope(nil), r.calls...)
}

// startServer opens a listener on a name unique to t and runs Serve on it.
// Listen is called synchronously and returns only once the pipe exists, so a
// client may dial the moment this returns: there is no listener/dial race to
// sleep through, and none of these tests sleeps.
//
// stop closes the listener, waits for Serve to return and hands back the
// error it returned. It is safe to call twice, and t.Cleanup calls it, so a
// test that ends early still tears the server down.
func startServer(t *testing.T, ingest IngestFunc) (name string, stop func() error) {
	t.Helper()
	return startHandler(t, Handler{Ingest: ingest})
}

// startHandler is startServer for a test that needs more of the Handler than
// the ingest seam.
func startHandler(t *testing.T, h Handler) (name string, stop func() error) {
	t.Helper()

	name = uniquePipeName(t)
	l, err := Listen(name, currentSID(t))
	if err != nil {
		t.Fatalf("Listen(%s): %v", name, err)
	}

	done := make(chan error, 1)
	go func() { done <- Serve(t.Context(), l, h) }()

	var (
		once     sync.Once
		serveErr error
	)
	stop = func() error {
		once.Do(func() {
			if err := l.Close(); err != nil {
				t.Errorf("close listener: %v", err)
			}
			select {
			case serveErr = <-done:
			case <-time.After(10 * time.Second):
				t.Error("Serve did not return within 10s of Close")
			}
		})
		return serveErr
	}
	t.Cleanup(func() { _ = stop() })

	return name, stop
}

// dial connects to name. The context carries a timeout because
// winio.DialPipe(path, nil) silently uses 2 s and this needs the failure to
// be legible rather than mysterious.
func dial(t *testing.T, name string) net.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		t.Fatalf("dial %s: %v", name, err)
	}
	return c
}

// request builds a request frame's payload by concatenation rather than with
// json.Marshal. json.Marshal compacts a json.RawMessage, so a test whose own
// encoder rewrote the payload could not tell a server that rewrote it from
// one that left it alone - which is exactly what gate clause 2 asks.
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
	b.WriteString(`,"payload":`)
	b.Write(payload)
	b.WriteString(`}`)
	return b.Bytes()
}

// exchange dials, sends one frame, reads one frame back and decodes the ACK.
func exchange(t *testing.T, name string, req []byte) ipc.Ack {
	t.Helper()
	raw := exchangeRaw(t, name, req)
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("decode ack %q: %v", raw, err)
	}
	return ack
}

// exchangeRaw is exchange without an opinion about what came back, for the
// reply documents that are not an ACK.
func exchangeRaw(t *testing.T, name string, req []byte) []byte {
	t.Helper()
	conn := dial(t, name)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close conn: %v", err)
		}
	}()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if err := ipc.WriteFrame(conn, req); err != nil {
		t.Fatalf("write request: %v", err)
	}
	raw, err := ipc.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}
	return raw
}

// TestFrameRoundTripsAndRoutesToIngest is gate clause 2. The payload is
// deliberately awkward JSON - whitespace inside the object, keys out of
// alphabetical order, an escape sequence, a nested object and a number that
// a re-encode would turn into 1.7e+09 - so that any server that decodes and
// re-encodes the payload fails on bytes rather than on meaning.
func TestFrameRoundTripsAndRoutesToIngest(t *testing.T) {
	payload := []byte(`{ "zeta": 1700000000, "alpha": {"b": "\u00e9"},"cwd":"C:\\Users\\x" }`)
	const id = "0192f0c0-0000-7000-8000-000000000001"

	rec := &recorder{status: ipc.Committed}
	name, _ := startServer(t, rec.ingest)

	ack := exchange(t, name, request(t, ipc.Version, ipc.IngestEvent, id, payload))

	if err := ack.Verify(id); err != nil {
		t.Fatalf("ack.Verify: %v (ack = %+v)", err, ack)
	}

	seen := rec.seen()
	if len(seen) != 1 {
		t.Fatalf("ingest called %d times, want 1", len(seen))
	}
	if seen[0].Type != ipc.IngestEvent {
		t.Errorf("routed type = %q, want %q", seen[0].Type, ipc.IngestEvent)
	}
	if seen[0].IngestID != id {
		t.Errorf("ingest id = %q, want %q", seen[0].IngestID, id)
	}
	if !bytes.Equal(seen[0].Payload, payload) {
		t.Errorf("payload was not byte-identical\n got %q\nwant %q", seen[0].Payload, payload)
	}
}

// TestFixturePayloadsRoundTripByteIdentically runs the same assertion over
// the four Phase 1 fixtures, which are the real bytes a host writes.
func TestFixturePayloadsRoundTripByteIdentically(t *testing.T) {
	for _, f := range fixtures.All() {
		t.Run(f.File, func(t *testing.T) {
			raw, err := f.Bytes()
			if err != nil {
				t.Fatalf("fixture bytes: %v", err)
			}
			// The fixture files end in a newline after the closing brace.
			// That byte is not part of the JSON value: the relay trims it
			// at its stdin boundary, so it never reaches the wire, and it
			// is trimmed here rather than asserted on. Asserting on it
			// would be asserting json.RawMessage's own semantics - the
			// byte lands outside the payload value inside the envelope,
			// where the decoder discards it as envelope structure. That
			// the two delivery paths agree once it is trimmed is the
			// Phase 1 gate's clause 1, which crosses this wire for real.
			// Everything inside the braces is left exactly as embedded -
			// these fixtures are pretty-printed across seven or more lines,
			// so the assertion below still fails on any re-encode.
			payload := bytes.TrimRight(raw, "\n")
			if len(payload) != len(raw)-1 {
				t.Fatalf("fixture %s does not end in exactly one newline", f.File)
			}

			rec := &recorder{status: ipc.Committed}
			name, _ := startServer(t, rec.ingest)

			id := "id-" + f.File
			ack := exchange(t, name, request(t, ipc.Version, ipc.IngestEvent, id, payload))
			if err := ack.Verify(id); err != nil {
				t.Fatalf("ack.Verify: %v", err)
			}

			seen := rec.seen()
			if len(seen) != 1 {
				t.Fatalf("ingest called %d times, want 1", len(seen))
			}
			if !bytes.Equal(seen[0].Payload, payload) {
				t.Errorf("payload differs: got %d bytes, want %d", len(seen[0].Payload), len(payload))
			}
		})
	}
}

// TestUnimplementedRequestTypesAreRejected covers the CLI read types I-08
// routes over the pipe when the Handler wires none of them, which is Phase 1's
// shape. The requirement is that the answer must not look like success:
// ipc.Ack.Verify only accepts Committed, so a Rejected ACK cannot be mistaken
// for one.
func TestUnimplementedRequestTypesAreRejected(t *testing.T) {
	for _, typ := range []ipc.RequestType{ipc.Status, ipc.Doctor, ipc.Search} {
		t.Run(string(typ), func(t *testing.T) {
			rec := &recorder{status: ipc.Committed}
			name, _ := startServer(t, rec.ingest)

			ack := exchange(t, name, request(t, ipc.Version, typ, "", []byte(`null`)))

			if ack.Status != ipc.Rejected {
				t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
			}
			if ack.Version != ipc.Version {
				t.Errorf("ack version = %q, want %q", ack.Version, ipc.Version)
			}
			if err := ack.Verify(""); !errors.Is(err, ipc.ErrAckRejected) {
				t.Errorf("Verify accepted a not-implemented reply: %v", err)
			}
			if n := len(rec.seen()); n != 0 {
				t.Errorf("ingest was called %d times for a %s request", n, typ)
			}
		})
	}
}

// TestMalformedEnvelopesAreRejectedWithoutIngesting is the routing boundary's
// job. internal/store deliberately does not police the envelope - Ingest
// trusts the id it is handed - so if a malformed one is not stopped here it
// is not stopped anywhere.
func TestMalformedEnvelopesAreRejectedWithoutIngesting(t *testing.T) {
	valid := []byte(`{"hook_event_name":"PostToolUse"}`)

	for _, tc := range []struct {
		name string
		req  []byte
	}{
		{"wrong version", nil},
		{"unknown type", nil},
		{"empty ingest id on IngestEvent", nil},
		{"not json", []byte(`{"version":`)},
		{"empty frame", []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			switch tc.name {
			case "wrong version":
				req = request(t, "0", ipc.IngestEvent, "id-1", valid)
			case "unknown type":
				req = request(t, ipc.Version, ipc.RequestType("Delete"), "id-1", valid)
			case "empty ingest id on IngestEvent":
				req = request(t, ipc.Version, ipc.IngestEvent, "", valid)
			}

			rec := &recorder{status: ipc.Committed}
			name, _ := startServer(t, rec.ingest)

			ack := exchange(t, name, req)

			if ack.Status != ipc.Rejected {
				t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
			}
			if n := len(rec.seen()); n != 0 {
				t.Errorf("ingest was called %d times for a malformed envelope", n)
			}
		})
	}
}

// TestValidateNamesTheReason pins which check fired. The wire test above can
// only see "rejected"; without this, all five malformed cases could be
// rejected for one wrong reason and both tests would stay green.
func TestValidateNamesTheReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  ipc.Envelope
		want error
	}{
		{"good ingest", ipc.Envelope{Version: ipc.Version, Type: ipc.IngestEvent, IngestID: "x"}, nil},
		{"good status", ipc.Envelope{Version: ipc.Version, Type: ipc.Status}, nil},
		{"empty version", ipc.Envelope{Type: ipc.IngestEvent, IngestID: "x"}, errVersion},
		{"future version", ipc.Envelope{Version: "2", Type: ipc.IngestEvent, IngestID: "x"}, errVersion},
		{"unknown type", ipc.Envelope{Version: ipc.Version, Type: "Delete", IngestID: "x"}, errRequestType},
		{"empty type", ipc.Envelope{Version: ipc.Version, IngestID: "x"}, errRequestType},
		{"no ingest id", ipc.Envelope{Version: ipc.Version, Type: ipc.IngestEvent}, errIngestID},
		// Version is checked before type, so a stale relay is told the
		// honest reason rather than being told its request type is unknown
		// when the real answer is that this build no longer speaks its
		// protocol.
		{"both wrong", ipc.Envelope{Version: "0", Type: "Delete"}, errVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.env)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("validate = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("validate = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestIngestErrorRejectsEvenIfTheHandlerSaidCommitted pins the one place the
// handler's status is overruled. A handler answering Committed alongside an
// error is wrong one way or the other; Rejected is the wrong answer that
// cannot lose the event, because the relay spools it and I-05 makes the
// replay of an event that did commit a no-op.
func TestIngestErrorRejectsEvenIfTheHandlerSaidCommitted(t *testing.T) {
	rec := &recorder{status: ipc.Committed, err: errors.New("disk on fire")}
	name, _ := startServer(t, rec.ingest)

	ack := exchange(t, name, request(t, ipc.Version, ipc.IngestEvent, "id-1", []byte(`{}`)))

	if ack.Status != ipc.Rejected {
		t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
	}
	if n := len(rec.seen()); n != 1 {
		t.Errorf("ingest called %d times, want 1", n)
	}
}

// TestMidFrameDisconnectDoesNotStopTheLoop is gate clause 3. The assertion
// that carries it is the second exchange: an accept loop that died on the
// first client's read error looks exactly like one that is idle, so proving
// the NEXT client is served is the only proof there is.
func TestMidFrameDisconnectDoesNotStopTheLoop(t *testing.T) {
	rec := &recorder{status: ipc.Committed}
	name, _ := startServer(t, rec.ingest)

	// Three ways to die, all before a complete frame arrives.
	for _, partial := range [][]byte{
		nil,                                // connect, then close without writing
		{0x40},                             // half a length header
		{0x40, 0x00, 0x00, 0x00, 'x', 'y'}, // a header claiming 64 bytes, then 2
	} {
		conn := dial(t, name)
		if len(partial) > 0 {
			if _, err := conn.Write(partial); err != nil {
				t.Fatalf("write partial frame: %v", err)
			}
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("close mid-frame: %v", err)
		}
	}

	const id = "id-after-the-broken-clients"
	ack := exchange(t, name, request(t, ipc.Version, ipc.IngestEvent, id, []byte(`{}`)))
	if err := ack.Verify(id); err != nil {
		t.Fatalf("the next client was not served: %v", err)
	}
	if n := len(rec.seen()); n != 1 {
		t.Errorf("ingest called %d times, want 1", n)
	}
}

// TestOversizedFrameDoesNotStopTheLoop covers the other read failure the
// codec can produce: a length header naming more than ipc.MaxFrameLen, which
// ReadFrame refuses before allocating anything.
func TestOversizedFrameDoesNotStopTheLoop(t *testing.T) {
	rec := &recorder{status: ipc.Committed}
	name, _ := startServer(t, rec.ingest)

	conn := dial(t, name)
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], ipc.MaxFrameLen+1)
	if _, err := conn.Write(header[:]); err != nil {
		t.Fatalf("write oversized header: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	const id = "id-after-the-oversized-frame"
	ack := exchange(t, name, request(t, ipc.Version, ipc.IngestEvent, id, []byte(`{}`)))
	if err := ack.Verify(id); err != nil {
		t.Fatalf("the next client was not served: %v", err)
	}
}

// TestConcurrentClientsAreEachAnswered exercises the one thing goroutine-per-
// connection buys, and it is also what gives ./scripts/race.sh something to
// look at in this package: without it every test here drives exactly one
// connection at a time and a green race detector would mean nothing.
//
// Each client sends a distinct ingest id and checks the ACK echoes its own
// back, so a server that crossed two connections' state fails on identity
// rather than on a count that happens to add up.
func TestConcurrentClientsAreEachAnswered(t *testing.T) {
	const clients = 16

	rec := &recorder{status: ipc.Committed}
	name, _ := startServer(t, rec.ingest)

	var wg sync.WaitGroup
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := "id-" + strconv.Itoa(i)
			payload := []byte(`{"n":` + strconv.Itoa(i) + `}`)
			ack := exchange(t, name, request(t, ipc.Version, ipc.IngestEvent, id, payload))
			if err := ack.Verify(id); err != nil {
				t.Errorf("client %d: ack.Verify: %v", i, err)
			}
		}()
	}
	wg.Wait()

	seen := rec.seen()
	if len(seen) != clients {
		t.Fatalf("ingest called %d times, want %d", len(seen), clients)
	}
	ids := make(map[string][]byte, clients)
	for _, env := range seen {
		ids[env.IngestID] = env.Payload
	}
	for i := range clients {
		id := "id-" + strconv.Itoa(i)
		want := []byte(`{"n":` + strconv.Itoa(i) + `}`)
		got, ok := ids[id]
		if !ok {
			t.Errorf("ingest never saw %q", id)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%q carried payload %q, want %q", id, got, want)
		}
	}
}

// pipeGoroutines returns the stacks of every goroutine currently running
// inside this package or inside go-winio's pipe listener.
//
// It reads WriteTo and not Count. Count answers with a number, and a number
// cannot say WHICH goroutine stayed behind; for the goroutine-leak profile it
// is worse than useless, because it reports 0 until a GC cycle has run the
// detection. WriteTo with debug=1 writes one stack per goroutine, and the
// stack is the evidence.
//
// go-winio's ioCompletionProcessor is deliberately not matched: it is a
// process-wide IOCP pump started once behind a sync.Once and it never exits,
// so treating it as a leak would make this check fail forever.
func pipeGoroutines() []string {
	return goroutinesMatching(
		"internal/pipe.Serve",
		"internal/pipe.serveConn",
		"listenerRoutine",
	)
}

// goroutinesMatching returns the stack of every goroutine whose profile entry
// mentions one of markers.
func goroutinesMatching(markers ...string) []string {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 1); err != nil {
		return []string{"goroutine profile unavailable: " + err.Error()}
	}
	var found []string
	for _, stack := range strings.Split(buf.String(), "\n\n") {
		for _, m := range markers {
			if strings.Contains(stack, m) {
				found = append(found, stack)
				break
			}
		}
	}
	return found
}

// waitUntilAccepted blocks until a serveConn goroutine exists.
//
// It is not a nicety. A dial returns as soon as the client's CreateFile
// succeeds, which is before Accept has handed the connection to serveConn.
// Closing the listener inside that window makes go-winio abandon the
// half-accepted connection and answer ErrPipeListenerClosed, so Serve returns
// with no connection goroutine to wait for - and a test that closed there
// would pass whether or not the code under test works.
//
// Measured, not theorised: the stalled-client test below passed in 0.01 s with
// the connection deadline deleted, until this wait was added.
func waitUntilAccepted(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for len(goroutinesMatching("internal/pipe.serveConn")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no serveConn goroutine appeared within 10s of the dial")
		}
		runtime.Gosched()
	}
}

// assertNoPipeGoroutines polls, because a goroutine that has returned is not
// removed from the profile at the instant its last statement runs.
func assertNoPipeGoroutines(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		left := pipeGoroutines()
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutine(s) left running after Close:\n\n%s",
				len(left), strings.Join(left, "\n\n"))
		}
		runtime.Gosched()
	}
}

// TestCloseStopsServeAndLeavesNoGoroutine is gate clause 4.
func TestCloseStopsServeAndLeavesNoGoroutine(t *testing.T) {
	rec := &recorder{status: ipc.Committed}
	name, stop := startServer(t, rec.ingest)

	// Serve one request first, so the accept loop and a connection
	// goroutine have both really existed by the time Close lands.
	const id = "id-before-close"
	if err := exchange(t, name, request(t, ipc.Version, ipc.IngestEvent, id, []byte(`{}`))).Verify(id); err != nil {
		t.Fatalf("ack.Verify: %v", err)
	}

	requireEndedByClose(t, stop())

	assertNoPipeGoroutines(t)
}

// TestCancellingTheContextStopsServe covers the other shutdown route. Without
// it, a caller that cancels the context and waits for Serve waits forever.
func TestCancellingTheContextStopsServe(t *testing.T) {
	name := uniquePipeName(t)
	l, err := Listen(name, currentSID(t))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, l, Handler{Ingest: (&recorder{status: ipc.Committed}).ingest}) }()

	cancel()
	select {
	case err := <-done:
		requireEndedByClose(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return within 10s of the context being cancelled")
	}

	// Close after Serve has returned is a no-op winio tolerates; it also
	// proves Serve is not holding the listener open.
	if err := l.Close(); err != nil {
		t.Errorf("close after cancel: %v", err)
	}
	assertNoPipeGoroutines(t)
}

// TestAStalledClientDoesNotHoldServeOpen is why serveConn sets a deadline at
// all. A client that dials and then says nothing would otherwise pin a
// connection goroutine forever, and Serve waits for those goroutines before
// returning - so a single stalled client would turn Close into a hang.
func TestAStalledClientDoesNotHoldServeOpen(t *testing.T) {
	restore := requestTimeout
	requestTimeout = 150 * time.Millisecond
	t.Cleanup(func() { requestTimeout = restore })

	rec := &recorder{status: ipc.Committed}
	name, stop := startServer(t, rec.ingest)

	// Dial and say nothing. The connection is deliberately not closed
	// before Close: the deadline, not the client, has to be what frees it.
	stalled := dial(t, name)
	t.Cleanup(func() { _ = stalled.Close() })
	waitUntilAccepted(t)

	requireEndedByClose(t, stop())

	// Checked here and not left to assertNoPipeGoroutines below, because
	// that one polls and this claim is about an instant: Serve promises it
	// does not return until its connection goroutines have finished, and a
	// poll with a 10 s budget cannot tell "waited for it" from "outlived it
	// by 150 ms". Deleting Serve's defer wg.Wait() was measured to leave
	// every other test in this package passing; this is the assertion that
	// fails.
	if left := goroutinesMatching("internal/pipe.serveConn"); len(left) != 0 {
		t.Errorf("Serve returned while %d connection goroutine(s) were still reading:\n\n%s",
			len(left), strings.Join(left, "\n\n"))
	}

	if n := len(rec.seen()); n != 0 {
		t.Errorf("ingest called %d times for a client that sent nothing", n)
	}
	assertNoPipeGoroutines(t)
}

// TestServeRefusesANilIngestFunc turns a wiring mistake into a startup error
// instead of a nil-pointer panic inside a goroutine on the first real event.
func TestServeRefusesANilIngestFunc(t *testing.T) {
	l, _ := listen(t)
	err := Serve(t.Context(), l, Handler{})
	if !errors.Is(err, errNoIngest) {
		t.Errorf("Serve with no Ingest = %v, want errNoIngest", err)
	}
}

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

// statusRequest is the frame a CLI sends. Status carries no ingest id and no
// payload - the id is meaningful only for IngestEvent (see ipc.Envelope).
func statusRequest(t *testing.T) []byte {
	t.Helper()
	return request(t, ipc.Version, ipc.Status, "", []byte("null"))
}

// TestStatusIsAnsweredWithAStatusReply. The handler's numbers come back
// unchanged and the two protocol fields are the server's, not the handler's:
// the reply below is built with a wrong version and a wrong type on purpose, so
// that "Serve stamps them" is asserted rather than assumed from a handler that
// happened to fill them in correctly.
func TestStatusIsAnsweredWithAStatusReply(t *testing.T) {
	want := ipc.StatusReply{
		Version:      "not the wire version",
		Type:         "not a status reply",
		SpoolDepth:   7,
		Events:       9001,
		UptimeMS:     1234,
		DatabasePath: `Z:\service\engramux.db`,
		// The breakdown travels the same way the scalars do (I-08). The
		// empty event name is the shape a payload with no
		// hook_event_name produces, and it has to survive the wire as
		// itself rather than as an omitted field.
		Cells: []ipc.Cell{
			{Host: "claude-code", EventName: "PostToolUse", Count: 310, FirstSeenMS: 11, LastSeenMS: 12},
			{Host: "unknown", EventName: "", Count: 1, FirstSeenMS: 13, LastSeenMS: 13},
		},
	}
	name, _ := startHandler(t, Handler{
		Ingest: (&recorder{status: ipc.Committed}).ingest,
		Status: func(context.Context) (ipc.StatusReply, error) { return want, nil },
	})

	raw := exchangeRaw(t, name, statusRequest(t))
	var got ipc.StatusReply
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if err := got.Verify(); err != nil {
		t.Fatalf("Verify: %v (reply = %q)", err, raw)
	}
	if got.SpoolDepth != want.SpoolDepth || got.Events != want.Events ||
		got.UptimeMS != want.UptimeMS || got.DatabasePath != want.DatabasePath ||
		!slices.Equal(got.Cells, want.Cells) {
		t.Errorf("the reply is not the handler's numbers\n got %+v\nwant %+v", got, want)
	}
}

// TestAStatusReplyIsNotAnAck is the other half of choosing a second reply
// document: a caller that verifies it as an ACK must not be able to accept it.
// ipc.Ack.Verify's three-way check is what the relay's delivery decision rests
// on (spec 5.3), and it has to stay exactly that whatever else travels on this
// pipe.
func TestAStatusReplyIsNotAnAck(t *testing.T) {
	name, _ := startHandler(t, Handler{
		Ingest: (&recorder{status: ipc.Committed}).ingest,
		Status: func(context.Context) (ipc.StatusReply, error) { return ipc.StatusReply{Events: 3}, nil },
	})

	raw := exchangeRaw(t, name, statusRequest(t))
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if err := ack.Verify(""); err == nil {
		t.Errorf("a status reply verified as a committed ACK: %q", raw)
	}
}

// TestStatusWithoutAHandlerIsRejected. A Handler that does not implement Status
// refuses it exactly as the three types Phase 1 does not implement are refused,
// and the refusal is an ACK - which is not a status reply, so a client cannot
// read zeroes out of it and print them.
func TestStatusWithoutAHandlerIsRejected(t *testing.T) {
	name, _ := startServer(t, (&recorder{status: ipc.Committed}).ingest)

	raw := exchangeRaw(t, name, statusRequest(t))
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if ack.Status != ipc.Rejected {
		t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
	}
	var reply ipc.StatusReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("decode as a status reply: %v", err)
	}
	if err := reply.Verify(); !errors.Is(err, ipc.ErrStatusType) {
		t.Errorf("StatusReply.Verify = %v, want ErrStatusType", err)
	}
}

// TestAFailingStatusHandlerIsRejected. Half a status is worse than none: a
// caller cannot tell a zero that was read from a zero that was never filled in,
// so a handler that could not answer produces a refusal and not a reply.
func TestAFailingStatusHandlerIsRejected(t *testing.T) {
	boom := errors.New("the database is not answering")
	name, _ := startHandler(t, Handler{
		Ingest: (&recorder{status: ipc.Committed}).ingest,
		Status: func(context.Context) (ipc.StatusReply, error) {
			return ipc.StatusReply{Events: 9001}, boom
		},
	})

	raw := exchangeRaw(t, name, statusRequest(t))
	var reply ipc.StatusReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if err := reply.Verify(); err == nil {
		t.Fatalf("a failed status was answered with an acceptable status reply: %q", raw)
	}
	if reply.Events != 0 {
		t.Errorf("the failed handler's numbers reached the wire: %q", raw)
	}
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("decode as an ack: %v", err)
	}
	if ack.Status != ipc.Rejected {
		t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
	}
}

// requireEndedByClose asserts that Serve stopped because its listener closed,
// and it accepts the two spellings Windows has for that.
//
// [net.ErrClosed] is the one a reader expects: Serve's [context.AfterFunc]
// closes the listener, the accept loop comes round, and the closed listener
// refuses it. The other spelling appears when the close lands while an Accept
// is ALREADY blocked in an overlapped read - the I/O is cancelled rather than
// refused, and winio surfaces ERROR_OPERATION_ABORTED. Both mean the same
// thing and which one happens is a race with no winner worth preferring.
//
// Measured before this existed: over 1,200 runs of
// TestCancellingTheContextStopsServe, 7 failed - about 0.6% - and all 7 carried
// the identical abort text. A suite whose greenness gates a phase cannot afford
// a one-in-a-hundred-and-seventy spurious red.
//
// It stays narrow on purpose. Anything that is not one of these two still
// fails: a timeout, an access denial, a handler's own error. Widening it to
// "some error came back" would pass against a Serve that returned for the wrong
// reason, which is what the assertion exists to rule out.
func requireEndedByClose(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Serve returned nil; it always returns the error that ended the accept loop")
	}
	if endedByClose(err) {
		return
	}
	t.Errorf("Serve returned %v, want net.ErrClosed or ERROR_OPERATION_ABORTED (%d) - "+
		"the two ways a closed listener ends an accept, and nothing else should end it",
		err, uintptr(syscall.ERROR_OPERATION_ABORTED))
}

// endedByClose is the decision above, split out so it can be tested. A helper
// that only ever runs against a passing case is a helper nobody has shown to
// reject anything.
func endedByClose(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ERROR_OPERATION_ABORTED)
}

// TestEndedByCloseRejectsEveryOtherError is [endedByClose]'s non-vacuity guard.
// The two accepted sentinels are asserted wrapped as well as bare, because that
// is how they arrive - Serve wraps its accept error - and a predicate written
// with == instead of errors.Is would pass the bare cases and fail the wrapped
// ones.
func TestEndedByCloseRejectsEveryOtherError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"net.ErrClosed", net.ErrClosed, true},
		{"net.ErrClosed wrapped, as Serve returns it", fmt.Errorf("pipe: accept: %w", net.ErrClosed), true},
		{"ERROR_OPERATION_ABORTED", syscall.ERROR_OPERATION_ABORTED, true},
		{"ERROR_OPERATION_ABORTED wrapped", fmt.Errorf("pipe: accept: %w", syscall.ERROR_OPERATION_ABORTED), true},
		{"nil is not a close", nil, false},
		{"a deadline is not a close", os.ErrDeadlineExceeded, false},
		{"an access denial is not a close", syscall.ERROR_ACCESS_DENIED, false},
		{"a bare error is not a close", errors.New("accept: something else"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := endedByClose(tc.err); got != tc.want {
				t.Errorf("endedByClose(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
