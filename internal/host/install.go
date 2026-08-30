package host

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// The two binaries, by the names they carry in both directories.
const (
	RelayName   = "engramux.exe"
	ServiceName = "engramux-service.exe"
)

// endpointWait is how long Install waits for the service to publish mcp.json
// after starting it.
//
// It is a bound and not a budget: the service writes that file early in its
// start, so on a machine where anything is going to happen it is there almost
// at once. What this covers is the case where nothing is going to happen -
// a service that failed to bind, or a start that did not take - and there the
// only useful behaviour is to stop waiting and say so.
const endpointWait = 10 * time.Second

// endpointPoll is how often the wait looks. Short enough that the common case
// costs one interval and no more.
const endpointPoll = 100 * time.Millisecond

// Options is everything an installation needs to know. Every path is explicit,
// with no reading of the environment inside, so a test can put a whole
// installation inside one temporary directory and a caller has one place to
// look for what it decided.
type Options struct {
	SourceDir   string
	BinDir      string
	ClaudePath  string
	CodexHooks  string
	CodexConfig string
	MCPJSON     string
	TaskName    string
	// Binaries defaults to the relay and the service. It is a field so that a
	// test can name something it can actually lock.
	Binaries []Binary
	// Apply is false for a dry run, which reports and writes nothing.
	Apply bool
	// Remove undoes an installation: the hooks come out of both hosts, the MCP
	// registration out of both, and the logon task off the machine. The
	// binaries are left where they are, because removing the relay while a
	// host still holds a stale hook entry is the one order that produces an
	// error at every event instead of none.
	//
	// It is a field on the same call rather than a second entry point, for the
	// reason every layer below it already spells removal as "no entry to
	// write": one path means install and remove cannot drift.
	Remove bool
	// EndpointWait overrides how long to wait for the service to publish
	// mcp.json. Zero means [endpointWait]. It is a field so that the test for
	// "nothing was ever published" does not have to sit out the whole bound -
	// a ten-second test in a package that otherwise runs in one is a cost paid
	// on every run for a case that is about giving up.
	EndpointWait time.Duration
}

// System is the part of an installation that touches the machine: the logon
// task, the service, and Claude Code's own CLI. A test supplies its own; the
// command that wires this up supplies the real ones, and it lives outside this
// package so that scheduling does not become a dependency of everything that
// imports host.
type System struct {
	RegisterTask   func(ctx context.Context, name, exe string) error
	UnregisterTask func(ctx context.Context, name string) error
	StartService   func(ctx context.Context, name string) error
	RegisterClaude func(ctx context.Context, ep *Endpoint) error
}

// Install copies the binaries, writes both hosts' hook configuration,
// registers the logon task, starts the service, and registers the MCP endpoint
// with both hosts - in one run.
//
// # One pass, which the installer this replaces could not do
//
// mcp.json is written by the service (spec 5.9), so on a first install there is
// no endpoint to register until the service has run. The previous installer
// stopped there and asked the user to start the service and run the whole thing
// again, and it never said so in its closing message; it also never mentioned
// the logon task, so a user who followed it end to end had a capture that
// stopped working at the next reboot. Starting the service here, through the
// task that was just registered, is what removes both.
//
// # What fails the run and what does not
//
// A host configuration that cannot be READ fails before anything is written,
// and a destination that already exists and cannot be written is refused before
// the first copy. Neither of those is the same as "a copy cannot fail
// half-way": copies happen one at a time, so a destination that passes the
// probe and then fails - a scanner taking the file, a disk filling - leaves the
// binaries before it replaced. That is why the report comes back with the error
// and names where it stopped, rather than the caller being told nothing
// happened. An earlier version of this comment claimed the stronger thing and
// was wrong.
// Everything after the hooks are in place is reported and survived: capture
// works without MCP, so a service that does not publish, or a Claude Code that
// is not installed, leaves a working capture and a report saying what is
// missing.
// entryFor answers the hook-entry builder for this run, or nil when the run is
// a removal - which is how every layer below spells removal, so that one path
// serves both.
func entryFor(opt Options, relay string, build func(string, string) jsontext.Value) func(string) jsontext.Value {
	if opt.Remove {
		return nil
	}
	return func(event string) jsontext.Value { return build(event, relay) }
}

