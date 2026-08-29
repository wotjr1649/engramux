package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// GetEventRequest is what a [GetEvent] envelope carries in its Payload: which
// event, and which project it must be in.
//
// # The project is not optional, and it is not decoration
//
// events.id is the relay-minted UUIDv7 (I-05) and is the primary key of the
// whole table, so it is unique across every project in the database. A request
// carrying an id alone would therefore read across projects, and a caller that
// learned an id from one project's search could read an event from another.
//
// The two are checked together in the WHERE clause rather than checked in turn,
// so there is no state between them and nothing to get out of order. An event
// that exists under a different project is answered exactly as one that does not
// exist at all: found is false, and no field says which of the two it was.
//
// Project is a path, not the derived id: a caller knows where its own worktree
// is and does not know what this product hashes it to. internal/project's
// FromArgument is what turns one into the other, and what refuses the shapes
// that must not be walked (spec 5.9).
type GetEventRequest struct {
	ID      string `json:"id"`
	Project string `json:"project"`
}

// Sentinel errors a [GetEvent] request is refused with, distinguishable with
// [errors.Is]. Both are the request being unanswerable rather than the event
// being absent - an absent event is a reply, not an error.
var (
	ErrNoEventID  = errors.New("ipc: the get-event request carries no id")
	ErrNoProject  = errors.New("ipc: the request carries no project")
	ErrEventIDLen = errors.New("ipc: the get-event id is too long")
)

// MaxEventIDBytes bounds the id a caller may ask about.
//
// events.id is a UUIDv7 in its 36-character form, which is what the relay mints
// and what every id in the table is. The column has no CHECK, so a longer string
// is storable in principle; what this bounds is the string a caller sends, and
// 64 leaves room for a longer identity scheme without leaving the parameter
// unbounded. It is a guard on a value that reaches a query and an error message,
// not a schema claim.
const MaxEventIDBytes = 64

// Validate reports whether the request can be answered at all. It says nothing
// about whether the event exists.
func (r GetEventRequest) Validate() error {
	switch {
	case r.ID == "":
		return ErrNoEventID
	case len(r.ID) > MaxEventIDBytes:
		return fmt.Errorf("%w: %d bytes, cap %d", ErrEventIDLen, len(r.ID), MaxEventIDBytes)
	case r.Project == "":
		return ErrNoProject
	}
	return nil
}

// MaxEventPayloadBytes bounds the masked payload a [GetEventReply] carries.
//
// # It is measured, and here is what against
//
// Every other egress in this product is bounded - a search excerpt to 240 runes,
// an event name to 64, a search reply to 100 hits - and a whole payload would be
// the first that is not. The number cannot be reasoned from the stored size,
// because masking *expands*: [github.com/wotjr1649/engramux/internal/secret.Mask]
// re-marshals whenever anything matched and encoding/json HTML-escapes, so one
// source byte can become six.
//
// Measured over the 901 captures of the local corpus, spec 7.5's self-test
// filtered out: 881 payloads grew, 19 shrank, 1 was unchanged, and every masked
// result was still valid JSON. The largest masked payload is 173,609 B and the
// worst expansion is 1.1220x, on a 295 B payload that became 331 B.
// TestMaskingExpandsAndTheGetEventBoundHoldsOverTheCorpus in internal/secret is
// the reproduction, and it re-checks the bound over whatever the corpus now
// holds rather than pinning the figures above.
//
// So 1 MiB is 6.0x the largest masked payload real traffic has produced, and a
// quarter of [MaxFrameLen], which leaves the rest of the reply and the frame's
// own structure three quarters of the room. It is a guard against a pathological
// payload, not a bound anything real approaches - the same shape spec 7.4 gives
// the 512 KiB field cap, and deliberately a different number so the two are not
// mistaken for one decision.
const MaxEventPayloadBytes = 1 << 20

