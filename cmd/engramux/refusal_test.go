package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestRepliedNamesTheServicesReason is backlog 27 at the terminal. Every CLI
// read decodes the frame as its own reply document and, when Verify refuses
// it, reports what came back; since the service says why it refused, that is
// what a person reads, and the raw frame is printed only when there is no
// reason to print instead.
func TestRepliedNamesTheServicesReason(t *testing.T) {
	verify := errors.New("ipc: reply is not a search reply")

	raw, err := json.Marshal(ipc.Ack{Version: ipc.Version, Status: ipc.Rejected, Reason: "the project path is a UNC share"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := replied(verify, raw)
	if !errors.Is(got, verify) {
		t.Errorf("the verify error is not wrapped: %v", got)
	}
	if !strings.Contains(got.Error(), "the service refused it: the project path is a UNC share") {
		t.Errorf("the reason is not reported: %v", got)
	}
	if strings.Contains(got.Error(), `"status"`) {
		t.Errorf("the raw frame is printed beside the reason: %v", got)
	}

	// No reason to show: an Ack from a build that carries none, or bytes
	// that are not an Ack at all. The frame is what there is, bounded.
	for _, raw := range [][]byte{
		[]byte(`{"version":"1","status":"rejected","ingest_id":""}`),
		[]byte(`not json at all`),
	} {
		got := replied(verify, raw)
		if !errors.Is(got, verify) || !strings.Contains(got.Error(), "the service replied") {
			t.Errorf("without a reason, replied(%q) = %v", raw, got)
		}
	}
}
