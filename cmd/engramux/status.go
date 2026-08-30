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
	case "cells":
		return cells()
	case "doctor":
		return doctor(args[1:])
	case "search":
		return search(args[1:])
	case "event":
		return showEvent(args[1:])
	case "sessions":
		return sessions(args[1:])
	case "install":
		return install(args[1:])
	case "register":
		return register(args[1:])
	case "unregister":
		return unregister(args[1:])
	default:
		warn("unknown command %.32q", args[0])
		warn("usage: engramux install [--apply] | status | cells | doctor | search | event | sessions | register | unregister")
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

// cells prints the per-cell capture breakdown - host x event name, with a count
// and the span it was captured over.
//
// # It is a second command over the same request
//
// It sends the Status request [status] sends, unchanged: spec 5.2 fixes the
// request set at five types and a breakdown is not a sixth thing to ask the
// service, it is part of what "how is the service doing" already answers. So
// the service cannot tell these two commands apart, and there is nothing to
// keep in sync between them.
//
// A separate command rather than extra lines on `status`, because the two
// answer different questions at different lengths. `status` is four lines a
// person reads to find out whether the service is alive; this is a table that
// grows with the corpus, and printing it every time would bury the four lines
// that were asked for. A flag would have been the same decision with an
// argument parser attached - this binary's argument handling is one switch in
// front of the relay path (see main), and it stays that way.
//
// # Absent, not zero
//
// Only cells the database holds are printed. A cell nothing has been captured
// for has no row, and no row ever carries a count of zero - see [ipc.Cell] for
// why the alternative, a grid pre-filled with zeroes, is not available here.
func cells() int {
	reply, err := askStatus()
	if err != nil {
		warn("cells: %v", err)
		return 1
	}
	// Stdout, for the same reason [status] uses it.
	//
	// The event name is quoted and bounded, and the host is not. host is
	// constrained by the events.host CHECK to three values this program
	// knows; event_name is whatever a payload's hook_event_name said, so it
	// is untrusted width, untrusted bytes, and - for a payload that carried
	// no name at all - empty, which unquoted would print as blank columns
	// that read like a missing value rather than a real cell.
	_, _ = fmt.Fprintf(os.Stdout, "%-11s  %-19s  %7s  %-19s  %s\n",
		"host", "event", "count", "first seen", "last seen")
	for _, c := range reply.Cells {
		_, _ = fmt.Fprintf(os.Stdout, "%-11s  %-19.64q  %7d  %-19s  %s\n",
			c.Host, c.EventName, c.Count, stamp(c.FirstSeenMS), stamp(c.LastSeenMS))
	}
	return 0
}

// stamp renders one of [ipc.Cell]'s epoch-millisecond timestamps in local time.
// The service that wrote it and the person reading it are the same Windows user
// on the same machine (spec 2), so there is one clock and one zone in play.
func stamp(ms int64) string { return time.UnixMilli(ms).Format(time.DateTime) }

// askStatus sends one Status request and returns the reply it can accept.
//
// The reply is checked with [ipc.StatusReply.Verify] before a single field is
// read, and that check is what tells a status reply from the rejected ACK the
// service answers a request it will not serve. Without it an Ack would decode
// into a StatusReply of zeroes and this command would print them as the
// service's real numbers.
func askStatus() (ipc.StatusReply, error) {
	var zero ipc.StatusReply

	raw, err := roundTrip(ipc.Status, nil)
	if err != nil {
		return zero, err
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

// roundTrip sends one request of type typ carrying payload and returns the
// reply frame, whatever document it holds. Deciding what that document is - and
// checking it with the right Verify - is the caller's, because the request type
// is what decides it (see [ipc.StatusReply]).
//
// This is the whole of the CLI's transport, and it is one function rather than
// one per command so that every read I-08 routes over the pipe gets the same
// budget, the same deadline and the same "no service is listening" wording.
//
// The dial is the relay's. winio.DialPipeContext fails immediately when the
// pipe does not exist rather than retrying, so "no service" is answered now
// instead of at the end of the budget.
//
// The envelope is built with json.Marshal rather than the relay's
// concatenation: that exists to keep a captured payload's bytes untouched, and
// nothing a CLI command sends is a captured payload.
func roundTrip(typ ipc.RequestType, payload json.RawMessage) ([]byte, error) {
	name, err := ipc.CurrentPipeName()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cliBudget)
	defer cancel()
	conn, err := dial(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("no service is listening on %s: %w", name, err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(cliBudget)); err != nil {
		return nil, fmt.Errorf("set the deadline: %w", err)
	}

	req, err := json.Marshal(ipc.Envelope{Version: ipc.Version, Type: typ, Payload: payload})
	if err != nil {
		return nil, fmt.Errorf("encode the request: %w", err)
	}
	if err := ipc.WriteFrame(conn, req); err != nil {
		return nil, fmt.Errorf("send the request: %w", err)
	}
	raw, err := ipc.ReadFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("read the reply: %w", err)
	}
	return raw, nil
}
