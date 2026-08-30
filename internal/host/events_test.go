package host

import (
	"encoding/json/jsontext"
	"strings"
	"testing"
)

// TestTheEventTableIsSpecFourOnesIntersection pins the set itself, because it
// is a decision and not an implementation detail: spec 4.1 says 1.0 handles the
// 11-event intersection and nothing else, and a table that quietly grew or lost
// one would still install cleanly.
func TestTheEventTableIsSpecFourOnesIntersection(t *testing.T) {
	want := []string{
		"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse",
		"Stop", "SubagentStart", "SubagentStop", "PreCompact", "PostCompact", "PermissionRequest",
	}
	got := EventNames()
	if len(got) != len(want) {
		t.Fatalf("the table holds %d events, want the %d of spec 4.1's intersection: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d is %q, want %q - the order is the table's and a reader compares against 4.1",
				i, got[i], want[i])
		}
	}

	seen := map[string]bool{}
	for _, e := range got {
		if seen[e] {
			t.Errorf("%q appears twice, which would install two hooks for one event", e)
		}
		seen[e] = true
	}
}

// TestOnlyCodexSessionEndIsClamped holds the one per-event exception, from the
// table rather than from the installer, so that a change to the table fails
// here rather than in a host's configuration file.
//
// Codex documents SessionEnd alone as one second by default and three at most,
// against 600 for every other event. Three is written explicitly because
// omitting it silently means one, which is already under the relay's own
// ceiling.
func TestOnlyCodexSessionEndIsClamped(t *testing.T) {
	for _, event := range EventNames() {
		got := CodexTimeout(event)
		want := TimeoutSeconds
		if event == "SessionEnd" {
			want = 3
		}
		if got != want {
			t.Errorf("CodexTimeout(%q) = %d, want %d", event, got, want)
		}
	}
	if TimeoutSeconds <= 3 {
		t.Errorf("TimeoutSeconds is %d, so the SessionEnd clamp is not observable and this test proves nothing",
			TimeoutSeconds)
	}
}

// TestTheMatcherSetIsWhatTheTableSays guards the drift the Node installer's
// header comment actually had: it listed events as carrying a matcher that the
// table gave none. Written as two exact sets so that moving an event between
// them is a visible change.
func TestTheMatcherSetIsWhatTheTableSays(t *testing.T) {
	withMatcher := map[string]bool{
		"SessionStart": true, "PreToolUse": true, "PostToolUse": true,
		"PreCompact": true, "PostCompact": true, "PermissionRequest": true,
	}
	for _, event := range EventNames() {
		got, ok := Matcher(event)
		if withMatcher[event] {
			if !ok || got != "*" {
				t.Errorf("Matcher(%q) = %q, %v; want \"*\", true", event, got, ok)
			}
			continue
		}
		if ok {
			t.Errorf("Matcher(%q) = %q, true; want no matcher at all - an omitted key, not an empty one",
				event, got)
		}
	}
	if len(withMatcher) == len(EventNames()) {
		t.Error("every event carries a matcher, so this test cannot see the omitted case")
	}
}

// TestClaudeEntryShape pins what goes into a Claude Code configuration.
func TestClaudeEntryShape(t *testing.T) {
	bs := string([]byte{92})
	relay := `C:` + bs + `Users` + bs + `x` + bs + `engramux.exe`

	entry := ClaudeEntry("PreToolUse", relay)
	s := string(entry)

	if strings.Contains(s, bs) {
		t.Errorf("the command carries a backslash; the installer writes forward slashes so that "+
			"a JSON string does not have to escape them\ngot: %s", s)
	}
	if !strings.Contains(s, `"command":"C:/Users/x/engramux.exe"`) {
		t.Errorf("the relay path is not what it should be\ngot: %s", s)
	}
	if !strings.Contains(s, `"matcher":"*"`) {
		t.Errorf("PreToolUse carries a matcher and this entry has none\ngot: %s", s)
	}
	if strings.Contains(s, "commandWindows") {
		t.Errorf("commandWindows is Codex's key; Claude Code has no such field and ignores it\ngot: %s", s)
	}
	if !strings.Contains(s, `"args":[]`) {
		t.Errorf("Claude Code takes exec form, so args is present and empty\ngot: %s", s)
	}

	// An event with no matcher must produce no matcher key rather than an
	// empty one: the two are different to the host.
	if s := string(ClaudeEntry("Stop", relay)); strings.Contains(s, "matcher") {
		t.Errorf("Stop has no matcher, so the key must be absent\ngot: %s", s)
	}
}

// TestCodexEntryShape pins what goes into a Codex configuration, which differs
// from Claude Code's in the two ways spec 4.2 records.
func TestCodexEntryShape(t *testing.T) {
	const relay = `C:/x/engramux.exe`

	s := string(CodexEntry("SessionEnd", relay))
	if !strings.Contains(s, `"commandWindows"`) {
		t.Errorf("Codex takes commandWindows and this entry has none\ngot: %s", s)
	}
	// command is set to the same value so the entry is not Windows-only by
	// accident on a host that reads the portable key.
	if strings.Count(s, `"\"C:/x/engramux.exe\""`) != 2 {
		t.Errorf("command and commandWindows should both be the quoted path\ngot: %s", s)
	}
	if !strings.Contains(s, `"timeout":3`) {
		t.Errorf("Codex SessionEnd is clamped to 3 and this entry is not\ngot: %s", s)
	}
	if s := string(CodexEntry("PreToolUse", relay)); !strings.Contains(s, `"timeout":5`) {
		t.Errorf("every Codex event but SessionEnd takes TimeoutSeconds\ngot: %s", s)
	}
}

// TestEveryEntryIsValidJSONAndSurvivesAMerge is the join between this file and
// mergehooks: an entry that is not valid JSON, or that MergeHooks cannot find
// again on a re-run, breaks the installer in a way neither file's own tests
// would see.
func TestEveryEntryIsValidJSONAndSurvivesAMerge(t *testing.T) {
	const relay = `C:/x/engramux.exe`
	for _, host := range []struct {
		name  string
		entry func(string, string) jsontext.Value
	}{
		{"claude-code", ClaudeEntry},
		{"codex", CodexEntry},
	} {
		t.Run(host.name, func(t *testing.T) {
			entryFor := func(event string) jsontext.Value { return host.entry(event, relay) }
			once, err := MergeHooks([]byte(`{}`), EventNames(), entryFor)
			if err != nil {
				t.Fatalf("first merge: %v", err)
			}
			twice, err := MergeHooks(once, EventNames(), entryFor)
			if err != nil {
				t.Fatalf("second merge: %v", err)
			}
			if string(once) != string(twice) {
				t.Errorf("a second install is not a no-op, so isEngramux does not recognise "+
					"what this host's entry writes\nfirst:\n%s\nsecond:\n%s", once, twice)
			}
			// Counted on the status message rather than on the path: the
			// Codex entry writes the path twice, into command and into
			// commandWindows, so counting the path measures the host's shape
			// and not the number of hooks. This assertion was wrong that way
			// first, and the code was right.
			if n := strings.Count(string(twice), `"engramux capture"`); n != len(EventNames()) {
				t.Errorf("%d hooks after two merges, want %d - one per event\ngot:\n%s",
					n, len(EventNames()), twice)
			}
		})
	}
}
