// Command engramux is the hook relay and the CLI. It is a console-subsystem
// binary so that it inherits the host's stdio when a hook invokes it, and one
// process runs per hook event (spec 5.1).
//
// # I-03 is the shape of this program
//
// A hook never blocks its host. This process exits 0 on every path, including
// panic: if the service is down, exit 0; if the pipe does not exist, exit 0;
// if the payload is garbage, exit 0; if the code panics, exit 0. A non-zero
// exit from a hook is the host's problem, and this product exists because the
// alternative breaks constantly on Windows.
//
// That is enforced structurally rather than by discipline. os.Exit is not
// called anywhere in this binary: main returns, which is exit 0, and the one
// deferred handler that runs on the way out recovers whatever was in flight.
// Adding an os.Exit would skip that handler, so its absence is the invariant.
//
// Two things follow from it and shape everything below:
//
//   - Stdin is read to completion before anything that can fail. Every later
//     failure path then still has the bytes and can spool them (I-04); a relay
//     that dies before reading stdin has nothing to save.
//   - A panic on a goroutine other than the one holding the recover kills the
//     process outright, exit code and all. The only goroutine here is the one
//     reading stdin, and it carries its own recover.
//
// The relay writes nothing on stdout, on any event (spec 4.5). Empty stdout is
// accepted by all 11 events on both hosts, and 1.0 is pull-only, so there is
// nothing to say. Failures go to stderr.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/spool"
)

// The timeout budget (spec 5.3). The total is a ceiling over the whole
// process, not the sum of the parts: a dial that returns in 199 ms buys the
// ACK no extra time.
const (
	totalBudget    = 1 * time.Second
	dialBudget     = 200 * time.Millisecond
	postDialBudget = 800 * time.Millisecond
)

var (
	errPayloadJSON  = errors.New("relay: stdin is not a JSON document")
	errStdinTimeout = errors.New("relay: stdin did not close inside the total budget")
	errPanic        = errors.New("relay: panic")
)

func main() {
	// Started before anything else, because the budget it anchors is the
	// wall clock this process is allowed, not the wall clock of the part
	// that talks to the service.
	start := time.Now()

	ev := &event{}
	// The single deferred handler, registered before the first thing that
	// can fail. It is the only recover in this binary - delete it and I-03
	// goes with it - and the only caller of the spool, so an event is
	// saved once or not at all.
	defer ev.settle()

	ev.payload, ev.err = readStdin(os.Stdin, start.Add(totalBudget))
	if ev.err != nil {
		return
	}

	// Minted once, before the first send attempt, and reused by every
	// retry including the spool record's name (I-05). It is the
	// idempotency key, so a fresh one on the spool path would let the
	// drain store the same event a second time.
	//
	// It is minted after stdin is read, because minting can fail and the
	// rule is that nothing which can fail runs before the bytes are in
	// hand.
	id, err := uuid.NewV7()
	if err != nil {
		ev.err = fmt.Errorf("relay: mint the ingest id: %w", err)
		return
	}
	ev.id = id.String()

	ev.err = deliver(start, ev.id, ev.payload)
}

// event is what the deferred handler needs to know: the bytes, the id they
// were sent under, and why the send failed. A nil err means the service
// committed it.
type event struct {
	payload []byte
	id      string
	err     error
}

// settle is the exit path. It recovers a panic, decides whether the event
// still has to be saved, and returns normally - which is what makes main
// return, which is exit 0 (I-03).
func (e *event) settle() {
	if p := recover(); p != nil {
		// debug.Stack is worth the bytes: this is the only record that a
		// panic happened at all, since the process is about to report
		// success to its host.
		warn("panic: %v\n%s", p, debug.Stack())
		if e.err == nil {
			e.err = errPanic
		}
	}
	if e.err == nil {
		return
	}
	warn("event not delivered: %v", e.err)

	// No id means the mint failed or stdin never finished, and neither
	// produces something the drain could replay: a record with no id has no
	// identity, and half a payload is not the event. Say so rather than
	// writing a file nothing can use.
	if e.id == "" {
		warn("nothing spooled: the event has no id")
		return
	}

	dir, err := spool.Dir()
	if err == nil {
		err = spool.Write(dir, e.id, e.payload)
	}
	if err != nil {
		warn("spool %s: %v", e.id, err)
		return
	}
	warn("spooled %s", e.id)
}

// warn writes one line to stderr. Stdout is never written to (spec 4.5).
func warn(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "engramux: "+format+"\n", args...)
}

