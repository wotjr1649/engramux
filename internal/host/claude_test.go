package host

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The environment variables that turn a re-executed copy of this test binary
// into the `claude` this package shells out to. Re-executing os.Args[0] rather
// than building a second program keeps the stub on the same toolchain and costs
// no build, and it is the shape internal/spool's TestMain already uses.
const (
	stubArgsEnv   = "ENGRAMUX_CLAUDE_STUB_ARGS"
	stubExitEnv   = "ENGRAMUX_CLAUDE_STUB_EXIT"
	stubStderrEnv = "ENGRAMUX_CLAUDE_STUB_STDERR"
)

func TestMain(m *testing.M) {
	if path := os.Getenv(stubArgsEnv); path != "" {
		claudeStub(path)
		return
	}
	os.Exit(m.Run())
}

// claudeStub writes the arguments it was given, one per line, and exits with
// the code it was told to. It is what lets a test assert the exact command line
// without a real Claude Code on the machine.
func claudeStub(path string) {
	if msg := os.Getenv(stubStderrEnv); msg != "" {
		_, _ = os.Stderr.WriteString(msg)
	}
	//nolint:gosec // G703: path is the temp file the parent test created and named in the environment.
	_ = os.WriteFile(path, []byte(strings.Join(os.Args[1:], "\n")), 0o600)
	code := 0
	if os.Getenv(stubExitEnv) == "1" {
		code = 1
	}
	os.Exit(code)
}

// useStub points the shell-out at this test binary and returns the file the
// arguments will land in.
func useStub(t *testing.T) string {
	t.Helper()
	argsFile := filepath.Join(t.TempDir(), "argv")
	t.Setenv(stubArgsEnv, argsFile)
	return argsFile
}

func stubArgs(t *testing.T, path string) []string {
	t.Helper()
	//nolint:gosec // G304: a path this test built under t.TempDir.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the stub was never run: %v", err)
	}
	return strings.Split(string(b), "\n")
}

