package pipe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
)

var (
	errNoIngest    = errors.New("pipe: Serve was given a nil IngestFunc")
	errVersion     = errors.New("pipe: unsupported protocol version")
	errRequestType = errors.New("pipe: unknown request type")
	errIngestID    = errors.New("pipe: IngestEvent carries no ingest id")
)

// requestTimeout bounds the I/O on one accepted connection - the wait for the
// request frame and the write of the ACK. It does not bound the [IngestFunc]:
// a handler waiting out SQLite's 10 s busy_timeout (spec 5.4) is not
// interrupted by it, because abandoning a write already in flight is worse
// than answering late.
//
// It exists because [Serve] waits for its connection goroutines before
// returning, so a client that dials and then says nothing would otherwise
// turn a caller's Close into a hang.
//
// 2 s is twice the relay's entire 1 s wall-clock budget (spec 5.3), which
// keeps the service's own budget deliberately larger than the relay's: the
// service never gives up on a relay still inside its own limits.
//
// It is a var so a test can shrink it; nothing else writes to it.
var requestTimeout = 2 * time.Second

// IngestFunc stores one event and answers the status that goes on the wire.
//
// It exists because internal/ipc cannot import internal/store - store imports
// ipc, not the other way round - so the accept loop is handed the database
// path rather than reaching for it. The service's implementation is a closure
// over the pool calling store.Ingest with store.SourcePipe; the drain calls
// the same store.Ingest with store.SourceSpool and never comes through here.
//
// The returned status is what the ACK carries, with one exception stated in
// [Serve]'s routing: a non-nil error forces ipc.Rejected whatever the status
// says.
type IngestFunc func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error)

// Serve accepts connections on l and answers each one, until l is closed or
// ctx is cancelled.
//
// It always returns a non-nil error - the one that ended the accept loop -
// the way http.Server.Serve does. On an orderly shutdown, by either route,
// that error satisfies errors.Is(err, net.ErrClosed); the caller decides
// whether the shutdown was expected, because nothing here can tell.
//
// When it returns, every goroutine it started has finished. A connection
// whose client stalls is bounded by requestTimeout rather than by the client,
// so that promise does not depend on the other end behaving.
//
// One connection carries one request and one ACK. That is the relay's shape -
// a transient process per hook event, alive about 11 ms (spec 5.1) - and
// keeping it means a client cannot hold a connection between events.
func Serve(ctx context.Context, l net.Listener, ingest IngestFunc) error {
	if ingest == nil {
		return errNoIngest
	}

	var wg sync.WaitGroup
	// Registered first so it runs last: "Close returned, therefore nothing
	// of ours is still running" is a property a caller can rely on rather
	// than a race it has to sleep through.
	defer wg.Wait()

	// context.AfterFunc rather than a goroutine parked on ctx.Done(): that
	// goroutine would itself be the leak this function claims not to have
	// whenever ctx is never cancelled, which for a service's root context
	// is the normal case. stop releases the registration either way.
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()

	for {
		conn, err := l.Accept()
		if err != nil {
			return fmt.Errorf("pipe: accept: %w", err)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			serveConn(ctx, conn, ingest)
		}()
	}
}

// serveConn reads one request from conn, answers it, and closes conn.
//
// Every failure here is logged and dropped rather than returned: there is
// nobody to return it to, and taking the accept loop down over one bad client
// is the failure mode gate clause 3 exists to catch. The client is not left
// guessing either - it either gets an ACK it can check with ipc.Ack.Verify,
// or it gets a closed connection, and both make its own timeout fire.
func serveConn(ctx context.Context, conn net.Conn, ingest IngestFunc) {
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(requestTimeout)); err != nil {
		slog.WarnContext(ctx, "pipe: set connection deadline", "error", err)
		return
	}

	payload, err := ipc.ReadFrame(conn)
	if err != nil {
		// No ACK: the frame never arrived, so there is no ingest id to
		// echo and nothing to have an opinion about. The relay's own
		// post-dial budget covers this and it spools (I-04).
		slog.WarnContext(ctx, "pipe: read request frame", "error", err)
		return
	}

	var env ipc.Envelope
	status := ipc.Rejected
	if err := json.Unmarshal(payload, &env); err != nil {
		// Reset rather than keep what a partial decode left behind: the
		// ACK echoes IngestID back, and echoing bytes out of a document
		// that did not parse is reflecting unvalidated input.
		env = ipc.Envelope{}
		slog.WarnContext(ctx, "pipe: decode request envelope", "error", err)
	} else {
		status = route(ctx, env, ingest)
	}

	ack, err := json.Marshal(ipc.Ack{
		Version:  ipc.Version,
		Status:   status,
		IngestID: env.IngestID,
	})
	if err != nil {
		slog.ErrorContext(ctx, "pipe: encode ack", "error", err)
		return
	}
	if err := ipc.WriteFrame(conn, ack); err != nil {
		slog.WarnContext(ctx, "pipe: write ack", "error", err)
	}
}

