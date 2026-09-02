package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tree is a whole installation inside one temporary directory: where the
// binaries come from, where they go, and the four files the two hosts own.
type tree struct {
	opt   Options
	steps *[]string
	sys   System
}

// newTree builds the directory layout and a System whose every seam records
// that it ran, in order, instead of touching the machine.
func newTree(t *testing.T) *tree {
	t.Helper()
	root := t.TempDir()
	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		return p
	}

	src := filepath.Join(root, "dist")
	bin := filepath.Join(root, "local", "engramux", "bin")
	for _, d := range []string{src, bin} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	seedRaw(t, filepath.Join(src, RelayName), "relay v2")
	seedRaw(t, filepath.Join(src, ServiceName), "service v2")

	opt := Options{
		SourceDir:   src,
		BinDir:      bin,
		ClaudePath:  mk("home", ".claude", "settings.json"),
		CodexHooks:  mk("home", ".codex", "hooks.json"),
		CodexConfig: mk("home", ".codex", "config.toml"),
		MCPJSON:     mk("local", "engramux", "mcp.json"),
		TaskName:    `\EngramuxTest`,
	}
	seedRaw(t, opt.ClaudePath, "{}\n")
	seedRaw(t, opt.CodexHooks, "{}\n")
	seedRaw(t, opt.CodexConfig, "model = \"gpt-5.4\"\n")

	steps := &[]string{}
	record := func(s string) { *steps = append(*steps, s) }
	sys := System{
		RegisterTask: func(_ context.Context, name, exe string) error {
			record("register-task " + name + " -> " + exe)
			return nil
		},
		StartService: func(_ context.Context, name string) error {
			record("start-service " + name)
			// A started service publishes the endpoint, which is what makes
			// the second pass unnecessary.
			seedRaw(t, opt.MCPJSON, `{"url":"`+probeURL+`","token":"`+probeToken+`"}`)
			return nil
		},
		RegisterClaude: func(_ context.Context, ep *Endpoint) error {
			if ep == nil {
				record("claude-remove")
				return nil
			}
			record("claude-add " + ep.URL)
			return nil
		},
	}
	return &tree{opt: opt, steps: steps, sys: sys}
}

// TestInstallNeedsOnlyOnePass is the defect this orchestration exists to fix.
//
// The installer this replaces had to be run twice: the first pass could not
// register the MCP endpoint because mcp.json is written by the service, which
// had not started yet, so the user had to start it and run the whole thing
// again.
//
// It did say so - "start the service and run this again", and a closing line
// naming the service and doctor. An earlier version of this comment claimed it
// said nothing, which was wrong and a review caught it. The defect is the two
// passes, not the silence: here the service is started and the endpoint waited
// for, in one run.
func TestInstallNeedsOnlyOnePass(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true

	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// The order is the property. Binaries before hook files, because a hook
	// pointing at a relay that is not there yet fires and fails; the task
	// before the service, because starting it through the task is what makes
	// the running process the one the machine will have; and the endpoint
	// last, because nothing publishes it until the service runs.
	want := []string{
		"register-task " + tr.opt.TaskName + " -> " + filepath.Join(tr.opt.BinDir, ServiceName),
		"start-service " + tr.opt.TaskName,
		"claude-add " + probeURL,
	}
	got := *tr.steps
	if len(got) != len(want) {
		t.Fatalf("the system was touched %d times, want %d:\n got %q\nwant %q", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d = %q, want %q", i, got[i], want[i])
		}
	}

	// And the files it wrote itself.
	if body := read(t, filepath.Join(tr.opt.BinDir, RelayName)); body != "relay v2" {
		t.Errorf("the relay was not copied: %q", body)
	}
	if body := read(t, tr.opt.ClaudePath); !strings.Contains(body, RelayName) {
		t.Errorf("the Claude hooks were not written:\n%s", body)
	}
	if body := read(t, tr.opt.CodexConfig); !strings.Contains(body, "[mcp_servers."+MCPName+"]") {
		t.Errorf("the Codex MCP table was not written:\n%s", body)
	}
	if !strings.Contains(strings.Join(report, "\n"), "engramux register") &&
		!strings.Contains(strings.Join(report, "\n"), "logon") {
		t.Errorf("the report never mentions the logon task, which is the step the old installer "+
			"left the user to discover:\n%s", strings.Join(report, "\n"))
	}
}

