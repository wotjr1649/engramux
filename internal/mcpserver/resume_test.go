package mcpserver

import (
	"context"
	"errors"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

func TestSessionResumeMCPPassesExactArgumentsAndReply(t *testing.T) {
	want := ipc.SessionResumeRequest{Project: `D:\work`, Host: "codex", HostSessionID: "literal-session"}
	h := stubHandler()
	var got ipc.SessionResumeRequest
	h.SessionResume = func(_ context.Context, in ipc.SessionResumeRequest) (ipc.SessionResumeReply, error) {
		got = in
		return ipc.SessionResumeReply{LatestReply: &ipc.ResumeMessage{EventID: stubEventID, Body: "recorded decision", ReceivedAtMS: 17}}, nil
	}
	endpoint, token := serveForTest(t, h)
	cs := connect(t, endpoint, token)
	var reply ipc.SessionResumeReply
	call(t, cs, "get_session_resume", map[string]any{"project": want.Project, "host": want.Host, "host_session_id": want.HostSessionID}, &reply)
	if got != want || reply.LatestReply == nil || reply.LatestReply.Body != "recorded decision" || reply.LatestReply.EventID != stubEventID || reply.LatestReply.ReceivedAtMS != 17 || reply.LatestPrompt != nil {
		t.Fatal("session resume arguments or reply changed in transport")
	}
	// Generic masking is separately audited through the real service path.
	if _, err := New(h); err != nil {
		t.Fatal(err)
	}
	h.SessionResume = nil
	if _, err := New(h); !errors.Is(err, ErrNoHandler) {
		t.Fatal("missing resume handler accepted")
	}
}
