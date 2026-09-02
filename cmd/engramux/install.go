package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/schedule"
)

// install copies the binaries, writes both hosts' hook configuration,
// registers the logon task, starts the service and registers the MCP endpoint.
//
// It replaces a Node script, and the Node is the point: the product's whole
// argument on Windows is two statically linked binaries and no runtime, and
// installation was the one place a runtime survived.
//
// Nothing is written without --apply.
func install(args []string) int {
	apply := slices.Contains(args, "--apply")
	remove := slices.Contains(args, "--remove")

	opt, err := installOptions(apply, args)
	if err != nil {
		warn("install: %v", err)
		return 1
	}

	opt.Remove = remove

	report, err := host.Install(context.Background(), opt, realSystem())
	for _, line := range report {
		fmt.Println(line)
	}
	if err != nil {
		// A locked destination prints its own diagnosis, which is several
		// lines of advice rather than a sentence, so it goes out whole.
		var locked *host.LockedError
		if errors.As(err, &locked) {
			warn("%s", locked.Error())
			return 1
		}
		warn("install: %v", err)
		return 1
	}
	return 0
}

// installOptions resolves every path an installation touches.
//
// It is one function rather than defaults scattered through [host.Install],
// because the resolution is the part that reads the environment and the part a
// reader wants in one place. host.Options carries no defaults of its own for
// the same reason.
//
// # Where the binaries come from
//
// The directory this binary is in, not a `dist/` under a repository root. A
// released Engramux is two executables in a directory the user unpacked; a
// developer's is two executables in `dist/`. Reading the running binary's own
// directory is the same answer for both, and it is the only one that does not
// assume a source tree.
func installOptions(apply bool, args []string) (host.Options, error) {
	self, err := os.Executable()
	if err != nil {
		return host.Options{}, fmt.Errorf("locate this binary: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return host.Options{}, fmt.Errorf("locate the home directory: %w", err)
	}
	return resolveOptions(self, os.Getenv("LOCALAPPDATA"), home, apply, args)
}

// resolveOptions is [installOptions] with everything it reads from the process
// passed in, which is what makes the decisions below testable at all - the
// alternative is a guard about the running binary's own location that nothing
// can ever exercise.
func resolveOptions(self, local, home string, apply bool, args []string) (host.Options, error) {
	opt, err := resolvePaths(local, home, args)
	if err != nil {
		return host.Options{}, err
	}
	opt.SourceDir = filepath.Dir(self)
	opt.Apply = apply

	// Running the installed copy would ask it to overwrite itself, which
	// Windows refuses for a mapped image anyway - but it refuses it as a
	// sharing violation on a destination, several steps in, which reads like
	// the service is running. Saying it here is the difference between a
	// diagnosis and a puzzle.
	//
	// It is here and not in [resolvePaths] because it is a rule about
	// copying, and `doctor` does not copy: run from the installed directory
	// is where `doctor` is most usefully run, not where it is refused.
	if filepath.Clean(opt.SourceDir) == filepath.Clean(opt.BinDir) {
		return host.Options{}, fmt.Errorf("this is the installed copy in %s; run the one you unpacked or built", opt.BinDir)
	}
	return opt, nil
}

// resolvePaths is every path an installation touches except the directory it
// copies FROM, which is the only one that depends on where the running binary
// happens to be.
//
// It is separate so that `doctor` reads the same answers from the same code.
// M-6 asks `doctor` to check the eleven hook entries against the installed
// relay, and a `doctor` that derived that path on its own would be checking
// them against a file `install` might not have written - two spellings of one
// definition, drifting the first time either moves.
func resolvePaths(local, home string, args []string) (host.Options, error) {
	if local == "" {
		return host.Options{}, errors.New("LOCALAPPDATA is not set, so the data directory cannot be located")
	}
	data := filepath.Join(local, "engramux")

	return host.Options{
		BinDir:      filepath.Join(data, "bin"),
		ClaudePath:  envOr("ENGRAMUX_CLAUDE_SETTINGS", filepath.Join(home, ".claude", "settings.json")),
		CodexHooks:  envOr("ENGRAMUX_CODEX_HOOKS", filepath.Join(home, ".codex", "hooks.json")),
		CodexConfig: envOr("ENGRAMUX_CODEX_CONFIG", filepath.Join(home, ".codex", "config.toml")),
		ClaudeMCP:   envOr("ENGRAMUX_CLAUDE_MCP", filepath.Join(home, ".claude.json")),
		MCPJSON:     filepath.Join(data, "mcp.json"),
		TaskName:    taskName(withoutFlags(args)),
	}, nil
}

// currentPaths is [resolvePaths] reading this process's own environment. It is
// what `doctor` calls; `install` goes through [installOptions], which needs the
// running binary's directory as well.
func currentPaths(args []string) (host.Options, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return host.Options{}, fmt.Errorf("locate the home directory: %w", err)
	}
	return resolvePaths(os.Getenv("LOCALAPPDATA"), home, args)
}

// envOr is the override seam the tests use. The three host files are the only
// paths that can move, because they are the only ones this product does not
// own - and a test that wrote a developer's real settings.json would be a test
// nobody could run twice.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// withoutFlags drops anything starting with a dash, so the optional task-name
// positional can be given alongside --apply in either order.
func withoutFlags(args []string) []string {
	var out []string
	for _, a := range args {
		if len(a) > 0 && a[0] != '-' {
			out = append(out, a)
		}
	}
	return out
}

// realSystem is the three effects [host.Install] does not perform itself.
//
// They live here rather than in internal/host so that scheduling does not
// become a dependency of every package that imports host - internal/store does,
// for host.Detect, and the service has no business linking Task Scheduler code.
func realSystem() host.System {
	return host.System{
		RegisterTask:   schedule.Register,
		UnregisterTask: schedule.Unregister,
		StartService:   schedule.Run,
		RegisterClaude: func(ctx context.Context, ep *host.Endpoint) error {
			bin, err := host.ClaudeCLI()
			if err != nil {
				return err
			}
			return host.RegisterClaudeMCP(ctx, bin, ep)
		},
	}
}
