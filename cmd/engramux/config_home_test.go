package main

import (
	"path/filepath"
	"testing"

	"github.com/wotjr1649/engramux/internal/claudehome"
)

// Backlog 54, as a gate rather than as the diagnostic it replaces.
//
// # The rule, and where each half of it was measured
//
// `CLAUDE_CONFIG_DIR` moves Claude Code's configuration home. What made this a
// row rather than a one-line fix is that the global application-state file -
// the one holding user-scope `mcpServers`, which `install` reads before
// deciding whether to register - does not sit inside that home by default: it
// is a *sibling* of it. So the override is not a single join.
//
// **Measured 2026-09-09 against the installed host**, which is the only thing
// that can answer it. With the variable pointed at an empty directory,
// `claude mcp list` answered "No MCP servers configured" where the same command
// without it found this product's endpoint - so the file moves. And the host
// then created its own copy of that file *inside* the directory the variable
// named, which is where the second half of the rule comes from. The default
// half is the shape this repository already shipped and `doctor` confirms on a
// machine with the variable unset.
//
// # What this test holds
//
// Both arms of every path, and the precedence of the `ENGRAMUX_*` overrides
// over both. The Codex paths are asserted unmoved in the overridden arm: this
// variable is Claude Code's, and a resolver that moved the other host's files
// with it would pass every Claude assertion here.
func TestClaudeConfigDirMovesEveryClaudePath(t *testing.T) {
	local, home, moved := t.TempDir(), t.TempDir(), t.TempDir()
	clearOverrides(t)
	t.Setenv("CLAUDE_CONFIG_DIR", moved)

	opt, err := resolvePaths(local, home, nil)
	if err != nil {
		t.Fatalf("resolvePaths: %v", err)
	}

	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"settings", opt.ClaudePath, filepath.Join(moved, "settings.json")},
		{"plugin cache", opt.PluginCache, filepath.Join(moved, "plugins", "cache")},
		{"application state", opt.ClaudeMCP, filepath.Join(moved, claudehome.AppStateName)},
		// Codex is a different host with a different variable. It must
		// not follow this one.
		{"codex hooks", opt.CodexHooks, filepath.Join(home, ".codex", "hooks.json")},
		{"codex config", opt.CodexConfig, filepath.Join(home, ".codex", "config.toml")},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestWithoutTheVariableTheDefaultsAreUnchanged. The override is not the only
// arm that can regress: a resolver that always joined onto the configuration
// home would put the application-state file inside `.claude`, where the host
// does not keep it, and would break every machine that has never set the
// variable - which is all of them so far.
func TestWithoutTheVariableTheDefaultsAreUnchanged(t *testing.T) {
	local, home := t.TempDir(), t.TempDir()
	clearOverrides(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	opt, err := resolvePaths(local, home, nil)
	if err != nil {
		t.Fatalf("resolvePaths: %v", err)
	}

	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"settings", opt.ClaudePath, filepath.Join(home, ".claude", "settings.json")},
		{"plugin cache", opt.PluginCache, filepath.Join(home, ".claude", "plugins", "cache")},
		// The sibling, and the whole reason this is not one join.
		{"application state", opt.ClaudeMCP, filepath.Join(home, claudehome.AppStateName)},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestTheExplicitOverridesStillWin. The `ENGRAMUX_*` variables are the seam the
// rest of the suite isolates itself with, and a configuration home that
// outranked them would quietly point every other test at a real host file.
func TestTheExplicitOverridesStillWin(t *testing.T) {
	local, home, moved := t.TempDir(), t.TempDir(), t.TempDir()
	explicit := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", moved)
	t.Setenv("ENGRAMUX_CLAUDE_SETTINGS", filepath.Join(explicit, "settings.json"))
	t.Setenv("ENGRAMUX_CLAUDE_PLUGINS", filepath.Join(explicit, "cache"))
	t.Setenv("ENGRAMUX_CLAUDE_MCP", filepath.Join(explicit, "state"))

	opt, err := resolvePaths(local, home, nil)
	if err != nil {
		t.Fatalf("resolvePaths: %v", err)
	}
	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"settings", opt.ClaudePath, filepath.Join(explicit, "settings.json")},
		{"plugin cache", opt.PluginCache, filepath.Join(explicit, "cache")},
		{"application state", opt.ClaudeMCP, filepath.Join(explicit, "state")},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// clearOverrides empties the ENGRAMUX_* seam so that a test reads the derived
// answer rather than whatever an earlier test or the ambient environment left.
func clearOverrides(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ENGRAMUX_CLAUDE_SETTINGS",
		"ENGRAMUX_CLAUDE_PLUGINS",
		"ENGRAMUX_CLAUDE_MCP",
	} {
		t.Setenv(k, "")
	}
}
