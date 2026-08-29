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
	"bytes"
	"encoding/json"
	"errors"
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
	node, script, tmp := installerTree(t)

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

	out, err := runInstaller(t, node, script, tmp, codexPath, claudePath, "--apply")
	if err != nil {
		t.Fatalf("install-hooks.mjs --apply: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(installerBin(tmp), "engramux.exe")); err != nil {
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

// TestInstallerSkipsACopyWhoseBytesAlreadyMatch runs the installer twice over
// one unchanged dist/ and asserts the second run copies nothing and still
// writes both hook files.
//
// This is the ordinary re-run, and it is what used to crash: the service is
// resident, Windows locks the image of a running process against writes, and
// rewriting a file with the bytes it already holds is still a write.
func TestInstallerSkipsACopyWhoseBytesAlreadyMatch(t *testing.T) {
	node, script, tmp := installerTree(t)
	codexPath := filepath.Join(tmp, "hooks.json")
	claudePath := filepath.Join(tmp, "settings.json")

	seed(t, codexPath, map[string]any{"hooks": map[string]any{}})
	seed(t, claudePath, map[string]any{"hooks": map[string]any{}})
	out, err := runInstaller(t, node, script, tmp, codexPath, claudePath, "--apply")
	if err != nil {
		t.Fatalf("first --apply: %v\n%s", err, out)
	}
	if n := copyLines(out); n != 2 {
		t.Fatalf("the first run copied %d binaries, want 2 - the second run proves nothing otherwise:\n%s", n, out)
	}

	// Re-seeded, so the second run has real work left at the hook files. Left
	// as the first run wrote them, the script would report "already up to
	// date" and the assertion below would pass without a write happening.
	seed(t, codexPath, map[string]any{"hooks": map[string]any{}})
	seed(t, claudePath, map[string]any{"hooks": map[string]any{}})
	out, err = runInstaller(t, node, script, tmp, codexPath, claudePath, "--apply")
	if err != nil {
		t.Fatalf("second --apply: %v\n%s", err, out)
	}
	for _, name := range []string{"engramux.exe", "engramux-service.exe"} {
		if !strings.Contains(string(out), "unchanged "+filepath.Join(installerBin(tmp), name)) {
			t.Errorf("the second run did not report %s as unchanged:\n%s", name, out)
		}
	}
	if n := copyLines(out); n != 0 {
		t.Errorf("the second run copied %d binaries over identical bytes, want 0:\n%s", n, out)
	}

	// Skipping the copies must not skip the rest of the run.
	for _, path := range []string{codexPath, claudePath} {
		mine := 0
		for _, hooks := range readHooks(t, path) {
			for _, h := range hooks {
				if ours(h) {
					mine++
				}
			}
		}
		if mine != 11 {
			t.Errorf("%s: %d engramux hooks after the second run, want the 11 of the script's EVENTS table",
				filepath.Base(path), mine)
		}
	}
}

// TestInstallerRefusesAllCopiesWhenOneDestinationCannotBeWritten asserts the
// whole-run refusal: a destination that must change but cannot be opened for
// writing stops the run before the FIRST copy, names the service and how to
// stop it, and leaves both hook files exactly as they were.
//
// What this reaches, precisely. The script probes each destination it has to
// overwrite by opening it for writing, and that probe is what a running image
// fails. Measured on this machine against the installed pair with the service
// up: the resident engramux-service.exe throws EBUSY (ERROR_SHARING_VIOLATION,
// errno -4082) while the relay - spawned per event, gone again - opens fine.
// This test does NOT hold an image lock and starts no process: it sets the
// read-only attribute, which fails the same open with EPERM. Same call, same
// branch, different errno. The guard is what is under test; the lock itself is
// the measurement above.
func TestInstallerRefusesAllCopiesWhenOneDestinationCannotBeWritten(t *testing.T) {
	node, script, tmp := installerTree(t)
	codexPath := filepath.Join(tmp, "hooks.json")
	claudePath := filepath.Join(tmp, "settings.json")

	// Both destinations differ from dist/, so a run that reached any copy at
	// all would rewrite both. The relay is the one to watch: it is copied
	// first, so if it changes, the run half-installed before it stopped.
	bin := installerBin(tmp)
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatalf("%v", err)
	}
	relay, service := filepath.Join(bin, "engramux.exe"), filepath.Join(bin, "engramux-service.exe")
	for _, dest := range []string{relay, service} {
		if err := os.WriteFile(dest, []byte("an older build\n"), 0o600); err != nil {
			t.Fatalf("%v", err)
		}
	}
	if err := os.Chmod(service, 0o400); err != nil {
		t.Fatalf("%v", err)
	}
	// Restored before t.TempDir's cleanup runs, which cannot delete a
	// read-only file on Windows.
	t.Cleanup(func() {
		if err := os.Chmod(service, 0o600); err != nil {
			t.Errorf("restoring %s: %v", filepath.Base(service), err)
		}
	})

	// Seeded with another tool's hook and none of ours, so a run that got as
	// far as the hook files would have to write them.
	doc := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": "C:/other-tool/bin/other.exe",
		}}}},
	}}
	seed(t, codexPath, doc)
	seed(t, claudePath, doc)
	before := make(map[string][]byte, 2)
	for _, path := range []string{codexPath, claudePath} {
		b, err := os.ReadFile(path) //nolint:gosec // G304: a path this test built under t.TempDir
		if err != nil {
			t.Fatalf("%v", err)
		}
		before[path] = b
	}

	out, err := runInstaller(t, node, script, tmp, codexPath, claudePath, "--apply")
	if err == nil {
		t.Fatalf("--apply exited 0 with a destination it cannot write:\n%s", out)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("--apply: %v\n%s", err, out)
	}

	// The message has to carry the diagnosis and the fix, because the user is
	// the one who stops the service - the script never does.
	for _, want := range []string{"engramux-service.exe", "service is running", `schtasks /end /tn "\Engramux"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the refusal does not mention %q:\n%s", want, out)
		}
	}

	// Nothing was half-done: the relay was not copied and neither hook file
	// was touched.
	got, err := os.ReadFile(relay) //nolint:gosec // G304: a path this test built under t.TempDir
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(got) != "an older build\n" {
		t.Errorf("the relay was copied before the run refused: %q - that is the half-install", got)
	}
	for _, path := range []string{codexPath, claudePath} {
		got, err := os.ReadFile(path) //nolint:gosec // G304: a path this test built under t.TempDir
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal(got, before[path]) {
			t.Errorf("%s was modified by a run that could not copy the binaries", filepath.Base(path))
		}
	}
}

// TestAWriteHandleOnARunningImageIsRefused pins the premise the installer's
// probe rests on, and which no other test here reaches: opening a resident
// executable for writing FAILS. If it silently succeeded, the probe would wave
// every run through and copyFileSync would throw exactly as it did before the
// fix, with every other test in this file still green - the guard would be
// decoration and nothing would say so.
//
// The subject is this test binary's own image, which go test built and is
// running right now. It is genuinely mapped, so the refusal is a real
// ERROR_SHARING_VIOLATION rather than a stand-in: it starts no process,
// touches no service, and needs nothing installed.
func TestAWriteHandleOnARunningImageIsRefused(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node is not on PATH, so the installer's own runtime cannot be asked: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locating this test binary: %v", err)
	}
	// The script's call, in the script's runtime, on a file known to be
	// mapped. Under -e, process.argv[1] is the first argument after the
	// script text.
	const probe = `const fs = require('node:fs')
try { fs.closeSync(fs.openSync(process.argv[1], 'r+')); console.log('OPENED') }
catch (e) { console.log(e.code) }`
	//nolint:gosec // G204: node is what LookPath resolved, and the argument is os.Executable()
	out, err := exec.CommandContext(t.Context(), node, "-e", probe, self).CombinedOutput()
	if err != nil {
		t.Fatalf("running the probe: %v\n%s", err, out)
	}

	got := strings.TrimSpace(string(out))
	if got == "OPENED" {
		t.Fatalf("a write handle on this running test binary was GRANTED: the installer's probe cannot detect a locked binary at all, and its refusal path is unreachable")
	}
	// EBUSY exactly - ERROR_SHARING_VIOLATION, what a mapped image gives. Any
	// throwing code keeps the script correct, since its catch takes all of
	// them, but a different one means this platform's answer moved and the
	// measurement the comments cite needs taking again.
	if got != "EBUSY" {
		t.Errorf("opening a running image for writing gave %q, want EBUSY", got)
	}
}

// copyLines counts the script's own copy lines, anchored at the line start.
//
// A substring search cannot do this job. The skip line ends "- identical
// bytes, not copied", so strings.Contains(out, "copied ") used as a NEGATIVE
// assertion inverts silently the moment anyone adds a character after that
// word, and the test guarding the whole skip path would pass forever.
func copyLines(out []byte) int {
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "copied ") {
			n++
		}
	}
	return n
}

// installerTree copies the installer into a tree of its own under t.TempDir(),
// beside placeholder binaries, and returns node, the copy, and the tree root.
//
// The script derives its repo root from its own location, refuses to run
// without dist/*.exe under it, and copies those binaries into %LOCALAPPDATA%.
// The copy keeps every test here off both: none needs a build, and none can
// reach anything of the caller's. It is byte-identical, and the script imports
// nothing but node builtins.
func installerTree(t *testing.T) (node, script, tmp string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node is not on PATH, so the installer cannot be run: %v", err)
	}
	tmp = t.TempDir()
	body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install-hooks.mjs")) //nolint:gosec // G304: this repository's own scripts directory by construction
	if err != nil {
		t.Fatalf("reading the installer: %v", err)
	}
	for _, dir := range []string{"scripts", "dist"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o750); err != nil {
			t.Fatalf("%v", err)
		}
	}
	script = filepath.Join(tmp, "scripts", "install-hooks.mjs")
	//nolint:gosec // G703: script is filepath.Join over t.TempDir and this file's own literals
	if err := os.WriteFile(script, body, 0o600); err != nil {
		t.Fatalf("%v", err)
	}
	for _, name := range []string{"engramux.exe", "engramux-service.exe"} {
		if err := os.WriteFile(filepath.Join(tmp, "dist", name), []byte("placeholder "+name+"\n"), 0o600); err != nil {
			t.Fatalf("%v", err)
		}
	}
	return node, script, tmp
}

// installerBin is where the script copies the binaries under the redirected
// %LOCALAPPDATA% runInstaller hands it.
func installerBin(tmp string) string { return filepath.Join(tmp, "local", "engramux", "bin") }

// runInstaller runs the copied script against the two hook files given.
//
// An explicit environment rather than os.Environ() plus overrides: nothing of
// the caller's LOCALAPPDATA or ENGRAMUX_* can leak in, so the binary copy
// cannot land outside tmp even if the override were ignored.
func runInstaller(t *testing.T, node, script, tmp, codexPath, claudePath string, args ...string) ([]byte, error) {
	t.Helper()
	//nolint:gosec // G204: node is what LookPath resolved, script is a path this test built under t.TempDir
	cmd := exec.CommandContext(t.Context(), node, append([]string{script}, args...)...)
	cmd.Dir = tmp
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"SystemRoot=" + os.Getenv("SystemRoot"),
		"HOME=" + filepath.Join(tmp, "home"),
		"USERPROFILE=" + filepath.Join(tmp, "home"),
		"LOCALAPPDATA=" + filepath.Join(tmp, "local"),
		"ENGRAMUX_CODEX_HOOKS=" + codexPath,
		"ENGRAMUX_CLAUDE_SETTINGS=" + claudePath,
	}
	return cmd.CombinedOutput()
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