// readStdin reads r to completion, or gives up at deadline.
//
// The deadline is not decoration. A host that opens the hook's stdin and never
// closes it would park io.ReadAll forever, and a relay that never returns is a
// hook that blocks its host - the one thing I-03 forbids. os.Stdin has no
// SetReadDeadline on Windows, so the read happens on its own goroutine and
// this one races it against the clock.
//
// That goroutine carries its own recover because main's cannot see it: a panic
// on any other goroutine takes the process down with a non-zero status, and no
// deferred handler in main runs at all.
//
// Nothing waits for the goroutine. When main returns the process exits, and a
// goroutine parked in a read syscall goes with it.
func readStdin(r io.Reader, deadline time.Time) ([]byte, error) {
	type read struct {
		b   []byte
		err error
	}
	// Buffered, so the goroutine never blocks on a send this function has
	// stopped listening for.
	ch := make(chan read, 1)

	go func() {
		defer func() {
			if p := recover(); p != nil {
				ch <- read{err: fmt.Errorf("%w reading stdin: %v\n%s", errPanic, p, debug.Stack())}
			}
		}()
		b, err := io.ReadAll(r)
		ch <- read{b: b, err: err}
	}()

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("relay: read stdin: %w", res.err)
		}
		return res.b, nil
	case <-timer.C:
		return nil, errStdinTimeout
	}
}

// deliver sends one event and waits for an ACK it can accept.
//
// Success is only an ACK whose version matches, whose status is committed, and
// whose ingest id equals the one sent (spec 5.3) - ipc.Ack.Verify is that
// check, and it is the only path here that decides the send worked. Everything
// else, including a rejected ACK, returns an error, and the caller spools
// (I-04).
func deliver(start time.Time, id string, payload []byte) error {
	deadline := start.Add(totalBudget)

	// Built before the dial, so a payload that can never travel does not
	// cost a connection. An empty or malformed stdin lands here.
	frame, err := encodeEnvelope(id, payload)
	if err != nil {
		return err
	}

	name, err := ipc.CurrentPipeName()
	if err != nil {
		return err
	}

	// winio.DialPipeContext fails immediately when the pipe does not
	// exist - it retries only ERROR_PIPE_BUSY - which is what the relay
	// wants: no service means spool now, not spool in 200 ms.
	dialCtx, cancel := context.WithDeadline(context.Background(), earliest(start.Add(dialBudget), deadline))
	defer cancel()
	conn, err := dial(dialCtx, name)
	if err != nil {
		return fmt.Errorf("relay: dial %s: %w", name, err)
	}
	defer func() { _ = conn.Close() }()

	// The post-dial budget is measured from now, and clamped by the total:
	// a slow dial eats into it rather than extending the ceiling.
	if err := conn.SetDeadline(earliest(time.Now().Add(postDialBudget), deadline)); err != nil {
		return fmt.Errorf("relay: set the post-dial deadline: %w", err)
	}

	if err := ipc.WriteFrame(conn, frame); err != nil {
		return fmt.Errorf("relay: send the event: %w", err)
	}
	raw, err := ipc.ReadFrame(conn)
	if err != nil {
		return fmt.Errorf("relay: read the ack: %w", err)
	}
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		return fmt.Errorf("relay: decode the ack: %w", err)
	}
	if err := ack.Verify(id); err != nil {
		return fmt.Errorf("relay: %w", err)
	}
	return nil
}

// encodeEnvelope builds the frame payload: an ipc.Envelope carrying payload
// verbatim.
//
// It concatenates rather than calling json.Marshal on an ipc.Envelope, and
// that is not a micro-optimisation. encoding/json compacts a json.RawMessage
// on the way out and HTML-escapes '<', '>' and '&' inside it, so marshalling
// would rewrite bytes that the store writes verbatim into a TEXT column and
// that Phase 1 gates on round-tripping unchanged.
//
// The three fields spliced in as literals are compile-time constants of this
// program's own choosing, neither of which contains a character JSON would
// need to escape. Only the id is encoded, because it is the one value that
// comes from somewhere else.
//
// A payload that is not a valid JSON document is refused. No envelope can
// carry it - the frame would not parse, so the service could not even read the
// ingest id off it - so there is nothing to send and the caller spools the
// bytes instead. Empty stdin lands here too: zero bytes is not a document.
func encodeEnvelope(id string, payload []byte) ([]byte, error) {
	if !json.Valid(payload) {
		return nil, fmt.Errorf("%w: %d bytes", errPayloadJSON, len(payload))
	}
	idJSON, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("relay: encode the ingest id: %w", err)
	}

	var b bytes.Buffer
	b.Grow(len(payload) + 96)
	b.WriteString(`{"version":"` + ipc.Version + `","type":"` + string(ipc.IngestEvent) + `","ingest_id":`)
	b.Write(idJSON)
	b.WriteString(`,"payload":`)
	b.Write(payload)
	b.WriteByte('}')
	return b.Bytes(), nil
}

// earliest returns whichever instant comes first. It is how a stage budget is
// clamped by the total one.
func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
