package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wotjr1649/engramux/internal/schedule"
)

// taskBudget bounds one schtasks invocation. It is not the pipe's budget: this
// is a local Windows command with no service on the other end, and the only
// reason it is bounded at all is that a wedged schtasks would otherwise hang a
// command a person typed.
const taskBudget = 30 * time.Second

// serviceName is the file engramux-service is built as. `register` looks for it
// beside this binary, because that is how the two ship (spec 5.1).
const serviceName = "engramux-service.exe"

// doctor reports the two things Phase 3 gates on, and it has two halves with
// different availability - which is spec 10's first open question, answered
// here rather than left open.
//
// The task registration is a Windows query. It needs no service, no pipe and no
// database, so it is readable exactly when Windows is running. The service's
// own numbers are only reachable over the pipe (I-07 leaves no other way to
// read them, I-08 routes them here).
//
// So a service that is down does not make this command useless, and the shape
// that follows is: read both halves, print both halves, and fail if either one
// could not be read. A version that returned at the first failure would print
// nothing about the registration whenever the service was down - which is
// exactly the moment somebody runs `doctor` to find out why.
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
	if !reportService() {
		ok = false
	}
	if !ok {
		return 1
	}
	return 0
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
// It sends the Status request `status` and `cells` send, unchanged. Spec 5.2
// fixes the request set at five types, and this is not a sixth thing to ask: it
// is the same "how is the service doing" question, printed beside the half that
// does not need the service at all.
func reportService() bool {
	_, _ = fmt.Fprintln(os.Stdout, "service")

	reply, err := askStatus()
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
	field("database", reply.DatabasePath)
	return true
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
