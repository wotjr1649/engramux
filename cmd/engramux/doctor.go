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
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/mcpconf"
	"github.com/wotjr1649/engramux/internal/schedule"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/spool"
	"github.com/wotjr1649/engramux/internal/version"
	"github.com/wotjr1649/engramux/internal/winacl"
)

// taskBudget bounds one schtasks invocation. It is not the pipe's budget: this
// is a local Windows command with no service on the other end, and the only
// reason it is bounded at all is that a wedged schtasks would otherwise hang a
// command a person typed.
const taskBudget = 30 * time.Second

// serviceName is the file engramux-service is built as. `register` looks for it
// beside this binary, because that is how the two ship (spec 5.1).
const serviceName = "engramux-service.exe"

// fullFlag prints the values this command masks by default.
const fullFlag = "--full"

// doctor reports everything knowable about this installation, and it is built
// around the moment it is most needed being the moment it is most broken - which
// is spec 10's first open question, answered here and recorded in spec 5.5.
//
// # It answers by stage, and that is memory spec M-6's first change
//
// "Not installed yet" and "installed and broken" are different answers with
// different next commands. Before this, a fresh machine got four failing
// sections and no instruction anywhere: no task, no service, no endpoint, no
// binary, every one of them true and none of them the point. The point was that
// nobody had run the installer. [nothingIsInstalled] is that judgement, and it
// is deliberately unanimous - a machine with any one of the three signs of an
// installation gets the full report, because at that point "what is broken" is
// the question and "install it" is not the answer.
//
// # Sections, and what each one needs to be readable
//
//   - The task registration is a Windows query. It needs no service, no pipe and
//     no database, so it is readable exactly when Windows is running.
//   - The eleven hook entries are two files under the user's home directory,
//     read with the paths `install` itself computes ([resolvePaths]). They are
//     M-6's third change and the one surface a working capture actually depends
//     on: a relay nothing invokes captures nothing, and nothing else in the
//     product looks at that table after it is written.
//   - The local state - where the two binaries are, how deep the spool is, what
//     the log last said - is files on disk. Also readable with nothing running.
//   - The service's own numbers, the real database path and the tokenizer
//     comparison are only reachable over the pipe: I-07 leaves no other way to
//     read them and I-08 routes them here.
//
// So a service that is down does not make this command useless. Every section
// is read and printed, the unreachable one is marked rather than omitted, and
// the exit code is 1 if any of them failed. A version that returned at the first
// failure would print nothing about the registration whenever the service was
// down - which is exactly the moment somebody runs this.
//
// # MCP is optional, which is M-6's second change
//
// The MCP section reports and does not decide the exit code. A capture-only
// installation is a supported state: the hooks are in, events are being stored,
// and nobody has pointed a host at the reader surface. That machine is green.
// The cost is stated rather than hidden - an endpoint that is published and not
// answering exits 0 with a loud NO in the report - and it is the same trade the
// service already makes, where [serveMCP] logs a failed endpoint and keeps
// ingesting rather than refusing to start.
//
// # The default is masked, and --full prints the real values
//
// This report is the output a person is most likely to paste into a public
// issue, and it carried two things worth removing from that paste: the real
// database path, which spec 5.9 hands only to this command, and the task
// principal, which is a Windows SID. Every value printed here goes through
// [secret.MaskString] unless --full was given, and the principal is a verdict
// rather than a SID for the same reason the tokenizer is a verdict rather than
// two strings - the question is "is this the right user", not "what is the
// number".
//
// Measured before importing internal/secret into this binary, which is the hook
// relay as well as the CLI (spec 5.1): +288,768 bytes, +6.5%, and 40 us to
// compile the rule table at init, against a relay process that lives about
// 11 ms. That is 0.36% of one hook event, and it is why this import was taken
// where net/http's +93.7% was refused in [probeMCP].
func doctor(args []string) int { return runDoctor(os.Stdout, args) }

