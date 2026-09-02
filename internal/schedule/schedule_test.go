package schedule

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os/user"
	"strings"
	"testing"
	"time"
)

// probeExe is the command the probe tasks point at. It does not exist and does
// not need to: schtasks registers a path without checking it, so nothing here
// depends on a built binary, and a task that somehow ran would run nothing.
const probeExe = `C:\engramux-does-not-exist\dist\engramux-service.exe`

// probe registers a task under a name no real install uses and guarantees it is
// gone when the test ends, including when the test fails partway through.
//
// The suffix is random rather than derived from the test's name: two runs of
// the same test overlapping - which -p 1 forbids and nothing enforces - would
// otherwise fight over one registration, and the loser would delete the
// winner's task out from under it.
//
// The cleanup runs on its own context. t.Context is already cancelled by the
// time cleanups run, and a cleanup that cannot reach schtasks would leave a
// scheduled task behind on the developer's machine.
func probe(t *testing.T) string {
	t.Helper()
	name := `\Engramux-test-` + rand.Text()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := Unregister(ctx, name); err != nil {
			t.Errorf("unregister %s: %v - it is still on this machine", name, err)
		}
	})
	if err := Register(t.Context(), name, probeExe); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	return name
}

// TestARegisteredTaskReadsBackEverySettingSpec55Requires is gate 1: the
// settings are asserted by value off a task Windows actually holds, not off the
// document this package generated.
//
// Every expectation below is a literal from spec 5.5 rather than one of this
// package's own constants. A test that compares the implementation against
// itself agrees with whatever the implementation says.
func TestARegisteredTaskReadsBackEverySettingSpec55Requires(t *testing.T) {
	name := probe(t)

	got, err := Query(t.Context(), name)
	if err != nil {
		t.Fatalf("query %s: %v", name, err)
	}

	// PT0S means "run indefinitely". Absence would leave the behaviour
	// unresolved, which is the whole reason spec 5.5 writes it out, so this
	// asserts the value and not the element's presence.
	if got.ExecutionTimeLimit != "PT0S" {
		t.Errorf("ExecutionTimeLimit = %q, want %q", got.ExecutionTimeLimit, "PT0S")
	}
	if got.RestartInterval != "PT1M" {
		t.Errorf("RestartOnFailure/Interval = %q, want %q", got.RestartInterval, "PT1M")
	}
	if got.RestartCount != 3 {
		t.Errorf("RestartOnFailure/Count = %d, want %d", got.RestartCount, 3)
	}
	if got.MultipleInstancesPolicy != "IgnoreNew" {
		t.Errorf("MultipleInstancesPolicy = %q, want %q - I-09's second half",
			got.MultipleInstancesPolicy, "IgnoreNew")
	}
	if !got.Hidden {
		t.Errorf("Hidden = false, want true")
	}
	if !got.HasLogonTrigger {
		t.Errorf("the task has no LogonTrigger, so nothing starts the service at logon")
	}
	if got.Command != probeExe {
		t.Errorf("Actions/Exec/Command = %q, want %q", got.Command, probeExe)
	}

	// The principal is the interactive user. Not SYSTEM (S-1-5-18), not a
	// service account, and not elevated - the shell that registered this may
	// itself be elevated and the task must not inherit that.
	if got.LogonType != "InteractiveToken" {
		t.Errorf("LogonType = %q, want %q", got.LogonType, "InteractiveToken")
	}
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	if got.UserID != u.Uid {
		// The SID is printed only when it is already wrong. A task
		// registered as SYSTEM reads back S-1-5-18 here.
		t.Errorf("Principals/Principal/UserId = %q, want this user's SID", got.UserID)
	}

	// A logon-triggered service on a laptop that will not start on battery
	// is a service that is down for the whole flight. Both defaults are
	// true, so both have to be written and both have to read back false.
	if got.DisallowStartIfOnBatteries {
		t.Errorf("DisallowStartIfOnBatteries = true - the service will not start on battery")
	}
	if got.StopIfGoingOnBatteries {
		t.Errorf("StopIfGoingOnBatteries = true - the service dies when the charger is unplugged")
	}
}

