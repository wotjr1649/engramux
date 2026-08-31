package host

import (
	"encoding/json/jsontext"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestHookCommandsReadsBackEveryEventMergeHooksWrote is the round trip that
// makes the read side worth having: whatever the installer writes, this finds,
// and it finds it as a command that [PointsAt] recognises.
//
// Both hosts, because their entries differ in exactly the way that breaks a
// naive reader - Codex quotes the path inside the command string and Claude
// Code does not (spec 4.2).
func TestHookCommandsReadsBackEveryEventMergeHooksWrote(t *testing.T) {
	relay := filepath.Join(t.TempDir(), "bin", "engramux.exe")

	for _, tc := range []struct {
		host  string
		build func(name, relay string) jsontext.Value
	}{
		{"claude-code", ClaudeEntry},
		{"codex", CodexEntry},
	} {
		t.Run(tc.host, func(t *testing.T) {
			merged, err := MergeHooks([]byte(`{}`), EventNames(),
				func(event string) jsontext.Value { return tc.build(event, relay) })
			if err != nil {
				t.Fatalf("MergeHooks: %v", err)
			}

			found, err := HookCommands(merged, EventNames())
			if err != nil {
				t.Fatalf("HookCommands: %v", err)
			}
			if len(found) != len(EventNames()) {
				t.Fatalf("HookCommands found %d events, want %d: %v", len(found), len(EventNames()), found)
			}
			for _, event := range EventNames() {
				commands := found[event]
				if len(commands) != 1 {
					t.Errorf("%s: %d commands, want 1: %q", event, len(commands), commands)
					continue
				}
				if !PointsAt(commands[0], relay) {
					t.Errorf("%s: PointsAt(%q, %q) = false", event, commands[0], relay)
				}
			}
		})
	}
}

// TestHookCommandsIgnoresHooksThatAreNotOurs holds the half of the rule that
// decides what `doctor` calls missing. A user's own hook under an event we
// capture must not read as an Engramux entry, or a machine with no capture
// installed would report itself wired.
func TestHookCommandsIgnoresHooksThatAreNotOurs(t *testing.T) {
	src := []byte(`{"hooks":{"SessionStart":[{"hooks":[` +
		`{"type":"command","command":"C:/tools/my-own-gate.exe"}]}]}}`)

	found, err := HookCommands(src, EventNames())
	if err != nil {
		t.Fatalf("HookCommands: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("HookCommands claimed %v, want nothing", found)
	}
}

// TestHookCommandsKeepsOurEntryBesideSomebodyElses is the same file with both
// in it, which is the shape [rewriteEntries] actually produces: the user's
// entry is kept and ours is appended after it.
func TestHookCommandsKeepsOurEntryBesideSomebodyElses(t *testing.T) {
	relay := `C:/Local/engramux/bin/engramux.exe`
	src := []byte(`{"hooks":{"Stop":[` +
		`{"hooks":[{"type":"command","command":"C:/tools/my-own-gate.exe"}]},` +
		`{"hooks":[{"type":"command","command":"` + relay + `"}]}]}}`)

	found, err := HookCommands(src, []string{"Stop"})
	if err != nil {
		t.Fatalf("HookCommands: %v", err)
	}
	if want := []string{relay}; !slices.Equal(found["Stop"], want) {
		t.Errorf("HookCommands = %q, want %q", found["Stop"], want)
	}
}

// TestHookCommandsSeparatesStaleFromMissing is the distinction the whole check
// exists for. An entry naming a relay that is not the installed one is present
// and wrong, which is a different sentence from absent - and only [PointsAt]
// can tell them apart, because [HookCommands] reports both as found.
func TestHookCommandsSeparatesStaleFromMissing(t *testing.T) {
	installed := `C:\Local\engramux\bin\engramux.exe`
	src := []byte(`{"hooks":{"Stop":[{"hooks":[` +
		`{"type":"command","command":"D:/old/dist/engramux.exe"}]}]}}`)

	found, err := HookCommands(src, []string{"Stop", "SessionStart"})
	if err != nil {
		t.Fatalf("HookCommands: %v", err)
	}
	if len(found["Stop"]) != 1 {
		t.Fatalf("Stop: %q, want one command", found["Stop"])
	}
	if PointsAt(found["Stop"][0], installed) {
		t.Errorf("PointsAt(%q, %q) = true; a stale entry read as the installed one",
			found["Stop"][0], installed)
	}
	if _, ok := found["SessionStart"]; ok {
		t.Errorf("SessionStart is not in the document and came back as %q", found["SessionStart"])
	}
}

// TestHookCommandsRefusesADocumentItCannotRead. A configuration file that is
// not JSON is a finding rather than an empty answer: reporting no hooks would
// send the user to re-run an installer that is going to refuse the same file.
func TestHookCommandsRefusesADocumentItCannotRead(t *testing.T) {
	if _, err := HookCommands([]byte(`{"hooks":`), EventNames()); err == nil {
		t.Fatal("HookCommands accepted a truncated document")
	}
	// No hook table at all is not an error - it is a host configuration
	// nobody has installed into yet.
	found, err := HookCommands([]byte(`{"model":"opus"}`), EventNames())
	if err != nil {
		t.Fatalf("HookCommands refused a document with no hook table: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("HookCommands claimed %v, want nothing", found)
	}
}

// TestPointsAtNormalisesTheThreeSpellingsThisProductWrites, and nothing else.
func TestPointsAtNormalisesTheThreeSpellingsThisProductWrites(t *testing.T) {
	const relay = `C:\Users\x\AppData\Local\engramux\bin\engramux.exe`
	for _, tc := range []struct {
		name    string
		command string
		want    bool
	}{
		{"forward slashes", `C:/Users/x/AppData/Local/engramux/bin/engramux.exe`, true},
		{"codex quoting", `"C:/Users/x/AppData/Local/engramux/bin/engramux.exe"`, true},
		{"a different case", `c:\users\x\appdata\local\engramux\BIN\Engramux.exe`, true},
		{"the same path", relay, true},
		{"another directory", `D:\dist\engramux.exe`, false},
		{"the service", `C:\Users\x\AppData\Local\engramux\bin\engramux-service.exe`, false},
		{"a prefix of it", `C:\Users\x\AppData\Local\engramux\bin\engramux`, false},
		{"empty", ``, false},
	} {
		if got := PointsAt(tc.command, relay); got != tc.want {
			t.Errorf("%s: PointsAt(%q, relay) = %v, want %v", tc.name, tc.command, got, tc.want)
		}
	}
}

// TestPointsAtDoesNotMatchOnASubstring guards the direction a Contains-based
// version would be wrong in: the installed relay's directory name appears
// inside every path this product writes.
func TestPointsAtDoesNotMatchOnASubstring(t *testing.T) {
	const relay = `C:\Local\engramux\bin\engramux.exe`
	if PointsAt(`C:\Local\engramux\bin\engramux.exe.old`, relay) {
		t.Error("PointsAt matched a longer path that merely starts with the relay")
	}
	if !strings.Contains(`C:\Local\engramux\bin\engramux.exe.old`, relay) {
		t.Fatal("the fixture no longer contains the relay, so this test proves nothing")
	}
}
