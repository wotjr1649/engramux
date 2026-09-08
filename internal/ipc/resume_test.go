package ipc

import (
	"errors"
	"strings"
	"testing"
)

func TestSessionResumeRequestRequiresExactScope(t *testing.T) {
	valid := SessionResumeRequest{Project: `D:\work`, Host: "claude-code", HostSessionID: "one"}
	for _, alter := range []func(*SessionResumeRequest){
		func(r *SessionResumeRequest) { r.Project = "" },
		func(r *SessionResumeRequest) { r.Host = "unknown" },
		func(r *SessionResumeRequest) { r.HostSessionID = "" },
		func(r *SessionResumeRequest) { r.HostSessionID = strings.Repeat("a", 257) },
	} {
		r := valid
		alter(&r)
		if !errors.Is(r.Validate(), ErrSessionResume) {
			t.Fatal("incomplete or overlong session scope accepted")
		}
	}
	for _, host := range []string{"claude-code", "codex"} {
		r := valid
		r.Host, r.HostSessionID = host, strings.Repeat("a", 256)
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
