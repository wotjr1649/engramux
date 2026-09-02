package ipc

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestARejectedAckCarriesItsReason is backlog 27. A refused request answered
// a bare rejected Ack, so the caller saw "the service replied rejected" and
// nothing else - a person could guess, a model could not correct itself. The
// reason rides on the Ack under a pinned name, Verify's error repeats it, and
// a committed Ack carries no such field at all, so an old relay's happy path
// reads byte for byte what it always read.
func TestARejectedAckCarriesItsReason(t *testing.T) {
	refused := Ack{Version: Version, Status: Rejected, IngestID: "x", Reason: "the project path is a UNC share"}
	err := refused.Verify("x")
	if !errors.Is(err, ErrAckRejected) {
		t.Fatalf("Verify = %v, want ErrAckRejected", err)
	}
	if !strings.Contains(err.Error(), "the project path is a UNC share") {
		t.Errorf("Verify's error does not carry the reason: %v", err)
	}

	b, err := json.Marshal(refused)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"reason":"the project path is a UNC share"`) {
		t.Errorf("the reason is not on the wire under its decided name: %s", b)
	}

	committed, err := json.Marshal(Ack{Version: Version, Status: Committed, IngestID: "x"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(committed), "reason") {
		t.Errorf("a committed Ack carries a reason field: %s", committed)
	}
}
