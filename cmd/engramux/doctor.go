package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/mcpconf"
	"github.com/wotjr1649/engramux/internal/schedule"
	"github.com/wotjr1649/engramux/internal/spool"
)

// taskBudget bounds one schtasks invocation. It is not the pipe's budget: this
// is a local Windows command with no service on the other end, and the only
// reason it is bounded at all is that a wedged schtasks would otherwise hang a
// command a person typed.
const taskBudget = 30 * time.Second

// serviceName is the file engramux-service is built as. `register` looks for it
// beside this binary, because that is how the two ship (spec 5.1).
const serviceName = "engramux-service.exe"

// doctor reports everything knowable about this installation, and it is built
// around the moment it is most needed being the moment it is most broken - which
// is spec 10's first open question, answered here and recorded in spec 5.5.
//
// Three halves with different availability:
//
//   - The task registration is a Windows query. It needs no service, no pipe and
//     no database, so it is readable exactly when Windows is running.
//   - The local state - where the two binaries are, how deep the spool is, what
//     the log last said - is files on disk. Also readable with nothing running.
//   - The service's own numbers, the real database path and the tokenizer
//     comparison are only reachable over the pipe: I-07 leaves no other way to
//     read them and I-08 routes them here.
//
// So a service that is down does not make this command useless. Every half is
// read and printed, the unreachable one is marked rather than omitted, and the
// exit code is 1 if any of them failed. A version that returned at the first
// failure would print nothing about the registration whenever the service was
// down - which is exactly the moment somebody runs this.
//
// Exit 1 on any failure, as spec 10 requires. There is no partial success: a
// machine with no registration is a machine where nothing starts the service at
// the next logon, and that is a finding, not a footnote.
func doctor(args []string) int {
	name := taskName(args)

	ctx, cancel := context.WithTimeout(context.Background(), taskBudget)
	defer cancel()

	// Stdout, because this is the CLI path and a report is what was asked
	// for. Every line below is a finding, including the ones that say
	// something could not be read.
	ok := reportTask(ctx, name)
	if !reportLocal() {
		ok = false
	}
	if !reportService() {
		ok = false
	}
	if !reportMCP(ctx) {
		ok = false
	}
	if !ok {
		return 1
	}
	return 0
}

// reportLocal prints what is knowable with nothing running, and reports whether
// all of it could be read.
//
// It is the half added because `doctor` is run when the service is down. The
// spool depth is a directory listing and the last log line is a file read;
// neither needs the pipe, and between them they answer "is the relay still
// capturing, and what did the service say before it stopped".
func reportLocal() bool {
	_, _ = fmt.Fprintln(os.Stdout, "local")

	ok := true
	self, err := os.Executable()
	if err != nil {
		field("this binary", "unreadable: "+err.Error())
		ok = false
	} else {
		field("this binary", self)
	}
	if exe, err := serviceExe(); err != nil {
		// Not fatal to the command and still a finding: a registration
		// pointing at a binary that is not there is a task that fails
		// silently at every logon.
		field("service binary", "MISSING - "+err.Error())
		ok = false
	} else {
		field("service binary", exe)
	}

	// Derived from the spool's own directory rather than from
	// internal/service's Dir, and that is not a preference. This binary is
	// the hook relay as well as the CLI (spec 5.1), and internal/service
	// imports internal/store, which links the SQLite driver: importing it
	// here put 4 MiB of database engine into a process that is spawned once
	// per hook event and never opens a database (I-07). internal/spool is
	// already on the relay path, and a test pins its derivation against the
	// service's - a CLI reading a different directory from the one the relay
	// writes to would report a spool that is not the spool.
	spoolPath, err := spool.Dir()
	if err != nil {
		field("data directory", "unreadable: "+err.Error())
		return false
	}
	dir := filepath.Dir(spoolPath)
	field("data directory", dir)

	if depth, err := spool.Depth(spoolPath); err != nil {
		field("spool", "unreadable: "+err.Error())
		ok = false
	} else {
		// A depth that is not zero with the service down is the count of
		// events waiting for it to come back - which is I-04 working,
		// not a fault.
		field("spool", fmt.Sprint(depth))
	}

	line, err := lastLogLine(filepath.Join(dir, logsDirName, logFileName))
	if err != nil {
		field("last log line", "unreadable: "+err.Error())
		ok = false
	} else {
		// Quoted and bounded: it is a line out of a file, and although
		// the service writes it through I-10's filter (spec 5.6), this
		// command has no way to know the file it just read was written
		// by this build.
		field("last log line", fmt.Sprintf("%.400q", line))
	}
	return ok
}

