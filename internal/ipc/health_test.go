package ipc

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTheHealthFieldsAreNamedTheSameOnBothDocuments is backlog 31's wire
// half. The status reply and the doctor reply both carry the error count and
// the last checkpoint result, a client reads them by name, and a service that
// has not checkpointed yet says so with null rather than with a zero instant.
func TestTheHealthFieldsAreNamedTheSameOnBothDocuments(t *testing.T) {
	last := &CheckpointResult{AtMS: 1700000000123, Error: "busy"}
	for _, doc := range []any{
		StatusReply{Errors: 2, LastCheckpoint: last},
		DoctorReply{Errors: 2, LastCheckpoint: last},
	} {
		b, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal %T: %v", doc, err)
		}
		for _, want := range []string{`"errors":2`, `"last_checkpoint":{"at_ms":1700000000123,"error":"busy"}`} {
			if !strings.Contains(string(b), want) {
				t.Errorf("%T lacks %s: %s", doc, want, b)
			}
		}
	}

	fresh, err := json.Marshal(StatusReply{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(fresh), `"last_checkpoint":null`) {
		t.Errorf("a service with no checkpoint yet does not say null: %s", fresh)
	}
	ok, err := json.Marshal(CheckpointResult{AtMS: 1})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(ok), "error") {
		t.Errorf("a checkpoint that succeeded carries an error field: %s", ok)
	}
}