// runDoctor is [doctor] with its writer passed in, which is what makes the
// masking testable: the alternative is a rule about what reaches a terminal
// that nothing can ever read back.
func runDoctor(w io.Writer, args []string) int {
	r := &report{w: w, full: slices.Contains(args, fullFlag)}

	opt, err := currentPaths(withoutFlags(args))
	if err != nil {
		r.line("doctor: %v", err)
		return 1
	}
	relay := filepath.Join(opt.BinDir, host.RelayName)

	ctx, cancel := context.WithTimeout(context.Background(), taskBudget)
	defer cancel()

	// Read before anything is printed, because the stage judgement needs all
	// three answers and the first section printed is already an answer.
	task, taskErr := schedule.Query(ctx, opt.TaskName)
	hooks := readHostHooks(opt, relay)
	installed := installedBinaries(opt.BinDir)

	if nothingIsInstalled(taskErr, hooks, installed) {
		r.reportNotInstalled(opt, relay)
		return 1
	}

	r.reportTask(opt.TaskName, task, taskErr)
	r.reportInstalled(opt.BinDir, relay, installed, hooks)
	r.reportLocal()
	r.reportService()
	r.reportMCP(ctx, opt.ClaudeMCP, opt.CodexConfig)

	if r.failed {
		return 1
	}
	return 0
}

// report is where every line of this command goes, and the one place the
// masking decision is made.
type report struct {
	w    io.Writer
	full bool
	// failed is set by [report.fail] and by nothing else, so "what makes this
	// command exit 1" is a list of call sites rather than a chain of returned
	// booleans that a new section can forget to join.
	failed bool
}

// mask is what every value this command prints goes through. It is applied to
// the whole formatted line rather than per value, because a line assembled from
// two sources has no seam a caller could be trusted to mark: the rule is that
// nothing reaches w unmasked, and one call site is how that is checked.
//
// A false positive costs a placeholder in a diagnostic and a false negative
// costs a user's SID in a public issue, which is internal/secret's own trade
// and the reason it is generous.
func (r *report) mask(v string) string {
	if r.full {
		return v
	}
	return secret.MaskString(v)
}

// line writes one unindented line.
func (r *report) line(format string, args ...any) {
	_, _ = fmt.Fprintln(r.w, r.mask(fmt.Sprintf(format, args...)))
}

// field writes one indented label and value. It is a fact, not a finding.
func (r *report) field(label, format string, args ...any) {
	_, _ = fmt.Fprintf(r.w, "  %-22s %s\n", label, r.mask(fmt.Sprintf(format, args...)))
}

// fail writes a field and makes the command exit 1.
func (r *report) fail(label, format string, args ...any) {
	r.failed = true
	r.field(label, format, args...)
}

// note writes a field that reports a problem the exit code deliberately does
// not carry. It exists so that "this is not counted" is visible at the call
// site rather than inferred from the absence of a [report.fail].
func (r *report) note(label, format string, args ...any) {
	r.field(label, format, args...)
}

// permissions reports what one file's DACL admits, for the three files a bearer
// token ends up in. It is memory spec §8's second publication condition, whose
// two halves are `mcp.json` narrowed and the host files reported rather than
// changed - the second half being the whole reason this is a report and not a
// fix. Those two files belong to the hosts, and narrowing another product's
// configuration is not this one's to do.
//
// # A verdict, not an ACE list
//
// Every line this command writes goes through [report.mask], and an ACE list is
// account names, machine names and SIDs - the shape that mask exists to keep out
// of a diagnostic somebody pastes into an issue. So this counts and classifies
// and never names a principal. `--full` does not widen it either: there is
// nothing here for it to unmask.
//
// # It is a note and never a fail
//
// The exit code answers "is this installation working", and an inherited DACL is
// working. Backlog 28 is a publication condition rather than a defect on the
// owner's machine, and spec 5.9 accepts the exposure there; making `doctor`
// exit 1 on it would make every currently correct installation report broken.
func (r *report) permissions(label, path string) {
	acc, err := winacl.Describe(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// The caller has already said the file is absent, and saying it
		// twice in different words reads like two findings.
	case err != nil:
		r.note(label, "permissions unreadable: %v", err)
	case acc.Narrowed():
		r.field(label, "narrowed to SYSTEM, Administrators and this user")
	case acc.Others > 0:
		r.note(label, "inherited DACL - %d principals beyond SYSTEM, Administrators "+
			"and this user reach a file holding the bearer token (backlog 28)", acc.Others)
	default:
		r.note(label, "inherited DACL - nothing beyond SYSTEM, Administrators and this "+
			"user reaches it today, but the parent directory decides that (backlog 28)")
	}
}

