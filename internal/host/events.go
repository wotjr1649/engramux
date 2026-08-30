package host

import (
	"encoding/json/jsontext"
	"strconv"
	"strings"
)

// TimeoutSeconds is what every hook gets except the one Codex clamps. It is a
// hook timeout and not a budget: the relay's own ceiling is spec 5.3's, and this
// is only how long a host waits before giving up on it.
const TimeoutSeconds = 5

// codexSessionEndTimeout is Codex's documented maximum for SessionEnd, which it
// alone caps at 1 second by default and 3 at most while every other event
// defaults to 600.
//
// 3 rather than 1, and written rather than omitted: the relay's ceiling is 1 s
// total plus process start, which is already over Codex's default, and an
// omitted value silently means 1. Codex also documents SessionEnd hooks as
// always running synchronously even when async is set, so async is not a way
// around it.
const codexSessionEndTimeout = 3

// event is one row of spec 4.1's intersection.
//
// matcher is a pointer so that "no matcher" and "an empty matcher" stay
// different things. The host reference documents `"*"`, `""` and an omitted key
// as equivalent, so nothing here is load-bearing to the host - what it is for is
// the reader: `"*"` is written on the events whose matcher would otherwise
// filter something, so that capturing everything reads as the intent rather
// than as an oversight.
type event struct {
	name    string
	matcher *string
	// codexTimeout is 0 when the event takes [TimeoutSeconds]. One table, so a
	// renamed event cannot leave a timeout stranded under the old name.
	codexTimeout int
}

func star() *string { s := "*"; return &s }

// events is spec 4.1's 11-event intersection, in the order a reader compares
// against that section. Claude Code exposes 31 and Codex 11; the Codex set is a
// subset, and 1.0 handles the intersection and nothing else.
var events = []event{
	{name: "SessionStart", matcher: star()},
	{name: "SessionEnd", codexTimeout: codexSessionEndTimeout},
	{name: "UserPromptSubmit"},
	{name: "PreToolUse", matcher: star()},
	{name: "PostToolUse", matcher: star()},
	{name: "Stop"},
	{name: "SubagentStart"},
	{name: "SubagentStop"},
	{name: "PreCompact", matcher: star()},
	{name: "PostCompact", matcher: star()},
	{name: "PermissionRequest", matcher: star()},
}

// EventNames is the intersection in table order.
func EventNames() []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.name
	}
	return out
}

func lookup(name string) (event, bool) {
	for _, e := range events {
		if e.name == name {
			return e, true
		}
	}
	return event{}, false
}

// Matcher returns the matcher an event's entry carries, and whether it carries
// one at all. A false second result means the key is omitted, which is not the
// same as an empty string.
func Matcher(name string) (string, bool) {
	e, ok := lookup(name)
	if !ok || e.matcher == nil {
		return "", false
	}
	return *e.matcher, true
}

// CodexTimeout is the hook timeout Codex gets for one event.
func CodexTimeout(name string) int {
	if e, ok := lookup(name); ok && e.codexTimeout != 0 {
		return e.codexTimeout
	}
	return TimeoutSeconds
}

// forwardSlashes rewrites a Windows path for a JSON string.
//
// Both hosts accept either separator, and a forward-slash path needs no
// escaping inside JSON - so the file a person opens holds the path they can
// read rather than one with a doubled backslash at every component.
func forwardSlashes(path string) string { return strings.ReplaceAll(path, `\`, "/") }

// ClaudeEntry is the entry installed for one event in a Claude Code
// configuration: exec form, so a command and an explicit empty argument list
// (spec 4.2).
func ClaudeEntry(name, relay string) jsontext.Value {
	command := forwardSlashes(relay)
	return buildEntry(name, `"type":"command",`+
		`"command":`+quote(command)+`,`+
		`"args":[],`+
		`"timeout":`+strconv.Itoa(TimeoutSeconds)+`,`+
		`"statusMessage":"engramux capture"`)
}

// CodexEntry is the entry installed for one event in a Codex configuration.
//
// Two differences from Claude Code's, both spec 4.2's. Codex takes
// commandWindows as a single string rather than an argument vector, so the path
// is quoted inside the value. And `command` is set to the same string rather
// than left out, so the entry is not Windows-only by accident on a host that
// reads the portable key.
func CodexEntry(name, relay string) jsontext.Value {
	quoted := quote(`"` + forwardSlashes(relay) + `"`)
	return buildEntry(name, `"type":"command",`+
		`"command":`+quoted+`,`+
		`"commandWindows":`+quoted+`,`+
		`"timeout":`+strconv.Itoa(CodexTimeout(name))+`,`+
		`"statusMessage":"engramux capture"`)
}

// buildEntry wraps one hook object in the entry [MergeHooks] appends, carrying
// the event's matcher when it has one.
//
// The JSON is assembled rather than marshalled from a struct so that the member
// order is this file's decision and not the encoder's, and so that "no matcher"
// is an absent key rather than a field that has to be tagged into absence.
func buildEntry(name, hook string) jsontext.Value {
	var b strings.Builder
	b.WriteByte('{')
	if matcher, ok := Matcher(name); ok {
		b.WriteString(`"matcher":` + quote(matcher) + `,`)
	}
	b.WriteString(`"hooks":[{` + hook + `}]}`)
	return jsontext.Value(b.String())
}
