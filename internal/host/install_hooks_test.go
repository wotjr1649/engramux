package host

// The hook installer is scripts/install-hooks.mjs, and no Go package reads or
// writes host hook configuration, so it has no owner in the tree. This test
// lives here because what it asserts is a *host* fact: Codex documents its
// SessionEnd hook timeout as 1 s by default and 3 s at most, while every other
// event defaults to 600 s (learn.chatgpt.com/docs/hooks, "Hook timeouts").
// internal/host is the package whose subject is the two hosts.
//
// It runs the script and reads what the script wrote. Asserting the source
// text instead would pass on a file that no longer produces that output.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// hooksDoc is the part of a hooks document this test navigates. Both hosts key
// their hooks the same way; only the hook objects differ. Hook objects are kept
// raw and decoded into maps so a foreign hook is compared field for field,
// including any field this test does not know the name of.
type hooksDoc struct {
	Hooks map[string][]struct {
		Hooks []json.RawMessage `json:"hooks"`
	} `json:"hooks"`
}

// readHooks returns every hook in path, grouped by event, in file order.
func readHooks(t *testing.T, path string) map[string][]map[string]any {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // G304: a path this test built under t.TempDir
	if err != nil {
		t.Fatalf("reading %s: %v", filepath.Base(path), err)
	}
	var doc hooksDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parsing %s: %v", filepath.Base(path), err)
	}
	out := make(map[string][]map[string]any, len(doc.Hooks))
	for event, entries := range doc.Hooks {
		for _, entry := range entries {
			for _, raw := range entry.Hooks {
				var h map[string]any
				if err := json.Unmarshal(raw, &h); err != nil {
					t.Fatalf("parsing a %s hook in %s: %v", event, filepath.Base(path), err)
				}
				out[event] = append(out[event], h)
			}
		}
	}
	return out
}

// ours recognises an Engramux hook the same way the script's own mergeEvents
// does - by the command path, case-folded - because that is what decides
// whether a re-run replaces the entry or appends a second one.
func ours(h map[string]any) bool {
	cmd, _ := h["command"].(string)
	return strings.Contains(strings.ToLower(cmd), "engramux")
}