// TestAnAbsentRunLevelReadsAsTheDefault is gate 2, and it is two assertions
// because either one alone is satisfied by a design the other forbids.
//
// Windows normalises away an element whose value equals its default: this
// package writes <RunLevel>LeastPrivilege</RunLevel> and the readback has no
// RunLevel at all. So a doctor that treats absence as misconfiguration reports
// a problem that does not exist.
//
//   - The raw document has no RunLevel element. Without this the test would
//     keep passing if Windows started echoing the element back, and the code
//     that supplies the default would never run again.
//   - [Query] answers LeastPrivilege anyway.
//
// The same holds for Settings/Enabled, which is also absent from the readback
// and also means its default.
func TestAnAbsentRunLevelReadsAsTheDefault(t *testing.T) {
	name := probe(t)

	raw, err := query(t.Context(), name)
	if err != nil {
		t.Fatalf("query %s: %v", name, err)
	}
	if bytes.Contains(raw, []byte("RunLevel")) {
		t.Fatalf("the readback carries a RunLevel element, so the default below is never exercised:\n%s", raw)
	}
	if bytes.Contains(raw, []byte("Enabled")) {
		t.Fatalf("the readback carries an Enabled element, so the default below is never exercised:\n%s", raw)
	}

	got, err := parse(raw)
	if err != nil {
		t.Fatalf("parse the readback: %v", err)
	}
	if got.RunLevel != "LeastPrivilege" {
		t.Errorf("RunLevel = %q, want %q - an absent element is the default, not a fault",
			got.RunLevel, "LeastPrivilege")
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true - an absent element is the default, not a fault")
	}
}

// TestUnregisterLeavesNothingBehindAndRunsTwice is gate 4.
//
// Registering twice matters as much as unregistering twice: spec 5.5's upgrade
// path replaces the binary, and a user who re-runs the command must not have to
// remove the old registration first.
func TestUnregisterLeavesNothingBehindAndRunsTwice(t *testing.T) {
	name := probe(t)

	if err := Register(t.Context(), name, probeExe); err != nil {
		t.Fatalf("register %s a second time: %v", name, err)
	}

	if err := Unregister(t.Context(), name); err != nil {
		t.Fatalf("unregister %s: %v", name, err)
	}
	// Gone, and gone in the only way that counts: Windows no longer has it.
	if _, err := Query(t.Context(), name); !errors.Is(err, ErrNotRegistered) {
		t.Errorf("Query after Unregister = %v, want ErrNotRegistered - something is still registered", err)
	}
	// Twice is not an error. The cleanup registered by probe makes it three
	// times, so this file would fail on its own rule if that were untrue.
	if err := Unregister(t.Context(), name); err != nil {
		t.Errorf("unregister %s a second time: %v", name, err)
	}
}

// TestQueryingATaskThatWasNeverRegistered pins the error a caller branches on.
// It is what tells `doctor` "nothing has been installed" from "schtasks is
// broken", and nothing else can: the message schtasks prints is localised, so
// it cannot be matched.
func TestQueryingATaskThatWasNeverRegistered(t *testing.T) {
	_, err := Query(t.Context(), `\Engramux-test-`+rand.Text())
	if !errors.Is(err, ErrNotRegistered) {
		t.Errorf("Query = %v, want ErrNotRegistered", err)
	}
}

// TestRun covers both ends of Run, and the success end is safe to exercise for
// the reason [probeExe] already documents: the registered path does not exist,
// so asking the scheduler to run it starts nothing. That is also what makes the
// assertion honest - what succeeds here is `schtasks` accepting the request,
// which is exactly the distinction Run's own comment draws. It is not evidence
// that anything came up.
func TestRun(t *testing.T) {
	for _, name := range []string{"", "no-leading-backslash", `\Engramux "quoted"`} {
		if err := Run(t.Context(), name); err == nil {
			t.Errorf("Run(%q) was accepted; checkName is what keeps a task name out of a command line", name)
		}
	}

	// A well-formed name that is not registered fails, and the error says
	// which name - so a typo reads as a typo rather than as a broken install.
	absent := `\Engramux-test-` + rand.Text()
	err := Run(t.Context(), absent)
	if err == nil {
		t.Fatalf("Run(%q) succeeded against a task that was never registered", absent)
	}
	if !strings.Contains(err.Error(), absent) {
		t.Errorf("the error does not name the task: %v", err)
	}

	// And a registered one is accepted.
	if err := Run(t.Context(), probe(t)); err != nil {
		t.Errorf("Run against a registered task: %v", err)
	}
}
