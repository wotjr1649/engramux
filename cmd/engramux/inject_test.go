package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/pipe"
)

// stdinReader hands a child its standard input, the same way [run] does. It is
// spelled out here because the injection tests need [runWithLocal] rather than
// [run], and that one takes the setup rather than the bytes.
func stdinReader(b []byte) func(*exec.Cmd) {
	return func(cmd *exec.Cmd) { cmd.Stdin = bytes.NewReader(b) }
}

// ingestedID is the id the relay delivered the event under, read off the wire
// rather than guessed - the same source [requireSpooledAs] takes its id from.
func ingestedID(t *testing.T, obs *observed) string {
	t.Helper()
	obs.mu.Lock()
	defer obs.mu.Unlock()
	for _, env := range obs.envs {
		if env.Type == ipc.IngestEvent {
			return env.IngestID
		}
	}
	t.Fatalf("the server saw no IngestEvent envelope")
	return ""
}

// promptPayload is one UserPromptSubmit document in the shape both hosts send.
// The three keys the relay reads are on every one of the 22 in the captured
// corpus; the rest of each host's own keys are not this test's business.
func promptPayload(prompt string) []byte {
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "UserPromptSubmit",
		"session_id":      "s-1",
		"cwd":             `D:\work\thing`,
		"prompt":          prompt,
	})
	if err != nil {
		panic(err)
	}
	return b
}

// enableInjection writes the switch into a data directory and returns it, for
// [runWithLocal] to hand the child as its LOCALAPPDATA.
func enableInjection(t *testing.T) string {
	t.Helper()
	local := t.TempDir()
	dir := filepath.Join(local, "engramux")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("make the data directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, inject.ConfigName), []byte(`{"enabled":true}`), 0o600); err != nil {
		t.Fatalf("write the switch: %v", err)
	}
	return local
}

// serveWithInject is [serveReal] with an Inject handler beside the Ingest one,
// so a test sees both envelopes the relay sends and can answer the second.
//
// inject is nil for the build that serves no Inject handler at all, which is
// the shape an older service has and the one a newer relay has to survive.
func serveWithInject(t *testing.T, reply *ipc.InjectReply) *observed {
	t.Helper()
	l := listenRelayPipe(t)
	obs := &observed{}
	var mu sync.Mutex

	h := pipe.Handler{
		Ingest: func(_ context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
			mu.Lock()
			defer mu.Unlock()
			obs.add(env)
			return ipc.Committed, nil
		},
	}
	if reply != nil {
		h.Inject = func(_ context.Context, req ipc.InjectRequest) (ipc.InjectReply, error) {
			mu.Lock()
			defer mu.Unlock()
			payload, err := json.Marshal(req)
			if err != nil {
				return ipc.InjectReply{}, err
			}
			// Recorded as an envelope so the one collector holds both
			// halves of the exchange in the order they happened.
			obs.add(ipc.Envelope{Version: ipc.Version, Type: ipc.Inject, Payload: payload})
			return *reply, nil
		}
	}

	done := make(chan error, 1)
	go func() { done <- pipe.Serve(t.Context(), l, h) }()
	t.Cleanup(func() {
		_ = l.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("pipe.Serve did not return within 10s of Close")
		}
	})
	return obs
}

// injectRequests returns every Inject request the server saw.
func injectRequests(t *testing.T, obs *observed) []ipc.InjectRequest {
	t.Helper()
	obs.mu.Lock()
	defer obs.mu.Unlock()
	var out []ipc.InjectRequest
	for _, env := range obs.envs {
		if env.Type != ipc.Inject {
			continue
		}
		var req ipc.InjectRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			t.Fatalf("decode an inject request: %v", err)
		}
		out = append(out, req)
	}
	return out
}

// additionalContext reads the text out of what the relay wrote on stdout, or
// fails. The shape is the hosts' and is asserted here rather than trusted: a
// document with the right text under the wrong key injects nothing.
func additionalContext(t *testing.T, stdout []byte) string {
	t.Helper()
	var doc struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout, &doc); err != nil {
		t.Fatalf("the relay's stdout is not a JSON document: %v\n%q", err, stdout)
	}
	if doc.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName = %q, want UserPromptSubmit", doc.HookSpecificOutput.HookEventName)
	}
	return doc.HookSpecificOutput.AdditionalContext
}

// Injection ships disabled, and the switch is checked before anything else: a
// machine that has never configured it sends no Inject request at all.
func TestInjectionIsOffWithoutTheSwitch(t *testing.T) {
	obs := serveWithInject(t, &ipc.InjectReply{Version: ipc.Version, Type: ipc.Inject, Context: "should not appear"})
	res := run(t, relayBin, promptPayload("what did the checkpoint threshold decide"))

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpoolEmpty(t)
	if reqs := injectRequests(t, obs); len(reqs) != 0 {
		t.Errorf("the relay sent %d Inject requests with the switch absent, want 0", len(reqs))
	}
}

