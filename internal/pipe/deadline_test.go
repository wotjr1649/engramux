package pipe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestASlowHandlerStillGetsItsReplyOut is backlog row 26, reproduced and then
// closed.
//
// # What was observed
//
// Once, on the installed service, one `engramux status` failed with
// `read the reply: ipc: read frame length: EOF` while the service logged
// `pipe: write reply` / `ipc: write frame length: i/o timeout` at the same
// instant. One human client, one CLI invocation, no contention, and the service
// stayed up. An immediate retry succeeded. Nothing reproduced it on demand.
//
// # What it was
//
// [serveConn] sets one deadline, before the request frame is read, and that one
// deadline covers the read, the handler and the write. The handler is
// deliberately not bounded by it - abandoning an ingest already in flight is
// worse than answering late - so a handler that runs long leaves the reply write
// whatever is left of [requestTimeout], and can leave it nothing. The write then
// fails with exactly `i/o timeout`, the client sees the connection close with no
// frame on it, and the service logs both halves. That is the observation, line
// for line.
//
// The fix is that the reply gets its own deadline, set after the handler
// returns. It does not extend what a client may hold the connection for by
// stalling - the read deadline still bounds that, and a stalled client sends no
// request, so no handler runs and no second deadline is ever set.
//
// This test is the reproduction. Broken deliberately by removing the second
// deadline, it goes red with the same EOF the observation carried.
func TestASlowHandlerStillGetsItsReplyOut(t *testing.T) {
	restore := requestTimeout
	requestTimeout = 150 * time.Millisecond
	t.Cleanup(func() { requestTimeout = restore })

	name, _ := startHandler(t, Handler{
		Ingest: (&recorder{status: ipc.Committed}).ingest,
		// Twice the deadline, so the write starts with none of it left.
		// It is a sleep and not real work on purpose: the defect is
		// about elapsed time on the connection, and what spent it does
		// not matter.
		Status: func(ctx context.Context) (ipc.StatusReply, error) {
			select {
			case <-time.After(2 * requestTimeout):
			case <-ctx.Done():
			}
			return ipc.StatusReply{Events: 7}, nil
		},
	})

	raw := exchangeSlow(t, name, request(t, ipc.Version, ipc.Status, "", []byte("null")))
	var reply ipc.StatusReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if err := reply.Verify(); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if reply.Events != 7 {
		t.Errorf("events = %d, want the 7 the handler answered", reply.Events)
	}
}

// exchangeSlow is [exchangeRaw] with a client deadline long enough to outlast a
// deliberately slow handler, so the only deadline that can fail the exchange is
// the server's.
func exchangeSlow(t *testing.T, name string, req []byte) []byte {
	t.Helper()
	conn := dial(t, name)
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set the client deadline: %v", err)
	}
	if err := ipc.WriteFrame(conn, req); err != nil {
		t.Fatalf("write the request: %v", err)
	}
	raw, err := ipc.ReadFrame(conn)
	if err != nil {
		if errors.Is(err, io.EOF) {
			t.Fatalf("the reply never arrived: %v - this is backlog row 26, "+
				"the reply write inheriting what the handler left of the connection deadline", err)
		}
		t.Fatalf("read the reply: %v", err)
	}
	return raw
}
