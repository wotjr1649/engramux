package ipc

import (
	"errors"
	"fmt"
)

// StatusReply is the service's reply to a [Status] request (spec 5.2), and it
// is a different document from [Ack] on purpose.
//
// # Why not a payload field on Ack
//
// Ack has no field a reply body could ride in, and adding one was the obvious
// move. It is the wrong one twice over:
//
//   - [AckStatus]'s two values are about the durability of a write. There is no
//     honest value for a read: Committed means "the event is durably stored",
//     and answering it to a request that stored nothing makes the word mean
//     two things.
//   - [Ack.Verify] is the exact three-way check spec 5.3 requires of the relay,
//     and the reason rev.2 lost events was a caller that skipped part of it.
//     Every request type sharing one reply document is one step from a caller
//     that verifies the version and forgets the rest, because the rest would no
//     longer apply to every reply.
//
// So the request type decides the reply document, and Type is what says which
// one this is. A client that asked for Status and got a rejection - an Ack,
// because that is what the service answers when it will not serve the request -
// decodes a StatusReply whose Type is empty, and [StatusReply.Verify] fails.
// Without the discriminator it would decode a document of zeroes and print
// them as the service's real numbers.
//
// # What is in it
//
// I-08 routes every CLI read over the pipe, because I-07 leaves no other way to
// see any of this: the service holds the database exclusively, so the numbers
// below exist in one process and nothing else can look.
//
// DatabasePath names a file under the user's local application data directory,
// so it carries a Windows user name. That is not an I-10 egress: spec 2 puts a
// single Windows SID inside the trust boundary, the pipe's DACL admits only
// that SID and SYSTEM (spec 5.2), and the CLI prints it on the same machine.
type StatusReply struct {
	// Version is the wire protocol version the service speaks, the same
	// constant [Ack] carries.
	Version string `json:"version"`
	// Type is always [Status]. It is the discriminator that separates this
	// document from an [Ack].
	Type RequestType `json:"type"`
	// SpoolDepth is the number of records waiting in the spool directory.
	SpoolDepth int `json:"spool_depth"`
	// Events is the number of rows in the events table.
	Events int64 `json:"events"`
	// UptimeMS is how long this service process has been running, in
	// milliseconds. A duration rather than a start instant, so a reader does
	// not have to trust two clocks.
	UptimeMS int64 `json:"uptime_ms"`
	// DatabasePath is the database the service opened.
	DatabasePath string `json:"database_path"`
}

// Sentinel errors [StatusReply.Verify] returns, distinguishable with
// [errors.Is].
var (
	ErrStatusVersion = errors.New("ipc: status reply version mismatch")
	ErrStatusType    = errors.New("ipc: reply is not a status reply")
)

// Verify holds only when the reply is a status reply from a service speaking
// this build's protocol version. It is the checked path for accepting one, the
// way [Ack.Verify] is for an ACK: everything a caller reads out of a
// StatusReply is meaningless until this returns nil, because an unrelated
// reply decodes into this struct without error and leaves every number zero.
func (s StatusReply) Verify() error {
	if s.Version != Version {
		return fmt.Errorf("%w: got %.64q, want %q", ErrStatusVersion, s.Version, Version)
	}
	if s.Type != Status {
		return fmt.Errorf("%w: type %.64q, want %q", ErrStatusType, s.Type, Status)
	}
	return nil
}
