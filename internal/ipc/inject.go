package ipc

import (
	"errors"
	"fmt"
)

// InjectRequest is what an [Inject] envelope carries: the prompt a user just
// typed and the scope it may be answered from (memory spec rev.8, M-4).
//
// # Why this is not a Search request with a flag on it
//
// The pull path and the push path are different questions with different
// answers, and the spec says so at the one place they differ: a search returning
// its own last capture is an answer, and the same document injected into the
// next prompt is a distractor. Making the difference a boolean on
// [SearchRequest] would put the two paths one typo apart, and would give the
// MCP tool surface - which is the pull path - a field it must never set.
//
// It is also what makes the exclusion fail closed. A relay built after this
// change talking to a service built before it gets a refusal for an unknown
// request type and injects nothing, which is the right answer; a flag an old
// service ignored would have injected the whole unfiltered result.
type InjectRequest struct {
	// Prompt is the user's text, verbatim. It is not a query and must not
	// be treated as one: internal/inject reduces it, and doing that here
	// would put the reduction on the wire where a second caller could
	// disagree with it.
	Prompt string `json:"prompt"`
	// Project is the absolute path of the worktree the prompt was typed in,
	// which both hosts send as `cwd`. Empty is every project, the meaning
	// [SearchRequest.Project] already carries.
	Project string `json:"project"`
	// ExcludeID is the id the prompt's own event was ingested under. The
	// relay delivers before it injects, so without this the prompt is its
	// own top hit every time.
	ExcludeID string `json:"exclude_id"`
}

// InjectReply is the service's answer: the finished payload, or nothing.
//
// The payload arrives assembled, capped and fenced rather than as a hit list.
// Gate M5 is a bound on the bytes a host receives and gate M9 is a property of
// those same bytes, so both are properties of one string - and a reply that
// handed back parts for the relay to join would put the two gates on a seam
// nothing measures.
type InjectReply struct {
	// Version is the wire protocol version the service speaks.
	Version string `json:"version"`
	// Type is always [Inject].
	Type RequestType `json:"type"`
	// Context is the fenced payload the host is given as additionalContext.
	// Empty is an abstention, which is capability P2 and a success.
	Context string `json:"context,omitempty"`
	// Reason names why an abstention was one. It is diagnostic only and
	// nothing branches on it; it carries no corpus text, only one of
	// internal/inject's own constants.
	Reason string `json:"reason,omitempty"`
}

// Sentinel errors [InjectReply.Verify] returns.
var (
	ErrInjectVersion = errors.New("ipc: inject reply version mismatch")
	ErrInjectType    = errors.New("ipc: reply is not an inject reply")
)

// Verify holds only when the reply is an inject reply from a service speaking
// this build's protocol version.
//
// It matters more here than on the other replies. A rejected [Ack] decodes into
// this struct without error and leaves Context empty, which is exactly what an
// abstention looks like - so without this a refused request would be reported
// as "nothing to inject" and the relay would never learn the service could not
// answer.
func (r InjectReply) Verify() error {
	if r.Version != Version {
		return fmt.Errorf("%w: got %.64q, want %q", ErrInjectVersion, r.Version, Version)
	}
	if r.Type != Inject {
		return fmt.Errorf("%w: type %.64q, want %q", ErrInjectType, r.Type, Inject)
	}
	return nil
}
