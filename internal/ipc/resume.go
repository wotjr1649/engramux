package ipc

import "errors"

// SessionResumeRequest names one host session exactly. It is an explicit MCP
// read, not an automatic injection request or an FTS query.
type SessionResumeRequest struct {
	Project       string `json:"project" jsonschema:"absolute project worktree path; UNC paths are refused"`
	Host          string `json:"host" jsonschema:"exact host from list_sessions: claude-code or codex"`
	HostSessionID string `json:"host_session_id" jsonschema:"exact host_session_id from list_sessions or the session the user named; not an event id"`
}

var ErrSessionResume = errors.New("ipc: session resume requires a project, a known host and a nonempty host session id of at most 256 bytes")

func (r SessionResumeRequest) Validate() error {
	if r.Project == "" || (r.Host != "claude-code" && r.Host != "codex") || r.HostSessionID == "" || len(r.HostSessionID) > 256 {
		return ErrSessionResume
	}
	return nil
}

// ResumeBodyBytes bounds each decoded body after whole-payload masking.
const ResumeBodyBytes = 2400

// SessionResumeReply carries captured text, never a generated summary. A null
// record means that event type was not captured in this exact scope.
type SessionResumeReply struct {
	LatestPrompt *ResumeMessage `json:"latest_prompt"`
	LatestReply  *ResumeMessage `json:"latest_reply"`
}

type ResumeMessage struct {
	// EventID is empty when too wide for get_event or changed by masking.
	EventID      string `json:"event_id"`
	ReceivedAtMS int64  `json:"received_at_ms"`
	Body         string `json:"body"`
	Truncated    bool   `json:"truncated"`
}
