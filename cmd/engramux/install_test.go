package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/schedule"
)

// TestResolveOptionsRefusesTheInstalledCopy is the guard that only exists
// because the resolution takes its inputs as arguments.
//
// Running the installed copy asks it to overwrite itself. Windows refuses that
// for a mapped image anyway, but it refuses it as a sharing violation on a
// destination several steps in, which reads like the service is running - so a
// user chasing that message would stop the service and try again and get the
// same thing.
func TestResolveOptionsRefusesTheInstalledCopy(t *testing.T) {
	local := filepath.Join(t.TempDir(), "Local")
	installed := filepath.Join(local, "engramux", "bin", "engramux.exe")

	_, err := resolveOptions(installed, local, t.TempDir(), true, nil)
	if err == nil {
		t.Fatal("resolveOptions accepted the installed copy as its own source")
	}
	if !strings.Contains(err.Error(), "installed copy") {
		t.Errorf("the error does not say what is wrong: %v", err)
	}

	// One directory over is fine, which is what a developer's dist/ and a
	// user's unpacked directory both look like.
	if _, err := resolveOptions(filepath.Join(t.TempDir(), "engramux.exe"), local, t.TempDir(), true, nil); err != nil {
		t.Errorf("resolveOptions refused an ordinary source directory: %v", err)
	}
}

// TestResolveOptionsDerivesEveryPath pins where an installation puts things,
// because every one of them is a decision and none of them is observable from
// a passing install.
func TestResolveOptionsDerivesEveryPath(t *testing.T) {
	local, home := t.TempDir(), t.TempDir()
	source := t.TempDir()

	opt, err := resolveOptions(filepath.Join(source, "engramux.exe"), local, home, true, nil)
	if err != nil {
		t.Fatalf("resolveOptions: %v", err)
	}
	for _, tc := range []struct{ name, got, want string }{
		{"SourceDir", opt.SourceDir, source},
		{"BinDir", opt.BinDir, filepath.Join(local, "engramux", "bin")},
		{"MCPJSON", opt.MCPJSON, filepath.Join(local, "engramux", "mcp.json")},
		{"ClaudePath", opt.ClaudePath, filepath.Join(home, ".claude", "settings.json")},
		{"CodexHooks", opt.CodexHooks, filepath.Join(home, ".codex", "hooks.json")},
		{"CodexConfig", opt.CodexConfig, filepath.Join(home, ".codex", "config.toml")},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if opt.TaskName != schedule.TaskName {
		t.Errorf("TaskName = %q, want %q", opt.TaskName, schedule.TaskName)
	}
	if !opt.Apply {
		t.Error("Apply did not survive")
	}
}

// TestResolveOptionsHonoursTheHostOverrides keeps the three host files
// movable. They are the only paths this product does not own, and a test that
// wrote a developer's real settings.json would be a test nobody could run
// twice.
func TestResolveOptionsHonoursTheHostOverrides(t *testing.T) {
	t.Setenv("ENGRAMUX_CLAUDE_SETTINGS", `C:\elsewhere\settings.json`)
	t.Setenv("ENGRAMUX_CODEX_HOOKS", `C:\elsewhere\hooks.json`)
	t.Setenv("ENGRAMUX_CODEX_CONFIG", `C:\elsewhere\config.toml`)

	opt, err := resolveOptions(filepath.Join(t.TempDir(), "engramux.exe"), t.TempDir(), t.TempDir(), false, nil)
	if err != nil {
		t.Fatalf("resolveOptions: %v", err)
	}
	if opt.ClaudePath != `C:\elsewhere\settings.json` ||
		opt.CodexHooks != `C:\elsewhere\hooks.json` ||
		opt.CodexConfig != `C:\elsewhere\config.toml` {
		t.Errorf("an override was ignored: %+v", opt)
	}
	// The data directory is not overridable, because the service owns it and
	// an installer pointing somewhere else would register an endpoint nothing
	// publishes.
	if strings.Contains(opt.MCPJSON, "elsewhere") {
		t.Errorf("MCPJSON followed a host override: %q", opt.MCPJSON)
	}
}

// TestResolveOptionsNeedsTheDataDirectory covers the one environment variable
// with no fallback.
func TestResolveOptionsNeedsTheDataDirectory(t *testing.T) {
	_, err := resolveOptions(filepath.Join(t.TempDir(), "engramux.exe"), "", t.TempDir(), true, nil)
	if err == nil {
		t.Fatal("resolveOptions accepted an empty LOCALAPPDATA")
	}
	if !strings.Contains(err.Error(), "LOCALAPPDATA") {
		t.Errorf("the error does not name what is missing: %v", err)
	}
}

// TestWithoutFlagsLetsTheTaskNameComeInEitherOrder keeps `--apply` from being
// read as the optional task-name positional, which would register a task
// literally named --apply.
func TestWithoutFlagsLetsTheTaskNameComeInEitherOrder(t *testing.T) {
	for _, args := range [][]string{
		{"--apply", `\Custom`},
		{`\Custom`, "--apply"},
	} {
		opt, err := resolveOptions(filepath.Join(t.TempDir(), "engramux.exe"), t.TempDir(), t.TempDir(), true, args)
		if err != nil {
			t.Fatalf("resolveOptions(%q): %v", args, err)
		}
		if opt.TaskName != `\Custom` {
			t.Errorf("resolveOptions(%q) gave task %q, want %q", args, opt.TaskName, `\Custom`)
		}
	}
	// And with no positional at all it is the real one.
	opt, err := resolveOptions(filepath.Join(t.TempDir(), "engramux.exe"), t.TempDir(), t.TempDir(), true, []string{"--apply"})
	if err != nil {
		t.Fatalf("resolveOptions: %v", err)
	}
	if opt.TaskName != schedule.TaskName {
		t.Errorf("TaskName = %q, want the real one %q", opt.TaskName, schedule.TaskName)
	}
}