// TestInstallWritesNothingWithoutApply is the dry run. It has to be able to say
// what it would do on a machine where everything is already installed, which is
// where it is most often used.
func TestInstallWritesNothingWithoutApply(t *testing.T) {
	tr := newTree(t)
	before := map[string]string{
		tr.opt.ClaudePath:  read(t, tr.opt.ClaudePath),
		tr.opt.CodexHooks:  read(t, tr.opt.CodexHooks),
		tr.opt.CodexConfig: read(t, tr.opt.CodexConfig),
	}

	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err != nil {
		t.Fatalf("Install (dry): %v", err)
	}
	if len(*tr.steps) != 0 {
		t.Errorf("a dry run touched the machine: %q", *tr.steps)
	}
	for path, body := range before {
		if got := read(t, path); got != body {
			t.Errorf("a dry run wrote %s", filepath.Base(path))
		}
	}
	if _, err := os.Stat(filepath.Join(tr.opt.BinDir, RelayName)); !os.IsNotExist(err) {
		t.Error("a dry run copied a binary")
	}
	joined := strings.Join(report, "\n")
	if !strings.Contains(joined, "would") {
		t.Errorf("a dry run does not say what it would do:\n%s", joined)
	}
	if !strings.Contains(joined, "--apply") {
		t.Errorf("a dry run does not say how to make it happen:\n%s", joined)
	}
}

