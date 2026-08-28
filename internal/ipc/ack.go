package ipc

import (
	"errors"
	"fmt"
)

// Version is the wire protocol version this build of Engramux speaks. Ack
// carries it back so a caller can tell a version-mismatched reply from a
// same-version one (spec 5.3).
const Version = "1"

// AckStatus is the outcome of an IngestEvent request.
type AckStatus string

const (
	// Committed means the event is durably stored. A duplicate ingest —
	// the idempotency key (I-05) already exists — also ACKs Committed; it
	// is not an error, and Verify accepts it exactly like a first commit.
	Committed AckStatus = "committed"
	// Rejected means the event was not stored, and the relay must treat it
	// as a failed send: it spools the event and exits 0. I-04 is explicit -
	// "if it cannot be delivered it is spooled" - and spec 5.3 accepts only
	// Committed as success, so Rejected is a delivery failure like any
	// other. The spool retries it and eventually quarantines it, which is
	// not the same as dropping it.
	//
	// rev.2's bug was the opposite reading: it unmarshalled the ACK without
	// checking status, a Rejected reply counted as success, and nothing
	// spooled the event - THAT is what made it permanently lost. Rejected
	// itself loses nothing.
	Rejected AckStatus = "rejected"
)

// Ack is the service's reply to an IngestEvent request (spec 5.3).
type Ack struct {
	Version  string    `json:"version"`
	Status   AckStatus `json:"status"`
	IngestID string    `json:"ingest_id"`
}

// Sentinel errors Verify returns, distinguishable with errors.Is.
var (
	ErrAckVersion  = errors.New("ipc: ack version mismatch")
	ErrAckRejected = errors.New("ipc: ack status is rejected")
	ErrAckIngestID = errors.New("ipc: ack ingest id mismatch")
)

// Verify is the checked path for accepting an Ack, and the only convenient
// one: it holds only when a.Version matches Version, a.Status is Committed,
// and a.IngestID equals wantIngestID — the exact three-way check spec 5.3
// requires. rev.2 unmarshalled an Ack and skipped the status check, so a
// Rejected reply counted as success and the event was lost for good; there
// is deliberately no shortcut field (an "OK bool") on Ack that lets a caller
// repeat that mistake.
func (a Ack) Verify(wantIngestID string) error {
	if a.Version != Version {
		return fmt.Errorf("%w: got %q, want %q", ErrAckVersion, a.Version, Version)
	}
	if a.Status != Committed {
		return fmt.Errorf("%w: status %q", ErrAckRejected, a.Status)
	}
	if a.IngestID != wantIngestID {
		return fmt.Errorf("%w: got %q, want %q", ErrAckIngestID, a.IngestID, wantIngestID)
	}
	return nil
}
