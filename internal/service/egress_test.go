package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/store"
)

// TestPhase5GateNoReplyFieldCarriesAUserPath is spec 8's Phase 5 egress clause,
// and it is a sweep rather than a list of fields.
//
// # Why the whole document and not the two fields that were wrong
//
// The two known holes are events.event_name, which is whatever a payload's
// hook_event_name said, and StatusReply.DatabasePath, which internal/ipc used to
// justify leaving unmasked on three grounds - one SID inside the trust boundary,
// the pipe's DACL admitting only that SID, and the CLI printing it on the same
// machine (spec 2). Every one of those is void when the reader is a model, which
// may repeat what it read into a transcript that leaves the machine.
//
// Asserting on those two fields by name would pass for a third field nobody
// masked. So this marshals the reply the service actually hands internal/pipe
// and runs the detector over the bytes: whatever a future field carries, it is
// in the document this walks. secret.Detect is the same detector ingest tags
// with, so "clean" here means the same thing it means in privacy_class.
//
// # The masked form is clean under a re-scan, and that is not circular
//
// ClassUserPath captures the user directory name alone, so masking leaves
// `\Users\[redacted-user-path]\...` behind - a string the same rule matches
// again. It reports nothing because isPlaceholder skips a span that masking
// already wrote. That is the property this relies on, and internal/secret's own
// tests are what hold it.
//
// # Nothing here prints a reply
//
// A failure names the classes that survived and not the bytes that carry them.
// A test whose failure output is the thing it was checking for is a bad habit to
// leave in a repository whose origin is public.
func TestPhase5GateNoReplyFieldCarriesAUserPath(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))

	if ack, err := store.Ingest(t.Context(), db, ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: "8f1c2a10-0000-7000-8000-00000000e9a5",
		Payload:  egressPayload(t),
	}, store.SourcePipe, time.Now()); err != nil || ack != ipc.Committed {
		t.Fatalf("ingest the event: status %q, err %v", ack, err)
	}

	// The database path is supplied rather than taken from dir, so this
	// asserts the same thing on a machine whose temporary directory is not
	// under the user profile. It is the shape a real install has: spec 5.6
	// puts the file under the user's local application data directory.
	t.Run("status", func(t *testing.T) {
		reply, err := status(t.Context(), db, egressDatabasePath, filepath.Join(dir, spoolDir), time.Now(), newHealth())
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		// Guards the setup rather than the invariant: a breakdown with
		// no rows would carry no event name to have masked.
		if len(reply.Cells) != 1 {
			t.Fatalf("the breakdown has %d cells, want the 1 that was ingested", len(reply.Cells))
		}
		requireNoSecretSurvives(t, "the status reply", reply)
	})

	t.Run("search", func(t *testing.T) {
		reply, err := searchEvents(t.Context(), db, ipc.SearchRequest{Query: egressTerm})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(reply.Hits) != 1 {
			t.Fatalf("the search returned %d hits, want the 1 that was ingested", len(reply.Hits))
		}
		requireNoSecretSurvives(t, "the search reply", reply)
	})
}

// egressDatabasePath is the shape spec 5.6's layout has under a real user
// profile. It is a literal and not os.UserCacheDir, so the assertion does not
// depend on what this machine's user is called - or on there being one.
const egressDatabasePath = `C:\Users\fixture\AppData\Local\engramux\engramux.db`

// egressEventName is a hook_event_name carrying a user path. The column has no
// CHECK and internal/store takes the value from the payload verbatim, so this is
// storable, and every reply that reports a cell or a hit carries it outward.
//
// It is kept well under the 64-rune bound a hit puts on the name, so that this
// clause measures masking rather than the interaction between masking and that
// bound. Truncating a placeholder is safe either way - isPlaceholder decides on
// the prefix - and nothing here depends on it.
const egressEventName = `C:\Users\fixture\PostToolUse`

// egressTerm is the invented word the search finds the event by. It is in no
// fixture and in no other payload, and it is not the thing being masked - the
// event has to be findable by something safe to type.
const egressTerm = "frobnicatorMasked"

// egressPayload is one hook event whose event name is a user path and whose
// text carries the term.
//
// It is built with json.Marshal rather than written out as a literal, so the
// backslashes are escaped by the encoder and the stored bytes are the ones a
// host would have sent.
func egressPayload(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]string{
		"session_id":      "phase5-egress",
		"hook_event_name": egressEventName,
		"prompt":          "the event says " + egressTerm + " and nothing else",
	})
	if err != nil {
		t.Fatalf("marshal the payload: %v", err)
	}
	return b
}

// requireNoSecretSurvives marshals a reply the way internal/pipe does and
// requires the detector to find nothing in it.
func requireNoSecretSurvives(t *testing.T, what string, reply any) {
	t.Helper()
	b, err := json.Marshal(reply)
	if err != nil {
		t.Fatalf("marshal %s: %v", what, err)
	}
	if classes := secret.Detect(b); len(classes) != 0 {
		// The classes, never the document. See this file's first
		// comment for why.
		t.Errorf("%s carries %v", what, classes)
	}
}
