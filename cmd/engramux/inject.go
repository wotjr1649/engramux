package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/wotjr1649/engramux/internal/injectconf"
	"github.com/wotjr1649/engramux/internal/ipc"
)

// The one event injection attaches to (memory spec rev.8, M-4).
//
// Codex documents additionalContext on seven of its events and Claude Code
// accepts it here too, so the choice was available. Only this one carries a
// query. SessionStart has none, so injection there would be a constant - and a
// constant context cost is exactly what P2 says native memory already pays and
// this product structurally does not have to. The 1.0 spec §5.8's "SessionStart
// emits nothing" survives Step 5 unchanged, for a better reason than the one it
// was written with.
const injectEvent = "UserPromptSubmit"

// promptEvent is the part of a UserPromptSubmit payload injection reads.
//
// **[verified]** 2026-09-03 over the captured corpus: 22 UserPromptSubmit
// documents, 12 from Codex and 10 from Claude Code, and all three of these keys
// are on every one of them. The hosts differ elsewhere - Codex adds `model` and
// `turn_id`, Claude Code adds `prompt_id` - and in nothing this reads.
type promptEvent struct {
	Name   string `json:"hook_event_name"`
	Prompt string `json:"prompt"`
	Cwd    string `json:"cwd"`
}

// injectContext writes the host's additionalContext for a UserPromptSubmit
// event, or writes nothing at all.
//
// # This is the first thing this product does on the user's critical path
//
// Everything else the relay does is fire-and-forget: a relay that cannot reach
// the service spools, and the drain replays (I-04). A relay that waits for an
// injection has no such door - the prompt is going to the model either way, and
// the only question is whether this process is still holding it. So every
// failure here is the same answer, zero bytes, and the budget is a ceiling
// rather than a target.
//
// # It never touches the event's own error
//
// The caller's [event] carries whether the capture was delivered, and the spool
// decision hangs off it. Injection failing is not a capture failing, and
// writing to that field here would spool an event the service already
// committed - so nothing below assigns to it.
//
// # The order is deliver, then inject
//
// Capture is the invariant and injection is the feature, so the feature does
// not get to delay the invariant. What it costs is that the prompt's own event
// is already a row whose text is the query by the time this runs, which is why
// the request carries the id to exclude - exactly, rather than by resemblance.
func injectContext(start time.Time, payload []byte, ingestID string) {
	// The switch first, so a machine with injection off pays one failed
	// os.ReadFile and never a dial. Absent means off (injectconf.ConfigName).
	if !injectconf.Enabled() {
		return
	}
	var ev promptEvent
	if err := json.Unmarshal(payload, &ev); err != nil || ev.Name != injectEvent || ev.Prompt == "" {
		return
	}

	// The budget is injectconf.Budget clamped by what is left of the relay's
	// own second (1.0 spec §5.3). It comes out of that second rather than
	// being added to it, so a delivery that ran long leaves injection less
	// time rather than pushing the process past its ceiling.
	deadline := earliest(time.Now().Add(injectconf.Budget), start.Add(totalBudget))
	text, err := askInject(deadline, ipc.InjectRequest{
		Prompt:    ev.Prompt,
		Project:   ev.Cwd,
		ExcludeID: ingestID,
	})
	if err != nil {
		warn("no context injected: %v", err)
		return
	}
	if text == "" {
		return
	}
	writeAdditionalContext(text)
}

// writeAdditionalContext writes the one document both hosts read.
//
// **[verified]** 2026-09-03 against both current references: the shape is
// identical - a hook writes hookSpecificOutput.additionalContext on stdout and
// the text is added as developer context before the prompt is processed. What
// differs is that Codex renders the injected text as a visible message in its
// transcript, which is §6's fifth mitigation arriving free on one host and owed
// on the other.
//
// This is the one thing that writes to the relay's stdout, and it is the
// exception the 1.0 spec §4.5 does not have: that section's own reasoning is
// "since 1.0 is pull-only", and M-4 is the row that changes for after 1.0. Every
// other event still writes nothing.
//
// json.Marshal and not a concatenation, because the payload is corpus text: it
// carries newlines, quotes and whatever bytes a captured file had, and a
// hand-built document would be a JSON injection with this product's own name on
// it. The write error is discarded for the reason [warn]'s are - the hook exits
// 0 whatever happens (I-03), and a host that closed our stdout has already
// stopped listening.
func writeAdditionalContext(text string) {
	doc, err := json.Marshal(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     injectEvent,
			"additionalContext": text,
		},
	})
	if err != nil {
		warn("no context injected: encode the reply: %v", err)
		return
	}
	_, _ = os.Stdout.Write(append(doc, '\n'))
}

// askInject sends one Inject request and returns the payload it can accept.
//
// It is a round trip of its own rather than [roundTrip] because that one is the
// CLI's and carries the CLI's five-second budget. A hook has a second in total
// and injection has half of it, and a person waiting at a prompt is not a
// person waiting at a terminal.
//
// An old service answers an unknown request type with a rejected ACK, which
// [ipc.InjectReply.Verify] refuses - so a relay newer than its service injects
// nothing rather than injecting something unfiltered. That is the shape
// [ipc.InjectRequest] was made a type of its own for.
func askInject(deadline time.Time, req ipc.InjectRequest) (string, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("encode the request: %w", err)
	}
	frame, err := json.Marshal(ipc.Envelope{Version: ipc.Version, Type: ipc.Inject, Payload: payload})
	if err != nil {
		return "", fmt.Errorf("encode the envelope: %w", err)
	}

	name, err := ipc.CurrentPipeName()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithDeadline(context.Background(),
		earliest(time.Now().Add(dialBudget), deadline))
	defer cancel()
	conn, err := dial(ctx, name)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", name, err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("set the deadline: %w", err)
	}
	if err := ipc.WriteFrame(conn, frame); err != nil {
		return "", fmt.Errorf("send the request: %w", err)
	}
	raw, err := ipc.ReadFrame(conn)
	if err != nil {
		return "", fmt.Errorf("read the reply: %w", err)
	}
	var reply ipc.InjectReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return "", fmt.Errorf("decode the reply: %w", err)
	}
	if err := reply.Verify(); err != nil {
		return "", replied(err, raw)
	}
	// The cap is the host's contract and the service is what enforces it,
	// but this is the process that hands the bytes over - so it checks
	// rather than trusts. A service one version ahead with a larger cap
	// would otherwise put a payload past this host's limit into a prompt.
	if len(reply.Context) > injectconf.MaxBytes {
		return "", fmt.Errorf("the reply is %d bytes, over the %d-byte cap",
			len(reply.Context), injectconf.MaxBytes)
	}
	return reply.Context, nil
}
