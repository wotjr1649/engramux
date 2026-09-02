package search_test

import (
	"encoding/json"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
)

// t6Payload is one event whose every hit field is decided by the document
// itself: `prompt_id` makes internal/host.Detect answer claude-code, the event
// name is what hook_event_name says, and the prompt is short enough that the
// whole masked text is the excerpt.
//
// The cwd is the repository's placeholder user path and is there to be masked -
// it is what makes [TestSearchCarriesWhatAHitNeeds]'s expected excerpt an
// assertion about the masking, the sorted key order a re-encode produces, and
// the leaf separator, all at once.
const t6Payload = `{
  "session_id": "t6-hit-fields",
  "hook_event_name": "UserPromptSubmit",
  "prompt_id": "t6-prompt",
  "cwd": "C:\\Users\\fixture\\workspace\\fixture-project",
  "prompt": "the borogove was mimsy that morning"
}`

// TestSearchCarriesWhatAHitNeeds pins every field of a hit by value.
//
// The excerpt is the interesting one. The payload is shorter than the window,
// so the excerpt is the whole of the masked document's text - which is not the
// document's own text: `fixture` is gone from the cwd, and the leaves arrive in
// sorted key order rather than in the order they were written, because masking
// changed something and therefore re-encoded. Both are stated here so that a
// change to either is a failure with the two strings side by side.
func TestSearchCarriesWhatAHitNeeds(t *testing.T) {
	docs := []doc{{name: "t6.json", payload: json.RawMessage(t6Payload)}}
	db := ingestAll(t, docs)

	hits, _, err := search.Search(t.Context(), db, "borogove", "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search returned %d hits, want 1", len(hits))
	}
	got := hits[0]

	if got.ID != docs[0].id {
		t.Errorf("ID = %q, want %q", got.ID, docs[0].id)
	}
	if got.Host != "claude-code" {
		t.Errorf("Host = %q, want %q", got.Host, "claude-code")
	}
	if got.EventName != "UserPromptSubmit" {
		t.Errorf("EventName = %q, want %q", got.EventName, "UserPromptSubmit")
	}

	// Read back from the row the hit came from, so that a field wired to
	// the wrong column - the rowid, the ingest clock, a neighbouring row -
	// is a mismatch and not a plausible-looking number.
	var receivedAt int64
	if err := db.QueryRowContext(t.Context(),
		`SELECT received_at FROM events WHERE id = ?`, docs[0].id).Scan(&receivedAt); err != nil {
		t.Fatalf("read events.received_at: %v", err)
	}
	if got.ReceivedAtMS != receivedAt {
		t.Errorf("ReceivedAtMS = %d, want %d", got.ReceivedAtMS, receivedAt)
	}

	want := `C:\Users\[redacted-user-path]\workspace\fixture-project` + "\n" +
		"UserPromptSubmit" + "\n" +
		"the borogove was mimsy that morning" + "\n" +
		"t6-prompt" + "\n" +
		"t6-hit-fields"
	if got.Excerpt != want {
		t.Errorf("Excerpt =\n%q\nwant\n%q", got.Excerpt, want)
	}
}
