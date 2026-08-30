package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Why Claude Code's own CLI writes this registration, and this code does not
// touch ~/.claude.json:
//
// That file is Claude Code's live state - per-project history alongside the MCP
// entries - and a running Claude Code rewrites it on its own schedule. A
// read-modify-write from here is a lost update against whatever it wrote in
// between. `claude mcp add` is that product's own supported write and the same
// route a person would use.
var (
	// ErrClaudeNotFound means there is no claude on PATH at all, which is an
	// ordinary state: a user with only Codex installed is an ordinary user.
	ErrClaudeNotFound = errors.New("host: no claude executable on PATH")

	// ErrClaudeNotSpawnable means a claude was found and cannot be used, which
	// is a different thing and gets different advice.
	ErrClaudeNotSpawnable = errors.New("host: the claude on PATH cannot be given a credential safely")
)

// claudeExe and claudeShims are the names searched for, in that order of
// preference.
const claudeExe = "claude.exe"

var claudeShims = []string{"claude.cmd", "claude.bat", "claude"}

// ClaudeCLI finds the claude executable this registration can be handed to.
//
// # Why not exec.LookPath, and why only an .exe
//
// **[verified] on this platform.** A .cmd shim - which is what an npm global
// install of Claude Code leaves - does run through Go's os/exec, because
// CreateProcess hands a .cmd to cmd.exe. What does not survive is the argument
// quoting: an argument carrying a quote and a space came back as `exit status
// 255` and a cmd.exe syntax error, because the quoting rules applied are cmd's
// and not CreateProcess's. The argument this registration passes is
// `Authorization: Bearer <token>` - a credential, on a command line, next to a
// quoting rule that can reinterpret the rest of it. That is refused rather than
// risked.
//
// exec.LookPath is not enough on its own for the same reason: it returns the
// first match in PATHEXT order, which is the shim, even when an .exe sits
// further along PATH. So PATH is walked, preferring the executable everywhere
// over a shim anywhere.
func ClaudeCLI() (string, error) {
	dirs := filepath.SplitList(os.Getenv("PATH"))

	var shim string
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, claudeExe)
		// gosec reads PATH as taint and calls this traversal. Searching PATH
		// for a program named on it is what PATH is: whoever can set it
		// already decides what runs, and nothing here opens the file - it is
		// a Stat, and the result is handed to exec, which does its own
		// resolution.
		//nolint:gosec // G703: see above.
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		if shim != "" {
			continue
		}
		for _, name := range claudeShims {
			//nolint:gosec // G703: see above.
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
				shim = filepath.Join(dir, name)
				break
			}
		}
	}

	if shim == "" {
		return "", ErrClaudeNotFound
	}
	// Actionable, and carrying no token: the value is in mcp.json, which is
	// the user's own file, and naming it is what lets them finish by hand.
	return "", fmt.Errorf("%w: %s is a shim, and an argument carrying a credential does not "+
		"survive cmd.exe quoting. register it yourself with the url and token from mcp.json:\n"+
		"  claude mcp add --scope user --transport http %s <url> --header \"Authorization: Bearer <token>\"",
		ErrClaudeNotSpawnable, shim, MCPName)
}

// RegisterClaudeMCP runs Claude Code's own CLI to add the endpoint, or to
// remove it when ep is nil - so install and remove are one path.
//
// # Nothing about a failure may be repeated
//
// The command line carries the bearer token (spec 6.1). So the child's output
// is read and **discarded**: a `claude` that echoes back what it was given
// would otherwise put a credential into whatever reads the installer's report.
// exec's own error carries the program name and not its arguments, which is why
// it can be wrapped, and cmd.String() is never called anywhere here.
//
// What is left is the exit status and where to look, which is what a person
// actually needs.
func RegisterClaudeMCP(ctx context.Context, bin string, ep *Endpoint) error {
	var args []string
	if ep == nil {
		args = []string{"mcp", "remove", "--scope", "user", MCPName}
	} else {
		if !safeValue(ep.URL) || !safeValue(ep.Token) {
			return fmt.Errorf("host: the endpoint's url or token will not be passed to claude")
		}
		args = []string{
			"mcp", "add", "--scope", "user", "--transport", "http",
			MCPName, ep.URL,
			"--header", "Authorization: Bearer " + ep.Token,
		}
	}

	//nolint:gosec // G204: bin is what ClaudeCLI resolved from PATH, and every
	// argument is either a literal above or a value safeValue has passed.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return fmt.Errorf("host: claude exited %d while registering %s; "+
				"run `claude mcp list` to see what it has", exit.ExitCode(), MCPName)
		}
		// Not an exit status: the program could not be started. exec.Error
		// carries the name and not the arguments, so it is safe to name it.
		return fmt.Errorf("host: could not run %s: %w", filepath.Base(bin), stripArgs(err))
	}
	return nil
}

// stripArgs keeps an error from carrying anything but the program name.
//
// exec.Error already holds only the name, and exec.ExitError holds only the
// status, so this is a guard against a future error type rather than against
// either of those - and it is cheap enough to be worth having where a token is
// one field away.
func stripArgs(err error) error {
	var e *exec.Error
	if errors.As(err, &e) {
		return fmt.Errorf("%s: %w", filepath.Base(e.Name), e.Err)
	}
	if strings.Contains(err.Error(), "Bearer ") {
		return errors.New("the error carried the command line and was withheld")
	}
	return err
}
