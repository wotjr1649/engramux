package host_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The `bin/engramux` shim, which is the only thing in this repository that is
// shell rather than Go, and the only thing whose caller is Claude Code's Bash
// tool rather than a person.
//
// # Why it exists and why it is not a copy of the binary
//
// Measured 2026-09-09 against the installed host: Claude Code adds every
// enabled plugin's `<root>/bin` to the Bash tool's PATH, and it does so whether
// or not that directory exists - all four plugins installed on the machine this
// was written on were on PATH with no `bin/` between them. So a file placed
// there is invokable as a bare command inside a Claude Code session, and
// nowhere else: the user's own shell never sees it.
//
// What must not go there is a second `engramux.exe`. The archive already
// carries one, and a plugin pinned at 0.1.0 beside a service updated to 0.2.0
// is two builds of one product answering the same name - which `doctor`'s
// "installed and running agree" line exists to make impossible. The shim
// resolves the *installed* copy instead, so there is one real binary however
// the name is reached.
//
// # What these tests hold
//
// One branch, both arms. The message on the missing arm is asserted for the
// command it has to name, because a shim that says only "not found" leaves the
// reader exactly where the shim was written to stop them being.
func TestTheShimPrefersTheInstalledBinary(t *testing.T) {
	bash := lookBash(t)
	shim := shimPath(t)

	// A stub standing in for the installed CLI. It answers with something no
	// real binary would, so a pass cannot come from the shim having found
	// some other engramux on this machine.
	local := t.TempDir()
	binDir := filepath.Join(local, "engramux", "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatalf("make the stub bin directory: %v", err)
	}
	// 0o600 rather than 0o700, and the shim's own `[ -x ]` still finds it:
	// this shell infers the executable bit from the file's content - a shebang
	// or a PE header - because the filesystem carries no POSIX mode. That is
	// the same inference the archive relies on, so testing under it is testing
	// what ships.
	stub := "#!/usr/bin/env bash\necho STUB-INSTALLED-COPY \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "engramux.exe"), []byte(stub), 0o600); err != nil {
		t.Fatalf("write the stub: %v", err)
	}

	out, err := runShim(t, bash, shim, local, "doctor", "--full")
	if err != nil {
		t.Fatalf("the shim failed with an installed copy present: %v\n%s", err, out)
	}
	// Exact text and exact arguments: a shim that found the stub but dropped
	// what it was asked to pass on is a shim nobody can use.
	if want := "STUB-INSTALLED-COPY doctor --full"; strings.TrimSpace(out) != want {
		t.Errorf("shim output = %q, want %q", strings.TrimSpace(out), want)
	}
}

func TestTheShimSaysWhatToRunWhenNothingIsInstalled(t *testing.T) {
	bash := lookBash(t)
	shim := shimPath(t)

	// An empty LOCALAPPDATA: the plugin is enabled and the product is not
	// installed, which is every user's state between `/plugin install` and
	// their first `install --apply`.
	out, err := runShim(t, bash, shim, t.TempDir(), "doctor")
	if err == nil {
		t.Fatalf("the shim succeeded with nothing installed; output was:\n%s", out)
	}
	if code := exitCode(err); code == 0 {
		t.Errorf("exit code = %d, want non-zero", code)
	}
	// The whole point of this arm. `install --apply` is what the reader has
	// to be told, and the shim has to name the copy they can run it from.
	for _, want := range []string{"install --apply", "engramux.exe"} {
		if !strings.Contains(out, want) {
			t.Errorf("the message does not mention %q; it said:\n%s", want, out)
		}
	}
}

// runShim invokes the shim through bash with LOCALAPPDATA pointed at local.
//
// USERPROFILE is cleared as well as LOCALAPPDATA being set, so that a shim
// falling back to the profile directory reaches the test's own tree rather than
// the real one - otherwise the missing-install arm would find the machine's
// actual installation and pass for the wrong reason.
func runShim(t *testing.T, bash, shim, local string, args ...string) (string, error) {
	t.Helper()
	// The test's own context, so a shim that hangs is killed with the test
	// rather than outliving it - this one execs another process, which is the
	// case where an orphan is easy to leave behind.
	//
	// #nosec G204 -- bash came from exec.LookPath, shim is this repository's own
	// committed file resolved against the test's directory, and args are this
	// file's own literals. There is no caller-supplied input in this command.
	cmd := exec.CommandContext(t.Context(), bash, append([]string{shim}, args...)...)
	cmd.Env = append(os.Environ(),
		"LOCALAPPDATA="+local,
		"USERPROFILE="+local,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func lookBash(t *testing.T) string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("no bash on PATH, and the shim is a bash script: %v", err)
	}
	return bash
}

// shimPath answers the shim's path, and fails rather than skips when it is
// absent: the file is committed, so a missing one is a deletion and not an
// unmet prerequisite.
func shimPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "bin", "engramux"))
	if err != nil {
		t.Fatalf("resolve the shim path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("bin/engramux is not there: %v", err)
	}
	return path
}

func exitCode(err error) int {
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}
