package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// showEvent prints one whole event, by id and project (I-08).
//
// Both are required by the wire (spec 5.9) and only the id is required here:
// events.id is unique across the database, so the project is what keeps a known
// id from reading across projects, and a person standing in their own worktree
// has already said which project they mean. The default is therefore this
// process's working directory - not the service's, which is a long-lived
// process Task Scheduler started and has nothing to do with the question.
//
// Whatever the project ends up being, it is made absolute here. The service
// refuses a relative path outright rather than resolving it against itself,
// which is the only correct thing it can do with one.
func showEvent(args []string) int {
	if len(args) == 0 || len(args) > 2 {
		warn("usage: engramux event <id> [project]")
		return 2
	}
	root, err := projectArg(args[1:])
	if err != nil {
		warn("event: %v", err)
		return 1
	}

	reply, err := askGetEvent(ipc.GetEventRequest{ID: args[0], Project: root})
	if err != nil {
		warn("event: %v", err)
		return 1
	}
	// Reached only after Verify, which is what makes a nil event mean "no
	// such event in that project" rather than "the service refused".
	if reply.Event == nil {
		_, _ = fmt.Fprintln(os.Stdout, "no such event in this project")
		return 1
	}
	printEvent(*reply.Event)
	return 0
}

// printEvent writes one event as a header block and its payload.
//
// The three untrusted fields are quoted for the reason [search] quotes its
// three: events.event_name, events.session_id and events.id carry whatever a
// payload said, and events.host is the one field a CHECK constrains.
//
// The payload is written as indented JSON rather than quoted, and that is safe
// rather than convenient: [json.Indent] accepts only valid JSON, and valid JSON
// carries no unescaped control byte - a terminal escape inside a string arrived
// as a six-character escape sequence and stays one. The service already
// guarantees the field is a JSON value whatever the row held, so the error
// branch is the one that cannot be argued away rather than one that fires.
func printEvent(e ipc.EventDocument) {
	_, _ = fmt.Fprintf(os.Stdout,
		"id        %q\nhost      %-11s\nevent     %s\nsession   %q\nreceived  %s\nprivacy   %q\npayload   %d bytes\n",
		e.ID, e.Host, cutName(e.EventName, e.EventNameTruncated), e.SessionID, stamp(e.ReceivedAtMS),
		e.PrivacyClass, e.PayloadBytes)

	if e.Payload == nil {
		_, _ = fmt.Fprintf(os.Stdout,
			"\nthe payload is not here: masked it is %d bytes, over the %d-byte bound\n",
			e.PayloadBytes, ipc.MaxEventPayloadBytes)
		return
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, e.Payload, "", "  "); err != nil {
		warn("the payload is not JSON: %v", err)
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, "\n%s\n", pretty.Bytes())
}

// sessions lists one project's sessions, newest first (I-08).
func sessions(args []string) int {
	if len(args) > 1 {
		warn("usage: engramux sessions [project]")
		return 2
	}
	root, err := projectArg(args)
	if err != nil {
		warn("sessions: %v", err)
		return 1
	}

	reply, err := askListSessions(ipc.ListSessionsRequest{Project: root})
	if err != nil {
		warn("sessions: %v", err)
		return 1
	}
	// The root the service resolved, masked, so it is clear which project
	// answered - a path that is not the one you meant is the failure this
	// line exists to make visible.
	_, _ = fmt.Fprintf(os.Stdout, "project   %q\n", reply.ProjectRoot)
	if len(reply.Sessions) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "no sessions")
		return 0
	}
	_, _ = fmt.Fprintf(os.Stdout, "%-11s  %-9s  %-19s  %-19s  %s\n",
		"host", "status", "first seen", "ended", "session")
	for _, s := range reply.Sessions {
		ended := "-"
		if s.EndedAtMS != 0 {
			ended = stamp(s.EndedAtMS)
		}
		_, _ = fmt.Fprintf(os.Stdout, "%-11s  %-9s  %-19s  %-19s  %q\n",
			s.Host, s.Status, stamp(s.CreatedAtMS), ended, s.HostSessionID)
	}
	return 0
}

// projectArg turns an optional project argument into the absolute path the
// service will accept: the argument made absolute, or this process's working
// directory when there is none.
func projectArg(args []string) (string, error) {
	if len(args) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("locate the working directory: %w", err)
		}
		return cwd, nil
	}
	abs, err := filepath.Abs(args[0])
	if err != nil {
		return "", fmt.Errorf("resolve %.128q: %w", args[0], err)
	}
	return abs, nil
}

// askGetEvent sends one GetEvent request and returns the reply it can accept.
//
// The reply is checked with [ipc.GetEventReply.Verify] before its event is
// read, and that check is what tells this reply from the rejected ACK the
// service answers a request it will not serve. Without it an Ack would decode
// into a reply whose event is nil and this command would report "no such event"
// for a request the service refused.
func askGetEvent(req ipc.GetEventRequest) (ipc.GetEventReply, error) {
	var zero ipc.GetEventReply

	payload, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("encode the request: %w", err)
	}
	raw, err := roundTrip(ipc.GetEvent, payload)
	if err != nil {
		return zero, err
	}

	var reply ipc.GetEventReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return zero, fmt.Errorf("decode the reply: %w", err)
	}
	if err := reply.Verify(); err != nil {
		// Bounded on its way into the message: it is bytes off the wire
		// and capped only by ipc.MaxFrameLen. A refusal is an ACK, so
		// 200 of them are three short fields; anything else is payload
		// text, which is why the bound is here at all.
		return zero, fmt.Errorf("%w: the service replied %.200q", err, raw)
	}
	return reply, nil
}

// askListSessions sends one ListSessions request and returns the reply it can
// accept. [ipc.ListSessionsReply.Verify] is what separates an empty listing
// from a refusal, the same way [askGetEvent]'s does.
func askListSessions(req ipc.ListSessionsRequest) (ipc.ListSessionsReply, error) {
	var zero ipc.ListSessionsReply

	payload, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("encode the request: %w", err)
	}
	raw, err := roundTrip(ipc.ListSessions, payload)
	if err != nil {
		return zero, err
	}

	var reply ipc.ListSessionsReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return zero, fmt.Errorf("decode the reply: %w", err)
	}
	if err := reply.Verify(); err != nil {
		return zero, fmt.Errorf("%w: the service replied %.200q", err, raw)
	}
	return reply, nil
}