func Install(ctx context.Context, opt Options, sys System) ([]string, error) {
	if len(opt.Binaries) == 0 {
		opt.Binaries = []Binary{
			{Name: RelayName, Role: Relay},
			{Name: ServiceName, Role: Service},
		}
	}
	var report []string
	say := func(format string, args ...any) { report = append(report, fmt.Sprintf(format, args...)) }

	// Everything that can be decided by reading is decided first, so that a
	// failure to read one file cannot leave another already written.
	copies, unchanged, err := PlanCopies(opt.SourceDir, opt.BinDir, opt.Binaries, opt.Apply && !opt.Remove)
	if err != nil {
		return report, err
	}
	for _, path := range unchanged {
		// Reported here rather than after the host files are planned: what is
		// known is said when it is known, so a later failure still comes back
		// with everything already established.
		say("unchanged %s - identical bytes, not copied", path)
	}
	relay := filepath.Join(opt.BinDir, RelayName)

	claudePlan, err := PlanMerge(opt.ClaudePath, "claude-code", EventNames(),
		entryFor(opt, relay, ClaudeEntry))
	if err != nil {
		return report, err
	}
	codexPlan, err := PlanMerge(opt.CodexHooks, "codex", EventNames(),
		entryFor(opt, relay, CodexEntry))
	if err != nil {
		return report, err
	}

	if !opt.Apply {
		for _, c := range copies {
			say("would copy %s", c.Dest)
		}
		for _, p := range []*Plan{claudePlan, codexPlan} {
			if p != nil {
				say("%s: would install %d events in %s", p.Label, len(EventNames()), p.Path)
			}
		}
		if opt.Remove {
			say("would unregister the logon task %s and take the MCP registration out of both hosts", opt.TaskName)
		} else {
			say("would register the logon task %s and start the service", opt.TaskName)
		}
		say("")
		say("nothing was written. re-run with --apply.")
		return report, nil
	}

	for _, c := range copies {
		if err := copyFile(c.Src, c.Dest); err != nil {
			// The report goes back with the error, and this line is why.
			// Copies happen one at a time, so a failure here can leave the
			// first binary replaced and the second not - and the caller has
			// to be able to say which. The probe in PlanCopies is what makes
			// that rare; it is not what makes it impossible.
			say("copy FAILED at %s - the ones above it were replaced and the ones below were not", c.Dest)
			return report, err
		}
		say("copied %s -> %s", c.Src, c.Dest)
	}
	saved, err := Commit([]*Plan{claudePlan, codexPlan})
	for _, path := range saved {
		say("backup %s", path)
	}
	if err != nil {
		return report, err
	}
	for _, p := range []*Plan{claudePlan, codexPlan} {
		if p == nil {
			continue
		}
		say("%s: installed %d events in %s", p.Label, len(EventNames()), p.Path)
	}

	// From here on nothing fails the run. The hooks are in place and capture
	// works without any of it.
	if opt.Remove {
		if err := sys.UnregisterTask(ctx, opt.TaskName); err != nil {
			say("logon task: could not remove %s (%v)", opt.TaskName, err)
		} else {
			say("logon task %s removed", opt.TaskName)
		}
		removeMCP(ctx, opt, sys, say)
		say("")
		say("removed. the binaries are still in %s; the service is still running until you stop it.", opt.BinDir)
		return report, nil
	}

	service := filepath.Join(opt.BinDir, ServiceName)
	if err := sys.RegisterTask(ctx, opt.TaskName, service); err != nil {
		say("logon task: FAILED (%v) - capture will not survive a reboot until this is fixed", err)
		return report, nil
	}
	say("logon task %s registered against %s", opt.TaskName, service)

	if err := sys.StartService(ctx, opt.TaskName); err != nil {
		say("service: could not start it (%v) - start it yourself and re-run to finish MCP", err)
		return report, nil
	}
	say("service started through the logon task")

	wait := opt.EndpointWait
	if wait == 0 {
		wait = endpointWait
	}
	ep, err := waitForEndpoint(ctx, opt.MCPJSON, wait)
	if err != nil {
		say("mcp: no endpoint in %s (%v) - capture works; re-run to finish MCP once the service publishes",
			opt.MCPJSON, err)
		return report, nil
	}

	codexText, exists, err := readOrEmpty(opt.CodexConfig)
	switch {
	case !exists:
		// A user with only Claude Code installed is an ordinary user, and the
		// installer this replaces skipped this file by name. Writing one would
		// leave a Codex configuration its owner never made.
		say("codex mcp: %s does not exist - skipped", opt.CodexConfig)
	case err != nil:
		say("codex mcp: cannot read %s (%v) - skipped", opt.CodexConfig, err)
	default:
		spliced, err := SpliceCodex(codexText, ep)
		switch {
		case err != nil:
			say("codex mcp: %v - skipped", err)
		case spliced == codexText:
			say("codex mcp: already up to date")
		default:
			saved, err := Commit([]*Plan{{Path: opt.CodexConfig, Label: "codex mcp", Text: []byte(spliced)}})
			for _, path := range saved {
				// Named because this one is a copy of a file that may already
				// have held the token.
				say("backup %s", path)
			}
			if err != nil {
				say("codex mcp: FAILED (%v)", err)
			} else {
				say("codex mcp: installed [mcp_servers.%s]", MCPName)
			}
		}
	}

	if err := sys.RegisterClaude(ctx, ep); err != nil {
		say("claude-code mcp: %v", err)
	} else {
		say("claude-code mcp: registered %s at user scope", MCPName)
	}

	say("")
	say("done. check it with: %s doctor", relay)
	return report, nil
}