// installedNames is what an installation puts in its bin directory.
var installedNames = []string{host.RelayName, host.ServiceName}

// installedBinaries returns the names of [installedNames] that are in bin.
func installedBinaries(bin string) []string {
	var found []string
	for _, name := range installedNames {
		if _, err := os.Stat(filepath.Join(bin, name)); err == nil {
			found = append(found, name)
		}
	}
	return found
}

// nothingIsInstalled reports whether this machine has no sign of an
// installation at all: no logon task, neither binary in place, and not one
// Engramux hook entry in either host.
//
// Unanimity is the rule, and it is the conservative direction. Any one sign
// present means somebody ran the installer, so the useful answer is what is
// broken - and a report that says "not installed" to a machine that is
// half-installed would send the user to `install --apply` while hiding the
// failure that half-install actually hit.
//
// A task that could not be read is not a task that is absent, and a host
// configuration that could not be read is not one with no entries. Both fall
// through to the full report, where they are findings with their own lines.
func nothingIsInstalled(taskErr error, hooks []hostHooks, installed []string) bool {
	if !errors.Is(taskErr, schedule.ErrNotRegistered) {
		return false
	}
	if len(installed) > 0 {
		return false
	}
	for _, h := range hooks {
		if h.err != nil || len(h.wired) > 0 || len(h.stale) > 0 {
			return false
		}
	}
	return true
}

// reportNotInstalled is the whole output on a machine nobody has installed
// this on: the three things that are absent, and the one command that changes
// that. It replaces four red sections that were each individually true.
func (r *report) reportNotInstalled(opt host.Options, relay string) {
	r.line("engramux is not installed for this user.")
	r.field("logon task", "%s is not registered", opt.TaskName)
	r.field("binaries", "neither is in %s", opt.BinDir)
	r.field("hook entries", "none in either host configuration")
	r.line("")

	self, err := os.Executable()
	if err != nil {
		// Not a finding: the answer below is still the answer, and the only
		// thing lost is being able to spell the command with a full path.
		self = host.RelayName
	}
	r.line("install it with: %s install --apply", self)
	r.line("that copies both binaries to %s, writes the hook entries into both hosts,", opt.BinDir)
	r.line("registers the logon task and starts the service - in one pass.")
}

// hostHooks is one host configuration's answer to M-6's question: are the
// eleven entries there, and do they point at the installed relay.
//
// The three lists are disjoint and together cover [host.EventNames], so a
// reader can tell present-and-wrong from absent - which are different problems
// with the same remedy but very different meanings. Stale means an earlier
// install wrote a path that has since moved, and every event still fires into
// a binary that is not there; missing means the merge never reached that event.
type hostHooks struct {
	label   string
	path    string
	err     error
	absent  bool
	wired   []string
	stale   []string
	missing []string
}

// readHostHooks reads both hosts' hook tables through the paths install itself
// computes.
func readHostHooks(opt host.Options, relay string) []hostHooks {
	return []hostHooks{
		readOneHostHooks("claude-code", opt.ClaudePath, relay),
		readOneHostHooks("codex", opt.CodexHooks, relay),
	}
}

