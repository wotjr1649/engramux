package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/mcpconf"
	"github.com/wotjr1649/engramux/internal/pipe"
)

// The three tests below are spec 8's Phase 5 transport clauses. They run
// against [Listen] and [Server.Serve] - the production wiring, the same two
// calls internal/service makes - because each mechanism they check is a
// middleware or a bind that a copy of the wiring could get right while the
// service got it wrong.

// TestPhase5GateNoTokenAndAWrongTokenAreBothRefused is the first clause. Spec
// 5.9 is explicit that binding 127.0.0.1 restricts the interface and not the
// principal - any process of any locally logged-on user can open a connection -
// so the bearer token is the only control there is, and its absence has to fail
// rather than degrade.
//
// The wrong token is the same length as a real one, so the constant-time
// comparison is exercised on the branch where it matters rather than on a
// length mismatch it would short-circuit anyway.
//
// Two things are asserted about the refusal beyond its status. The body must
// not repeat the token back, which is spec 6.1 at the one place in this product
// that holds a secret in memory to compare against. And there must be no
// WWW-Authenticate header: MCP revision 2026-07-28 makes authorization optional
// and binds only servers that opt into its OAuth 2.1 framework, so a challenge
// here would be claiming a conformance this server does not have (spec 5.9).
func TestPhase5GateNoTokenAndAWrongTokenAreBothRefused(t *testing.T) {
	endpoint, token := serveForTest(t, stubHandler())

	for _, tc := range []struct {
		name string
		auth string
		want int
	}{
		{"no Authorization header at all", "", http.StatusUnauthorized},
		{"a wrong token of the right length", "Bearer " + strings.Repeat("A", len(token)), http.StatusUnauthorized},
		{"the right token with no scheme", token, http.StatusUnauthorized},
		{"the right token with the wrong scheme", "Basic " + token, http.StatusUnauthorized},
		{"this start's token", "Bearer " + token, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := initialize(t, endpoint, func(r *http.Request) {
				if tc.auth != "" {
					r.Header.Set("Authorization", tc.auth)
				}
			})
			if resp.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want != http.StatusUnauthorized {
				return
			}
			if strings.Contains(body, token) {
				// The body is not printed. A failure here means
				// it carries the token.
				t.Error("the refusal repeats the token back")
			}
			if h := resp.Header.Get("WWW-Authenticate"); h != "" {
				t.Errorf("the refusal advertises a challenge: %q", h)
			}
		})
	}
}

// TestPhase5GateACrossOriginRequestIsRejected is the second clause, and it is
// against the middleware.
//
// [mcp.StreamableHTTPOptions]'s CrossOriginProtection field is deprecated, is
// nil by default, and nil applies nothing - so a test that set it would be
// testing a field telegraphed for removal, and a test that relied on the
// default would be testing no protection at all. What is wrapped around the
// handler is [net/http.NewCrossOriginProtection], and both of the signals it
// reads are exercised here.
//
// The token is valid in every case, so what refuses the request is the origin
// and not the credential. The last case is the control: the same request
// without either header is answered, which is what a non-browser client sends
// and is the only reason this endpoint is usable at all.
func TestPhase5GateACrossOriginRequestIsRejected(t *testing.T) {
	endpoint, token := serveForTest(t, stubHandler())

	for _, tc := range []struct {
		name   string
		header [2]string
		want   int
	}{
		{"an Origin from another site", [2]string{"Origin", "http://evil.example"}, http.StatusForbidden},
		{"Sec-Fetch-Site says cross-site", [2]string{"Sec-Fetch-Site", "cross-site"}, http.StatusForbidden},
		{"neither header, which is what a non-browser client sends", [2]string{}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := initialize(t, endpoint, func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+token)
				if tc.header[0] != "" {
					r.Header.Set(tc.header[0], tc.header[1])
				}
			})
			if resp.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

// TestPhase5GateTheListenerIsOnLoopbackAndNoOtherInterface is the third clause.
//
// It asserts three things, and the first two are what stop it passing on an
// endpoint nobody is listening to: the published URL names 127.0.0.1, and a
// dial of that address on that port connects. Only then does the sweep mean
// anything - every other address this machine holds, plus the IPv6 loopback,
// must refuse or fail to answer on the same port.
//
// ::1 is in the sweep deliberately. It is a loopback address too, so a bind
// that took "localhost" rather than 127.0.0.1 would very likely accept there,
// and neither of the first two assertions would notice.
//
// A dial that times out counts as unreachable, the same as one refused. A
// firewall answering for the interface is not a weaker result than a closed
// port: nothing on the far side got a connection either way.
func TestPhase5GateTheListenerIsOnLoopbackAndNoOtherInterface(t *testing.T) {
	endpoint, _ := serveForTest(t, stubHandler())

	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse the published endpoint: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split the published address: %v", err)
	}
	if host != wantHost {
		t.Fatalf("the endpoint is bound to %q, want %q", host, wantHost)
	}

	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.DialContext(t.Context(), "tcp", net.JoinHostPort(wantHost, port))
	if err != nil {
		t.Fatalf("dial the endpoint's own address: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close the control dial: %v", err)
	}

	for _, addr := range otherLocalAddresses(t) {
		c, err := d.DialContext(t.Context(), "tcp", net.JoinHostPort(addr, port))
		if err == nil {
			_ = c.Close()
			t.Errorf("the endpoint answers on %s, which is not %s", addr, wantHost)
		}
	}
}

