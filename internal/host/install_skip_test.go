package host

import (
	"slices"
	"strings"
	"testing"
)

// TestInstallSkipsTheClaudeRegistrationTheHostAlreadyHas is backlog 35. The
// first real re-install asked Claude Code's CLI to add a registration the
// previous installer had already made, the CLI exited 1, and the run reported
// a failure nothing had caused. A host whose own file already names the
// endpoint is left alone and said to be; one that names another URL - the
// stale case spec 5.9 has doctor report - is registered again.
func TestInstallSkipsTheClaudeRegistrationTheHostAlreadyHas(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true
	seedRaw(t, tr.opt.ClaudeMCP, `{"mcpServers":{"engramux":{"type":"http","url":"`+probeURL+`"}}}`)

	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if slices.ContainsFunc(*tr.steps, func(s string) bool { return strings.HasPrefix(s, "claude-add") }) {
		t.Errorf("the CLI was asked to add a registration the host already had:\n%q", *tr.steps)
	}
	if !slices.Contains(report, "claude-code mcp: already points at this endpoint") {
		t.Errorf("the report does not say the host already points at the endpoint:\n%s", strings.Join(report, "\n"))
	}

	stale := newTree(t)
	stale.opt.Apply = true
	seedRaw(t, stale.opt.ClaudeMCP, `{"mcpServers":{"engramux":{"type":"http","url":"http://127.0.0.1:1/mcp"}}}`)
	if _, err := Install(t.Context(), stale.opt, stale.sys); err != nil {
		t.Fatalf("Install (stale): %v", err)
	}
	if !slices.Contains(*stale.steps, "claude-add "+probeURL) {
		t.Errorf("a host pointing at another URL was not registered again:\n%q", *stale.steps)
	}
}
