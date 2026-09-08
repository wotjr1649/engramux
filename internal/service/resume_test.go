package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
)

func TestSessionResumeReadsTheExactScopeAndLatestBodies(t *testing.T) {
	db, a, b := twoProjects(t)
	add := func(id, root, session, kind, field, text string) {
		t.Helper()
		payload, err := json.Marshal(map[string]string{
			"cwd": root, "session_id": session, "hook_event_name": kind,
			"transcript_path": "/synthetic/.claude/projects/session.jsonl", field: text,
		})
		if err != nil {
			t.Fatal(err)
		}
		ingestOne(t, db, id, payload)
		if _, err := db.ExecContext(t.Context(), "UPDATE events SET received_at=100 WHERE id=?", id); err != nil {
			t.Fatal(err)
		}
	}
	add("reply-old", a.root, a.session, "Stop", "last_assistant_message", "old decision")
	add("reply-new", a.root, a.session, "Stop", "last_assistant_message", "accepted: keep retry count at three")
	add("prompt", a.root, a.session, "UserPromptSubmit", "prompt", "continue the pending validation")
	add("subagent", a.root, a.session, "SubagentStop", "last_assistant_message", "unrelated delegated answer")
	add("tool", a.root, a.session, "PreToolUse", "prompt", "misleading short metadata")
	add("other-session", a.root, "other", "Stop", "last_assistant_message", "wrong session")
	add("other-project", b.root, a.session, "Stop", "last_assistant_message", "wrong project")
	// Host classification is independently stored; this row shares the opaque
	// session key but must not defeat the event host predicate.
	add("other-host", a.root, a.session, "Stop", "last_assistant_message", "wrong host")
	if _, err := db.ExecContext(t.Context(), "UPDATE events SET host='codex' WHERE id='other-host'"); err != nil {
		t.Fatal(err)
	}
	codexPayload, err := json.Marshal(map[string]string{"cwd": a.root, "session_id": a.session, "hook_event_name": "Stop", "transcript_path": "/synthetic/.codex/sessions/session.jsonl", "last_assistant_message": "separate codex decision"})
	if err != nil {
		t.Fatal(err)
	}
	ingestOne(t, db, "codex-reply", codexPayload)
	req := ipc.SessionResumeRequest{Project: a.root, Host: "claude-code", HostSessionID: a.session}
	got, err := sessionResume(t.Context(), db, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.LatestReply == nil || got.LatestReply.EventID != "reply-new" || got.LatestReply.Body != "accepted: keep retry count at three" || got.LatestReply.ReceivedAtMS != 100 {
		t.Fatalf("exact latest reply not recovered: %+v", got.LatestReply)
	}
	if got.LatestPrompt == nil || got.LatestPrompt.EventID != "prompt" || got.LatestPrompt.Body != "continue the pending validation" {
		t.Fatalf("latest prompt not recovered: %+v", got.LatestPrompt)
	}
	codexReq := req
	codexReq.Host = "codex"
	codexReply, err := sessionResume(t.Context(), db, codexReq)
	if err != nil || codexReply.LatestReply == nil || codexReply.LatestReply.EventID != "codex-reply" || codexReply.LatestReply.Body != "separate codex decision" || codexReply.LatestPrompt != nil {
		t.Fatal("same host session id crossed host identity")
	}
	for _, id := range []string{"missing", "' OR 1=1 --"} {
		req.HostSessionID = id
		absent, err := sessionResume(t.Context(), db, req)
		if err != nil || absent.LatestPrompt != nil || absent.LatestReply != nil {
			t.Fatal("absent or syntax-shaped identity escaped exact scope")
		}
	}
}

func TestSessionResumeSharesTheReadGateAndDeadline(t *testing.T) {
	db, a, _ := twoProjects(t)
	gate := newReadGate()
	gate.enterIngest()
	defer gate.leaveIngest()
	h := handlers(db, "", filepath.Join(t.TempDir(), "spool"), time.Now(), gate, newHealth())
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	got, err := h.SessionResume(ctx, ipc.SessionResumeRequest{Project: a.root, Host: "claude-code", HostSessionID: a.session})
	if !errors.Is(err, context.DeadlineExceeded) || got.LatestPrompt != nil || got.LatestReply != nil {
		t.Fatal("resume bypassed the occupied read gate or returned late bodies")
	}
}

func TestSessionResumeRedactionAudit(t *testing.T) {
	db, a, _ := twoProjects(t)
	h := handlers(db, "", filepath.Join(t.TempDir(), "spool"), time.Now(), newReadGate(), newHealth())
	cs := auditSession(t, h)
	for i, sample := range secrettest.All() {
		for _, arm := range []struct{ kind, field string }{{"Stop", "last_assistant_message"}, {"UserPromptSubmit", "prompt"}} {
			payload, err := json.Marshal(map[string]string{"cwd": a.root, "session_id": a.session, "hook_event_name": arm.kind, "transcript_path": "/synthetic/.claude/projects/session.jsonl", arm.field: sample.Value})
			if err != nil {
				t.Fatal(err)
			}
			if len(secret.Detect(payload)) == 0 {
				t.Fatal("audit sample is not detected before masking")
			}
			ingestOne(t, db, fmt.Sprintf("audit-%d-%s", i, arm.kind), payload)
		}
		req := ipc.SessionResumeRequest{Project: a.root, Host: "claude-code", HostSessionID: a.session}
		got, err := h.SessionResume(t.Context(), req)
		if err != nil || got.LatestReply == nil || got.LatestPrompt == nil || got.LatestReply.Body == "" || got.LatestPrompt.Body == "" {
			t.Fatal("audit did not reach both bodies")
		}
		auditClean(t, "resume reply", []secrettest.Sample{sample}, got)
		res := auditCall(t, cs, "get_session_resume", map[string]any{"project": a.root, "host": req.Host, "host_session_id": a.session})
		if res.IsError || res.StructuredContent == nil {
			t.Fatal("audit did not reach the MCP result")
		}
		auditClean(t, "resume MCP result", []secrettest.Sample{sample}, res)
	}
	res := auditCall(t, cs, "get_session_resume", map[string]any{"project": auditUNCProject, "host": "claude-code", "host_session_id": a.session})
	if !res.IsError {
		t.Fatal("UNC project was not refused")
	}
	auditClean(t, "resume MCP refusal", []secrettest.Sample{{Class: secret.ClassUserPath, Shape: "UNC", Value: auditUNCProject, Secret: auditUser}}, res)
}

func TestSessionResumeMasksBeforeExtractingAndBoundsUTF8(t *testing.T) {
	db, a, _ := twoProjects(t)
	const marker = "synthetic-private-value"
	payload, err := json.Marshal(map[string]any{
		"cwd": a.root, "session_id": a.session, "hook_event_name": "Stop", "transcript_path": "/synthetic/.claude/projects/session.jsonl",
		"last_assistant_message": "a" + strings.Repeat("한", 1000), "password": marker,
	})
	if err != nil {
		t.Fatal(err)
	}
	ingestOne(t, db, "bounded-reply", payload)
	req := ipc.SessionResumeRequest{Project: a.root, Host: "claude-code", HostSessionID: a.session}
	got, err := sessionResume(t.Context(), db, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.LatestReply == nil || len(got.LatestReply.Body) != 2398 || !utf8.ValidString(got.LatestReply.Body) || !got.LatestReply.Truncated {
		t.Fatal("masked body bound did not hold")
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), marker) || strings.Contains(string(b), "session.jsonl") {
		t.Fatal("metadata entered resume body")
	}
	// A credential nested in the body must be masked as well.
	payload, err = json.Marshal(map[string]string{"cwd": a.root, "session_id": a.session, "hook_event_name": "Stop", "transcript_path": "/synthetic/.claude/projects/session.jsonl", "last_assistant_message": `{"password":"` + marker + `"}`})
	if err != nil {
		t.Fatal(err)
	}
	ingestOne(t, db, "masked-reply", payload)
	got, err = sessionResume(t.Context(), db, req)
	if err != nil || got.LatestReply == nil || strings.Contains(got.LatestReply.Body, marker) || got.LatestReply.Body == "" {
		t.Fatal("body masking failed")
	}
	// Latest malformed body stays empty; an older answer is not substituted.
	payload, err = json.Marshal(map[string]any{"cwd": a.root, "session_id": a.session, "hook_event_name": "Stop", "transcript_path": "/synthetic/.claude/projects/session.jsonl", "last_assistant_message": 17})
	if err != nil {
		t.Fatal(err)
	}
	ingestOne(t, db, "empty-reply", payload)
	got, err = sessionResume(t.Context(), db, req)
	if err != nil || got.LatestReply == nil || got.LatestReply.EventID != "empty-reply" || got.LatestReply.Body != "" {
		t.Fatal("missing body was fabricated")
	}
	req.Project = ""
	if _, err := sessionResume(t.Context(), db, req); !errors.Is(err, ipc.ErrSessionResume) {
		t.Fatal("empty project was accepted")
	}
}

func TestSessionResumeOmitsOversizePayloadsAndUnusableReferences(t *testing.T) {
	db, a, _ := twoProjects(t)
	for _, tc := range []struct {
		id, body string
		omitted  bool
	}{
		{"8f1c2a10-0000-7000-8000-0000000000c3", "small answer", false},
		{strings.Repeat("z", 65), "no readable reference", false},
		{`C:\Users\synthetic-auditor\answer`, "masked reference", false},
		{"oversize", strings.Repeat("한", ipc.MaxEventPayloadBytes/3+1), true},
	} {
		payload, err := json.Marshal(map[string]string{"cwd": a.root, "session_id": a.session, "hook_event_name": "Stop", "transcript_path": "/synthetic/.claude/projects/session.jsonl", "last_assistant_message": tc.body})
		if err != nil {
			t.Fatal(err)
		}
		ingestOne(t, db, tc.id, payload)
		got, err := sessionResume(t.Context(), db, ipc.SessionResumeRequest{Project: a.root, Host: "claude-code", HostSessionID: a.session})
		if err != nil || got.LatestReply == nil {
			t.Fatal("latest reply absent")
		}
		if tc.omitted {
			if got.LatestReply.Body != "" || !got.LatestReply.Truncated {
				t.Fatal("oversize payload was read into a body")
			}
			continue
		}
		if got.LatestReply.Body != tc.body || got.LatestReply.Truncated {
			t.Fatal("small body changed")
		}
		if len(tc.id) > ipc.MaxEventIDBytes || secret.MaskString(tc.id) != tc.id {
			if got.LatestReply.EventID != "" {
				t.Fatal("unusable reference returned")
			}
		} else {
			full, err := getEvent(t.Context(), db, ipc.GetEventRequest{Project: a.root, ID: got.LatestReply.EventID})
			if err != nil || full.Event == nil || full.Event.ID != tc.id {
				t.Fatal("resume reference did not round-trip through get_event")
			}
		}
	}
}
