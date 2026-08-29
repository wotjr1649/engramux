package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wotjr1649/engramux/internal/mcpconf"
	"github.com/wotjr1649/engramux/internal/pipe"
)

// The transport spec 5.9 fixes.
//
// loopback is the whole of the network access control this endpoint has at the
// interface level, and spec 5.9 is explicit that it is not a principal check:
// any process of any locally logged-on user can open a connection to it, and
// the bearer token is the only thing that decides whether it is answered.
// Windows offers no Winsock equivalent of the named pipe's DACL.
//
// endpointPath is where the handler is mounted, so that a request to any other
// path is a 404 from a mux rather than something the MCP handler has an opinion
// about. It is part of the URL written to mcp.json, which is where the
// installer reads it - this constant is the only place the path is spelled.
const (
	loopback     = "127.0.0.1"
	endpointPath = "/mcp"
)

// readHeaderTimeout bounds how long a client may take to send its request
// headers. It is not the request budget: a tool call's own bound is the read
// gate's query deadline (spec 5.9), and an SSE stream is deliberately unbounded,
// which is why there is no WriteTimeout here.
const readHeaderTimeout = 10 * time.Second

// shutdownTimeout bounds the wait for in-flight tool calls on the way out.
//
// It is not politeness. The caller closes the database as soon as this returns
// (spec 5.4's one connection), so a handler still inside a query when the
// listener stops would be reading a closed pool. A read is already bounded by
// the gate's 4 s query deadline, so this is that plus room to write the reply.
//
// A Streamable HTTP GET stream is long-lived by design and Shutdown waits for
// it, so this timeout is the normal path whenever a host is connected rather
// than the exceptional one. Past it the server is closed outright.
const shutdownTimeout = 5 * time.Second

// Server is a bound MCP endpoint that has not started serving yet.
//
// Binding and serving are separate calls because the endpoint has to be
// knowable before the first request is answered: [Listen] is what mints the
// token and publishes mcp.json, and the caller logs what it published. A test
// gets the same seam, which is what lets spec 8's three transport clauses run
// against the production wiring rather than against a copy of it.
type Server struct {
	ln       net.Listener
	endpoint string
	http     *http.Server
}

// Listen binds the endpoint, mints the bearer token, and publishes both to
// spec 5.6's mcp.json. It does not accept anything until [Server.Serve] runs.
//
// # The port is sticky and is not derived
//
// Spec 5.9 measured this machine's ephemeral range at 1024-15000, not Windows'
// documented 49152-65535, so the usual advice to pick a fixed port in the
// registered range would put it exactly where the allocator lives. There is no
// derivation: the previous start's port is tried first so a host configuration
// holding the URL survives an ordinary restart, and port 0 is the fallback, so
// the bind is its own probe and no race with a prober exists.
//
// The token is minted per start (spec 5.9), so a stale mcp.json never
// authenticates against a new service. That is also why nothing reads a token
// back off disk - see internal/mcpconf.
func Listen(ctx context.Context, dir string, h pipe.Handler) (*Server, error) {
	srv, err := New(h)
	if err != nil {
		return nil, err
	}

	previous, err := mcpconf.URL(dir)
	if err != nil {
		// Not fatal: a file that cannot be read costs the sticky port
		// and nothing else, and the alternative is a service whose MCP
		// endpoint never starts because of a file it is about to
		// replace.
		slog.WarnContext(ctx, "engramux-service: read the published MCP endpoint", "error", err)
	}
	ln, err := listen(ctx, mcpconf.Port(previous))
	if err != nil {
		return nil, err
	}

	// rand.Text is at least 128 bits of randomness in the RFC 4648 base32
	// alphabet, so it needs no encoding step of its own and carries nothing
	// that has to be escaped into a header, a JSON document or a TOML
	// string.
	token := rand.Text()
	endpoint := "http://" + ln.Addr().String() + endpointPath
	if err := mcpconf.Write(dir, endpoint, token); err != nil {
		_ = ln.Close()
		return nil, err
	}

	return &Server{
		ln:       ln,
		endpoint: endpoint,
		http: &http.Server{
			Handler:           guard(srv, token),
			ReadHeaderTimeout: readHeaderTimeout,
		},
	}, nil
}

