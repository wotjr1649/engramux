package host

import (
	"encoding/json/jsontext"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// spacedRelay builds the relay path of a user whose Windows account name
// carries a space, which is the shape backlog 51 is about: %LOCALAPPDATA%
// carries the account name, so the space reaches every path this product
// installs into.
//
// exists says how much of it is on disk. The installer plans both host files
// before it copies a single binary ([Install] decides everything readable
// first), so on a first install neither the relay nor its directory is there
// yet when the entry is written - and a spelling that only works for a path
// that already exists would work on every machine except a new one.
//
// It answers the relay path and the account directory the space is in, because
// the skip below has to ask Windows about that directory rather than ask
// [spaceFree] about its own answer.
func spacedRelay(t *testing.T, exists bool) (relay, account string) {
	t.Helper()
	account = filepath.Join(t.TempDir(), "First Last")
	local := filepath.Join(account, "AppData", "Local")
	bin := filepath.Join(local, "engramux", "bin")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatalf("create the profile: %v", err)
	}
	relay = filepath.Join(bin, RelayName)
	if exists {
		if err := os.MkdirAll(bin, 0o700); err != nil {
			t.Fatalf("create the install directory: %v", err)
		}
		if err := os.WriteFile(relay, []byte("MZ"), 0o600); err != nil {
			t.Fatalf("create the relay: %v", err)
		}
	}
	return relay, account
}

// requireShortNames skips when Windows has no space-free spelling of account to
// give anyone, which is a real configuration rather than a defect: 8.3 name
// generation is per-volume and can be turned off, and a directory created while
// it was off has no short name at all.
func requireShortNames(t *testing.T, account string) {
	t.Helper()
	// [shortName] and not [spaceFree]: a guard written against the answer
	// under test skips exactly when that answer is wrong, which is a test
	// that cannot go red. This one asks Windows the question directly.
	short, err := shortName(account)
	if err != nil {
		t.Skipf("Windows has no short name for a directory with a space in it here: %v", err)
	}
	if strings.Contains(short, " ") {
		t.Skip("this volume has no 8.3 names, so the spelling backlog 51 measured cannot be built here")
	}
}

// TestCodexTakesAPathWithNoSpaceInIt is backlog 51, and it is the second half
// of backlog 50: the quotes that came out are exactly what a path with a space
// would have needed, and taking them out left that path with no spelling at all.
//
// # What was measured, on 2026-09-05, and against what
//
// A Codex hook command is not an argument vector. codex-rs's
// hooks/src/engine/command_runner.rs at tag rust-v0.150.1 - the version
// installed on the machine this was measured on - hands the value to a shell:
// COMSPEC with /C when no shell is configured, and the session's own shell
// otherwise, as one ordinary argument. This machine's Codex runs the second
// shape: features.shell_snapshot is on and its session rollout records
// "shell":"powershell".
//
// Both shapes were reproduced outside Codex against a stub that writes nothing
// on stdout and records that it ran, over cmd.exe, powershell.exe and pwsh.exe,
// with and without a space in the path:
//
//   - the plain path runs everywhere except in a PowerShell with a space in it,
//     where the first space ends the command name;
//   - the self-quoted path - the pre-fix spelling - runs under cmd.exe and is
//     **printed rather than run** under both PowerShells, exit 0, which is
//     backlog 50's defect reproduced exactly and is what identified the shell;
//   - the call operator, `& "path"`, runs under both PowerShells and is a
//     syntax error under cmd.exe;
//   - the 8.3 short path, plain, ran in all six.
//
// So there is no quoting that serves both shells, and the answer is to write a
// path that needs none. That is what [spaceFree] does, and this test is the
// assertion that it reaches the file the Codex entry is for.
func TestCodexTakesAPathWithNoSpaceInIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		exists bool
	}{
		{"the relay is not copied yet", false},
		{"the relay is already installed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			relay, account := spacedRelay(t, tc.exists)
			if !strings.Contains(relay, " ") {
				t.Fatalf("the fixture carries no space, so this test proves nothing: %s", relay)
			}
			requireShortNames(t, account)

			config, err := MergeHooks([]byte(`{}`), EventNames(),
				func(event string) jsontext.Value { return CodexEntry(event, relay) })
			if err != nil {
				t.Fatalf("build a Codex configuration: %v", err)
			}
			found, err := HookCommands(config, EventNames())
			if err != nil {
				t.Fatalf("HookCommands: %v", err)
			}
			for _, event := range EventNames() {
				commands := found[event]
				if len(commands) != 1 {
					t.Fatalf("%s: %d Engramux commands found, want 1", event, len(commands))
				}
				got := commands[0]
				if strings.Contains(got, " ") {
					t.Errorf("%s: the command carries a space, which no shell spelling survives:\n%s", event, got)
				}
				// Only the part that has to change may change. The
				// installer recognises its own hook by the word
				// engramux in the command, so a rewrite that reached
				// the install directory or the binary would make the
				// next install add a second hook beside this one.
				if want := "/engramux/bin/" + RelayName; !strings.HasSuffix(got, want) {
					t.Errorf("%s: the tail below the account directory was rewritten too\ngot:  %s\nwant suffix: %s", event, got, want)
				}
				if !PointsAt(got, relay) {
					t.Errorf("%s: doctor would call this entry stale and it is the one the installer just wrote\ngot: %s", event, got)
				}
			}
			if !tc.exists {
				return
			}
			// The spelling is only worth anything if it names the same
			// file, and on Windows two spellings of one path is exactly
			// what a short name is.
			written, err := os.Stat(filepath.FromSlash(found[EventNames()[0]][0]))
			if err != nil {
				t.Fatalf("stat what was written into the entry: %v", err)
			}
			want, err := os.Stat(relay)
			if err != nil {
				t.Fatalf("stat the relay: %v", err)
			}
			if !os.SameFile(written, want) {
				t.Errorf("the entry names a different file from the relay it was built for")
			}
		})
	}
}