// The names spec 5.6 gives the log file this command reads without the service.
// They are restated here rather than taken from internal/service, which owns the
// layout and cannot be imported into this binary - see [reportLocal] for why.
const (
	logsDirName = "logs"
	logFileName = "engramux-service.log"
)

// maxLogTail is how much of the end of the log file is read to find its last
// line. The log is one file appended to forever - spec 5.6 asks for rotation and
// nothing rotates yet - so reading it whole is not an option, and 8 KiB is far
// more than one slog JSON record and is one read.
const maxLogTail = 8 << 10

// lastLogLine returns the last non-empty line of the log file.
//
// It reads the tail rather than the file, and it does not wait for anything: the
// file is open for append in another process, which is fine to read on Windows -
// Go opens it with FILE_SHARE_READ - and may be mid-write, in which case the last
// line is partial and is reported as it is. A partial line is the honest answer;
// waiting for a complete one would block a diagnostic on the process it is
// diagnosing.
func lastLogLine(path string) (string, error) {
	//nolint:gosec // G304: path is the service directory joined with two
	// constants of this file. No part of it is input.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	at := max(info.Size()-maxLogTail, 0)
	buf := make([]byte, info.Size()-at)
	if _, err := f.ReadAt(buf, at); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	lines := strings.Split(strings.ReplaceAll(string(buf), "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i], nil
		}
	}
	return "", nil
}

// reportTask prints the registered task, and reports whether it could be read.
func reportTask(ctx context.Context, name string) bool {
	_, _ = fmt.Fprintf(os.Stdout, "task     %s\n", name)

	t, err := schedule.Query(ctx, name)
	if err != nil {
		if errors.Is(err, schedule.ErrNotRegistered) {
			field("not registered", "nothing starts the service at logon - run `engramux register`")
		} else {
			field("unreadable", err.Error())
		}
		return false
	}

	field("command", t.Command)
	// The principal, which is the setting most worth seeing: this must be
	// the interactive user rather than SYSTEM, and unelevated, even when the
	// shell that registered it was elevated.
	field("principal", t.UserID)
	field("logon type", t.LogonType)
	// RunLevel and enabled are absent from what Windows hands back whenever
	// they equal their defaults, and these are the defaults. So these two
	// lines report a value that was very probably never on the wire - which
	// is the correct reading of absence, not a gap being papered over.
	field("run level", t.RunLevel)
	field("enabled", fmt.Sprint(t.Enabled))
	field("logon trigger", yesNo(t.HasLogonTrigger, "present", "MISSING - nothing starts it at logon"))
	field("hidden", fmt.Sprint(t.Hidden))
	// The two spec 5.5 names explicitly, and the two Phase 3's [manual] gate
	// asks for.
	field("execution time limit", t.ExecutionTimeLimit)
	field("multiple instances", t.MultipleInstancesPolicy)
	if t.RestartInterval == "" {
		field("restart on failure", "none")
	} else {
		field("restart on failure", fmt.Sprintf("%d times, one every %s", t.RestartCount, t.RestartInterval))
	}
	field("on battery", yesNo(!t.DisallowStartIfOnBatteries, "starts", "WILL NOT START"))
	field("onto battery", yesNo(!t.StopIfGoingOnBatteries, "keeps running", "STOPS"))
	return true
}

