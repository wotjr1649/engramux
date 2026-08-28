package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// cliBudget bounds one CLI request end to end. It is not the relay's budget and
// has no reason to be: nothing is blocked on it (I-03 is about the hook path),
// and a person who typed a command would rather wait a moment than be told the
// service is down when it was only busy. The service's own connection deadline
// is 2 s, so this is the outer bound on a service that accepted and then stalled.
const cliBudget = 5 * time.Second

// cli runs the command a person typed and returns the process's exit code.
//
// Unknown commands are refused rather than falling through to the relay: a
// typo that silently read stdin and spooled an empty event would be a strange
// thing to debug.
func cli(args []string) int {
	switch args[0] {
	case "status":
		return status()
	default:
		warn("unknown command %.32q", args[0])
		warn("usage: engramux status")
		return 2
	}
}

// status asks the service how it is doing and prints the answer (I-08).
//
// There is no fallback that reads the database directly, and there cannot be:
// the service holds it exclusively (I-07), so this pipe is the only way to see
// any of these numbers. A service that is down therefore fails here rather than
// producing a second, lesser answer.
func status() int {
	reply, err := askStatus()
	if err != nil {
		warn("status: %v", err)
		return 1
	}
	// Stdout, because this is the CLI path and a person asked for it. The
	// relay's silence on stdout (spec 4.5) is a rule about hook events.
	_, _ = fmt.Fprintf(os.Stdout,
		"uptime    %s\nevents    %d\nspool     %d\ndatabase  %s\n",
		(time.Duration(reply.UptimeMS) * time.Millisecond).Round(time.Millisecond),
		reply.Events, reply.SpoolDepth, reply.DatabasePath)
	return 0
}

// askStatus sends one Status request and returns the reply it can accept.
//
// The reply is checked with [ipc.StatusReply.Verify] before a single field is
// read, and that check is what tells a status reply from the rejected ACK the
// service answers a request it will not serve. Without it an Ack would decode
// into a StatusReply of zeroes and this command would print them as the
// service's real numbers.
func askStatus() (ipc.StatusReply, error) {
	var zero ipc.StatusReply

	name, err := ipc.CurrentPipeName()
	if err != nil {
		return zero, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cliBudget)
	defer cancel()
	// The same dial the relay uses. winio.DialPipeContext fails immediately
	// when the pipe does not exist rather than retrying, so "no service" is
	// answered now instead of at the end of the budget.
	conn, err := dial(ctx, name)
	if err != nil {
		return zero, fmt.Errorf("no service is listening on %s: %w", name, err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(cliBudget)); err != nil {
		return zero, fmt.Errorf("set the deadline: %w", err)
	}

	// json.Marshal rather than the relay's concatenation: that exists to
	// keep a captured payload's bytes untouched, and this request carries no
	// payload at all.
	req, err := json.Marshal(ipc.Envelope{Version: ipc.Version, Type: ipc.Status})
	if err != nil {
		return zero, fmt.Errorf("encode the request: %w", err)
	}
	if err := ipc.WriteFrame(conn, req); err != nil {
		return zero, fmt.Errorf("send the request: %w", err)
	}
	raw, err := ipc.ReadFrame(conn)
	if err != nil {
		return zero, fmt.Errorf("read the reply: %w", err)
	}

	var reply ipc.StatusReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return zero, fmt.Errorf("decode the reply: %w", err)
	}
	if err := reply.Verify(); err != nil {
		// The reply is bounded on its way into the message: it is bytes
		// off the wire and capped only by ipc.MaxFrameLen.
		return zero, fmt.Errorf("%w: the service replied %.200q", err, raw)
	}
	return reply, nil
}