// wantHost is spec 5.9's address, written out rather than read from the
// constant the implementation binds. A test comparing the two would move with a
// change to that constant and report nothing, and the break-it pass is what
// showed it matters: binding "0.0.0.0" produced a listener whose own address is
// "::", so only a literal separates that from a pass.
const wantHost = "127.0.0.1"

// otherLocalAddresses is every address this machine holds except 127.0.0.1,
// plus the IPv6 loopback whether or not an interface reports it.
//
// A link-local IPv6 address is skipped: it needs a zone to dial at all, so a
// dial of one fails for a reason that has nothing to do with what is bound.
func otherLocalAddresses(t *testing.T) []string {
	t.Helper()

	out := []string{"::1"}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatalf("read this machine's addresses: %v", err)
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.String() == wantHost || n.IP.IsLinkLocalUnicast() {
			continue
		}
		if s := n.IP.String(); s != "::1" {
			out = append(out, s)
		}
	}
	return out
}

// initializeBody is one MCP initialize request. It is the smallest thing that
// makes the server do real work rather than reject a shape, so a 200 for it
// means the transport is answering and not merely accepting bytes.
const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":` +
	`{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"engramux-gate","version":"1"}}}`

// initialize sends [initializeBody] to endpoint, with decorate applied to the
// request, and returns the response and its body.
func initialize(t *testing.T, endpoint string, decorate func(*http.Request)) (*http.Response, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(initializeBody))
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	decorate(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send the request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	// Bounded: this is a response body read into a failure message.
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		t.Fatalf("read the response: %v", err)
	}
	return resp, string(b)
}

// serveForTest binds and serves h on a temporary directory, and returns the
// published endpoint and this start's token.
//
// The token is read out of mcp.json rather than returned by [Listen], which is
// how the installer gets it, so this also pins the on-disk shape that reader
// depends on. internal/mcpconf deliberately cannot hand a caller a token; a
// test that minted one into its own temporary directory is a different thing
// from a package that could hand one to a CLI.
func serveForTest(t *testing.T, h pipe.Handler) (endpoint, token string) {
	t.Helper()

	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	s, err := Listen(ctx, dir, h)
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})

	b, err := os.ReadFile(mcpconf.Path(dir))
	if err != nil {
		t.Fatalf("read %s: %v", mcpconf.Name, err)
	}
	var doc struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode %s: %v", mcpconf.Name, err)
	}
	if doc.Token == "" {
		t.Fatalf("%s carries no token", mcpconf.Name)
	}
	if doc.URL != s.Endpoint() {
		t.Fatalf("%s publishes a URL the server does not serve", mcpconf.Name)
	}
	return s.Endpoint(), doc.Token
}

// TestTheTokenAndThePortBothSurviveARestart is the defect a first revision of
// this package shipped, held open by a test.
//
// The port was sticky and the token was minted every start, so on a machine
// where the service is a logon-triggered task the two host configurations held
// the previous logon's token from the moment the user logged in. Every tool
// call answered 401, `doctor` reported it healthy because the URL still
// matched, and no smoke test inside one service lifetime could see it.
//
// Two Listens over one directory are what a restart is. Both values have to
// come back, because either one alone leaves a host configuration that does not
// work: the URL without the credential, or the credential at an address nothing
// answers.
func TestTheTokenAndThePortBothSurviveARestart(t *testing.T) {
	dir := t.TempDir()

	first, firstToken := listenOnce(t, dir)
	second, secondToken := listenOnce(t, dir)

	if second != first {
		t.Errorf("the endpoint moved across a restart: %q then %q", first, second)
	}
	if secondToken != firstToken {
		// Neither value is printed: they are the tokens this test
		// minted, and a mismatch is the whole finding.
		t.Error("the bearer token was minted again across a restart, which is what breaks both hosts at every logon")
	}
}

