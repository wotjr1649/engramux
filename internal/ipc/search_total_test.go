package ipc

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSearchReplyCarriesTheTotalOnTheWire is backlog 33's wire half: the
// field's spelling is what an MCP client and the CLI both read, so it is
// pinned by name and round-tripped by value.
func TestSearchReplyCarriesTheTotalOnTheWire(t *testing.T) {
	in := SearchReply{Version: Version, Type: Search, Total: 137}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"total":137`) {
		t.Errorf("the reply does not carry the total under its decided name: %s", b)
	}
	var out SearchReply
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 137 {
		t.Errorf("total after a round trip = %d, want 137", out.Total)
	}
}
