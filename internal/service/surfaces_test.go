package service

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wotjr1649/engramux/internal/mcpconf"
)

// TestBothSurfacesShareOneReadGate is spec 5.9's contention design at the level
// nothing else reaches: **one gate, two surfaces**.
//
// [handlers] is built once in [run] and handed to internal/pipe and
// internal/mcpserver alike, so an MCP tool call and a CLI read contend with each
// other on spec 5.4's single connection. Every other test in this package holds
// a gate it built itself, which cannot say anything about what [run] passes
// where - and a deliberate break that gave the MCP server its own
// [newReadGate] left `internal/service`, `internal/mcpserver` and
// `cmd/engramux-service` all green.
//
// # Why overlap and not the ingest budget
//
// The instrument that suggests itself - time an ingest against many concurrent
// MCP readers, as [TestPhase5GateAReaderDoesNotPushIngestPastItsBudget] does -
// passes with two gates. A second gate serialises the MCP readers among
// themselves, so at most two reads ever reach the pool and the 800 ms budget
// holds comfortably. It is the same trap the three mechanisms have: a clause
// that passes on a subset says nothing about the rest.
//
// Overlap does discriminate. Two reads on one gate take about twice one read;
// two reads on two gates take about one. That needs a read whose duration this
// test sets, which is what [readHold] is for.
//
// # What it runs against
//
// [Run], on a directory of its own, over the pipe and over the endpoint that
// run published - the production wiring end to end, and the only place in the
// suite where both surfaces of one service are driven at once.
func TestBothSurfacesShareOneReadGate(t *testing.T) {
	claimAFreePipeName(t)
	dir := t.TempDir()

	// Set before Run starts, and that ordering is the point rather than
	// tidiness: every goroutine that reads readHold is created by Run, so
	// writing it first makes the goroutine-creation edge the whole of the
	// synchronisation and leaves nothing to reason about.
	//
	// It was written after Run to start with. The race detector reported
	// nothing then, and reported nothing either when a deliberate concurrent
	// write was injected beside it - while reporting an unsynchronised x++
	// in this same package immediately, so it was looking. That is a quiet
	// result nobody here can explain, which is exactly the kind of result not
	// to build on. The cost of not building on it is one startup poll paying
	// one hold.
	const hold = 400 * time.Millisecond
	readHold = hold
	t.Cleanup(func() { readHold = 0 })

	// The shutdown is a cleanup rather than a line at the end, because every
	// assertion below can end the test before that line: a Fatalf would
	// otherwise leave this service listening on the pipe for the rest of the
	// package, and the next test's claimAFreePipeName would fail with a message
	// about a leaked listener rather than about the failure that caused it.
	stop := running(t, dir)
	t.Cleanup(func() {
		if err := stop(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	endpoint, token := publishedEndpoint(t, dir)
	cs := connectMCP(t, endpoint, token)

	// Both reads are launched from one barrier so that neither is measured
	// against the other's start-up. The pipe read is servingOK, which is a
	// whole Status round trip.
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	done.Add(2)

	var pipeOK bool
	go func() {
		defer done.Done()
		start.Wait()
		pipeOK = servingOK(t)
	}()
	var mcpErr error
	go func() {
		defer done.Done()
		start.Wait()
		_, mcpErr = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "status", Arguments: map[string]any{}})
	}()

	began := time.Now()
	start.Done()
	done.Wait()
	elapsed := time.Since(began)

	// Guards the measurement rather than the invariant: an elapsed time
	// under one hold would mean neither read reached the gate at all.
	if !pipeOK {
		t.Fatal("the pipe read did not complete, so the elapsed time below measures nothing")
	}
	if mcpErr != nil {
		t.Fatalf("the MCP tool call did not complete: %v", mcpErr)
	}
	if elapsed < hold {
		t.Fatalf("both reads finished in %v, under one hold of %v - the hold did not apply", elapsed, hold)
	}

	// Serialised is about 2x the hold; two gates would be about 1x. The
	// threshold sits between them with room for scheduling on either side.
	if elapsed < 2*hold-hold/4 {
		t.Fatalf("a pipe read and an MCP tool call overlapped: %v for two reads holding %v each.\n"+
			"They are not contending, so the MCP surface is not on the gate `handlers` was built with.", elapsed, hold)
	}
}

// publishedEndpoint is the URL and token the service under test published.
//
// The token is read out of the file rather than from internal/mcpconf, which
// deliberately cannot hand one back, and out of the file rather than from
// internal/mcpserver, which this package must not need a test hook into.
func publishedEndpoint(t *testing.T, dir string) (endpoint, token string) {
	t.Helper()

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
	if doc.URL == "" || doc.Token == "" {
		t.Fatalf("%s is missing the url or the token", mcpconf.Name)
	}
	return doc.URL, doc.Token
}

// connectMCP opens one MCP session against the running service.
func connectMCP(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{Name: "engramux-surfaces-test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearerHeader{token: token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect to %s: %v", endpoint, err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// bearerHeader adds the Authorization header the endpoint requires.
type bearerHeader struct{ token string }

func (b bearerHeader) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}
