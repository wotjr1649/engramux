package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestAnOverlongEventNameIsCutAndFlaggedOnBothReplies is backlog 16 and 17
// through the production read path. events.event_name is whatever a payload's
// hook_event_name said and nothing bounds it before the wire, so the service
// cuts it at maxEventNameRunes - and since backlog 17 says so on the hit and
// on the event document both, because a cut name with no mark reads as a real
// name of exactly the bound's length. A name under the bound carries no flag.
func TestAnOverlongEventNameIsCutAndFlaggedOnBothReplies(t *testing.T) {
	db, a, _ := twoProjects(t)
	long := strings.Repeat("가", maxEventNameRunes+1)
	const id = "8f1c2a10-0000-7000-8000-0000000000c0"
	payload, err := json.Marshal(map[string]string{
		"session_id":      "session-long",
		"hook_event_name": long,
		"cwd":             a.root,
		"prompt":          "an overlong name in project a",
	})
	if err != nil {
		t.Fatalf("marshal the payload: %v", err)
	}
	ingestOne(t, db, id, payload)
	want := strings.Repeat("가", maxEventNameRunes)

	reply, err := searchEvents(t.Context(), db, ipc.SearchRequest{Query: "overlong", Project: a.root})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(reply.Hits) != 1 || reply.Hits[0].ID != id {
		t.Fatalf("search returned %d hits, want the one overlong event", len(reply.Hits))
	}
	if hit := reply.Hits[0]; hit.EventName != want || !hit.EventNameTruncated {
		t.Errorf("hit: name of %d runes, truncated = %v; want %d runes and true",
			len([]rune(hit.EventName)), hit.EventNameTruncated, maxEventNameRunes)
	}

	whole, err := searchEvents(t.Context(), db, ipc.SearchRequest{Query: "event", Project: a.root})
	if err != nil {
		t.Fatalf("search the fixture: %v", err)
	}
	if len(whole.Hits) != 1 {
		t.Fatalf("the fixture search returned %d hits, want 1", len(whole.Hits))
	}
	if hit := whole.Hits[0]; hit.EventNameTruncated {
		t.Errorf("a name under the bound is flagged as cut: %q", hit.EventName)
	}

	got, err := getEvent(t.Context(), db, ipc.GetEventRequest{ID: id, Project: a.root})
	if err != nil {
		t.Fatalf("get_event: %v", err)
	}
	if got.Event == nil {
		t.Fatal("get_event answered no event")
	}
	if got.Event.EventName != want || !got.Event.EventNameTruncated {
		t.Errorf("document: name of %d runes, truncated = %v; want %d runes and true",
			len([]rune(got.Event.EventName)), got.Event.EventNameTruncated, maxEventNameRunes)
	}
}
