package ipc

import "encoding/json"

// RequestType is the request type carried in an Envelope (spec 5.2),
// because I-08 routes every CLI read over the same pipe an ingest write
// uses.
type RequestType string

// The request types spec 5.2 names: the four Phase 1 ones, then the two Phase 5
// adds for the tool surface (spec 5.9).
//
// There is no Drain. Spec 5.2 declared one for an upgrade step that never had a
// wire path, and it was withdrawn on 2026-08-30 (backlog 32): the spool is
// durable and the service drains it at every start, so stopping a service
// without draining it loses nothing and an upgrade is stop, replace, start.
const (
	IngestEvent RequestType = "IngestEvent"
	Status      RequestType = "Status"
	Doctor      RequestType = "Doctor"
	Search      RequestType = "Search"
	// GetEvent reads one whole event back, by id and project together.
	GetEvent RequestType = "GetEvent"
	// ListSessions lists one project's sessions.
	ListSessions RequestType = "ListSessions"
	// GetMemory reads one whole native memory item back, by id (memory spec
	// rev.2, M-2 decision 9). It is the fifth tool and the first request
	// type added since Step 1 withdrew Drain.
	GetMemory RequestType = "GetMemory"
)

// Envelope is the JSON document a frame's payload holds. Payload is kept as
// raw JSON rather than parsed here: its shape depends on Type, and this
// package does not own any request type's payload schema — only the outer
// envelope and the frame it travels in.
//
// Version is the wire protocol version the sender speaks, using the same
// Version constant Ack does. The relay already detects a mismatched service
// via Ack.Verify (spec 5.3); this field lets the service detect the other
// direction — a stale relay. That matters because spec 5.5's upgrade path
// (stop, replace, start) can leave an old relay binary, still
// installed as a hook, talking to a new service, and the service is the
// side holding the database.
//
// IngestID is the relay-minted idempotency key (I-05) that becomes
// events.id, and the value Ack.IngestID must echo back for a Verify call to
// accept (spec 5.3) — the "one it sent" that section requires. It is
// meaningful only when Type is IngestEvent; the other four request types
// leave it empty rather than each getting its own per-type request wrapper.
type Envelope struct {
	Version  string          `json:"version"`
	Type     RequestType     `json:"type"`
	IngestID string          `json:"ingest_id"`
	Payload  json.RawMessage `json:"payload"`
}