// route decides the ACK status for one decoded envelope.
func route(ctx context.Context, env ipc.Envelope, ingest IngestFunc) ipc.AckStatus {
	if err := validate(env); err != nil {
		slog.WarnContext(ctx, "pipe: rejected a malformed request", "error", err)
		return ipc.Rejected
	}

	if env.Type != ipc.IngestEvent {
		// The other four types belong to the CLI reads I-08 routes over
		// this pipe, and Phase 1 implements none of them. Rejected is the
		// honest answer, and because ipc.Ack.Verify accepts only
		// Committed it cannot be mistaken for success.
		//
		// It carries no reason, because ipc.Ack has no field for one, and
		// inventing that reply schema now would freeze it before the
		// phase that adds a caller has seen the problem. env.Type is safe
		// to log verbatim here and only here: validate has already
		// confirmed it is one of the five constants.
		slog.WarnContext(ctx, "pipe: request type is not implemented in phase 1", "type", env.Type)
		return ipc.Rejected
	}

	status, err := ingest(ctx, env)
	if err != nil {
		// The status is the handler's to decide, except here. A handler
		// answering Committed alongside an error is wrong one way or the
		// other, and Rejected is the wrong answer that cannot lose the
		// event: the relay spools it, the drain replays it, and I-05
		// makes the replay of an event that did commit a no-op.
		slog.ErrorContext(ctx, "pipe: ingest failed", "error", err)
		return ipc.Rejected
	}
	return status
}

// validate is the routing boundary's envelope check, and it is the only one
// there is. internal/store deliberately does not repeat it: store.Ingest
// trusts the id it is handed, because two events sent under one id are one
// event by definition, and minting a replacement there would turn a broken
// relay into duplicate rows instead of a caught bug. That leaves exactly one
// place a malformed envelope can be stopped, and this is it.
//
// All three failures answer ipc.Rejected, and for the empty ingest id that is
// not the data-loss decision it would be for a well-formed event. An event
// whose id is missing has no identity: events.id is the primary key and the
// idempotency key both (I-05), so letting it through would collapse every
// such event onto the single row keyed by "" - silently, permanently, and
// looking like data. Rejected sends it back to the relay instead, which
// spools it and eventually quarantines it (spec 5.6), so a broken relay
// leaves a diagnosable file rather than one absorbed row.
//
// The version is checked before the type on purpose. Spec 5.5's upgrade path
// - drain, stop, replace, start - can leave an old relay binary talking to a
// new service, and a relay speaking a protocol this build does not should be
// told that, not told its request type is unknown.
func validate(env ipc.Envelope) error {
	if env.Version != ipc.Version {
		return fmt.Errorf("%w: got %.64q, want %q", errVersion, env.Version, ipc.Version)
	}

	switch env.Type {
	case ipc.IngestEvent:
		if env.IngestID == "" {
			return errIngestID
		}
	case ipc.Status, ipc.Doctor, ipc.Search, ipc.Drain:
	default:
		// Bounded with a precision: the type is arbitrary bytes off the
		// wire, capped only by ipc.MaxFrameLen, and this string reaches a
		// log.
		return fmt.Errorf("%w: %.64q", errRequestType, env.Type)
	}
	return nil
}
