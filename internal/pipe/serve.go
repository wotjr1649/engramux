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
	errNoIngest    = errors.New("pipe: Serve was given a Handler with no Ingest")
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

// StatusFunc answers a Status request with the numbers only the service can
// see (I-08). Version and Type are not its business: [Serve] stamps both, so
// the protocol fields on the wire cannot disagree with the protocol this
// package speaks.
//
// An error answers ipc.Rejected, which the CLI reports as a failure. There is
// no partial status: half-read numbers presented as a service's state are
// worse than a refusal.
type StatusFunc func(ctx context.Context) (ipc.StatusReply, error)

// SearchFunc answers a Search request. It is handed the decoded request
// document, so the routing boundary owns "is this a document at all" and the
// handler owns what the query and the limit mean.
//
// Version and Type are not its business either: [Serve] stamps both, for the
// same reason it stamps a [StatusFunc]'s.
//
// An error answers ipc.Rejected. There is no partial search, and the reason is
// sharper here than for status: an empty hit list is a real answer - nothing
// matched - so a handler that failed must not be able to produce one.
type SearchFunc func(ctx context.Context, req ipc.SearchRequest) (ipc.SearchReply, error)

// GetEventFunc answers a GetEvent request, and [ListSessionsFunc] a
// ListSessions one. Both have [SearchFunc]'s contract exactly: the decoded
// request in, the reply document out, Version and Type stamped by [Serve], and
// an error answering ipc.Rejected because a nil event and an empty session list
// are both real answers that a failed handler must not be able to produce.
type (
	GetEventFunc     func(ctx context.Context, req ipc.GetEventRequest) (ipc.GetEventReply, error)
	ListSessionsFunc func(ctx context.Context, req ipc.ListSessionsRequest) (ipc.ListSessionsReply, error)
)

// Handler is what [Serve] answers requests with - one function per request
// type it implements.
//
// A struct rather than a parameter per type, because spec 5.2's request types
// arrive over several phases and a handler that is not supplied has one
// behaviour: the request is refused. Ingest is the exception and is required,
// because a service that cannot store an event has nothing to serve.
type Handler struct {
	// Ingest stores one event. Required.
	Ingest IngestFunc
	// Status answers a Status request. A nil Status refuses one, exactly
	// as the types this build does not implement are refused.
	Status StatusFunc
	// Search answers a Search request. A nil Search refuses one, the same
	// way.
	Search SearchFunc
	// GetEvent answers a GetEvent request, ListSessions a ListSessions
	// one. A nil field refuses that type, the same way.
	GetEvent     GetEventFunc
	ListSessions ListSessionsFunc
}

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
// One connection carries one request and one reply. That is the relay's shape -
// a transient process per hook event, alive about 11 ms (spec 5.1) - and
// keeping it means a client cannot hold a connection between events.
func Serve(ctx context.Context, l net.Listener, h Handler) error {
	if h.Ingest == nil {
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
			serveConn(ctx, conn, h)
		}()
	}
}