// readOneHostHooks classifies every event in one host configuration.
//
// A file that is not there is not a fault. [host.PlanMerge] skips one rather
// than creating it, because a user with only one of the two hosts installed is
// an ordinary user - so an absent file here means that host is not on this
// machine, and reporting it red would make every single-host machine fail.
func readOneHostHooks(label, path, relay string) hostHooks {
	state := hostHooks{label: label, path: path}

	text, err := host.ReadCapped(path, host.MaxHostConfig)
	switch {
	case errors.Is(err, os.ErrNotExist):
		state.absent = true
		return state
	case err != nil:
		state.err = err
		return state
	}

	found, err := host.HookCommands([]byte(text), host.EventNames())
	if err != nil {
		state.err = err
		return state
	}
	for _, event := range host.EventNames() {
		commands := found[event]
		switch {
		case len(commands) == 0:
			state.missing = append(state.missing, event)
		case slices.ContainsFunc(commands, func(c string) bool { return host.PointsAt(c, relay) }):
			state.wired = append(state.wired, event)
		default:
			state.stale = append(state.stale, event)
		}
	}
	return state
}

// trouble names what is wrong with a host's entries, in the two ways it can be.
func (h hostHooks) trouble() string {
	var parts []string
	if len(h.missing) > 0 {
		parts = append(parts, fmt.Sprintf("%d missing (%s)", len(h.missing), strings.Join(h.missing, ", ")))
	}
	if len(h.stale) > 0 {
		parts = append(parts, fmt.Sprintf("%d pointing somewhere else (%s)", len(h.stale), strings.Join(h.stale, ", ")))
	}
	return strings.Join(parts, ", ")
}

// reportInstalled prints what an installation is: the two binaries in the
// directory `install` copies them to, and the eleven hook entries in each host
// checked against the relay in that directory.
//
// This section decides the exit code, and that is the point of adding it. The
// hook table is what makes capture happen at all - memory spec M-6's third
// change - and until now it was the one major surface `doctor` never looked at:
// a machine could pass every other section while every event fired into a path
// that no longer existed.
//
// The binaries are here rather than in [report.reportLocal] because these two
// are the installation's and that one's are whichever copy of the CLI the user
// happened to run. Both are worth printing and only these decide whether the
// installation works.
func (r *report) reportInstalled(bin, relay string, installed []string, hooks []hostHooks) {
	r.line("installation %s", bin)

	for _, want := range installedNames {
		if slices.Contains(installed, want) {
			r.field(want, "present")
		} else {
			r.fail(want, "MISSING - run `engramux install --apply`")
		}
	}

	events := len(host.EventNames())
	for _, h := range hooks {
		switch {
		case h.absent:
			r.note(h.label, "no configuration file at %s - this host is not installed here", h.path)
		case h.err != nil:
			r.fail(h.label, "unreadable: %v", h.err)
		case len(h.wired) == events:
			r.field(h.label, "%d of %d events point at the installed relay", events, events)
		default:
			r.fail(h.label, "%d of %d point at it, %s - run `engramux install --apply`",
				len(h.wired), events, h.trouble())
		}
	}
}

