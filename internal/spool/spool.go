// Package spool is the on-disk holding area for events the relay could not
// deliver. It is what makes I-04 true: an event that cannot reach the service
// goes to a file instead of being lost.
//
// This package owns the writer, the three bounds spec 5.6 puts on the
// directory, and the [Drainer] that replays records back into the service.
//
// # The record
//
// One record is one file: the name is the relay-minted id with a ".json"
// suffix, and the body is the event's bytes as the relay defined them at its
// stdin boundary - the same bytes it would have put on the wire, which is what
// makes the two delivery paths store one byte string rather than two.
//
// The id is in the name and nowhere else, which is the whole point. The body
// is bytes a host wrote and this process never validated - Phase 1 gates on a
// byte-for-byte round trip, so nothing here parses, compacts or re-encodes it
// - and a body that will not parse would take its own id down with it if the
// id lived inside the document. A drain that cannot read a record can still
// name it, quarantine it, and tell a human which event it was.
//
// It also removes a way to get I-05 wrong: there is no id field for a drain to
// re-mint, because the id is the file it opened.
//
// Everything else the service needs is a constant. The envelope the drain
// rebuilds carries ipc.Version and ipc.IngestEvent, and events.source is set
// by the ingest path, not carried in the record (spec 6).
package spool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ext is the suffix a completed record carries. It is what separates a record
// from the temp file a write is staged in, so the drain listing "*.json" can
// never pick up a write in progress.
const ext = ".json"

// tempPattern stages a write. The leading dot and the missing ext keep it out
// of the drain's listing; os.CreateTemp replaces the '*'.
const tempPattern = ".partial-*"

// ErrID is returned when the id is not a UUID in the canonical form.
var ErrID = errors.New("spool: id is not a canonical uuid")

// canonicalUUID reports whether id is a UUID spelled the one way this package
// accepts: the canonical 36 characters, lower case. It is what [Write] requires
// of a record's name and what [scan] reads one back as, so a name Write could
// not have produced is not a record.
//
// uuid.Validate is not that check. It accepts four spellings - the canonical
// 36, "urn:uuid:<36>", "{<36>}", and 32 hex digits with no hyphens - and
// uuid.Parse accepts upper-case hex in all of them. Each alternate is a bug
// here rather than a convenience:
//
//   - "urn:uuid:..." puts a ':' in the file name, and on Windows ':' opens an
//     alternate data stream: os.Rename either fails with ERROR_INVALID_NAME or
//     writes a stream on a file called "urn" that os.ReadDir will never list.
//     Either way the event is gone.
//   - The other three are legal file names that round-trip through scan and
//     reach the database under an events.id that is a *different string* from
//     the canonical spelling of the same UUID. Two rows for one event, which is
//     I-05 broken.
//
// Parsing and comparing the round trip is the whole test: uuid.UUID.String
// emits the canonical form, so id survives it only if that is what it already
// was.
//
// Nothing reaches this today - cmd/engramux passes uuid.NewV7().String() - so
// it is a guard one caller away from mattering rather than a bug being fixed.
func canonicalUUID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u.String() == id
}

// Dir is the spool directory: "spool" under Engramux's directory under the
// user's local application data directory (spec 5.6). os.UserCacheDir returns
// %LocalAppData% on Windows.
func Dir() (string, error) {
	local, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("spool: locate the local application data directory: %w", err)
	}
	return filepath.Join(local, "engramux", "spool"), nil
}

// Write saves one record: payload under id, in dir, creating dir if it does
// not exist. It writes a temp file, syncs it, and renames it over the final
// name (spec 5.6), so a reader never sees a half-written record and a machine
// that loses power mid-write loses the temp file rather than the record.
//
// Writing the same id twice leaves one record, because the id is the
// idempotency key (I-05) and two writes under one id are one event by
// definition. os.Rename maps to MoveFileEx with replace semantics on Windows,
// so the second write wins rather than failing.
//
// payload is written exactly as given - no length cap, no validation, no
// re-encoding. The cap is the directory's, not the record's: a full spool
// refuses the write with [ErrRecordBound] or [ErrByteBound] rather than
// truncating anything, and validating a payload here would be a second place
// that can decide to drop an event.
func Write(dir, id string, payload []byte) error {
	// id becomes a file name. It is minted by uuid.NewV7 today and so is
	// always a canonical UUID, but a "..\\.." would escape dir, and
	// rejecting every shape that is not one removes the escape instead of
	// blacklisting the characters that reach it. It is also a real check on
	// the caller: an empty id means the mint was skipped, and a record with
	// no id cannot be replayed idempotently.
	if !canonicalUUID(id) {
		return fmt.Errorf("%w: %.64q", ErrID, id)
	}

	// The bounds, measured against what is already on disk (spec 5.6). The
	// relay is the only thing that writes a record, so a bound it does not
	// enforce is a bound the directory does not have. The same scan drops
	// records past the age bound, which is what usually makes the room the
	// two bounds below are asking for.
	//
	// Refusing is not a drop: the caller still holds the bytes and reports
	// that it could not save them. A spool that silently discarded the
	// oldest record to make room would turn a full disk into missing events
	// nobody counted.
	held, used, err := scan(dir, time.Now())
	if err != nil {
		return err
	}
	if len(held) >= maxRecords {
		return fmt.Errorf("%w: %d records, cap %d", ErrRecordBound, len(held), maxRecords)
	}
	if used+int64(len(payload)) > maxBytes {
		return fmt.Errorf("%w: %d bytes held plus %d, cap %d", ErrByteBound, used, len(payload), maxBytes)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("spool: create %s: %w", dir, err)
	}

	f, err := os.CreateTemp(dir, tempPattern)
	if err != nil {
		return fmt.Errorf("spool: create a temp record in %s: %w", dir, err)
	}
	tmp := f.Name()
	// Every failure below removes the temp file. Leaving it would grow the
	// directory without ever showing up in the drain's listing, which is
	// the one kind of spool file nothing would clean up.
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if err = f.Chmod(0o600); err != nil {
		return fmt.Errorf("spool: restrict %s: %w", tmp, err)
	}
	if _, err = f.Write(payload); err != nil {
		return fmt.Errorf("spool: write %s: %w", tmp, err)
	}
	// The event is already undeliverable; this file is the only copy of it,
	// so it is worth an fsync.
	if err = f.Sync(); err != nil {
		return fmt.Errorf("spool: sync %s: %w", tmp, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("spool: close %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, filepath.Join(dir, id+ext)); err != nil {
		return fmt.Errorf("spool: rename %s: %w", tmp, err)
	}
	return nil
}
