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
// called anywhere on the relay path: [relay] returns, main returns, and that is
// exit 0, with one deferred handler on the way out recovering whatever was in
// flight. Adding an os.Exit below that point would skip the handler, so its
// absence there is the invariant.
//
// This binary is also the CLI, and `engramux status` has to exit non-zero when
// the service is down (I-08). That path calls os.Exit and is chosen in main
// before the relay's handler is ever registered, so the two cannot interfere:
// a hook invokes this program with no arguments and reaches [relay], and
// nothing with arguments reaches it at all.
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
// The relay writes on stdout for exactly one event, and only when the user has
// turned injection on. Empty stdout is accepted by all 11 events on both hosts,
// and the 1.0 spec §4.5 says the relay writes nothing on any of them - but that
// section's own reasoning is "since 1.0 is pull-only", and the memory spec
// rev.8's M-4 is the row that changes for after 1.0. So: UserPromptSubmit, with
// injection enabled, writes one hookSpecificOutput document and nothing else
// does. See [injectContext]. Failures go to stderr on every path including that
// one.
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

// main picks which of the two programs in this binary is being run.
//
// A hook invokes it with no arguments - spec 4.2's invocation shape lets us
// configure it that way on both hosts - and everything a person types starts
// with a command word. The split is made here, before the relay's deferred
// handler is registered, and that is what lets `engramux status` exit non-zero
// while I-03 still holds.
//
// I-03 is a promise about the relay, and the relay is the argument-free path:
// it registers settle and calls os.Exit nowhere, so nothing can skip the
// handler that makes a panic exit 0. The CLI path registers no handler and is
// the only place os.Exit appears. Adding an os.Exit below the split would
// break I-03; adding one here cannot.
func main() {
	if len(os.Args) > 1 {
		os.Exit(cli(os.Args[1:]))
	}
	relay()
}

// relay is the hook path: read stdin, deliver or spool, exit 0 (I-03).
func relay() {
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
	ev.payload = trimFraming(ev.payload)

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

	// Injection, and it is deliberately last and deliberately separate from
	// ev.err: capture is the invariant, injection is the feature, and a
	// feature that failed must not make the relay spool an event the
	// service already committed. It ships disabled (memory spec rev.8,
	// M-4), so on an installation nobody has configured this is one failed
	// os.ReadFile and a return.
	//
	// It sits inside relay rather than after it, so that [event.settle]'s
	// recover covers it: a panic here exits 0 like every other panic on
	// this path (I-03).
	injectContext(start, ev.payload, ev.id)
}

// trimFraming strips leading and trailing JSON whitespace from the bytes read
// from stdin. It is where this program decides what the event's bytes are, and
// it runs once, before anything branches.
//
// A trailing newline is the writer's framing, not part of the JSON document,
// and two hosts disagreeing about whether to emit one must not produce two
// different stored payloads for one event. Without this the two delivery paths
// disagree: the envelope splices the payload in as raw JSON, so a trailing
// newline lands between the payload's last byte and the envelope's closing
// brace - structural whitespace, which json.RawMessage discards - while the
// spool has no decoder in its path and keeps every byte. I-05 then hides which
// one the row got, because whichever path commits first is the one
// ON CONFLICT DO NOTHING keeps. Trimming here makes both paths carry the same
// bytes by construction rather than by agreement.
//
// This is the one thing Phase 1's byte-for-byte round trip permits, and it
// still forbids everything it was written to forbid: no re-marshalling, no
// compaction, no key reordering, no HTML escaping, nothing inside the outermost
// JSON value altered. One deterministic normalisation, applied once, before the
// paths diverge.
//
// The set is RFC 8259's four whitespace bytes and not bytes.TrimSpace's
// unicode.IsSpace, which also eats U+0085 and U+00A0. A JSON parser rejects
// those, so trimming them would turn a document this relay has to refuse into
// one it sends.
func trimFraming(b []byte) []byte { return bytes.Trim(b, " \t\r\n") }

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
	// The second recover, and the reason there is one: the recover below
	// catches what main was doing, and nothing catches what happens after
	// it. A panic in spool.Dir, spool.Write or warn would take the process
	// down with a non-zero exit - the one thing I-03 forbids - through the
	// handler whose whole job is to stop that. This is a deferred call of
	// settle, so it covers settle's entire body.
	//
	// There is nothing after it to save the event with, so it reports and
	// returns: an event lost to a panic in the code that saves events is
	// already lost, and a hook that fails its host on the way out is worse.
	defer func() {
		if p := recover(); p != nil {
			warn("panic while settling the event: %v\n%s", p, debug.Stack())
		}
	}()

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

	// ponytail: the 1 s total budget covers readStdin and deliver and stops
	// short of here. The write below enumerates the spool directory - up to
	// maxRecords entries - then writes, fsyncs and renames, under no
	// deadline at all. Spec 5.3 licenses that much, since "blowing any relay
	// limit means spool and exit 0" puts the spool after the limit, and the
	// common case is measured inside the ceiling; the ceiling is that a full
	// or slow spool has no numeric bound. Windows will not cancel an fsync,
	// so the upgrade path is a cheaper sweep before the write rather than a
	// timeout around it.
	dir, err := spool.Dir()
	if err == nil {
		err = spoolWrite(dir, e.id, e.payload)
	}
	if err != nil {
		warn("spool %s: %v", e.id, err)
		return
	}
	warn("spooled %s", e.id)
}