// removeMCP takes the endpoint out of both hosts. It needs no endpoint to do
// it, which is the point: a machine whose service never published still has a
// stale registration to remove.
func removeMCP(ctx context.Context, opt Options, sys System, say func(string, ...any)) {
	if text, exists, err := readOrEmpty(opt.CodexConfig); !exists {
		say("codex mcp: %s does not exist - skipped", opt.CodexConfig)
	} else if err != nil {
		say("codex mcp: cannot read %s (%v) - skipped", opt.CodexConfig, err)
	} else if spliced, err := SpliceCodex(text, nil); err != nil {
		say("codex mcp: %v - skipped", err)
	} else if spliced == text {
		say("codex mcp: nothing registered")
	} else {
		saved, err := Commit([]*Plan{{Path: opt.CodexConfig, Label: "codex mcp", Text: []byte(spliced)}})
		for _, path := range saved {
			say("backup %s", path)
		}
		if err != nil {
			say("codex mcp: FAILED (%v)", err)
		} else {
			say("codex mcp: removed [mcp_servers.%s]", MCPName)
		}
	}
	if err := sys.RegisterClaude(ctx, nil); err != nil {
		say("claude-code mcp: %v", err)
	} else {
		say("claude-code mcp: removed %s", MCPName)
	}
}

// waitForEndpoint polls until the service publishes mcp.json or the bound runs
// out. It reads that file and never writes it (spec 5.9).
func waitForEndpoint(ctx context.Context, path string, wait time.Duration) (*Endpoint, error) {
	deadline := time.Now().Add(wait)
	for {
		ep, err := ReadEndpoint(path)
		if err == nil {
			return ep, nil
		}
		if !errors.Is(err, ErrEndpointNotPublished) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("nothing published within %s", wait)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(endpointPoll):
		}
	}
}

// readOrEmpty reads a file and says whether it was there.
//
// The second result is the whole point and it was missing: without it a Codex
// configuration that does not exist read as an empty one, the splice produced a
// table, and the write failed at the backup with a FAILED line on every install
// of a machine that has no Codex. Measured before this was fixed.
func readOrEmpty(path string) (string, bool, error) {
	//nolint:gosec // G304: a caller-computed path inside the trust boundary; see the note in write.go.
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

// copyFile writes src over dest through a temporary file and a rename, the same
// way a host configuration is written and for the same reason: a truncated
// binary is worse than an old one.
func copyFile(src, dest string) error {
	//nolint:gosec // G304: a path built from the caller's own directories.
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("host: open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	body, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("host: read %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return fmt.Errorf("host: create %s: %w", filepath.Dir(dest), err)
	}
	return writeAtomic(dest, body)
}