// serveConn reads one request from conn, answers it, and closes conn.
//
// Every failure here is logged and dropped rather than returned: there is
// nobody to return it to, and taking the accept loop down over one bad client
// is the failure mode gate clause 3 exists to catch. The client is not left
// guessing either - it either gets a reply it can check with ipc.Ack.Verify or
// ipc.StatusReply.Verify, or it gets a closed connection, and both make its
// own timeout fire.
func serveConn(ctx context.Context, conn net.Conn, h Handler) {
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(requestTimeout)); err != nil {
		slog.WarnContext(ctx, "pipe: set connection deadline", "error", err)
		return
	}

	payload, err := ipc.ReadFrame(conn)
	if err != nil {
		// No reply: the frame never arrived, so there is no ingest id to
		// echo and nothing to have an opinion about. The relay's own
		// post-dial budget covers this and it spools (I-04).
		slog.WarnContext(ctx, "pipe: read request frame", "error", err)
		return
	}

	var env ipc.Envelope
	var reply []byte
	if err := json.Unmarshal(payload, &env); err != nil {
		// Reset rather than keep what a partial decode left behind: the
		// ACK echoes IngestID back, and echoing bytes out of a document
		// that did not parse is reflecting unvalidated input.
		env = ipc.Envelope{}
		slog.WarnContext(ctx, "pipe: decode request envelope", "error", err)
		reply = encodeAck(ctx, ipc.Rejected, "")
	} else {
		reply = route(ctx, env, h)
	}
	if reply == nil {
		// The reply could not be encoded, which is already logged. There
		// is nothing to send that would be better than silence: the
		// client's own deadline is what covers it.
		return
	}
	// The reply gets its own deadline rather than whatever the handler left
	// of the one above, and this is a fix rather than a tidy-up.
	//
	// One deadline for the whole connection covers the read, the handler and
	// the write. The handler is deliberately not bounded by it - see
	// [requestTimeout] - so a handler that runs long leaves the write
	// nothing, and the write then fails with `i/o timeout` while the client
	// sees the connection close with no frame on it. That was observed once
	// on the installed service and could not be reproduced on demand;
	// TestASlowHandlerStillGetsItsReplyOut reproduces it by making the
	// handler slow, which is the condition that was never suspected.
	//
	// It does not extend what a client may hold a connection for by stalling:
	// a stalled client sends no request, so no handler runs and this line is
	// never reached. The deadline above is still the whole of that bound.
	if err := conn.SetWriteDeadline(time.Now().Add(requestTimeout)); err != nil {
		slog.WarnContext(ctx, "pipe: set the reply deadline", "error", err)
		return
	}
	if err := ipc.WriteFrame(conn, reply); err != nil {
		slog.WarnContext(ctx, "pipe: write reply", "error", err)
	}
}

