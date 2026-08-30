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
// A binary that cannot be copied, or a host configuration that cannot be read,
// fails before anything is written - there is no half-install to recover from.
// Everything after the hooks are in place is reported and survived: capture
// works without MCP, so a service that does not publish, or a Claude Code that
// is not installed, leaves a working capture and a report saying what is
// missing.
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
	copies, unchanged, err := PlanCopies(opt.SourceDir, opt.BinDir, opt.Binaries, opt.Apply)
	if err != nil {
		return nil, err
	}
	relay := filepath.Join(opt.BinDir, RelayName)

	claudePlan, err := PlanMerge(opt.ClaudePath, "claude-code", EventNames(),
		func(event string) jsontext.Value { return ClaudeEntry(event, relay) })
	if err != nil {
		return nil, err
	}
	codexPlan, err := PlanMerge(opt.CodexHooks, "codex", EventNames(),
		func(event string) jsontext.Value { return CodexEntry(event, relay) })
	if err != nil {
		return nil, err
	}

	for _, path := range unchanged {
		say("unchanged %s - identical bytes, not copied", path)
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
		say("would register the logon task %s and start the service", opt.TaskName)
		say("")
		say("nothing was written. re-run with --apply.")
		return report, nil
	}

	for _, c := range copies {
		if err := copyFile(c.Src, c.Dest); err != nil {
			return nil, err
		}
		say("copied %s", c.Dest)
	}
	if err := Commit([]*Plan{claudePlan, codexPlan}); err != nil {
		return nil, err
	}
	for _, p := range []*Plan{claudePlan, codexPlan} {
		if p == nil {
			continue
		}
		say("%s: installed %d events in %s", p.Label, len(EventNames()), p.Path)
	}

	// From here on nothing fails the run. The hooks are in place and capture
	// works without any of it.
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

	codexText, err := readOrEmpty(opt.CodexConfig)
	switch {
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
			if err := Commit([]*Plan{{Path: opt.CodexConfig, Label: "codex mcp", Text: []byte(spliced)}}); err != nil {
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

// readOrEmpty reads a file, answering an empty string for one that is not
// there. A Codex configuration that does not exist yet is an ordinary state.
func readOrEmpty(path string) (string, error) {
	//nolint:gosec // G304: a caller-computed path inside the trust boundary; see the note in write.go.
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
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
