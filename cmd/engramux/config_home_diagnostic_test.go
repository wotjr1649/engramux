package main

import (
	"path/filepath"
	"testing"
)

// This measures backlog 54 without reading or changing any host configuration.
func TestMeasureClaudeConfigHomeResolution(t *testing.T) {
	local, home, moved := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", moved)
	t.Setenv("ENGRAMUX_CLAUDE_SETTINGS", "")
	t.Setenv("ENGRAMUX_CLAUDE_PLUGINS", "")
	opt, err := resolvePaths(local, home, nil)
	if err != nil {
		t.Fatal(err)
	}
	settingsMoved := opt.ClaudePath == filepath.Join(moved, "settings.json")
	pluginsMoved := opt.PluginCache == filepath.Join(moved, "plugins", "cache")
	if opt.ClaudePath != filepath.Join(home, ".claude", "settings.json") || opt.PluginCache != filepath.Join(home, ".claude", "plugins", "cache") {
		t.Fatal("recorded path behavior changed; reassess diagnostic")
	}
	t.Logf("settings_honors_config_home=%v plugins_honor_config_home=%v; process success is not a correctness verdict", settingsMoved, pluginsMoved)
}