// route decides the reply document for one decoded envelope. It returns the
// encoded bytes, or nil when encoding failed and there is nothing to send.
//
// The reply document is chosen by the request type, not by the outcome: an
// IngestEvent is answered with an ipc.Ack, a served Status with an
// ipc.StatusReply and a served Search with an ipc.SearchReply. A request this
// build will not serve is answered with a rejected ipc.Ack whatever it asked
// for, which those documents' own Verify methods recognise as not being a
// reply of their kind.
func route(ctx context.Context, env ipc.Envelope, h Handler) []byte {
	if err := validate(env); err != nil {
		slog.WarnContext(ctx, "pipe: rejected a malformed request", "error", err)
		return encodeAck(ctx, ipc.Rejected, env.IngestID)
	}

	switch env.Type {
	case ipc.IngestEvent:
		status, err := h.Ingest(ctx, env)
		if err != nil {
			// The status is the handler's to decide, except here. A
			// handler answering Committed alongside an error is
			// wrong one way or the other, and Rejected is the wrong
			// answer that cannot lose the event: the relay spools
			// it, the drain replays it, and I-05 makes the replay of
			// an event that did commit a no-op.
			slog.ErrorContext(ctx, "pipe: ingest failed", "error", err)
			status = ipc.Rejected
		}
		return encodeAck(ctx, status, env.IngestID)

	case ipc.Status:
		if h.Status == nil {
			slog.WarnContext(ctx, "pipe: this build serves no Status handler")
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		reply, err := h.Status(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "pipe: status failed", "error", err)
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		// Stamped here rather than trusted from the handler: the wire
		// protocol is this package's, and a handler that left them
		// empty would produce a reply no client could accept.
		reply.Version, reply.Type = ipc.Version, ipc.Status
		b, err := json.Marshal(reply)
		if err != nil {
			slog.ErrorContext(ctx, "pipe: encode status reply", "error", err)
			return nil
		}
		return b

	case ipc.Search:
		if h.Search == nil {
			slog.WarnContext(ctx, "pipe: this build serves no Search handler")
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		// The envelope check cannot see inside Payload - its shape is
		// the request type's, not the envelope's - so this is the only
		// place a Search document that is not one can be stopped. The
		// handler is not called with a zero request: an empty query
		// would be refused downstream, but a zero limit means the
		// default, so a document that did not decode would silently
		// become a valid search for nothing.
		var req ipc.SearchRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			slog.WarnContext(ctx, "pipe: decode the search request", "error", err)
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		reply, err := h.Search(ctx, req)
		if err != nil {
			// The query is not logged. It is text a person typed
			// and a log is an egress (I-10, spec 7.5); internal/search
			// keeps its own errors free of it for the same reason.
			slog.ErrorContext(ctx, "pipe: search failed", "error", err)
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		reply.Version, reply.Type = ipc.Version, ipc.Search
		b, err := json.Marshal(reply)
		if err != nil {
			slog.ErrorContext(ctx, "pipe: encode search reply", "error", err)
			return nil
		}
		return b

	case ipc.GetEvent:
		if h.GetEvent == nil {
			slog.WarnContext(ctx, "pipe: this build serves no GetEvent handler")
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		// Decoded here for the reason the Search case is: the envelope
		// check cannot see inside Payload, and a document that did not
		// decode would otherwise become a zero request - which for this
		// type is a request the handler refuses anyway, but only
		// because it validates. Refusing it here does not depend on
		// that.
		var req ipc.GetEventRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			slog.WarnContext(ctx, "pipe: decode the get-event request", "error", err)
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		reply, err := h.GetEvent(ctx, req)
		if err != nil {
			// Neither the id nor the project is logged. The project
			// is a path a caller sent and a log is an egress (I-10);
			// the errors internal/project and internal/ipc return
			// carry their own bounded copy when it is safe to.
			slog.ErrorContext(ctx, "pipe: get event failed", "error", err)
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		reply.Version, reply.Type = ipc.Version, ipc.GetEvent
		b, err := json.Marshal(reply)
		if err != nil {
			slog.ErrorContext(ctx, "pipe: encode get-event reply", "error", err)
			return nil
		}
		return b

	case ipc.ListSessions:
		if h.ListSessions == nil {
			slog.WarnContext(ctx, "pipe: this build serves no ListSessions handler")
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		var req ipc.ListSessionsRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			slog.WarnContext(ctx, "pipe: decode the list-sessions request", "error", err)
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		reply, err := h.ListSessions(ctx, req)
		if err != nil {
			slog.ErrorContext(ctx, "pipe: list sessions failed", "error", err)
			return encodeAck(ctx, ipc.Rejected, env.IngestID)
		}
		reply.Version, reply.Type = ipc.Version, ipc.ListSessions
		b, err := json.Marshal(reply)
		if err != nil {
			slog.ErrorContext(ctx, "pipe: encode list-sessions reply", "error", err)
			return nil
		}
		return b

	default:
		// Doctor and Drain are the CLI reads I-08 routes over this
		// pipe that this build does not implement. Rejected is the
		// honest answer, and because ipc.Ack.Verify accepts only
		// Committed it cannot be mistaken for success.
		//
		// It carries no reason, because ipc.Ack has no field for one.
		// env.Type is safe to log verbatim here and only here: validate
		// has already confirmed it is one of spec 5.2's constants.
		slog.WarnContext(ctx, "pipe: request type is not implemented in this build", "type", env.Type)
		return encodeAck(ctx, ipc.Rejected, env.IngestID)
	}
}

// encodeAck marshals one ACK, returning nil when it cannot - which for three
// strings is a condition that does not occur, and is still not swallowed.
func encodeAck(ctx context.Context, status ipc.AckStatus, ingestID string) []byte {
	b, err := json.Marshal(ipc.Ack{Version: ipc.Version, Status: status, IngestID: ingestID})
	if err != nil {
		slog.ErrorContext(ctx, "pipe: encode ack", "error", err)
		return nil
	}
	return b
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
	case ipc.Status, ipc.Doctor, ipc.Search, ipc.Drain, ipc.GetEvent, ipc.ListSessions:
	default:
		// Bounded with a precision: the type is arbitrary bytes off the
		// wire, capped only by ipc.MaxFrameLen, and this string reaches a
		// log.
		return fmt.Errorf("%w: %.64q", errRequestType, env.Type)
	}
	return nil
}