// TestCodexSessionEndTimeoutIsWithinTheDocumentedLimit installs over a
// hooks.json that already carries a stale Engramux SessionEnd hook at 5 s with
// another tool's hook beside it - the upgrade path, not a fresh install - and
// asserts the result: SessionEnd at 3, every other Codex event still 5, Claude
// Code still 5, the foreign hook untouched, and exactly one Engramux hook per
// event.
func TestCodexSessionEndTimeoutIsWithinTheDocumentedLimit(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node is not on PATH, so the installer cannot be run: %v", err)
	}

	tmp := t.TempDir()

	// The script derives its repo root from its own location, refuses to run
	// without dist/*.exe under it, and copies those binaries into
	// %LOCALAPPDATA%. Running a copy of it under t.TempDir(), beside two
	// placeholder files, keeps the test off both: it needs no build, and it
	// cannot reach anything of the caller's. The copy is byte-identical, and
	// the script imports nothing but node builtins.
	src := filepath.Join("..", "..", "scripts", "install-hooks.mjs")
	body, err := os.ReadFile(src) //nolint:gosec // G304: this repository's own scripts directory by construction
	if err != nil {
		t.Fatalf("reading the installer: %v", err)
	}
	script := filepath.Join(tmp, "scripts", "install-hooks.mjs")
	for _, dir := range []string{"scripts", "dist"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o750); err != nil {
			t.Fatalf("%v", err)
		}
	}
	//nolint:gosec // G703: script is filepath.Join over t.TempDir and this file's own literals
	if err := os.WriteFile(script, body, 0o600); err != nil {
		t.Fatalf("%v", err)
	}
	for _, name := range []string{"engramux.exe", "engramux-service.exe"} {
		if err := os.WriteFile(filepath.Join(tmp, "dist", name), []byte("placeholder\n"), 0o600); err != nil {
			t.Fatalf("%v", err)
		}
	}

	// The state the upgrade has to survive: one entry holding another tool's
	// hook and a stale Engramux hook at the old 5 s.
	foreign := map[string]any{
		"type":          "command",
		"command":       "C:/other-tool/bin/other.exe",
		"timeout":       float64(10),
		"statusMessage": "another tool",
	}
	stale := map[string]any{
		"type":           "command",
		"command":        `"C:/prior/engramux/bin/engramux.exe"`,
		"commandWindows": `"C:/prior/engramux/bin/engramux.exe"`,
		"timeout":        float64(5),
		"statusMessage":  "engramux capture",
	}
	codexPath := filepath.Join(tmp, "hooks.json")
	claudePath := filepath.Join(tmp, "settings.json")
	seed(t, codexPath, map[string]any{"hooks": map[string]any{
		"SessionEnd": []any{map[string]any{"hooks": []any{foreign, stale}}},
	}})
	seed(t, claudePath, map[string]any{"hooks": map[string]any{}})

	// An explicit environment rather than os.Environ() plus overrides: nothing
	// of the caller's LOCALAPPDATA or ENGRAMUX_* can leak in, so the binary
	// copy cannot land outside tmp even if the override were ignored.
	local := filepath.Join(tmp, "local")
	//nolint:gosec // G204: node is what LookPath resolved, script is a path this test built under t.TempDir
	cmd := exec.CommandContext(t.Context(), node, script, "--apply")
	cmd.Dir = tmp
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"SystemRoot=" + os.Getenv("SystemRoot"),
		"HOME=" + filepath.Join(tmp, "home"),
		"USERPROFILE=" + filepath.Join(tmp, "home"),
		"LOCALAPPDATA=" + local,
		"ENGRAMUX_CODEX_HOOKS=" + codexPath,
		"ENGRAMUX_CLAUDE_SETTINGS=" + claudePath,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-hooks.mjs --apply: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(local, "engramux", "bin", "engramux.exe")); err != nil {
		t.Fatalf("the script did not honour the redirected LOCALAPPDATA: %v\n%s", err, out)
	}

	codex := readHooks(t, codexPath)
	if len(codex) != 11 {
		t.Errorf("codex: %d events written, want the 11 of the script's EVENTS table", len(codex))
	}
	for event, hooks := range codex {
		var mine []map[string]any
		for _, h := range hooks {
			if ours(h) {
				mine = append(mine, h)
			}
		}
		if len(mine) != 1 {
			t.Errorf("codex %s: %d engramux hooks, want exactly 1 - a re-run replaces, it does not append", event, len(mine))
			continue
		}
		want, why := float64(5), "only SessionEnd is capped; the rest of Codex defaults to 600 s"
		if event == "SessionEnd" {
			want, why = 3, "Codex documents SessionEnd at 1 s by default and 3 s at most, and clamps anything higher"
		}
		if got := mine[0]["timeout"]; got != want {
			t.Errorf("codex %s timeout = %v, want %v: %s", event, got, want, why)
		}
	}

	// The foreign hook keeps its place, its order and every field.
	if got := codex["SessionEnd"]; len(got) != 2 {
		t.Errorf("codex SessionEnd: %d hooks, want 2 - another tool's, then ours", len(got))
	} else if !reflect.DeepEqual(got[0], foreign) {
		t.Errorf("another tool's SessionEnd hook changed:\n got %v\nwant %v", got[0], foreign)
	}

	// The cap is Codex's alone. Claude Code's SessionEnd budget is 1.5 s
	// raised to the longest per-hook timeout (spec 7.1), so 5 stays there.
	claude, found := readHooks(t, claudePath), 0
	for _, h := range claude["SessionEnd"] {
		if !ours(h) {
			continue
		}
		found++
		if got := h["timeout"]; got != float64(5) {
			t.Errorf("claude-code SessionEnd timeout = %v, want 5: the 3 s cap is Codex's alone", got)
		}
	}
	if found != 1 {
		t.Errorf("claude-code SessionEnd: %d engramux hooks, want exactly 1", found)
	}
}

func seed(t *testing.T, path string, doc map[string]any) {
	t.Helper()
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatalf("%v", err)
	}
}
