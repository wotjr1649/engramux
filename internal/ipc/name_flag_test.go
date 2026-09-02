package ipc

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTheTruncationFlagIsNamedTheSameOnBothDocuments is backlog 17's wire
// half. A hit and an event document both carry a bounded event name, and a
// client reads the flag by its JSON name, so the name is pinned on both and
// its absence is pinned too: a name that was not cut carries no flag, so an
// old client's document is byte-identical to what it read before.
func TestTheTruncationFlagIsNamedTheSameOnBothDocuments(t *testing.T) {
	hit, err := json.Marshal(SearchHit{EventName: "x", EventNameTruncated: true})
	if err != nil {
		t.Fatalf("marshal a hit: %v", err)
	}
	if !strings.Contains(string(hit), `"event_name_truncated":true`) {
		t.Errorf("a cut hit does not carry the flag under its decided name: %s", hit)
	}
	doc, err := json.Marshal(EventDocument{EventName: "x", EventNameTruncated: true})
	if err != nil {
		t.Fatalf("marshal a document: %v", err)
	}
	if !strings.Contains(string(doc), `"event_name_truncated":true`) {
		t.Errorf("a cut document does not carry the flag under its decided name: %s", doc)
	}

	whole, err := json.Marshal(SearchHit{EventName: "x"})
	if err != nil {
		t.Fatalf("marshal a whole hit: %v", err)
	}
	if strings.Contains(string(whole), "event_name_truncated") {
		t.Errorf("a name that was not cut carries the flag anyway: %s", whole)
	}
}
