package ipc

import (
	"errors"
	"fmt"
)

// MemoryHit is one matching native memory item, carried beside the event hits
// in a [SearchReply] (memory spec rev.2, M-2 decision 9).
//
// # Why it travels in the search reply and not in a tool of its own
//
// P4 is defined as one query reaching answers that exist only in the other
// host's sessions or memory. A second search tool breaks that literally: the
// caller has to know to make a second call, and a model that does not is the
// agent-retrieved regression SWE Context Bench measured at four points below
// having no memory at all. So the reply grows an array rather than the surface
// growing a second search.
//
// # Why it is a separate array and not a kind flag on SearchHit
//
// The two lists are ranked by two indexes, and bm25 is computed against one
// index's document frequencies. Merging them into one ordered list would mean
// inventing a normalisation rule between populations of a few hundred and tens
// of thousands, which would then be an unmeasured input to M3's own recall
// number. Two arrays say what is true: each is ranked, and the two rankings are
// not comparable.
type MemoryHit struct {
	// ID is memory_items.id, derived from the host, the file and the block
	// rather than minted, so it survives the collection tick that reads the
	// same block again. It is what [GetMemory] takes.
	ID string `json:"id"`
	// Host is which host wrote it: the same two values events.host carries,
	// minus "unknown" - a memory file is found under one host's home or the
	// other's and there is no third answer.
	Host string `json:"host"`
	// Kind names the artefact and the block within it, free text on the same
	// terms the column is: a new artefact in either host's directory should
	// widen this rather than fail.
	Kind string `json:"kind"`
	// SourcePath is the file the item came from, masked (I-10). It is not a
	// path a caller can open, and it is not meant to be - what it says is
	// which file, to a reader who is on the machine that owns it.
	SourcePath string `json:"source_path"`
	// Title is a display line cut from the same text the body holds, masked
	// for the same reason: a Claude Code note's title is its own name and a
	// Codex section's is its first line, and either can be a path.
	Title string `json:"title"`
	// HostModifiedMS is the host's own timestamp - when the note was
	// written, which is not when the fact was true (spec 3, P3) - in
	// milliseconds since the Unix epoch. Zero means the host wrote none, and
	// that is an ordinary answer: 1 of the 18 Claude Code notes read on
	// 2026-09-02 carries no modified key.
	HostModifiedMS int64 `json:"host_modified_ms"`
	// Excerpt is a window of the masked body around the match, on the same
	// terms [SearchHit.Excerpt] is, offsets included - which is to say there
	// are none.
	Excerpt string `json:"excerpt"`
}

// MemoryDocument is one whole native memory item, what [GetMemoryReply] carries.
//
// It is [MemoryHit] plus the body and the project the host filed it under. The
// division is the one [EventDocument] makes against [SearchHit]: a hit is enough
// to decide whether to read it, and the document is the read.
type MemoryDocument struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	Kind       string `json:"kind"`
	SourcePath string `json:"source_path"`
	// EntryKey is the block within the file, and "" means the item is the
	// whole file or its leading block. Masked: a Codex section's key is a
	// thread id and a Claude Code index entry's is a file name.
	EntryKey string `json:"entry_key"`
	// ProjectPath is what the host filed it under, masked. The two hosts
	// write different things there - a directory key and an absolute cwd -
	// and neither converts to the other without the filesystem, which is why
	// scoping compares it against both forms of the asked-for project.
	ProjectPath    string `json:"project_path"`
	Title          string `json:"title"`
	HostModifiedMS int64  `json:"host_modified_ms"`
	// PrivacyClass is what internal/secret made of the stored body, in the
	// same stored form events.privacy_class carries.
	PrivacyClass string `json:"privacy_class"`
	// BodyBytes is the size of the masked body, set whether or not Body is.
	// So "too large" is distinguishable from "no such item", which answers a
	// nil Item instead.
	BodyBytes int `json:"body_bytes"`
	// Body is the masked body, left out entirely when it is over
	// [MaxMemoryBodyBytes] rather than cut.
	Body string `json:"body,omitempty"`
}

// MaxMemoryBodyBytes bounds the masked body a [GetMemoryReply] carries, on the
// rule [MaxEventPayloadBytes] is under: every reply field is bounded by
// something, and the number cannot be reasoned from the stored size because
// masking expands.
//
// Measured 2026-09-02 over this machine's native memory, 303 items parsed out of
// 81 files: the largest body is 20,156 B and the largest masked body is also
// 20,156 B - nothing in that item matched a rule, so what this bound has to
// survive is an expansion that corpus does not exhibit. 128 KiB is 6.6x the
// largest measured, which is the ratio MaxEventPayloadBytes was set at, and an
// eighth of [MaxFrameLen].
//
// TestGateM1EveryNativeMemoryFileParsesAndKeepsItsText in internal/memory logs
// both figures on its passing path, so the number above comes out of the
// committed harness rather than a probe nobody kept.
const MaxMemoryBodyBytes = 128 << 10

// GetMemoryRequest asks for one memory item by id, within one project.
//
// Project has [SearchRequest.Project]'s meaning and not [GetEventRequest]'s:
// empty is every project. A memory item may belong to no project the database
// has a row for - Codex's memory is global and Claude Code's is filed under a
// directory key - so requiring one here would make those items unreachable.
type GetMemoryRequest struct {
	ID      string `json:"id"`
	Project string `json:"project"`
}

// Sentinel errors [GetMemoryRequest.Validate] and [GetMemoryReply.Verify]
// return, distinguishable with [errors.Is].
var (
	ErrNoMemoryID    = errors.New("ipc: get-memory request carries no id")
	ErrMemoryIDLen   = errors.New("ipc: get-memory id is too long")
	ErrMemoryVersion = errors.New("ipc: get-memory reply version mismatch")
	ErrMemoryType    = errors.New("ipc: reply is not a get-memory reply")
)

// Validate reports whether the request can be answered at all. It says nothing
// about whether the item exists.
func (r GetMemoryRequest) Validate() error {
	switch {
	case r.ID == "":
		return ErrNoMemoryID
	case len(r.ID) > MaxEventIDBytes:
		// The same bound an event id gets, and for the same reason: this is
		// a guard on a value that reaches a query and an error message, not
		// a schema claim. A memory id is 32 hex characters today.
		return fmt.Errorf("%w: %d bytes, cap %d", ErrMemoryIDLen, len(r.ID), MaxEventIDBytes)
	}
	return nil
}

// GetMemoryReply is the service's reply to a [GetMemory] request.
type GetMemoryReply struct {
	Version string      `json:"version"`
	Type    RequestType `json:"type"`
	// Item is the item, or nil when the id matches no row in that scope. Nil
	// is a real answer and is only distinguishable from a refusal after
	// [GetMemoryReply.Verify].
	Item *MemoryDocument `json:"item"`
}

// Verify holds only when the reply is a get-memory reply from a service speaking
// this build's protocol version, and it is the checked path for accepting one.
func (r GetMemoryReply) Verify() error {
	if r.Version != Version {
		return fmt.Errorf("%w: got %.64q, want %q", ErrMemoryVersion, r.Version, Version)
	}
	if r.Type != GetMemory {
		return fmt.Errorf("%w: type %.64q, want %q", ErrMemoryType, r.Type, GetMemory)
	}
	return nil
}
