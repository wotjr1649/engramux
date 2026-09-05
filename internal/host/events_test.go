package host

import (
	"encoding/json/jsontext"
	"strconv"
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
	if strings.Count(s, `"C:/x/engramux.exe"`) != 2 {
		t.Errorf("command and commandWindows should both be the plain path\ngot: %s", s)
	}
	if !strings.Contains(s, `"timeout":3`) {
		t.Errorf("Codex SessionEnd is clamped to 3 and this entry is not\ngot: %s", s)
	}
	if s := string(CodexEntry("PreToolUse", relay)); !strings.Contains(s, `"timeout":5`) {
		t.Errorf("every Codex event but SessionEnd takes TimeoutSeconds\ngot: %s", s)
	}
}

// TestCodexTakesThePathWithItsOwnQuotesStrippedOut is backlog 50, and it is a
// measurement rather than a preference.
//
// Until 2026-09-05 [CodexEntry] wrapped the relay path in quotes *inside* the
// value, so the file held a command whose every character including the first
// was one quoted token. Codex did not execute it: it echoed it. Measured on the
// owner's machine, where the echoed path arrived back as the hook's own stdout
// - `hook context: C:/.../engramux.exe` - while `readOneHostHooks` went on
// answering `11 of 11 events point at the installed relay` for nine days and
// not one Codex event was ever captured. The same file with the quotes removed
// delivered a `codex UserPromptSubmit` on the first prompt.
//
// # What this test cannot see
//
// A relay path containing a space. This spelling is measured on a path with
// none, and the quotes it drops are what a path with a space would have needed
// - that is backlog 51 and it is open. The assertion here is deliberately about
// the quotes and not about spaces, so that whatever answers 51 does not have to
// argue with a test that pinned the wrong half.
func TestCodexTakesThePathWithItsOwnQuotesStrippedOut(t *testing.T) {
	for _, event := range EventNames() {
		s := string(CodexEntry(event, `C:/x/engramux.exe`))
		if strings.Contains(s, `\"`) {
			t.Errorf("%s: the entry carries an escaped quote, so the host is handed a command "+
				"whose program token is quoted and it echoes the line instead of running it\ngot: %s",
				event, s)
		}
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

// TestTheMergedDocumentCarriesEveryEventAtItsOwnTimeout is the assertion the
// script's own test held and this package did not: not what an entry builder
// returns, but what ends up in the file after a merge.
//
// The distinction is backlog 23's. That row was closed by widening the script's
// test from SessionEnd alone to all eleven, because a change lowering the other
// ten would otherwise pass - a relay given less than spec 5.3's budget on every
// event but the one the test was named for. Deleting the script must not delete
// that.
func TestTheMergedDocumentCarriesEveryEventAtItsOwnTimeout(t *testing.T) {
	const relay = `C:/x/engramux.exe`
	for _, h := range []struct {
		name  string
		entry func(string, string) jsontext.Value
		want  func(string) int
	}{
		{"claude-code", ClaudeEntry, func(string) int { return TimeoutSeconds }},
		{"codex", CodexEntry, CodexTimeout},
	} {
		t.Run(h.name, func(t *testing.T) {
			merged, err := MergeHooks([]byte(`{}`), EventNames(),
				func(event string) jsontext.Value { return h.entry(event, relay) })
			if err != nil {
				t.Fatalf("MergeHooks: %v", err)
			}
			table, ok, err := getMember(jsontext.Value(merged), "hooks")
			if err != nil || !ok {
				t.Fatalf("no hooks table in the merged document: %v", err)
			}
			for _, event := range EventNames() {
				entries, ok, err := getMember(table, event)
				if err != nil || !ok {
					t.Errorf("%s is not in the merged document: %v", event, err)
					continue
				}
				// The merged document is indented, so the needle carries the space
				// the encoder puts after the colon. Written compact first, which
				// found nothing because it matched nothing.
				want := `"timeout": ` + strconv.Itoa(h.want(event))
				if !strings.Contains(string(entries), want) {
					t.Errorf("%s carries the wrong timeout; want %s in:\n%s", event, want, entries)
				}
			}
		})
	}
}
