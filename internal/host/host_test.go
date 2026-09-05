package host

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/fixtures"
)

// TestDetectFixtures asserts the exact host value for each of the four Phase 1
// fixtures, naming the spec 4.3 step each one must be classified through.
func TestDetectFixtures(t *testing.T) {
	// Every fixture carries a transcript_path, so under spec 4.3's corrected
	// order the path decides all of them. What each label records is what the
	// key fallback would have said, which is the only thing that can disagree.
	steps := map[string]string{
		fixtures.ClaudePostToolUseObject: "path .claude, and prompt_id agrees",
		fixtures.CodexPostToolUseString:  "path .codex, and model agrees",
		fixtures.CodexPostToolUseArray:   "path .codex, and model agrees",
		fixtures.CodexSessionEnd:         "path .codex, and no key rule can reach it",
		fixtures.ClaudeSessionStart:      "path .claude, and the key rule says codex - the cell this fixture exists for",
	}

	all := fixtures.All()
	if len(all) != len(steps) {
		t.Fatalf("fixtures.All() returned %d fixtures, this test has step mappings for %d; add or remove one", len(all), len(steps))
	}

	for _, f := range all {
		step, ok := steps[f.File]
		if !ok {
			t.Fatalf("fixture %s has no step mapping in this test; add one", f.File)
		}
		t.Run(f.File+"/"+step, func(t *testing.T) {
			raw, err := f.Bytes()
			if err != nil {
				t.Fatalf("Bytes: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("unmarshal %s: %v", f.File, err)
			}
			if got := Detect(payload); got != f.Host {
				t.Errorf("Detect() = %q, want %q (%s)", got, f.Host, step)
			}
		})
	}
}

// TestDetectUnknown asserts spec 4.3's fallthrough: a payload carrying none of
// the step 1/2/3 keys classifies as "unknown", not an error and not a default
// to either host (I-04).
func TestDetectUnknown(t *testing.T) {
	cases := map[string]map[string]any{
		"empty payload":        {},
		"irrelevant keys only": {"hook_event_name": "Notification", "session_id": "abc123", "cwd": "C:\\Users\\x"},
		"transcript_path outside both hosts' directories": {"transcript_path": "C:\\Users\\x\\workspace\\notes.jsonl"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Detect(payload); got != "unknown" {
				t.Errorf("Detect(%v) = %q, want %q", payload, got, "unknown")
			}
		})
	}
}

// TestDetectPresentNullBoundary tests the choice documented on present(): an
// explicit JSON null still counts as the key being present, because
// encoding/json cannot tell "absent" and "null" apart by value alone and the
// key's existence - not its value - is what signals which host's payload
// schema produced the document. See present()'s doc comment for the reasoning.
func TestDetectPresentNullBoundary(t *testing.T) {
	cases := []struct {
		json string
		want string
	}{
		{`{"prompt_id": null}`, "claude-code"},
		{`{"effort": null}`, "claude-code"},
		{`{"model": null}`, "codex"},
		{`{"turn_id": null}`, "codex"},
	}
	for _, c := range cases {
		t.Run(c.json, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(c.json), &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := Detect(payload); got != c.want {
				t.Errorf("Detect(%s) = %q, want %q (explicit null must still count as present)", c.json, got, c.want)
			}
		})
	}
}

// corpusDir is the local, gitignored raw capture corpus, relative to this
// package. Same path and skip pattern as internal/fixtures.
var corpusDir = filepath.Join("..", "..", ".capture", "fixtures-raw")

