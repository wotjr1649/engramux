// Package fixtures holds the synthesised hook payloads Phase 1 tests run against.
//
// Every value in them is invented. The real captures under .capture/fixtures-raw/
// carry the user's prompts, commands, file contents and absolute paths, so nothing
// is copied out of them; only shape is (spec 6.2, 7.5). The guard against an
// invented fixture drifting away from what the hosts actually send is the shape
// test in this package.
package fixtures

import (
	"embed"
	"slices"
)

//go:embed testdata/*.json
var files embed.FS

// Fixture is one of the Phase 1 fixtures (spec 8): the bytes a host writes
// to a hook process's stdin, and the host x event cell it belongs to.
type Fixture struct {
	// File is the fixture's name under testdata/.
	File string
	// Host is the value host detection must arrive at (spec 4.3).
	Host string
	// Event is the payload's hook_event_name.
	Event string
}

// File names, so tests name a fixture without repeating a string literal.
const (
	CodexSessionEnd         = "codex-sessionend.json"
	CodexPostToolUseString  = "codex-posttooluse-string.json"
	CodexPostToolUseArray   = "codex-posttooluse-array.json"
	ClaudePostToolUseObject = "claude-code-posttooluse-object.json"
	ClaudeSessionStart      = "claude-code-sessionstart.json"
)

var all = []Fixture{
	{File: CodexSessionEnd, Host: "codex", Event: "SessionEnd"},
	{File: CodexPostToolUseString, Host: "codex", Event: "PostToolUse"},
	{File: CodexPostToolUseArray, Host: "codex", Event: "PostToolUse"},
	{File: ClaudePostToolUseObject, Host: "claude-code", Event: "PostToolUse"},
	{File: ClaudeSessionStart, Host: "claude-code", Event: "SessionStart"},
}

// All returns the Phase 1 fixtures.
func All() []Fixture { return slices.Clone(all) }

// Bytes returns the fixture's exact bytes. Later phases gate on a byte-for-byte
// round-trip, so nothing here parses or re-marshals: what was embedded is what
// comes back.
func (f Fixture) Bytes() ([]byte, error) { return files.ReadFile("testdata/" + f.File) }