// reportLocal prints what is knowable with nothing running.
//
// It is the section added because `doctor` is run when the service is down. The
// spool depth is a directory listing and the last log line is a file read;
// neither needs the pipe, and between them they answer "is the relay still
// capturing, and what did the service say before it stopped".
func (r *report) reportLocal() {
	r.line("local")

	// First, because it changes what every pipe read below means. The
	// variable exists for the test suite (ipc.TestPipeSIDEnv), every process
	// that sees it derives a different pipe name, and a shell that still
	// exports it makes this command diagnose a pipe nobody is serving - which
	// reads exactly like a service that is down (backlog 6). The name is
	// printed and the value is not: the value is a SID.
	//
	// A note and not a fail: the suite runs this command under the override
	// on purpose, against a service it started on that pipe, and a report
	// that failed for being asked about the right pipe would fail the gate
	// that asks. What the line changes is how the sections below are read,
	// and the sections below still decide the exit code.
	if os.Getenv(ipc.TestPipeSIDEnv) != "" {
		r.note("pipe name", "%s is set in this environment, so the service and mcp sections "+
			"below read a test pipe and not the installed service's - unset it", ipc.TestPipeSIDEnv)
	}

	if self, err := os.Executable(); err != nil {
		r.fail("this binary", "unreadable: %v", err)
	} else {
		r.field("this binary", "%s", self)
	}
	// Reported and not counted. This is the pair `engramux register` would
	// use, which is a fact about the copy of the CLI that was run rather than
	// about the installation - a user running the one they unpacked, or a
	// developer running dist/, has no service binary beside it and nothing is
	// wrong. [report.reportInstalled] is where the binaries that matter are
	// checked.
	if exe, err := serviceExe(); err != nil {
		r.note("service beside it", "none - `engramux register` from here would fail: %v", err)
	} else {
		r.field("service beside it", "%s", exe)
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
		r.fail("data directory", "unreadable: %v", err)
		return
	}
	dir := filepath.Dir(spoolPath)
	r.field("data directory", "%s", dir)

	// Reported and never counted, on either answer. Off is the shipped
	// state (memory spec rev.8, M-4) and on is a thing the user chose, so
	// neither is a fault - what the line is for is the other half of §6's
	// fifth mitigation: a switch you can see. Without it, "why is nothing
	// being injected" and "why is something being injected" are both
	// questions with no command that answers them.
	//
	// The path is printed on the off answer because that is where a person
	// has to write the file, and the file is the whole of the interface.
	if inject.Enabled() {
		r.field("injection", "on - hook-time context is being added to UserPromptSubmit")
	} else {
		r.field("injection", "off - write {\"enabled\":true} to %s to turn it on",
			filepath.Join(dir, inject.ConfigName))
	}

	if depth, err := spool.Depth(spoolPath); err != nil {
		r.fail("spool", "unreadable: %v", err)
	} else {
		// A depth that is not zero with the service down is the count of
		// events waiting for it to come back - which is I-04 working,
		// not a fault.
		r.field("spool", "%d", depth)
	}

	line, err := lastLogLine(filepath.Join(dir, logsDirName, logFileName))
	if err != nil {
		r.fail("last log line", "unreadable: %v", err)
	} else {
		// Quoted and bounded: it is a line out of a file, and although
		// the service writes it through I-10's filter (spec 5.6), this
		// command has no way to know the file it just read was written
		// by this build - which is also why it goes through the mask on
		// its way out like everything else here.
		r.field("last log line", "%.400q", line)
	}
}

// The names spec 5.6 gives the log file this command reads without the service.
// They are restated here rather than taken from internal/service, which owns the
// layout and cannot be imported into this binary - see [report.reportLocal] for
// why.
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

// reportTask prints the registered task.
func (r *report) reportTask(name string, t schedule.Task, err error) {
	r.line("task     %s", name)

	if err != nil {
		if errors.Is(err, schedule.ErrNotRegistered) {
			r.fail("not registered", "nothing starts the service at logon - run `engramux register`")
		} else {
			r.fail("unreadable", "%v", err)
		}
		return
	}

	r.field("command", "%s", t.Command)
	r.reportPrincipal(t.UserID)
	r.field("logon type", "%s", t.LogonType)
	// RunLevel and enabled are absent from what Windows hands back whenever
	// they equal their defaults, and these are the defaults. So these two
	// lines report a value that was very probably never on the wire - which
	// is the correct reading of absence, not a gap being papered over.
	r.field("run level", "%s", t.RunLevel)
	r.field("enabled", "%t", t.Enabled)
	r.field("logon trigger", "%s", yesNo(t.HasLogonTrigger, "present", "MISSING - nothing starts it at logon"))
	r.field("hidden", "%t", t.Hidden)
	// The two spec 5.5 names explicitly, and the two Phase 3's [manual] gate
	// asks for.
	r.field("execution time limit", "%s", t.ExecutionTimeLimit)
	r.field("multiple instances", "%s", t.MultipleInstancesPolicy)
	if t.RestartInterval == "" {
		r.field("restart on failure", "none")
	} else {
		r.field("restart on failure", "%d times, one every %s", t.RestartCount, t.RestartInterval)
	}
	r.field("on battery", "%s", yesNo(!t.DisallowStartIfOnBatteries, "starts", "WILL NOT START"))
	r.field("onto battery", "%s", yesNo(!t.StopIfGoingOnBatteries, "keeps running", "STOPS"))
}

