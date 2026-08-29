package ipc

import (
	"errors"
	"fmt"
)

// DoctorReply is the service's reply to a [Doctor] request (spec 5.2): what a
// local diagnostic needs and what no other reply carries.
//
// # This one is not masked, and it is the only one
//
// DatabasePath is the real path (spec 5.9). Every other reply masks it, because
// the reader of those may be a model that repeats what it read into a transcript
// that leaves the machine. This reply has exactly one caller - `engramux doctor`,
// printing to the terminal of the SID that owns the file - and it is deliberately
// **not** one of the four MCP tools. A tool that exposed it would put the
// unmasked path back on the surface the masking exists for.
//
// # Why the comparison and not the strings alone
//
// TokenizerLive and TokenizerExpected are both here, and [DoctorReply.TokenizerAgrees]
// is derived rather than sent. goose records that a migration ran and never
// re-runs it, and it does not checksum the file, so an applied migration edited
// in place leaves an index built by the old clause and a file claiming the new
// one. Nothing in the product noticed, because I-07 leaves the service as the
// only process that can look at the live schema and nothing asked it to.
//
// The strings travel so that a disagreement says what to do about it. The
// verdict is computed at both ends from the same two strings, so there is no
// third value that could be wrong on its own.
type DoctorReply struct {
	// Version is the wire protocol version the service speaks, the same
	// constant [Ack] carries.
	Version string `json:"version"`
	// Type is always [Doctor]. It is the discriminator that separates this
	// document from an [Ack].
	Type RequestType `json:"type"`
	// UptimeMS, Events and SpoolDepth are [StatusReply]'s numbers, repeated
	// here so that `doctor` asks one question and has one refusal path
	// rather than two.
	UptimeMS   int64 `json:"uptime_ms"`
	Events     int64 `json:"events"`
	SpoolDepth int   `json:"spool_depth"`
	// DatabasePath is the real path of the database the service opened.
	DatabasePath string `json:"database_path"`
	// TokenizerLive is the tokenizer the live search index was created
	// with, read out of sqlite_schema; TokenizerExpected is the one the
	// embedded migration declares. Both are empty when the index carries no
	// explicit clause, which is a real state and not a failure.
	TokenizerLive     string `json:"tokenizer_live"`
	TokenizerExpected string `json:"tokenizer_expected"`
	// TokenizerReadError is why the two above are empty when they could not
	// be read at all - a database with no index, most plainly. Empty when
	// the read succeeded.
	TokenizerReadError string `json:"tokenizer_read_error"`
}

// TokenizerAgrees reports whether the live index and the migration declare the
// same tokenizer. It is false when either could not be read, because "no answer"
// is not agreement.
func (d DoctorReply) TokenizerAgrees() bool {
	return d.TokenizerReadError == "" && d.TokenizerLive == d.TokenizerExpected
}

// Sentinel errors [DoctorReply.Verify] returns, distinguishable with
// [errors.Is].
var (
	ErrDoctorVersion = errors.New("ipc: doctor reply version mismatch")
	ErrDoctorType    = errors.New("ipc: reply is not a doctor reply")
)

// Verify holds only when the reply is a doctor reply from a service speaking
// this build's protocol version, and it is the checked path for accepting one.
//
// Nothing in it means anything until this returns nil. A rejected [Ack] decodes
// into this struct without error and leaves every number zero and every string
// empty - which a caller would otherwise print as a service with no events, no
// database and a tokenizer that agrees with itself.
func (d DoctorReply) Verify() error {
	if d.Version != Version {
		return fmt.Errorf("%w: got %.64q, want %q", ErrDoctorVersion, d.Version, Version)
	}
	if d.Type != Doctor {
		return fmt.Errorf("%w: type %.64q, want %q", ErrDoctorType, d.Type, Doctor)
	}
	return nil
}
