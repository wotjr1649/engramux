package secret_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/secret"
)

// corpusDir is the local, gitignored raw capture corpus, relative to this
// package. Same path and skip pattern as internal/search's corpus mode.
var corpusDir = filepath.Join("..", "..", ".capture", "fixtures-raw")

// TestMaskingExpandsAndTheGetEventBoundHoldsOverTheCorpus is the reproduction
// behind spec 7.1's mask-expansion row and behind
// [ipc.MaxEventPayloadBytes] being a measured number rather than a chosen one.
//
// # Why the number could not be reasoned
//
// Masking is the last thing to touch a payload before it leaves the service, and
// it *expands*: [secret.Mask] re-marshals whenever anything matched, and
// encoding/json HTML-escapes, so one source byte can become six. Spec 6.1's
// user-path class matches essentially every real payload, so the re-encode path
// is the normal one. A bound derived from the stored size would therefore be a
// bound on the wrong number.
//
// # What it asserts, and what it only reports
//
// It asserts two things: that every masked payload in the corpus is under the
// bound, which is what makes the bound a guard against pathological input rather
// than one real traffic runs into, and that masking never turned a payload into
// something that is not JSON, which is what lets a reply splice it in raw
// instead of escaping it into a string.
//
// The sizes and the ratio are logged and not asserted. They are the numbers spec
// 7.1 carries, and pinning them would make this test fail whenever the corpus
// grows - which is the wrong thing to be told.
//
// # Nothing here prints a file name or a payload
//
// The corpus is 900 of 902 captures carrying an absolute user path (spec 7.5).
// Counts and sizes are safe; anything drawn from a document is not.
func TestMaskingExpandsAndTheGetEventBoundHoldsOverTheCorpus(t *testing.T) {
	payloads := corpusPayloads(t)

	var grew, shrank, unchanged, notJSON, over int
	var maxStored, maxMasked int
	var worstRatio float64
	var worstStored, worstMasked int
	for _, p := range payloads {
		m := secret.Mask(p)
		switch {
		case len(m) > len(p):
			grew++
		case len(m) < len(p):
			shrank++
		default:
			unchanged++
		}
		if !json.Valid(m) {
			notJSON++
		}
		if len(m) > ipc.MaxEventPayloadBytes {
			over++
		}
		maxStored = max(maxStored, len(p))
		maxMasked = max(maxMasked, len(m))
		if r := float64(len(m)) / float64(len(p)); r > worstRatio {
			worstRatio, worstStored, worstMasked = r, len(p), len(m)
		}
	}

	t.Logf("%d payloads: %d grew, %d shrank, %d unchanged", len(payloads), grew, shrank, unchanged)
	t.Logf("largest stored %d B, largest masked %d B, worst expansion %.4fx (%d -> %d)",
		maxStored, maxMasked, worstRatio, worstStored, worstMasked)

	if notJSON != 0 {
		t.Errorf("%d masked payloads are not valid JSON: a reply splices the masked payload in "+
			"as a JSON value, so this is the property that lets it", notJSON)
	}
	if over != 0 {
		t.Errorf("%d masked payloads exceed the %d-byte bound: the bound is meant to be a guard "+
			"against pathological input, and real traffic reaching it means the number is wrong",
			over, ipc.MaxEventPayloadBytes)
	}
	// The premise, checked rather than assumed: a corpus that masks to
	// nothing would satisfy both assertions above and measure nothing.
	if grew == 0 {
		t.Error("no payload grew under masking, so this measures no expansion at all")
	}
}

// corpusPayloads reads every capture's payload, or skips the test when the
// corpus is not on this machine.
//
// The payload is the `payload` member of the capture file and not the file: the
// file also carries the probe's own `_cap` metadata, and measuring the whole
// file would measure that too. What the service stores is this member.
//
// One caveat belongs with the numbers this feeds: the probe wrote each payload
// back out through its own encoder, so these bytes are HTML-escaped where the
// host's original bytes were not - the largest here is 173,288 B where the
// probe recorded 171,764 B of stdin for the same event. It does not move the
// result: masking re-encodes with that same encoder, so a payload that masks at
// all masks to the same bytes from either spelling, and the difference only
// makes the unmasked side of the comparison slightly larger.
func corpusPayloads(t *testing.T) [][]byte {
	t.Helper()
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Skipf("no capture corpus on this machine: %v", err)
	}

	var out [][]byte
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		//nolint:gosec // G304: reading the local corpus directory by construction
		raw, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			t.Fatalf("read a capture: %v", err)
		}
		var capture struct {
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &capture); err != nil {
			t.Fatalf("parse a capture: %v", err)
		}
		if len(capture.Payload) == 0 {
			t.Fatal("a capture carries no payload")
		}
		var head struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(capture.Payload, &head)
		if head.SessionID == "selftest" { // spec 7.5
			continue
		}
		out = append(out, capture.Payload)
	}
	if len(out) == 0 {
		t.Fatal("the corpus directory holds no captures")
	}
	return out
}