// Endpoint is the URL [Listen] published. It carries no token.
func (s *Server) Endpoint() string { return s.endpoint }

// Serve answers requests until ctx is cancelled, and returns nil for that
// shutdown.
//
// It does not return until the shutdown has finished, which is the property the
// caller depends on: the database is closed once this returns, so a tool call
// still in a query at that moment would be reading a closed pool.
func (s *Server) Serve(ctx context.Context) error {
	done := make(chan struct{})
	// context.AfterFunc rather than a goroutine parked on ctx.Done(), for
	// the reason internal/pipe's Serve gives: that goroutine is itself a
	// leak whenever ctx is never cancelled.
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		// WithoutCancel because ctx is already cancelled - that is what
		// started this - and Shutdown would otherwise return before it
		// had waited for anything.
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		if err := s.http.Shutdown(sctx); err != nil {
			// A long-lived SSE stream is what usually spends this
			// timeout, and the close is what ends it.
			_ = s.http.Close()
		}
	})
	defer stop()

	err := s.http.Serve(s.ln)
	if errors.Is(err, http.ErrServerClosed) {
		<-done
		return nil
	}
	return fmt.Errorf("mcpserver: serve: %w", err)
}

// listen binds the loopback interface, preferring port, and letting Windows
// choose when that fails or when there is no previous port to prefer.
//
// A [net.ListenConfig] rather than net.Listen so the bind takes the caller's
// context, which is the same reason every other call in this product does.
func listen(ctx context.Context, port int) (net.Listener, error) {
	var lc net.ListenConfig
	if port != 0 {
		ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(loopback, strconv.Itoa(port)))
		if err == nil {
			return ln, nil
		}
		// The sticky port is gone - something else took it while the
		// service was down. The URL in a host configuration is stale
		// until the installer is re-run, and `doctor` is what says so.
		slog.WarnContext(ctx, "engramux-service: the previous MCP port is taken, binding a new one",
			"port", port, "error", err)
	}
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(loopback, "0"))
	if err != nil {
		return nil, fmt.Errorf("mcpserver: bind %s: %w", loopback, err)
	}
	return ln, nil
}

// guard is the two middlewares spec 8's Phase 5 gate names, wrapped around the
// Streamable HTTP handler at [endpointPath].
//
// Cross-origin protection is outermost, so a browser page that reached this
// endpoint is turned away before anything looks at a credential. It is
// [http.NewCrossOriginProtection] and not [mcp.StreamableHTTPOptions]'s field of
// the same name: that field is deprecated, is nil by default, and nil applies
// no protection at all - a previous revision of spec 5.9 said "cross-origin
// protection enabled", which does not describe a default and is corrected
// there. The SDK's own compatibility shim for the old default is telegraphed
// for removal in v1.8.0, so depending on it would be depending on a shim.
//
// DNS-rebinding protection is the SDK's and is on by default
// (DisableLocalhostProtection is false), so a request arriving on loopback with
// a non-loopback Host header is already refused. It is left alone rather than
// restated.
func guard(srv *mcp.Server, token string) http.Handler {
	// Logger is deliberately nil, which the SDK reads as "do not log". A
	// log is an egress (I-10) and this handler sees a bearer token in every
	// request header; nothing in this product has read what the SDK would
	// write.
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)

	mux := http.NewServeMux()
	mux.Handle(endpointPath, h)
	return http.NewCrossOriginProtection().Handler(requireBearer(token, mux))
}

// requireBearer refuses every request that does not carry exactly this start's
// token.
//
// # There is no challenge header
//
// MCP revision 2026-07-28 makes authorization OPTIONAL and binds only servers
// that opt into its OAuth 2.1 framework. The condition spec 5.9 records is that
// a server must not half-claim conformance by advertising a WWW-Authenticate
// challenge naming protected-resource metadata it does not serve. This one
// serves none, so it sends none: a 401 with a body and no header.
//
// The comparison is constant-time. It is a local endpoint and a timing oracle
// over it is a stretch, but the alternative is a string == on a secret, and
// there is no reason to write that one instead.
func requireBearer(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			// The message says what is wrong and repeats nothing
			// back: neither the token that was sent nor the one that
			// was wanted is in it (spec 6.1).
			http.Error(w, "engramux: this endpoint needs the bearer token from mcp.json", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
