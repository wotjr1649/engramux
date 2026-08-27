package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const wantIngestID = "018f9a3b-0000-7000-8000-000000000001"

// TestAckGolden pins Ack's wire shape the same way TestEnvelopeGolden pins
// Envelope's: a mutated JSON tag must fail this test, not just a
// marshal-then-unmarshal round trip of the same struct.
func TestAckGolden(t *testing.T) {
	ack := Ack{Version: Version, Status: Committed, IngestID: wantIngestID}

	got, err := json.Marshal(ack)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	//nolint:gosec // G304: reading this package's own testdata directory by construction
	want, err := os.ReadFile(filepath.Join("testdata", "ack.golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Marshal(Ack) =\n%s\nwant\n%s", got, want)
	}

	var decoded Ack
	if err := json.Unmarshal(want, &decoded); err != nil {
		t.Fatalf("Unmarshal golden: %v", err)
	}
	if decoded != ack {
		t.Errorf("decoded Ack = %+v, want %+v", decoded, ack)
	}
}

// TestAckVerify_Accepts is the one shape Verify must let through: matching
// version, Committed status, matching ingest ID.
func TestAckVerify_Accepts(t *testing.T) {
	ack := Ack{Version: Version, Status: Committed, IngestID: wantIngestID}
	if err := ack.Verify(wantIngestID); err != nil {
		t.Errorf("Verify() = %v, want nil", err)
	}
}

// TestAckVerify_AcceptsDuplicateCommit documents spec 5.3's edge case so a
// later reader does not "fix" it away: a duplicate ingest ACKs Committed,
// not Rejected, and Verify must accept it exactly like a first-time commit.
func TestAckVerify_AcceptsDuplicateCommit(t *testing.T) {
	ack := Ack{Version: Version, Status: Committed, IngestID: wantIngestID}
	if err := ack.Verify(wantIngestID); err != nil {
		t.Errorf("Verify() on a duplicate-ingest Ack = %v, want nil", err)
	}
}

// TestAckVerify_Rejects covers the three named failure modes, each with its
// own sentinel so a caller (and this test) can tell them apart with
// errors.Is. This is the regression test for rev.2: it unmarshalled an Ack
// without checking status, so a Rejected reply counted as success.
func TestAckVerify_Rejects(t *testing.T) {
	tests := []struct {
		name string
		ack  Ack
		want error
	}{
		{
			name: "version mismatch",
			ack:  Ack{Version: "9", Status: Committed, IngestID: wantIngestID},
			want: ErrAckVersion,
		},
		{
			name: "rejected status",
			ack:  Ack{Version: Version, Status: Rejected, IngestID: wantIngestID},
			want: ErrAckRejected,
		},
		{
			name: "ingest id mismatch",
			ack:  Ack{Version: Version, Status: Committed, IngestID: "not-the-id-we-sent"},
			want: ErrAckIngestID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ack.Verify(wantIngestID)
			if !errors.Is(err, tt.want) {
				t.Errorf("Verify() error = %v, want errors.Is(_, %v)", err, tt.want)
			}
		})
	}
}
