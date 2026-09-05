package fixtures

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// corpusDir is the local, gitignored raw capture corpus, relative to this
// package. It is read for shape only; no value is ever copied out of it.
var corpusDir = filepath.Join("..", "..", ".capture", "fixtures-raw")

// jsonType names the JSON type of a value decoded by encoding/json into any.
// A key that is absent from an object reads back as nil, so "null" also means
// "not present" - which is what the containment checks below want.
func jsonType(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	}
	return fmt.Sprintf("unknown(%T)", v)
}

// shapeOf returns every (JSON path, JSON type) pair reachable in v, keyed by an
// unambiguous encoding of the pair and valued by a readable rendering that is
// only used in failure messages.
//
// An array element extends the path by one index-free segment, so a two-element
// fixture still matches a five-element capture.
//
// Object keys are length-prefixed instead of dot-joined because real captures
// carry object keys that contain '.' - user text used as a map key - and a
// dot-joined path would let one document forge a path another does not have.
// "<len>:<key>" per key and "-" per array element is injective, so a match is a
// real match.
func shapeOf(v any) map[string]string {
	pairs := map[string]string{}
	var walk func(v any, key, readable string)
	walk = func(v any, key, readable string) {
		t := jsonType(v)
		pairs[key+"\t"+t] = readable + " " + t
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				walk(e, key+strconv.Itoa(len(k))+":"+k, readable+"."+k)
			}
		case []any:
			for _, e := range x {
				walk(e, key+"-", readable+".[]")
			}
		}
	}
	walk(v, "", "$")
	return pairs
}

// payloadOf decodes a fixture, reaching it through All() so that a fixture
// dropped from that list fails here rather than going quietly untested.
func payloadOf(t *testing.T, file string) map[string]any {
	t.Helper()
	for _, f := range All() {
		if f.File != file {
			continue
		}
		raw, err := f.Bytes()
		if err != nil {
			t.Fatalf("Bytes(%s): %v", file, err)
		}
		var p map[string]any
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		return p
	}
	t.Fatalf("All() does not list %q", file)
	return nil
}

func mustEvent(t *testing.T, p map[string]any, want string) {
	t.Helper()
	if got := p["hook_event_name"]; got != want {
		t.Errorf("hook_event_name = %#v, want %q", got, want)
	}
}

func mustType(t *testing.T, p map[string]any, key, want string) {
	t.Helper()
	if got := jsonType(p[key]); got != want {
		t.Errorf("%s is %s, want %s", key, got, want)
	}
}

// mustAnyOf asserts at least one of want's keys is present, and that every key
// that is present has exactly the type want gives it.
func mustAnyOf(t *testing.T, p map[string]any, want map[string]string) {
	t.Helper()
	found := 0
	for _, k := range slices.Sorted(maps.Keys(want)) {
		got := jsonType(p[k])
		if got == "null" {
			continue
		}
		found++
		if got != want[k] {
			t.Errorf("%s is %s, want %s", k, got, want[k])
		}
	}
	if found == 0 {
		t.Errorf("carries none of %v; host detection has nothing to key on", slices.Sorted(maps.Keys(want)))
	}
}

// mustPathComponent asserts dir is a whole component of the path at key, not a
// substring of one.
func mustPathComponent(t *testing.T, p map[string]any, key, dir string) {
	t.Helper()
	raw, ok := p[key].(string)
	if !ok {
		t.Fatalf("%s is %s, want string", key, jsonType(p[key]))
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '\\' || r == '/' })
	if !slices.Contains(parts, dir) {
		t.Errorf("%s splits into %q, want one component to be exactly %q", key, parts, dir)
	}
}