// TestDetectCorpusMeasurement runs Detect over every real capture in the raw
// corpus and asserts the spec 4.3 measurement still holds: 887 captures
// classify via steps 1-2, 13 via step 3, 900 classified in total, 0 unknown.
// It skips when the corpus is absent, following the pattern internal/fixtures
// established (os.ReadDir + errors.Is(fs.ErrNotExist) + t.Skipf).
//
// Two captures are filtered before classification, per spec 7.5 and 7.1:
//   - the synthetic self-test payload, identified by session_id == "selftest"
//   - the synthetic capture-probe payload, identified by its structure (its
//     payload is exactly the single key "probe") rather than its filename -
//     this file's name also happens to contain "selftest", which is exactly
//     why filtering by filename would be wrong.
func TestDetectCorpusMeasurement(t *testing.T) {
	entries, err := os.ReadDir(corpusDir)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no raw corpus at %s; host-detection measurement is unchecked on this machine", corpusDir)
	}
	if err != nil {
		t.Fatalf("read %s: %v", corpusDir, err)
	}

	var (
		filteredSelftest int
		filteredProbe    int
		viaStep1or2      int
		viaStep3         int
		unknown          int
		disagree         int
	)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(corpusDir, e.Name())
		//nolint:gosec // G304: reading the local corpus directory by construction
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var capture struct {
			Cap struct {
				Host  string `json:"host"`
				Event string `json:"event_declared"`
			} `json:"_cap"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(raw, &capture); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		if capture.Payload["session_id"] == "selftest" {
			filteredSelftest++
			continue
		}
		if _, ok := capture.Payload["probe"]; ok && len(capture.Payload) == 1 {
			filteredProbe++
			continue
		}

		host := Detect(capture.Payload)
		hasStep1or2 := present(capture.Payload, "prompt_id") || present(capture.Payload, "effort") ||
			present(capture.Payload, "model") || present(capture.Payload, "turn_id")

		// The two rules disagreeing is what backlog 49 was. Counting it
		// here is the assertion that would have caught it, had the
		// corpus held one Claude Code SessionStart: the key rules
		// answered Codex for a payload whose transcript sits under
		// .claude, and the old order let the wrong one win.
		if byKey := keyRuleSays(capture.Payload); byKey != "" && transcriptDir(capture.Payload) != "" && byKey != host {
			disagree++
			t.Errorf("%s: transcript_path says %q and the key rule says %q", e.Name(), host, byKey)
		}

		switch {
		case host == "unknown":
			unknown++
		case hasStep1or2:
			viaStep1or2++
		default:
			viaStep3++
			if host != "codex" || capture.Cap.Event != "SessionEnd" {
				t.Errorf("%s: step-3 classification = (%s, %s), want (codex, SessionEnd)", e.Name(), host, capture.Cap.Event)
			}
		}
	}

	if disagree != 0 {
		t.Errorf("%d captures where the path rule and the key rule disagree, want 0. "+
			"a non-zero count is not a failure of this test - it is a cell where spec 4.3's "+
			"two rules give different answers, and which one is right has to be decided", disagree)
	}
	if filteredSelftest != 1 {
		t.Errorf("filtered %d selftest captures, want 1 (spec 7.5)", filteredSelftest)
	}
	if filteredProbe != 1 {
		t.Errorf("filtered %d probe captures, want 1 (spec 7.1: 902 files, 901 real plus 1 probe)", filteredProbe)
	}
	if unknown != 0 {
		t.Errorf("%d captures classified unknown, want 0 after filtering selftest and probe", unknown)
	}
	if viaStep1or2 != 887 {
		t.Errorf("classified %d via steps 1-2, want 887 (spec 4.3)", viaStep1or2)
	}
	if viaStep3 != 13 {
		t.Errorf("classified %d via step 3, want 13 (spec 4.3)", viaStep3)
	}
	if classified := viaStep1or2 + viaStep3; classified != 900 {
		t.Errorf("classified %d total, want 900 (spec 4.3, 7.1)", classified)
	}
}

// keyRuleSays is spec 4.3's key half on its own: what the payload's keys claim
// the host is, or "" when they claim nothing.
//
// It exists so that TestDetectCorpusMeasurement can ask the two halves of the
// rule separately and notice when they disagree. Detect answers with the whole
// rule, so it cannot be used to check itself against one of its own arms.
func keyRuleSays(payload map[string]any) string {
	switch {
	case present(payload, "prompt_id") || present(payload, "effort"):
		return "claude-code"
	case present(payload, "model") || present(payload, "turn_id"):
		return "codex"
	}
	return ""
}