// reportPrincipal prints who the task runs as, as a verdict rather than as the
// SID Windows hands back.
//
// The principal is the setting most worth seeing - it must be the interactive
// user rather than SYSTEM, and unelevated, even when the shell that registered
// it was elevated - and it is also the one value in this report that names the
// machine's owner. Comparing it here answers the question and prints nothing
// that identifies anybody; --full prints the SID, which is what somebody
// reading an unfamiliar registration actually needs.
//
// [schedule.Register] registers os/user's Uid, so the same package answers both
// sides of this comparison and a mismatch means the task really was registered
// by another account.
func (r *report) reportPrincipal(sid string) {
	if r.full {
		r.field("principal", "%s", sid)
		return
	}
	u, err := user.Current()
	switch {
	case err != nil:
		r.fail("principal", "cannot be compared: %v", err)
	case u.Uid == sid:
		r.field("principal", "this user")
	default:
		r.fail("principal", "ANOTHER USER - the task does not run as the user whose hooks these are; "+
			"re-run with %s to see both", fullFlag)
	}
}

// reportService prints what only the service can answer.
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
func (r *report) reportService() {
	r.line("service")

	reply, err := askDoctor()
	if err != nil {
		// The error names the pipe, which is the whole point: it says
		// what could not be read rather than only that something could
		// not be.
		r.fail("not answering", "%v", err)
		return
	}
	r.reportVersions(reply.Product)
	r.field("uptime", "%s", (time.Duration(reply.UptimeMS) * time.Millisecond).Round(time.Millisecond))
	r.field("events", "%d", reply.Events)
	r.field("spool", "%d", reply.SpoolDepth)
	// Backlog 31's two: what the service has logged at ERROR since it
	// started, and how its last checkpoint went. Facts, not findings - a
	// count is worth opening the log for, and only the log says what it
	// was.
	r.field("errors", "%d", reply.Errors)
	r.field("checkpoint", "%s", checkpointLine(reply.LastCheckpoint))
	// The real path, and this is the one command that gets it (spec 5.9) -
	// which is exactly why the default masks it again on the way out.
	r.field("database", "%s", reply.DatabasePath)

	switch {
	case reply.TokenizerReadError != "":
		r.fail("index tokenizer", "unreadable: %s", reply.TokenizerReadError)
	case reply.TokenizerAgrees():
		r.field("index tokenizer", "agrees with the migration (%.64q)", reply.TokenizerLive)
	default:
		r.fail("index tokenizer", "DISAGREES - the index was built with %.64q, "+
			"the migration declares %.64q; the index needs a rebuild",
			reply.TokenizerLive, reply.TokenizerExpected)
	}
}

// reportVersions is M-7's three versions, and it prints the two that can be
// answered without a delivery channel.
//
// **Installed against running** catches a replacement that was copied and never
// restarted, which is the state an interrupted reinstall leaves. **Cache against
// installed** is the "there is something newer" half, and there is no plugin
// cache to read yet - so it says so rather than printing a blank. M-7's own
// reading is that the first pair is the more useful half anyway, because a user
// who never installs the plugin still gets it.
//
// Nothing here fails the report. A version disagreement is a fact about the
// machine and the two commands that resolve it are named in the line itself;
// `doctor`'s exit code is M-6's and belongs to the stages, not to this.
func (r *report) reportVersions(running string) {
	installed := version.Product()
	switch running {
	case "":
		r.field("version", "%s installed; the service does not report one, so it is older than "+
			"this build - restart it with `engramux update --from <dir>`", installed)
	case installed:
		r.field("version", "%s, installed and running agree", installed)
	default:
		r.field("version", "%s installed but %s running - the binary was replaced and the "+
			"service was not restarted; `engramux update --from <dir>` does both",
			installed, running)
	}
	r.field("newest available", "unknown - there is no delivery channel yet, so `update` reads "+
		"a directory you point it at")
}

