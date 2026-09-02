package ipc

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/engramux/internal/fixtures"
)

// fixtureBytes returns the exact bytes of the named Phase 1 fixture,
// reached through fixtures.All() so that a fixture dropped from that list
// fails here rather than going quietly untested (mirrors fixtures_test.go's
// payloadOf).
func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	for _, f := range fixtures.All() {
		if f.File != name {
			continue
		}
		b, err := f.Bytes()
		if err != nil {
			t.Fatalf("fixtures: Bytes(%s): %v", name, err)
		}
		return b
	}
	t.Fatalf("fixtures.All() does not list %q", name)
	return nil
}

// compactJSON reduces raw to its compact form, the same transform
// encoding/json applies to a json.RawMessage field when marshalling its
// parent. The fixture on disk is pretty-printed for human readers; using
// the compact form as the envelope's Payload keeps env.Payload equal to
// what a decoder actually hands back, so the round-trip check below
// compares like with like instead of pretty bytes against wire bytes.
func compactJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("json.Compact: %v", err)
	}
	return buf.Bytes()
}

// TestEnvelopeGolden is the wire-format gate: it asserts the encoder
// produces exactly the committed bytes, and the decoder reads exactly the
// original struct back out of them. A struct round-trip (marshal then
// unmarshal the same value) would pass even with every JSON tag wrong; this
// does not, because the golden file is a second, independent source of
// truth for what the bytes must be.
func TestEnvelopeGolden(t *testing.T) {
	env := Envelope{
		Version:  Version,
		Type:     IngestEvent,
		IngestID: wantIngestID,
		Payload:  compactJSON(t, fixtureBytes(t, fixtures.CodexSessionEnd)),
	}

	got, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	//nolint:gosec // G304: reading this package's own testdata directory by construction
	want, err := os.ReadFile(filepath.Join("testdata", "envelope.golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Marshal(Envelope) =\n%s\nwant\n%s", got, want)
	}

	var decoded Envelope
	if err := json.Unmarshal(want, &decoded); err != nil {
		t.Fatalf("Unmarshal golden: %v", err)
	}
	if decoded.Version != env.Version {
		t.Errorf("decoded Version = %q, want %q", decoded.Version, env.Version)
	}
	if decoded.Type != env.Type {
		t.Errorf("decoded Type = %q, want %q", decoded.Type, env.Type)
	}
	if decoded.IngestID != env.IngestID {
		t.Errorf("decoded IngestID = %q, want %q", decoded.IngestID, env.IngestID)
	}
	if !bytes.Equal(decoded.Payload, env.Payload) {
		t.Errorf("decoded Payload =\n%s\nwant\n%s", decoded.Payload, env.Payload)
	}
}

// TestRequestTypes pins the six spelling values I-08 and spec 5.2 require:
// a typo here would silently misroute every request of that type.
func TestRequestTypes(t *testing.T) {
	want := map[RequestType]string{
		IngestEvent:  "IngestEvent",
		Status:       "Status",
		Doctor:       "Doctor",
		Search:       "Search",
		GetEvent:     "GetEvent",
		ListSessions: "ListSessions",
	}
	for got, want := range want {
		if string(got) != want {
			t.Errorf("RequestType = %q, want %q", got, want)
		}
	}
}