// GetEventReply is the service's reply to a [GetEvent] request, and a fourth
// reply document for the reason [StatusReply] is a second one: the request type
// decides the reply document, and Type is what says which one this is.
type GetEventReply struct {
	// Version is the wire protocol version the service speaks, the same
	// constant [Ack] carries.
	Version string `json:"version"`
	// Type is always [GetEvent]. It is the discriminator that separates
	// this document from an [Ack].
	Type RequestType `json:"type"`
	// Event is the event, or nil when the (id, project) pair matches no
	// row. Nil is a real answer and is only distinguishable from a refusal
	// after [GetEventReply.Verify] - a rejected Ack decodes into this
	// struct without error and leaves it nil too.
	Event *EventDocument `json:"event"`
}

// EventDocument is one whole event as it leaves the service: every field
// masked, and the payload bounded (I-10, spec 5.9).
//
// It carries what a reader needs to know what it is looking at and nothing
// more. project_id is absent because the caller named the project; tool_name
// and tool_use_id are absent because they are read out of the payload, which is
// here in full.
type EventDocument struct {
	// ID is events.id, the id the request asked for.
	ID string `json:"id"`
	// Host is events.host, one of internal/host.Detect's three values.
	Host string `json:"host"`
	// EventName is events.event_name, masked and bounded exactly as
	// [SearchHit.EventName] is, and for the same reasons.
	EventName string `json:"event_name"`
	// SessionID is events.session_id: spec 6's host-joined-to-host-session
	// identity, which is what [Session.ID] carries.
	SessionID string `json:"session_id"`
	// ReceivedAtMS is events.received_at, milliseconds since the Unix
	// epoch.
	ReceivedAtMS int64 `json:"received_at_ms"`
	// PrivacyClass is events.privacy_class: which of spec 6.1's classes
	// the stored payload matched, in internal/secret's stored spelling. It
	// is on the wire because it is the only thing that tells a reader the
	// payload below was rewritten rather than being what the host sent.
	PrivacyClass string `json:"privacy_class"`
	// Payload is the masked payload, spliced in as raw JSON rather than
	// carried as a string: a payload is a JSON document, and encoding one
	// as a string would escape every quote in it and roughly double what
	// the frame has to carry.
	//
	// It is nil when the masked payload exceeds [MaxEventPayloadBytes].
	// That is not a truncation: a cut JSON document is a document that no
	// longer parses, and one that does parse but is short is worse - it
	// looks whole. PayloadBytes still says how large it was, so a caller
	// can tell "too large" from "no such event", which answers Event nil
	// instead.
	//
	// A masked payload that is not valid JSON - which no capture in the
	// corpus produces, because the relay refuses a payload that is not a
	// JSON document - is carried as a JSON string, so this field always
	// holds something a decoder accepts.
	Payload json.RawMessage `json:"payload"`
	// PayloadBytes is the size of the masked payload, whether or not it is
	// carried. It is the masked size and not the stored size, because the
	// masked size is what the bound is about.
	PayloadBytes int `json:"payload_bytes"`
}

// Sentinel errors [GetEventReply.Verify] returns, distinguishable with
// [errors.Is].
var (
	ErrGetEventVersion = errors.New("ipc: get-event reply version mismatch")
	ErrGetEventType    = errors.New("ipc: reply is not a get-event reply")
)

// Verify holds only when the reply is a get-event reply from a service speaking
// this build's protocol version, and it is the checked path for accepting one.
//
// Nothing in it means anything until this returns nil. A rejected [Ack] decodes
// into this struct without error and leaves Event nil, which a caller would
// otherwise report as "no such event".
func (r GetEventReply) Verify() error {
	if r.Version != Version {
		return fmt.Errorf("%w: got %.64q, want %q", ErrGetEventVersion, r.Version, Version)
	}
	if r.Type != GetEvent {
		return fmt.Errorf("%w: type %.64q, want %q", ErrGetEventType, r.Type, GetEvent)
	}
	return nil
}