// reportMCP prints spec 5.9's endpoint, whether it is answering, and whether
// each host is pointed at it.
//
// # Nothing here decides the exit code, and that is memory spec M-6
//
// MCP is optional. A capture-only installation - hooks in, events stored, no
// host pointed at the reader surface - is a supported state and must be able to
// be green, and before this an endpoint that was never published made it red.
// The two lines this used to fail on, an unpublished endpoint and one nothing
// is listening on, are still printed and still loud; what changed is that they
// no longer decide whether the installation is working, because capture does
// not depend on them.
//
// The host lines were already outside the exit code, for a reason that still
// holds: a host configuration is another product's file, edited by the user,
// and a `doctor` pointed at a test service on a machine whose real
// configuration names the real one would report stale, correctly, and fail a
// test that has nothing to do with it.
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
// answering now is a dial - see [probeMCP] for why it is a dial and not a
// request with no bearer token.
//
// # The host check is a substring search and not a parser
//
// Claude Code's MCP configuration lives in ~/.claude.json - at the top level
// for the user scope and under projects.<path> for the local one - and Codex's
// in ~/.codex/config.toml. Two formats, two schemas, three places, and the hook
// table this command now parses properly is JSON in both hosts while these are
// not.
//
// A host that names this endpoint is pointed at it, whichever scope wrote it. A
// host that names engramux without naming the endpoint is stale, which is the
// state spec 5.9 says `doctor` has to report: the sticky port was lost, the
// service bound another, and the URL in that file no longer answers. Neither
// check can be fooled by a shape this product did not write, because both
// strings are ones it publishes.
func (r *report) reportMCP(ctx context.Context, claudeMCP, codexConfig string) {
	r.line("mcp      optional - capture works without any of this")

	spoolPath, err := spool.Dir()
	if err != nil {
		r.note("endpoint", "unreadable: %v", err)
		return
	}
	dir := filepath.Dir(spoolPath)

	endpoint, err := mcpconf.URL(dir)
	switch {
	case err != nil:
		r.note("endpoint", "unreadable: %v", err)
		return
	case endpoint == "":
		r.note("endpoint", "NOT PUBLISHED - the service has not started since this build, or it could not bind")
		return
	}
	r.field("endpoint", "%s", endpoint)
	r.permissions("mcp.json", mcpconf.Path(dir))

	if err := probeMCP(ctx, endpoint); err != nil {
		r.note("listening", "NO - %v", err)
	} else {
		r.field("listening", "yes")
	}

	// The two paths are the installer's own (resolvePaths), so what this
	// section reads is what `install --apply` would write to or skip - one
	// definition, which is memory spec M-6's rule for the hook table too.
	for _, h := range []struct{ label, path, marker string }{
		{"claude code", claudeMCP, `"engramux":`},
		{"codex", codexConfig, "[mcp_servers.engramux]"},
	} {
		r.reportHostMCP(h.label, h.path, endpoint, h.marker)
	}
}

// reportHostMCP prints one host's registration against endpoint.
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
func (r *report) reportHostMCP(label, path, endpoint, marker string) {
	text, err := host.ReadCapped(path, host.MaxHostConfig)
	switch {
	case errors.Is(err, os.ErrNotExist):
		r.note(label, "no configuration file at %s", path)
	case err != nil:
		r.note(label, "unreadable: %v", err)
	case strings.Contains(text, endpoint):
		r.field(label, "points at this endpoint")
		r.permissions(label+" file", path)
	case strings.Contains(text, marker):
		r.note(label, "STALE - it names engramux at another URL; re-run `engramux install --apply`")
		r.permissions(label+" file", path)
	default:
		r.note(label, "not registered - run `engramux install --apply`")
	}
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
		return zero, replied(err, raw)
	}
	return reply, nil
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