// reportService prints what only the service can answer, and reports whether it
// answered.
//
// It sends the Doctor request, which is what spec 5.2 reserved that type for. It
// is not the Status request with extra fields on it: this reply carries the real
// database path where every other reply masks it (spec 5.9), and it carries the
// tokenizer comparison, which nothing else asks for.
//
// The tokenizer line is a *comparison* and not two strings for a person to read
// against each other. goose does not checksum a migration, so an applied one
// edited in place leaves an index built by the old clause and a file claiming the
// new one, with nothing saying so; the strings are printed only when they
// disagree, because that is when they are worth reading.
func reportService() bool {
	_, _ = fmt.Fprintln(os.Stdout, "service")

	reply, err := askDoctor()
	if err != nil {
		// The error names the pipe, which is the whole point: it says
		// what could not be read rather than only that something could
		// not be.
		field("not answering", err.Error())
		return false
	}
	field("uptime", (time.Duration(reply.UptimeMS) * time.Millisecond).Round(time.Millisecond).String())
	field("events", fmt.Sprint(reply.Events))
	field("spool", fmt.Sprint(reply.SpoolDepth))
	// The real path, and this is the one command that gets it (spec 5.9).
	field("database", reply.DatabasePath)

	switch {
	case reply.TokenizerReadError != "":
		field("index tokenizer", "unreadable: "+reply.TokenizerReadError)
		return false
	case reply.TokenizerAgrees():
		field("index tokenizer", fmt.Sprintf("agrees with the migration (%.64q)", reply.TokenizerLive))
	default:
		field("index tokenizer", fmt.Sprintf("DISAGREES - the index was built with %.64q, "+
			"the migration declares %.64q; the index needs a rebuild",
			reply.TokenizerLive, reply.TokenizerExpected))
		return false
	}
	return true
}

// reportMCP prints spec 5.9's endpoint, whether it is answering, and whether
// each host is pointed at it, and reports whether all of that held.
//
// # The token is not here, and could not be
//
// internal/mcpconf's read side decodes the URL and has no field for a token
// (spec 6.1), so this command cannot print one by accident. What it does print
// is the endpoint, which carries a port and no user path.
//
// # The guard is probed rather than assumed
//
// A published URL says the service bound something once. What says it is
// answering now, with the bearer check in front of it, is a request with no
// token: 401 is the guard doing its job, a refused connection is a stale
// mcp.json, and any other status is something else answering on that port.
//
// # The host check is a substring search and not a parser
//
// Claude Code's MCP configuration lives in ~/.claude.json - at the top level
// for the user scope and under projects.<path> for the local one - and Codex's
// in ~/.codex/config.toml. Two formats, two schemas, three places, and this
// binary is the hook relay (spec 5.1), so a TOML parser and a JSON schema would
// be linked into a process spawned once per hook event to answer a question
// with three states.
//
// A host that names this endpoint is pointed at it, whichever scope wrote it. A
// host that names engramux without naming the endpoint is stale, which is the
// state spec 5.9 says `doctor` has to report: the sticky port was lost, the
// service bound another, and the URL in that file no longer answers. Neither
// check can be fooled by a shape this product did not write, because both
// strings are ones it publishes.
//
// # The host lines are reported and do not decide the exit code
//
// The exit code is about this installation. A host configuration is another
// product's file, edited by the user, and neither of its states is a fault of
// the service: not registered is a machine where capture works and nobody has
// asked for the reader surface yet, and even stale is a line to act on rather
// than a service that is down. Folding either into the exit code would also
// make the exit code depend on whose machine it ran on - a `doctor` pointed at
// a test service on a machine whose real configuration names the real one would
// report stale, correctly, and fail a test that has nothing to do with it.
//
// What the exit code is for here is the two lines above: an endpoint that was
// never published, and one nothing is listening on.
func reportMCP(ctx context.Context) bool {
	_, _ = fmt.Fprintln(os.Stdout, "mcp")

	spoolPath, err := spool.Dir()
	if err != nil {
		field("endpoint", "unreadable: "+err.Error())
		return false
	}
	dir := filepath.Dir(spoolPath)

	endpoint, err := mcpconf.URL(dir)
	switch {
	case err != nil:
		field("endpoint", "unreadable: "+err.Error())
		return false
	case endpoint == "":
		field("endpoint", "NOT PUBLISHED - the service has not started since this build, or it could not bind")
		return false
	}
	field("endpoint", endpoint)

	ok := true
	if err := probeMCP(ctx, endpoint); err != nil {
		field("listening", "NO - "+err.Error())
		ok = false
	} else {
		field("listening", "yes")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		field("hosts", "unreadable: "+err.Error())
		return ok
	}
	for _, h := range []struct{ label, path, marker string }{
		{"claude code", filepath.Join(home, ".claude.json"), `"engramux":`},
		{"codex", filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.engramux]"},
	} {
		reportHostMCP(h.label, h.path, endpoint, h.marker)
	}
	return ok
}

