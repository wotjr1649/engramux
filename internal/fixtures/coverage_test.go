package fixtures

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// allEvents is the eleven events an install hooks, in the order internal/host
// declares them. Named here rather than imported because internal/host imports
// this package.
var allEvents = []string{
	"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse",
	"Stop", "SubagentStart", "SubagentStop", "PreCompact", "PostCompact", "PermissionRequest",
}

// corpusCells is which of the twenty-two host x event cells the raw corpus
// actually holds, measured 2026-09-04 and pinned here.
//
// It is not a target. It is the denominator every claim made "against the
// corpus" is really made against, and until this list existed nothing wrote it
// down - which is how spec 4.3's host detection came to be recorded as 900/900
// while being wrong for a cell the corpus does not have. Nine of the
// twenty-two are empty, and two of the nine are the two that produced the
// counter-example.
//
// A change here is not a failure. It means somebody captured a shape this
// corpus had never seen, and every measurement taken over it is worth re-reading
// before the list is updated.
var corpusCells = []string{
	"claude-code/PermissionRequest",
	"claude-code/PostToolUse",
	"claude-code/PreToolUse",
	"claude-code/Stop",
	"claude-code/SubagentStart",
	"claude-code/SubagentStop",
	"claude-code/UserPromptSubmit",
	"codex/PostToolUse",
	"codex/PreToolUse",
	"codex/SessionEnd",
	"codex/SessionStart",
	"codex/Stop",
	"codex/UserPromptSubmit",
}

// TestTheCorpusCoverageIsWhatIsRecorded pins the scope of every measurement
// this repository takes over the raw corpus.
//
// # Why this test exists
//
// Spec 4.3's ordered host-detection rule was recorded as 900 of 900 over this
// corpus and was wrong for Claude Code's SessionStart, whose payload carries
// `model` and no `prompt_id` and which the key rules therefore called Codex.
// Every Claude Code session minted a Codex session that had never existed.
// Nothing caught it for a month, and the reason is here: the corpus holds no
// `claude-code SessionStart` capture and no `claude-code SessionEnd` capture,
// so the rule was measured against evidence that could not contain its own
// counter-example. A perfect score over an incomplete denominator reads exactly
// like a perfect score.
//
// # Why the host comes from the payload and not from the capture's label
//
// The first attempt to find the defect replayed detection over the corpus and
// compared the answer against each capture's recorded host. It came back clean
// and proved nothing, because that label had been written by the same function.
// This reads `transcript_path` out of the payload, which is the one field
// neither rule nor label derives from the other.
func TestTheCorpusCoverageIsWhatIsRecorded(t *testing.T) {
	entries, err := os.ReadDir(corpusDir)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no raw corpus at %s; the scope of every corpus measurement is unchecked on this machine", corpusDir)
	}
	if err != nil {
		t.Fatalf("read %s: %v", corpusDir, err)
	}

	found := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		//nolint:gosec // G304: reading the local corpus directory by construction
		raw, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var capture struct {
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(raw, &capture); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		if capture.Payload["session_id"] == "selftest" {
			continue
		}
		host := hostFromTranscript(capture.Payload)
		event, _ := capture.Payload["hook_event_name"].(string)
		if host == "" || event == "" {
			continue
		}
		found[host+"/"+event]++
	}

	have := slices.Sorted(maps.Keys(found))
	want := slices.Clone(corpusCells)
	slices.Sort(want)
	if !slices.Equal(have, want) {
		t.Errorf("corpus coverage moved.\n  recorded: %v\n  actual:   %v\n"+
			"this is not a bug in this test. A cell appearing means a shape nobody had captured "+
			"now exists, and every measurement taken over this corpus is worth re-reading before "+
			"the recorded list is updated", want, have)
	}

	// The gap, named rather than left as a subtraction the reader has to do.
	var missing []string
	for _, h := range []string{"claude-code", "codex"} {
		for _, ev := range allEvents {
			if found[h+"/"+ev] == 0 {
				missing = append(missing, h+"/"+ev)
			}
		}
	}
	t.Logf("corpus covers %d of 22 host x event cells; %d captures in the largest", len(have), largest(found))
	t.Logf("empty cells (%d): %v", len(missing), missing)
}

// hostFromTranscript is the host a capture's transcript_path names, or "" when
// it names neither. Whole path components, so a project directory called
// .claude-notes is not a Claude Code transcript.
func hostFromTranscript(payload map[string]any) string {
	path, ok := payload["transcript_path"].(string)
	if !ok {
		return ""
	}
	for _, part := range strings.Split(strings.ReplaceAll(path, `\`, "/"), "/") {
		switch part {
		case ".claude":
			return "claude-code"
		case ".codex":
			return "codex"
		}
	}
	return ""
}

// largest is the biggest cell's capture count, which is what says whether a
// covered cell is covered by one capture or by three hundred.
func largest(m map[string]int) int {
	var n int
	for _, v := range m {
		n = max(n, v)
	}
	return n
}