// TestClaudeCodeKeepsTheReadablePath pins the asymmetry, so that a later change
// does not tidy it away as an inconsistency.
//
// Claude Code takes exec form - a command and an argument array, spec 4.2 - so
// no shell parses the path and a space in it is not a problem to solve. Writing
// the 8.3 spelling there would cost the one thing [forwardSlashes] exists for,
// which is that the file a person opens holds a path they can read.
func TestClaudeCodeKeepsTheReadablePath(t *testing.T) {
	relay, account := spacedRelay(t, true)
	requireShortNames(t, account)

	s := string(ClaudeEntry("Stop", relay))
	if !strings.Contains(s, forwardSlashes(relay)) {
		t.Errorf("the Claude Code entry does not carry the relay path as computed\ngot: %s", s)
	}
}

// TestSpaceFreeLeavesEveryOtherPathExactlyAsItIs is the other half: the
// rewrite is reachable only by a path that carries a space, so every machine
// that has been measured keeps the spelling that was measured on it.
func TestSpaceFreeLeavesEveryOtherPathExactlyAsItIs(t *testing.T) {
	for _, path := range []string{
		`C:/Users/x/AppData/Local/engramux/bin/engramux.exe`,
		`C:\Users\x\AppData\Local\engramux\bin\engramux.exe`,
		``,
		`engramux.exe`,
	} {
		if got := spaceFree(path); got != path {
			t.Errorf("spaceFree(%q) = %q, want it returned unchanged", path, got)
		}
	}
}

// TestSpaceFreeCoversEverySpaceInThePath pins the rule rather than the one
// instance of it, because the two are not the same edit.
//
// The prefix that gets rewritten has to reach the LAST space in the path. A
// version that stops at the first one is right for every real installation
// measured so far - %LOCALAPPDATA% carries one spaced component, the account
// name - and leaves a space in the answer the moment a second one appears
// above the install directory. What comes back then is a path this product
// would write into a Codex entry believing it had solved the problem.
func TestSpaceFreeCoversEverySpaceInThePath(t *testing.T) {
	account := filepath.Join(t.TempDir(), "First Last")
	dir := filepath.Join(account, "Local Settings", "engramux", "bin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create the profile: %v", err)
	}
	requireShortNames(t, account)

	relay := filepath.Join(dir, RelayName)
	got := spaceFree(relay)
	if strings.Contains(got, " ") {
		t.Errorf("a space above the install directory survived the rewrite\ngot: %s", got)
	}
	if want := string(filepath.Separator) + filepath.Join("engramux", "bin", RelayName); !strings.HasSuffix(got, want) {
		t.Errorf("the tail below the spaces was rewritten too\ngot: %s\nwant suffix: %s", got, want)
	}
}
