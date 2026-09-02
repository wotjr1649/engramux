package ipc

import (
	"errors"
	"fmt"
)

// SearchRequest is what a [Search] envelope carries in its Payload: the text a
// person typed and how many hits they want back.
//
// Query is not FTS5 query syntax and must not be treated as any - internal/search
// splits it and quotes every token, so nothing a person happens to type is an
// operator. It travels as typed because the tokens the excerpt has to find are
// derived from the same string on the service's side; splitting it here would
// put the same rule in two places.
type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
	// Project scopes the search to one project, by path. **Empty means
	// every project** (spec 5.9).
	//
	// That is the wire's meaning and it does not change: an existing
	// invocation sends no project and must keep returning what it always
	// returned, and a field with one meaning is one thing for a reader of
	// a frame to know. The MCP tool schema is where the argument is
	// *required*, because there the SDK enforces it structurally and the
	// caller is a model that has no working directory to mean.
	//
	// It is a path and not the derived project id: a caller knows where its
	// own worktree is and does not know what this product hashes it to.
	// internal/project's FromArgument turns one into the other and refuses
	// the shapes that must not be walked.
	Project string `json:"project"`
}

// The bounds on Limit. Both are here rather than in internal/search because
// they are about what fits in a reply frame, which is this package's business.
//
// DefaultSearchLimit is what an unset limit means. 20 is about a screen of
// blocks, and a person who wants more asks for more.
//
// MaxSearchLimit is the cap, and [MaxFrameLen] is what sets it. A reply frame
// holds 4 MiB, so 100 hits leave about 41 KiB per hit. A hit's own fields are
// far smaller than that - a 36-byte id, one of three host values, an epoch
// millisecond count, and an excerpt internal/search bounds to a few hundred
// runes - so the headroom is for events.event_name, which is whatever a
// payload's hook_event_name said and is therefore untrusted width. The cap is a
// margin and not a proof: a reply that still exceeds MaxFrameLen is refused by
// [WriteFrame], and the CLI reports a failed read rather than a short answer.
const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 100
)

// ErrSearchLimit is returned for a negative limit. It is refused rather than
// clamped because SQLite reads a negative LIMIT as "no limit", so letting one
// through would put an unbounded result set in a bounded frame - the one
// failure the cap above exists to prevent.
var ErrSearchLimit = errors.New("ipc: the search limit is negative")

// EffectiveLimit is the limit to use: the default for 0, the value itself up to
// [MaxSearchLimit], and that cap for anything above it.
//
// Over the cap is clamped rather than refused because the caller still gets an
// answer to the question it asked, only shorter; below zero there is no such
// reading, see [ErrSearchLimit].
func (r SearchRequest) EffectiveLimit() (int, error) {
	switch {
	case r.Limit < 0:
		return 0, fmt.Errorf("%w: %d", ErrSearchLimit, r.Limit)
	case r.Limit == 0:
		return DefaultSearchLimit, nil
	case r.Limit > MaxSearchLimit:
		return MaxSearchLimit, nil
	default:
		return r.Limit, nil
	}
}

// SearchReply is the service's reply to a [Search] request (spec 5.2), and a
// third reply document for the same reason [StatusReply] is a second one: the
// request type decides the reply document, and Type is what says which one this
// is. Read [StatusReply]'s doc comment for why that is not a payload field on
// [Ack].
//
// A search reply is the second place I-10's masking has to hold. The database
// keeps what it captured, secrets included; nothing that leaves the service
// carries one. Every Excerpt below is cut from a payload internal/secret has
// already masked as a whole document, so a hit on a secret returns the event
// without returning the secret.
type SearchReply struct {
	// Version is the wire protocol version the service speaks, the same
	// constant [Ack] carries.
	Version string `json:"version"`
	// Type is always [Search]. It is the discriminator that separates this
	// document from an [Ack].
	Type RequestType `json:"type"`
	// Hits are the matching events, best first: the slice index is the rank
	// FTS5 gave them. An empty slice is a search that matched nothing, and
	// is only distinguishable from a refusal after [SearchReply.Verify].
	Hits []SearchHit `json:"hits"`
	// Total is how many events matched before the limit cut the list, so
	// Total >= len(Hits) always, and Total > len(Hits) says the caller saw
	// the first len(Hits) of more (backlog 33). It is the count of what the
	// MATCH and the project filter admitted together, taken in the same
	// statement as the hits, so the two cannot disagree about which rows
	// were in.
	Total int64 `json:"total"`
}

// SearchHit is one matching event: enough to find it again and read it, and
// nothing that is not safe to print.
//
// # There are no offsets
//
// An excerpt carries no position into the stored payload, and cannot. The
// excerpt is cut from what [github.com/wotjr1649/engramux/internal/secret.Mask]
// returned, and masking re-encodes the document when it changed anything - so
// object keys come back in sorted order and '<', '>' and '&' come back
// HTML-escaped. The masked bytes are therefore neither the stored bytes nor the
// indexed text, and any offset against either would be a number that points at
// the wrong character. The excerpt is the whole of what a hit says about where
// the match was.
type SearchHit struct {
	// ID is events.id - the relay-minted UUIDv7 the event was ingested
	// under, and what identifies the event to every other command.
	ID string `json:"id"`
	// Host is events.host, one of internal/host.Detect's three values.
	Host string `json:"host"`
	// EventName is events.event_name: whatever the payload's
	// hook_event_name said, including "" for one that did not say (I-04).
	// It is untrusted width and untrusted bytes, so it is masked and then
	// bounded, in that order (I-10, spec 5.9) - the bound is about what fits
	// a frame, and masking is about what may leave the machine.
	EventName string `json:"event_name"`
	// EventNameTruncated is true when EventName is the leading part of a
	// longer name the bound cut (backlog 17). Absent otherwise, so a hit
	// whose name was whole is the document it always was. It is the only
	// way a reader can tell a cut name from a real one of exactly the
	// bound's length; there is no marker inside the string, because a
	// marker would be a character the name might legitimately end with.
	EventNameTruncated bool `json:"event_name_truncated,omitempty"`
	// ReceivedAtMS is events.received_at - milliseconds since the Unix
	// epoch, the same clock [Cell] reports.
	ReceivedAtMS int64 `json:"received_at_ms"`
	// Excerpt is a window of the masked payload's text around the match, or
	// its leading window when no query token survived masking. It is empty
	// for a payload with no string leaves at all.
	Excerpt string `json:"excerpt"`
}

// Sentinel errors [SearchReply.Verify] returns, distinguishable with
// [errors.Is].
var (
	ErrSearchVersion = errors.New("ipc: search reply version mismatch")
	ErrSearchType    = errors.New("ipc: reply is not a search reply")
)

// Verify holds only when the reply is a search reply from a service speaking
// this build's protocol version, and it is the checked path for accepting one.
//
// Nothing in Hits means anything until this returns nil. A rejected [Ack] - what
// the service answers when it will not serve the request - decodes into this
// struct without error and leaves Hits nil, which a caller would otherwise
// print as "nothing matched".
func (s SearchReply) Verify() error {
	if s.Version != Version {
		return fmt.Errorf("%w: got %.64q, want %q", ErrSearchVersion, s.Version, Version)
	}
	if s.Type != Search {
		return fmt.Errorf("%w: type %.64q, want %q", ErrSearchType, s.Type, Search)
	}
	return nil
}