// With the switch on, a UserPromptSubmit event gets the one document both hosts
// read - and the request carries the prompt, the host's cwd and the id of the
// event this very relay just delivered.
func TestInjectionWritesAdditionalContext(t *testing.T) {
	const want = "recalled text\nwith a \"quote\" and a <tag>"
	obs := serveWithInject(t, &ipc.InjectReply{Version: ipc.Version, Type: ipc.Inject, Context: want})

	local := enableInjection(t)
	res := runWithLocal(t, relayBin, local, stdinReader(promptPayload("what did the checkpoint threshold decide")))

	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}
	if got := additionalContext(t, res.stdout); got != want {
		t.Errorf("additionalContext = %q, want %q", got, want)
	}

	reqs := injectRequests(t, obs)
	if len(reqs) != 1 {
		t.Fatalf("the relay sent %d Inject requests, want 1", len(reqs))
	}
	if reqs[0].Prompt != "what did the checkpoint threshold decide" {
		t.Errorf("the request carries the prompt %q", reqs[0].Prompt)
	}
	if reqs[0].Project != `D:\work\thing` {
		t.Errorf("the request carries the project %q, want the payload's cwd", reqs[0].Project)
	}
	// The prompt's own event is already stored by the time injection runs,
	// and its text is the query - so an exclusion that is not the id the
	// relay just delivered under excludes nothing.
	ingested := ingestedID(t, obs)
	if reqs[0].ExcludeID != ingested {
		t.Errorf("the request excludes %q, want the id it delivered under, %q", reqs[0].ExcludeID, ingested)
	}
}

// Every other event still writes nothing on stdout, which is the 1.0 spec
// §4.5's rule surviving for the ten events M-4 does not touch.
func TestInjectionIgnoresEveryOtherEvent(t *testing.T) {
	obs := serveWithInject(t, &ipc.InjectReply{Version: ipc.Version, Type: ipc.Inject, Context: "should not appear"})
	local := enableInjection(t)

	for _, name := range []string{"SessionStart", "PostToolUse", "Stop", "PreCompact"} {
		payload, err := json.Marshal(map[string]any{"hook_event_name": name, "prompt": "a prompt", "cwd": "c:/w"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		res := runWithLocal(t, relayBin, local, stdinReader(payload))
		if len(res.stdout) != 0 {
			t.Errorf("%s wrote %q on stdout, want nothing", name, res.stdout)
		}
	}
	if reqs := injectRequests(t, obs); len(reqs) != 0 {
		t.Errorf("the relay sent %d Inject requests for events that are not UserPromptSubmit", len(reqs))
	}
}

// A service that does not serve Inject refuses it, and the relay injects
// nothing - which is what makes a relay newer than its service fail closed
// rather than fail open. The event is still delivered and still not spooled:
// injection failing is not capture failing.
func TestAServiceThatRefusesInjectInjectsNothing(t *testing.T) {
	serveWithInject(t, nil)
	local := enableInjection(t)
	res := runWithLocal(t, relayBin, local, stdinReader(promptPayload("what did the checkpoint threshold decide")))

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpoolEmpty(t)
	if !strings.Contains(string(res.stderr), "no context injected") {
		t.Errorf("stderr does not say the injection was refused: %s", res.stderr)
	}
}

// A reply over the cap is refused by the relay too. The service is what
// enforces M5, and this is the process that hands the bytes to the host - so a
// service one version ahead with a larger cap cannot put a payload past this
// host's limit into a prompt.
func TestInjectionRefusesAReplyOverTheCap(t *testing.T) {
	over := strings.Repeat("x", inject.MaxBytes+1)
	serveWithInject(t, &ipc.InjectReply{Version: ipc.Version, Type: ipc.Inject, Context: over})
	local := enableInjection(t)
	res := runWithLocal(t, relayBin, local, stdinReader(promptPayload("what did the checkpoint threshold decide")))

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpoolEmpty(t)
}

// An abstention is an empty Context, and it writes nothing rather than an empty
// document: a host handed additionalContext of "" has still been handed a
// document to parse, and P2 is exactly zero bytes.
func TestAnAbstentionWritesNothingAtAll(t *testing.T) {
	serveWithInject(t, &ipc.InjectReply{
		Version: ipc.Version, Type: ipc.Inject, Reason: inject.ReasonNoHits,
	})
	local := enableInjection(t)
	res := runWithLocal(t, relayBin, local, stdinReader(promptPayload("what did the checkpoint threshold decide")))

	res.requireExitZeroAndSilentStdout(t)
	res.requireSpoolEmpty(t)
}
