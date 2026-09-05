package host

import (
	"encoding/json/jsontext"
	"strings"
	"testing"
)

// codexEntryBeforeTheQuotingFix is what [CodexEntry] wrote until 2026-09-05:
// the relay path wrapped in quotes inside the value, on both keys.
//
// It is spelled out here rather than reached for through a flag, because what
// this file tests is a shape the product must never write again and must go on
// reading. A helper shared with the writer would let one change move both.
func codexEntryBeforeTheQuotingFix(name, relay string) jsontext.Value {
	quoted := quote(`"` + forwardSlashes(relay) + `"`)
	return buildEntry(name, `"type":"command",`+
		`"command":`+quoted+`,`+
		`"commandWindows":`+quoted+`,`+
		`"statusMessage":"engramux capture"`)
}

// TestAnEntryFromBeforeTheQuotingFixStillReadsAsWired is the upgrade path, and
// it is a test rather than a comment because the code that carries it looks
// like dead code.
//
// Backlog 50's fix changed what [CodexEntry] writes. It changed nothing about
// the file already sitting in `~/.codex/hooks.json` on every machine that has
// not re-run the installer, and `doctor` reads that file. [normalizeCommand]
// trims the quotes, so the old spelling still resolves to the installed relay
// and those machines are reported wired rather than stale - which is the true
// answer: the entry does name the relay, and what was wrong with it was that
// the host would not run it, not where it pointed.
//
// Delete the trim as no longer needed and every un-upgraded installation turns
// red at once, with `install --apply` as the advice - which is the right advice
// for a different reason and would arrive as a lie about the file.
func TestAnEntryFromBeforeTheQuotingFixStillReadsAsWired(t *testing.T) {
	const relay = `C:/x/engramux.exe`

	old, err := MergeHooks([]byte(`{}`), EventNames(),
		func(event string) jsontext.Value { return codexEntryBeforeTheQuotingFix(event, relay) })
	if err != nil {
		t.Fatalf("build a pre-fix configuration: %v", err)
	}
	// The fixture has to be the old shape, or this test passes by testing
	// the new one. Asserted rather than assumed, because the helper above
	// is one edit away from drifting into the current spelling.
	if !strings.Contains(string(old), `\"`) {
		t.Fatalf("the fixture is not the pre-fix shape, so this test proves nothing:\n%s", old)
	}

	found, err := HookCommands(old, EventNames())
	if err != nil {
		t.Fatalf("HookCommands: %v", err)
	}
	for _, event := range EventNames() {
		commands := found[event]
		if len(commands) != 1 {
			t.Errorf("%s: %d Engramux commands found, want 1", event, len(commands))
			continue
		}
		if !PointsAt(commands[0], relay) {
			t.Errorf("%s: a pre-fix entry no longer resolves to the installed relay, so every "+
				"machine that has not re-run the installer reads as stale\ngot: %s",
				event, commands[0])
		}
	}

	// And the installer still recognises it as its own, so a re-install
	// replaces the old entry rather than appending beside it.
	fixed, err := MergeHooks(old, EventNames(),
		func(event string) jsontext.Value { return CodexEntry(event, relay) })
	if err != nil {
		t.Fatalf("re-merge over a pre-fix configuration: %v", err)
	}
	if n := strings.Count(string(fixed), `"engramux capture"`); n != len(EventNames()) {
		t.Errorf("a re-install over the old spelling left %d entries, want %d - isEngramux did not "+
			"recognise what the previous version wrote", n, len(EventNames()))
	}
	if strings.Contains(string(fixed), `\"`) {
		t.Errorf("a re-install left the old quoted spelling in the file:\n%s", fixed)
	}
}