// TestATokenThisNeverWroteIsReplaced. A reused token goes straight into the
// comparison every request is checked against, so the file it comes from is a
// trust boundary even though the service wrote it: an empty one would make a
// bare `Authorization: Bearer ` header compare equal.
func TestATokenThisNeverWroteIsReplaced(t *testing.T) {
	for _, tc := range []struct{ name, token string }{
		{"empty", ""},
		{"too short", "ABCDEFGHIJKLMNOPQRSTUVWXY"},
		{"lower case, which the alphabet has none of", "abcdefghijklmnopqrstuvwxy2"},
		{"a space, which would reach a TOML string and an argv", "ABCDEFGHIJKLM NOPQRSTUVWXY2"},
		{"a quote, the same way", `ABCDEFGHIJKLM"NOPQRSTUVWXY2`},
		{"a digit outside base32", "ABCDEFGHIJKLMNOPQRSTUVWXY9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := mcpconf.Write(dir, "http://127.0.0.1:1/mcp", tc.token); err != nil {
				t.Fatalf("write %s: %v", mcpconf.Name, err)
			}
			if got := reusableToken(dir); got != "" {
				t.Errorf("a token of %d characters was reused", len(got))
			}
		})
	}

	// And the control: what this package writes is reused, or the six
	// cases above would pass on a function that always refused.
	dir := t.TempDir()
	_, token := listenOnce(t, dir)
	if reusableToken(dir) != token {
		t.Error("a token this package minted was not reused")
	}
}

// listenOnce binds, publishes, and closes again without ever serving, and
// returns the endpoint and the token that were published.
func listenOnce(t *testing.T, dir string) (endpoint, token string) {
	t.Helper()

	s, err := Listen(t.Context(), dir, stubHandler())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	// Closed rather than served: the next Listen has to be able to take the
	// same port back, which is the half of this that is about the port.
	if err := s.ln.Close(); err != nil {
		t.Fatalf("close the listener: %v", err)
	}

	b, err := os.ReadFile(mcpconf.Path(dir))
	if err != nil {
		t.Fatalf("read %s: %v", mcpconf.Name, err)
	}
	var doc struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode %s: %v", mcpconf.Name, err)
	}
	return doc.URL, doc.Token
}

// stubHandler answers every request type with a fixed document.
//
// The transport clauses are about what reaches a handler, not about what a
// handler answers, so a stub is the right thing here: a database would make
// three tests about HTTP depend on a schema. The four tools are exercised
// against real replies in tools_test.go, and against a real database in
// internal/service.
func stubHandler() pipe.Handler {
	return pipe.Handler{
		Ingest: func(context.Context, ipc.Envelope) (ipc.AckStatus, error) {
			return ipc.Committed, nil
		},
		Status: func(context.Context) (ipc.StatusReply, error) {
			return ipc.StatusReply{Events: 1, SpoolDepth: 0, UptimeMS: 1}, nil
		},
		Search: func(_ context.Context, req ipc.SearchRequest) (ipc.SearchReply, error) {
			return ipc.SearchReply{Hits: []ipc.SearchHit{{
				ID: stubEventID, Host: "codex", EventName: "PostToolUse",
				ReceivedAtMS: 1, Excerpt: "matched " + req.Query,
			}}}, nil
		},
		GetEvent: func(_ context.Context, req ipc.GetEventRequest) (ipc.GetEventReply, error) {
			if req.ID != stubEventID {
				return ipc.GetEventReply{}, nil
			}
			return ipc.GetEventReply{Event: &ipc.EventDocument{
				ID: stubEventID, Host: "codex", EventName: "PostToolUse",
				SessionID: "codex:1", ReceivedAtMS: 1, PrivacyClass: "",
				Payload: json.RawMessage(`{"hook_event_name":"PostToolUse"}`), PayloadBytes: 34,
			}}, nil
		},
		ListSessions: func(context.Context, ipc.ListSessionsRequest) (ipc.ListSessionsReply, error) {
			return ipc.ListSessionsReply{ProjectRoot: `D:\work`, Sessions: []ipc.Session{{
				ID: "codex:1", Host: "codex", HostSessionID: "1", Status: "active", CreatedAtMS: 1,
			}}}, nil
		},
	}
}

// stubEventID is the one id [stubHandler]'s get_event knows, so a test can tell
// "found" from "no such event" without a database.
const stubEventID = "0198f2a0-0000-7000-8000-00000000beef"