// reportHostMCP prints one host's registration against endpoint. It reports
// nothing back: see [reportMCP] for why these lines do not decide the exit
// code.
//
// # marker, and why it is not just "engramux"
//
// It has to say "this file holds an Engramux MCP entry" without saying "this
// file mentions Engramux", and the loose version is not a hypothetical: both
// host files carry per-project state keyed by working directory, so a checkout
// in a directory called engramux puts the word in both of them and every
// installation everywhere reports itself stale. Observed, on this repository.
//
// So marker is the exact string the installer writes and a path cannot produce.
// Claude Code's `"engramux":` needs a quote immediately before the name, which
// a path key like "D:\\src\\engramux" does not have - the character there is
// a backslash. Codex's is a TOML table header. A hand-written entry spelled
// some other way reads as not registered, which is a false negative and is the
// direction to be wrong in.
func reportHostMCP(label, path, endpoint, marker string) {
	text, err := readCapped(path, maxHostConfig)
	switch {
	case errors.Is(err, os.ErrNotExist):
		field(label, "no configuration file at "+path)
	case err != nil:
		field(label, "unreadable: "+err.Error())
	case strings.Contains(text, endpoint):
		field(label, "points at this endpoint")
	case strings.Contains(text, marker):
		field(label, "STALE - it names engramux at another URL; re-run scripts/install-hooks.mjs --apply")
	default:
		field(label, "not registered - run scripts/install-hooks.mjs --apply")
	}
}

// maxHostConfig bounds how much of a host configuration is read.
//
// ~/.claude.json is Claude Code's own state file rather than a static
// configuration - it holds per-project history alongside the MCP entries - so
// it is not a small file and nothing bounds how large it gets. 16 MiB is far
// past any observed size and is a bound rather than a budget: a file over it is
// reported as unreadable instead of being half-searched, because a substring
// that is not in the first 16 MiB is indistinguishable from one that is not
// there at all.
const maxHostConfig = 16 << 20

// readCapped reads path, refusing a file over cap rather than truncating it.
func readCapped(path string, cap int64) (string, error) {
	//nolint:gosec // G304: path is the user's home directory joined with
	// constants of this file. No part of it is input.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > cap {
		return "", fmt.Errorf("%s is %d bytes, over the %d this reads", path, info.Size(), cap)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// probeMCP opens a TCP connection to the published endpoint and closes it.
//
// # It is a dial and not an HTTP request, and the reason is measured
//
// The obvious probe is a request with no bearer token, requiring the 401 that
// proves the guard is in front of the handler. It costs net/http in this
// binary, and this binary is the hook relay as well as the CLI (spec 5.1):
// measured, importing net/http here takes it from 3,862,528 to 7,482,368 bytes,
// +93.7%, in a process spawned once per hook event. That is the same trade
// internal/service lost when it put the SQLite driver here, and it loses again.
//
// A dial answers the question `doctor` is actually for - is anything listening
// on the URL that was published - and what it gives up is already held
// elsewhere: TestPhase5GateNoTokenAndAWrongTokenAreBothRefused is what says a
// request with no token is refused, and it runs against the production wiring
// every time the suite does.
func probeMCP(ctx context.Context, endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	d := net.Dialer{Timeout: mcpProbeBudget}
	c, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return err
	}
	return c.Close()
}