// TestFixturesAreWhatTheyClaim is the anti-vacuity guard. TestFixtureShapesOccurInCorpus
// is one-directional, so an empty fixture passes it trivially; this test asserts
// each fixture still exercises the path spec 8 chose it for. It needs no corpus
// and always runs.
func TestFixturesAreWhatTheyClaim(t *testing.T) {
	// Five since 2026-09-04. Spec 8 chose four, one per detection path it
	// knew about; the fifth is the cell that turned out not to have a
	// detection path - Claude Code's SessionStart, whose key set is a strict
	// subset of Codex's, so only transcript_path can separate them.
	if got := len(All()); got != 5 {
		t.Fatalf("All() returned %d fixtures, want the 5 of spec 8", got)
	}

	t.Run("codex SessionEnd reaches detection step 3", func(t *testing.T) {
		p := payloadOf(t, CodexSessionEnd)
		mustEvent(t, p, "SessionEnd")
		// Steps 1 and 2 must not fire, or the transcript_path fallback is never
		// exercised and a two-step rule would look correct (spec 4.3).
		for _, k := range []string{"prompt_id", "effort", "model", "turn_id"} {
			if v, ok := p[k]; ok {
				t.Errorf("carries %q = %#v; detection would stop before step 3", k, v)
			}
		}
		mustPathComponent(t, p, "transcript_path", ".codex")
	})

	t.Run("codex PostToolUse carries tool_response as a string", func(t *testing.T) {
		p := payloadOf(t, CodexPostToolUseString)
		mustEvent(t, p, "PostToolUse")
		mustType(t, p, "tool_response", "string")
		mustAnyOf(t, p, map[string]string{"model": "string", "turn_id": "string"})
	})

	t.Run("codex PostToolUse carries tool_response as an array", func(t *testing.T) {
		p := payloadOf(t, CodexPostToolUseArray)
		mustEvent(t, p, "PostToolUse")
		mustType(t, p, "tool_response", "array")
		mustAnyOf(t, p, map[string]string{"model": "string", "turn_id": "string"})
	})

	t.Run("claude-code PostToolUse carries tool_response as an object with error keys", func(t *testing.T) {
		p := payloadOf(t, ClaudePostToolUseObject)
		mustEvent(t, p, "PostToolUse")
		mustType(t, p, "tool_response", "object")
		resp, ok := p["tool_response"].(map[string]any)
		if !ok {
			t.Fatalf("tool_response is %s, want object", jsonType(p["tool_response"]))
		}
		// The error keys occur in only 241 of 310 real Claude captures (spec 4.4);
		// a fixture without them leaves the one shape that has them untested.
		if got := jsonType(resp["stderr"]); got != "string" {
			t.Errorf("tool_response.stderr is %s, want string", got)
		}
		if got := jsonType(resp["interrupted"]); got != "bool" {
			t.Errorf("tool_response.interrupted is %s, want bool", got)
		}
		mustAnyOf(t, p, map[string]string{"prompt_id": "string", "effort": "object"})
	})
}

// TestFixtureBytesAreExact pins the loader to the bytes on disk. Later phases
// gate on a byte-for-byte round-trip, so a loader that parsed and re-marshalled
// would make that gate measure itself.
func TestFixtureBytesAreExact(t *testing.T) {
	for _, f := range All() {
		t.Run(f.File, func(t *testing.T) {
			got, err := f.Bytes()
			if err != nil {
				t.Fatalf("Bytes: %v", err)
			}
			//nolint:gosec // G304: reading this package's own testdata directory by construction
			want, err := os.ReadFile(filepath.Join("testdata", f.File))
			if err != nil {
				t.Fatalf("read testdata: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("Bytes() returned %d bytes, testdata holds %d; the loader must not re-marshal", len(got), len(want))
			}
		})
	}
}

// cell is one host x event bucket of the corpus.
type cell struct{ host, event string }

// loadCorpus reads the raw captures and reduces each wanted cell to the union of
// its (path, type) pairs. It skips the test when the corpus is absent, so a
// contributor without it can still run everything else.
func loadCorpus(t *testing.T) map[cell]map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(corpusDir)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no raw corpus at %s; fixture shape drift is unchecked on this machine", corpusDir)
	}
	if err != nil {
		t.Fatalf("read %s: %v", corpusDir, err)
	}

	want := map[cell]bool{}
	for _, f := range All() {
		want[cell{f.Host, f.Event}] = true
	}

	got := map[cell]map[string]bool{}
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
		// Spec 7.5: one capture is a synthetic self-test, not something a host sent.
		if capture.Payload["session_id"] == "selftest" {
			continue
		}
		c := cell{capture.Cap.Host, capture.Cap.Event}
		if !want[c] {
			continue
		}
		if got[c] == nil {
			got[c] = map[string]bool{}
		}
		for key := range shapeOf(capture.Payload) {
			got[c][key] = true
		}
	}
	return got
}

// TestFixtureShapesOccurInCorpus is the drift guard: every (path, type) pair a
// fixture carries must also occur in at least one real capture of the same
// host x event cell. One direction only - a real capture may carry keys the
// fixture omits.
func TestFixtureShapesOccurInCorpus(t *testing.T) {
	corpus := loadCorpus(t)
	for _, f := range All() {
		t.Run(f.File, func(t *testing.T) {
			real := corpus[cell{f.Host, f.Event}]
			if len(real) == 0 {
				// A skip and not a failure, and only because the
				// gap is pinned elsewhere. This corpus holds 13
				// of the 22 host x event cells and
				// TestTheCorpusCoverageIsWhatIsRecorded is the
				// list; a cell becoming covered turns that test
				// red, which is what brings somebody back here
				// to run this guard for real. Without the pin
				// this skip would be the same silence that let
				// spec 4.3 be recorded as 900/900.
				t.Skipf("corpus holds no %s %s capture, so this fixture's shape is unguarded - "+
					"see TestTheCorpusCoverageIsWhatIsRecorded for the nine cells it lacks", f.Host, f.Event)
			}
			pairs := shapeOf(payloadOf(t, f.File))
			for _, key := range slices.Sorted(maps.Keys(pairs)) {
				if !real[key] {
					t.Errorf("fixture carries %s, which no real %s %s capture has", pairs[key], f.Host, f.Event)
				}
			}
		})
	}
}