// TestRegisterClaudeMCPPassesTheDocumentedArguments pins the command line,
// because it is the whole of the registration and nothing else observes it. A
// wrong flag here means MCP quietly does not work on one host.
func TestRegisterClaudeMCPPassesTheDocumentedArguments(t *testing.T) {
	argsFile := useStub(t)
	if err := RegisterClaudeMCP(t.Context(), os.Args[0], probeEndpoint()); err != nil {
		t.Fatalf("RegisterClaudeMCP: %v", err)
	}

	want := []string{
		"mcp", "add", "--scope", "user", "--transport", "http",
		MCPName, probeURL,
		"--header", "Authorization: Bearer " + probeToken,
	}
	got := stubArgs(t, argsFile)
	if len(got) != len(want) {
		t.Fatalf("argv has %d elements, want %d:\n got %q\nwant %q", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// The header is one argument and not three. On Windows an argument is
	// quoted into a single command line by the parent, so splitting it here
	// would send Codex's Authorization value as separate arguments.
	if n := strings.Count(strings.Join(got, "\n"), "Authorization"); n != 1 {
		t.Errorf("the Authorization value is not one argument: %q", got)
	}
}

// TestUnregisterClaudeMCPPassesTheDocumentedArguments is the remove half,
// spelled by passing no endpoint so the two cannot drift apart.
func TestUnregisterClaudeMCPPassesTheDocumentedArguments(t *testing.T) {
	argsFile := useStub(t)
	if err := RegisterClaudeMCP(t.Context(), os.Args[0], nil); err != nil {
		t.Fatalf("RegisterClaudeMCP(nil): %v", err)
	}
	want := strings.Join([]string{"mcp", "remove", "--scope", "user", MCPName}, "\n")
	if got := strings.Join(stubArgs(t, argsFile), "\n"); got != want {
		t.Errorf("argv =\n%s\nwant\n%s", got, want)
	}
}

// TestAFailingClaudeDoesNotLeakTheToken is the reason this shell-out is written
// by hand rather than with the obvious CombinedOutput.
//
// The command line carries the bearer token (spec 6.1), so an error that
// includes it - which is what a wrapped exec error or a surfaced stderr would
// do - would put a credential into whatever read the installer's output. The
// stub is told to fail AND to print the token on stderr, which is the worst
// case: a `claude` that echoes back what it was given.
func TestAFailingClaudeDoesNotLeakTheToken(t *testing.T) {
	useStub(t)
	t.Setenv(stubExitEnv, "1")
	t.Setenv(stubStderrEnv, "failed to add server with header Authorization: Bearer "+probeToken+"\n")

	err := RegisterClaudeMCP(t.Context(), os.Args[0], probeEndpoint())
	if err == nil {
		t.Fatal("a failing claude was reported as success")
	}
	msg := err.Error()
	if strings.Contains(msg, probeToken) {
		t.Errorf("the error carries the bearer token: %v", err)
	}
	if strings.Contains(msg, "Authorization") {
		t.Errorf("the error carries the header the token rides in: %v", err)
	}
	// It still has to be actionable: the exit status, and where to look.
	if !strings.Contains(msg, "claude mcp list") {
		t.Errorf("the error does not say how to find out what happened: %v", err)
	}
}

// TestClaudeCLIWantsAnExe holds what the probe measured: Go runs a .cmd shim
// through cmd.exe, whose argument quoting is not CreateProcess's, and an
// argument carrying a quote fails outright. The Authorization value is a
// credential, so a shim is refused rather than risked.
func TestClaudeCLIWantsAnExe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	if _, err := ClaudeCLI(); !errors.Is(err, ErrClaudeNotFound) {
		t.Errorf("an empty PATH gave %v, want ErrClaudeNotFound", err)
	}

	// A shim and nothing else: found, and refused for a reason the caller can
	// tell from "not installed".
	shim := filepath.Join(dir, "claude.cmd")
	seedRaw(t, shim, "@echo off\r\n")
	_, err := ClaudeCLI()
	if !errors.Is(err, ErrClaudeNotSpawnable) {
		t.Fatalf("a .cmd shim gave %v, want ErrClaudeNotSpawnable", err)
	}
	if !strings.Contains(err.Error(), "claude mcp add") {
		t.Errorf("the error does not tell the user how to register by hand: %v", err)
	}
	// The advice names the placeholder rather than a value. Asserting the
	// absence of probeToken here would prove nothing - no token is in scope on
	// this path - so what is asserted is that the placeholder is what is
	// printed. A review pointed out the difference.
	if !strings.Contains(err.Error(), "<token>") {
		t.Errorf("the advice does not tell the user where the token comes from: %v", err)
	}

	// An .exe beside it wins, so a machine with both is not refused.
	exe := filepath.Join(dir, "claude.exe")
	seedRaw(t, exe, "not a real program")
	got, err := ClaudeCLI()
	if err != nil {
		t.Fatalf("ClaudeCLI with an exe present: %v", err)
	}
	if got != exe {
		t.Errorf("ClaudeCLI = %q, want %q", got, exe)
	}
}

// TestClaudeCLISearchesEveryPathEntry covers the case exec.LookPath gets wrong
// for this purpose: a shim earlier on PATH than the executable. LookPath
// returns the first match by PATHEXT order and would answer the shim.
func TestClaudeCLISearchesEveryPathEntry(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	seedRaw(t, filepath.Join(first, "claude.cmd"), "@echo off\r\n")
	exe := filepath.Join(second, "claude.exe")
	seedRaw(t, exe, "not a real program")
	t.Setenv("PATH", first+string(os.PathListSeparator)+second)

	got, err := ClaudeCLI()
	if err != nil {
		t.Fatalf("ClaudeCLI: %v", err)
	}
	if got != exe {
		t.Errorf("ClaudeCLI = %q, want %q - the shim on the earlier entry is not the answer", got, exe)
	}
}
