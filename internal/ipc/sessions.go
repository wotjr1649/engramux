package ipc

import (
	"errors"
	"fmt"
)

// ListSessionsRequest is what a [ListSessions] envelope carries in its Payload:
// which project, and how many sessions.
//
// Project scopes the listing to one project, by path. **Empty means every
// project**, which is [SearchRequest.Project]'s meaning and now this one's
// (backlog 47): the two are the read commands a person runs next to each other,
// and opposite defaults had the first person to run both read "six hits" and
// "no sessions" as capture being broken. The MCP tool schema still requires one,
// where the caller is a model with no working directory - spec 5.9 states that
// rule at the level each surface can enforce it.
type ListSessionsRequest struct {
	Project string `json:"project"`
	Limit   int    `json:"limit"`
}

// EffectiveLimit is the limit to use, with the same three rules
// [SearchRequest.EffectiveLimit] applies and for the same reasons - the default
// for 0, the value up to [MaxSearchLimit], that cap above it, and a refusal
// below zero because SQLite reads a negative LIMIT as "no limit".
//
// It shares those two constants rather than minting a pair of its own. A session
// row is far smaller than a search hit, so a cap sized for hits is generous
// here; a second pair of numbers would be two things to keep in step for no
// measured reason.
func (r ListSessionsRequest) EffectiveLimit() (int, error) {
	return SearchRequest{Limit: r.Limit}.EffectiveLimit()
}

// ListSessionsReply is the service's reply to a [ListSessions] request, and a
// fifth reply document for the reason [StatusReply] is a second one.
type ListSessionsReply struct {
	// Version is the wire protocol version the service speaks, the same
	// constant [Ack] carries.
	Version string `json:"version"`
	// Type is always [ListSessions]. It is the discriminator that separates
	// this document from an [Ack].
	Type RequestType `json:"type"`
	// Sessions are the project's sessions, newest first. An empty slice is
	// a project with no sessions - including a project nothing has ever
	// ingested into, which is the same state - and is only distinguishable
	// from a refusal after [ListSessionsReply.Verify].
	Sessions []Session `json:"sessions"`
}

// Session is one row of the sessions table, less the project it belongs to,
// which the reply carries once.
type Session struct {
	// ID is sessions.id: spec 6's host joined to the host session id.
	ID string `json:"id"`
	// ProjectRoot is projects.root for the project this session belongs to,
	// **masked**: the column holds a normalised worktree root, which is the
	// exact shape internal/secret's user-path class matches in 900 of 902
	// captures (spec 6.1).
	//
	// It is per session and not one field for the whole reply, and that is
	// backlog 47's other half: an empty project lists every project, and a
	// listing spanning several has no single root to name. It is still what
	// makes "a path that is not the one you meant" visible when a project
	// *was* named - the same job the reply-level field had.
	ProjectRoot string `json:"project_root"`
	// Host is sessions.host, one of internal/host.Detect's three values.
	Host string `json:"host"`
	// HostSessionID is sessions.host_session_id - the id the host itself
	// used, which is what a person looking at a host's own transcript
	// would recognise. It is "" for a payload that carried none (I-04).
	HostSessionID string `json:"host_session_id"`
	// Status is sessions.status: active, stopped or ended.
	Status string `json:"status"`
	// CreatedAtMS is when the service first saw the session, not when the
	// session started - rows are created lazily on the first event, and
	// only some sessions produce a SessionStart at all.
	CreatedAtMS int64 `json:"created_at_ms"`
	// EndedAtMS is sessions.ended_at, or 0 for a session that has not
	// ended. The column is the one nullable one in the table and 0 is how
	// its NULL travels: an epoch millisecond of 0 is 1970, which is not a
	// time any row holds.
	EndedAtMS int64 `json:"ended_at_ms"`
}

// Sentinel errors [ListSessionsReply.Verify] returns, distinguishable with
// [errors.Is].
var (
	ErrListSessionsVersion = errors.New("ipc: list-sessions reply version mismatch")
	ErrListSessionsType    = errors.New("ipc: reply is not a list-sessions reply")
)

// Verify holds only when the reply is a list-sessions reply from a service
// speaking this build's protocol version, and it is the checked path for
// accepting one.
//
// Nothing in it means anything until this returns nil. A rejected [Ack] decodes
// into this struct without error and leaves Sessions nil, which a caller would
// otherwise report as "this project has no sessions".
func (r ListSessionsReply) Verify() error {
	if r.Version != Version {
		return fmt.Errorf("%w: got %.64q, want %q", ErrListSessionsVersion, r.Version, Version)
	}
	if r.Type != ListSessions {
		return fmt.Errorf("%w: type %.64q, want %q", ErrListSessionsType, r.Type, ListSessions)
	}
	return nil
}