// warn writes one line to stderr. The relay writes nothing on stdout, on any
// event (spec 4.5); the CLI path does, because a person asked it to.
//
// # These lines are not an I-10 egress, and this is the reasoning
//
// Some of them carry the spool path, which is under %LOCALAPPDATA% and so
// contains a Windows user name - a value internal/secret would tag
// ClassUserPath if it ever saw it. It never does: this is fmt.Fprintf to
// os.Stderr and not slog, so secret.NewLogHandler is not in the path, and that
// is deliberate rather than an oversight.
//
// I-10 says a secret "never leaves the machine". Spec 2 puts a single Windows
// SID inside the trust boundary. The relay's stderr goes to the process that
// invoked the hook - the host, running as that same SID, on this machine - so
// nothing leaves anything. Routing these through slog would buy nothing and
// cost the relay a regex compile per hook event in a process that lives about
// 11 ms (spec 5.1).
//
// Claude Code's hook reference was checked rather than assumed, and it closes
// the same door from the other side: hook stderr reaches the persisted session
// transcript ONLY on a non-zero exit - exit 2 in full, any other non-zero exit
// as its first line - while on exit 0 it goes to the debug log and nowhere
// else, and only when debug logging is on.
//
// I-03 makes this process exit 0 on every path, including panic. So the
// invariant that keeps the relay from blocking its host is also what keeps
// these lines out of a file on disk. The two are the same guarantee, which is
// worth knowing before anyone "improves" either one.
//
// Two things would invalidate it, and either one means this has to change
// rather than be re-argued:
//
//   - spec 2's trust boundary stops being one SID. A second user, a service
//     account, or a shared machine makes the host a different principal.
//   - a host starts forwarding hook stderr off the machine - into an uploaded
//     transcript, a telemetry channel, a support bundle. Then this is an
//     egress with no filter on it, and the fix is a filtered handler here, not
//     a quieter message.
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
//
// ponytail: the read is unbounded in size. A host that writes without stopping
// takes this process down with an out-of-memory fatal error, which is not a
// panic and no recover catches it - ipc.ReadFrame checks a length before
// allocating for exactly this reason, and there is no equivalent here. The
// ceiling is that real payloads are small: spec 7.4's largest observed is
// 171,764 B, and anything over ipc.MaxFrameLen already fails at WriteFrame and
// spools, so this is a guard against pathological input rather than a path with
// traffic. The upgrade path is an io.LimitReader, and it needs a number and a
// decision Phase 1 has not made: a cap that truncates produces a corrupt
// payload that looks whole, and a cap that refuses drops an event I-04 says is
// never dropped.
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