// TestInstallStopsBeforeWritingWhenAHostFileCannotBeRead carries the two-file
// property up to the orchestration: a syntax error in one host's configuration
// must not leave the other already changed, and must not leave binaries copied
// either.
func TestInstallStopsBeforeWritingWhenAHostFileCannotBeRead(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true
	seedRaw(t, tr.opt.CodexHooks, "{ this is not JSON")
	claudeBefore := read(t, tr.opt.ClaudePath)

	if _, err := Install(t.Context(), tr.opt, tr.sys); err == nil {
		t.Fatal("Install accepted a host configuration that is not JSON")
	}
	if got := read(t, tr.opt.ClaudePath); got != claudeBefore {
		t.Errorf("the readable host file was written anyway:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(tr.opt.BinDir, RelayName)); !os.IsNotExist(err) {
		t.Error("a binary was copied before the plan was complete")
	}
	if len(*tr.steps) != 0 {
		t.Errorf("the machine was touched before the plan was complete: %q", *tr.steps)
	}
}

// TestInstallCarriesOnWhenTheEndpointNeverArrives covers a service that starts
// and does not publish. The hooks are installed and useful on their own -
// capture works without MCP - so this reports and continues rather than failing
// the whole run.
func TestInstallCarriesOnWhenTheEndpointNeverArrives(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true
	tr.opt.EndpointWait = 50 * time.Millisecond
	tr.sys.StartService = func(_ context.Context, name string) error {
		*tr.steps = append(*tr.steps, "start-service "+name)
		return nil // and writes no mcp.json
	}

	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	joined := strings.Join(report, "\n")
	if !strings.Contains(joined, "mcp") {
		t.Errorf("the report does not say the endpoint is missing:\n%s", joined)
	}
	if strings.Contains(strings.Join(*tr.steps, "\n"), "claude-add") {
		t.Error("claude was registered with no endpoint to register")
	}
	if body := read(t, tr.opt.ClaudePath); !strings.Contains(body, RelayName) {
		t.Error("the hooks were rolled back because MCP failed; capture does not depend on MCP")
	}
}

// TestInstallReportsAClaudeItCannotUse keeps a missing or unusable claude from
// failing an install that otherwise worked.
func TestInstallReportsAClaudeItCannotUse(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true
	tr.sys.RegisterClaude = func(context.Context, *Endpoint) error { return ErrClaudeNotFound }

	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err != nil {
		t.Fatalf("Install failed because claude is not installed: %v", err)
	}
	if !strings.Contains(strings.Join(report, "\n"), "claude") {
		t.Errorf("the report does not mention claude at all:\n%s", strings.Join(report, "\n"))
	}
	if body := read(t, tr.opt.CodexConfig); !strings.Contains(body, "[mcp_servers."+MCPName+"]") {
		t.Error("Codex was not registered because Claude Code could not be")
	}
}

// TestInstallRegistersTheTaskAgainstTheInstalledService is the trap the CLI's
// own register command has: it derives the service path from the neighbour of
// whatever binary is running, so running it out of a build tree points the
// logon task at the build tree. Install knows the destination and must use it.
func TestInstallRegistersTheTaskAgainstTheInstalledService(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true

	if _, err := Install(t.Context(), tr.opt, tr.sys); err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := "register-task " + tr.opt.TaskName + " -> " + filepath.Join(tr.opt.BinDir, ServiceName)
	for _, step := range *tr.steps {
		if strings.HasPrefix(step, "register-task ") {
			if step != want {
				t.Errorf("the task points at %q, want %q - a registration against the source "+
					"directory is a task that breaks the moment the build tree moves", step, want)
			}
			return
		}
	}
	t.Error("no task was registered")
}

// TestInstallRefusesWhenABinaryIsLocked closes the loop with PlanCopies: the
// refusal has to happen before anything at all is written, hook files included.
func TestInstallRefusesWhenABinaryIsLocked(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	tr := newTree(t)
	tr.opt.Apply = true
	// Point the destination at the running test binary's directory, and name
	// the service after it, so the probe meets a mapped image.
	tr.opt.BinDir = filepath.Dir(self)
	seedRaw(t, filepath.Join(tr.opt.SourceDir, filepath.Base(self)), "different")
	claudeBefore := read(t, tr.opt.ClaudePath)

	_, err = Install(t.Context(), Options{
		SourceDir: tr.opt.SourceDir, BinDir: tr.opt.BinDir,
		ClaudePath: tr.opt.ClaudePath, CodexHooks: tr.opt.CodexHooks,
		CodexConfig: tr.opt.CodexConfig, MCPJSON: tr.opt.MCPJSON,
		TaskName: tr.opt.TaskName, Apply: true,
		Binaries: []Binary{{Name: filepath.Base(self), Role: Service}},
	}, tr.sys)
	if err == nil {
		t.Fatal("Install did not refuse a locked destination")
	}
	var locked *LockedError
	if !errors.As(err, &locked) {
		t.Errorf("error is %T, want *LockedError so the caller can print the advice: %v", err, err)
	}
	if got := read(t, tr.opt.ClaudePath); got != claudeBefore {
		t.Error("the hook file was written even though a binary could not be")
	}
}

// TestInstallRemovesWhatItInstalled is the path the port had lost entirely.
//
// Every layer below spells removal as "no entry to write", and the orchestration
// had no way to say it: Options carried no Remove and Install always installed.
// A review found it, and it was a blocker for deleting the script this replaces,
// because that script could uninstall and this could not.
func TestInstallRemovesWhatItInstalled(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true
	var unregistered string
	tr.sys.UnregisterTask = func(_ context.Context, name string) error {
		unregistered = name
		*tr.steps = append(*tr.steps, "unregister-task "+name)
		return nil
	}

	if _, err := Install(t.Context(), tr.opt, tr.sys); err != nil {
		t.Fatalf("install: %v", err)
	}
	installedClaude := read(t, tr.opt.ClaudePath)
	if !strings.Contains(installedClaude, RelayName) {
		t.Fatalf("nothing was installed to remove:\n%s", installedClaude)
	}
	*tr.steps = nil

	tr.opt.Remove = true
	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	for _, path := range []string{tr.opt.ClaudePath, tr.opt.CodexHooks} {
		if body := read(t, path); strings.Contains(body, "engramux capture") {
			t.Errorf("a hook survived the removal in %s:\n%s", filepath.Base(path), body)
		}
	}
	if body := read(t, tr.opt.CodexConfig); strings.Contains(body, "[mcp_servers."+MCPName+"]") {
		t.Errorf("the Codex MCP table survived the removal:\n%s", body)
	}
	if unregistered != tr.opt.TaskName {
		t.Errorf("the logon task was not removed: %q", unregistered)
	}
	if steps := strings.Join(*tr.steps, "\n"); !strings.Contains(steps, "claude-remove") {
		t.Errorf("Claude Code's registration was not removed: %q", steps)
	}
	if strings.Contains(strings.Join(*tr.steps, "\n"), "start-service") {
		t.Error("a removal started the service")
	}
	// The binaries stay. Removing the relay while a host still holds a stale
	// hook entry is the one order that produces an error at every event.
	if _, err := os.Stat(filepath.Join(tr.opt.BinDir, RelayName)); err != nil {
		t.Errorf("the removal deleted the relay: %v", err)
	}
	if !strings.Contains(strings.Join(report, "\n"), "removed") {
		t.Errorf("the report does not say what happened:\n%s", strings.Join(report, "\n"))
	}
}

// TestInstallKeepsItsReportWhenItFails is the other half of a partial copy
// being possible: a caller told only "it failed" cannot say which files moved.
//
// The failure here is at planning time, so what the report has to carry is what
// was already established before it - the binaries found identical and not
// copied. An earlier version of this test used a tree where nothing had been
// established yet and asserted a non-empty report anyway, which was wrong: a
// failure before anything happens has nothing to report, and saying so is
// honest rather than a defect.
func TestInstallKeepsItsReportWhenItFails(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true
	// Both binaries already in place and identical, so PlanCopies has
	// something to say before the host files are read.
	for _, b := range []string{RelayName, ServiceName} {
		seedRaw(t, filepath.Join(tr.opt.BinDir, b), read(t, filepath.Join(tr.opt.SourceDir, b)))
	}
	seedRaw(t, tr.opt.CodexHooks, "{ this is not JSON")

	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err == nil {
		t.Fatal("Install accepted a host configuration that is not JSON")
	}
	joined := strings.Join(report, "\n")
	if !strings.Contains(joined, "unchanged") {
		t.Errorf("the report was discarded with the error, so nothing says how far it got:\n%s", joined)
	}
}