// mcpProbeBudget bounds the probe. It is a loopback request to a process on
// this machine, so the only thing it can wait for is a port nothing is
// listening on - which on Windows is refused immediately - or a listener that
// has wedged, which is exactly what this is asking about.
const mcpProbeBudget = 2 * time.Second

// askDoctor sends one Doctor request and returns the reply it can accept.
//
// [ipc.DoctorReply.Verify] is what tells this reply from the rejected ACK the
// service answers a request it will not serve. Without it an Ack would decode
// into a reply of zeroes and this command would print them as a service with no
// events, no database, and a tokenizer that agrees with itself.
func askDoctor() (ipc.DoctorReply, error) {
	var zero ipc.DoctorReply

	raw, err := roundTrip(ipc.Doctor, nil)
	if err != nil {
		return zero, err
	}

	var reply ipc.DoctorReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return zero, fmt.Errorf("decode the reply: %w", err)
	}
	if err := reply.Verify(); err != nil {
		// Bounded on its way into the message: it is bytes off the wire
		// and capped only by ipc.MaxFrameLen.
		return zero, fmt.Errorf("%w: the service replied %.200q", err, raw)
	}
	return reply, nil
}

// field prints one indented label and value.
func field(label, value string) {
	_, _ = fmt.Fprintf(os.Stdout, "  %-22s %s\n", label, value)
}

func yesNo(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

// register installs the Task Scheduler entry that starts the service at logon
// (spec 5.5). It is a real system change and the user is the one who makes it,
// by typing this.
func register(args []string) int {
	name := taskName(args)

	exe, err := serviceExe()
	if err != nil {
		warn("register: %v", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), taskBudget)
	defer cancel()
	if err := schedule.Register(ctx, name, exe); err != nil {
		warn("register: %v", err)
		return 1
	}
	_, _ = fmt.Fprintf(os.Stdout, "registered %s to run %s at this user's logon\n", name, exe)
	_, _ = fmt.Fprintln(os.Stdout, "it starts at the next logon - start it now by running that binary, or check with `engramux doctor`")
	return 0
}

// unregister removes the entry. Removing one that is not there is not an error,
// so this is safe to run when nobody remembers whether it was installed.
func unregister(args []string) int {
	name := taskName(args)

	ctx, cancel := context.WithTimeout(context.Background(), taskBudget)
	defer cancel()
	if err := schedule.Unregister(ctx, name); err != nil {
		warn("unregister: %v", err)
		return 1
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s is not registered\n", name)
	_, _ = fmt.Fprintln(os.Stdout, "a service that is already running keeps running; nothing starts it at the next logon")
	return 0
}

// taskName is the task the three commands above act on: the one a real install
// uses, unless a name was given.
//
// The override is what makes these three testable at all. A test may never
// touch the name a real install owns, so without it every path where the
// registration is in place - which is every path that matters - would be one
// nothing could ever exercise. It is also what somebody mid-rename reaches for.
func taskName(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return schedule.TaskName
}

// serviceExe is the service binary beside this one.
//
// Derived rather than configured: the two binaries ship together, and a
// registration pointing at a path that does not exist is a task that fails
// silently at every logon. The stat is what turns that into a message now.
func serviceExe() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate this binary: %w", err)
	}
	exe := filepath.Join(filepath.Dir(self), serviceName)
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("%s is not beside this binary: %w", serviceName, err)
	}
	return exe, nil
}
