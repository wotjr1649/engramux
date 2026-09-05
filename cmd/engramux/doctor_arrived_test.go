package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/ipc"
)

// deliveryLines returns the `<host> received` lines one run of the
// installation section printed, keyed by host label.
//
// It reads the rendered output rather than calling [arrived], because what is
// under test is that the section prints the line at all. A test that called the
// helper directly would pass with the call site deleted, which is the shape of
// mistake this whole row is about.
func deliveryLines(t *testing.T, out string) map[string]string {
	t.Helper()
	found := map[string]string{}
	for line := range strings.SplitSeq(out, "\n") {
		label, value, ok := strings.Cut(strings.TrimSpace(line), " received ")
		if !ok {
			continue
		}
		found[label] = strings.TrimSpace(value)
	}
	return found
}

// runInstalledSection prints the installation section for two wired hosts
// against one service reply, and returns what it wrote.
func runInstalledSection(reply ipc.DoctorReply, replyErr error) (string, bool) {
	all := host.EventNames()
	var out bytes.Buffer
	r := &report{w: &out}
	r.reportInstalled("bin", "bin/engramux.exe",
		[]string{host.RelayName, host.ServiceName},
		[]hostHooks{{label: "claude-code", wired: all}, {label: "codex", wired: all}},
		reply, replyErr)
	return out.String(), r.failed
}

// TestTheInstallationSectionSaysWhatEachHostHasActuallyDelivered is backlog
// 50's guard, and the assertion that matters is the last one.
//
// Both hosts are wired identically here, so the configuration-derived line is
// the same sentence for both - `11 of 11 events point at the installed relay`.
// That sentence was true and useless for nine days while `codex` had never
// delivered an event. The two hosts must therefore be distinguishable in this
// section from something the installer did not write, and the events table is
// the only such thing there is.
func TestTheInstallationSectionSaysWhatEachHostHasActuallyDelivered(t *testing.T) {
	const lastMS = 1_788_600_000_000
	reply := ipc.DoctorReply{
		Events: 42,
		Cells: []ipc.Cell{
			{Host: "claude-code", EventName: "UserPromptSubmit", Count: 40, LastSeenMS: lastMS - 5000},
			{Host: "claude-code", EventName: "Stop", Count: 2, LastSeenMS: lastMS},
		},
	}

	out, failed := runInstalledSection(reply, nil)
	lines := deliveryLines(t, out)

	if failed {
		t.Errorf("a host that has delivered nothing moved the exit code:\n%s", out)
	}
	if got, want := lines["claude-code"], "42 events, the last at "+stamp(lastMS); got != want {
		t.Errorf("claude-code delivery line = %q, want %q\n%s", got, want, out)
	}
	if got := lines["codex"]; !strings.Contains(got, "nothing, ever") {
		t.Errorf("codex delivery line = %q, want it to say nothing has ever arrived\n%s", got, out)
	}
	if lines["claude-code"] == lines["codex"] {
		t.Errorf("both hosts read the same in this section, which is the defect:\n%s", out)
	}
}

// TestTheDeliveryLineNeverGuessesWhenNobodyCanAnswer covers the two states that
// are not zero and would read as zero if the absence were taken at face value:
// a service that is down, and a service too old to send the breakdown.
//
// Reading either as "this host has never delivered an event" would be this
// command inventing evidence, which is the failure the line exists to end.
func TestTheDeliveryLineNeverGuessesWhenNobodyCanAnswer(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reply    ipc.DoctorReply
		replyErr error
	}{
		{"the service is not answering", ipc.DoctorReply{}, errors.New("dial: the pipe is not there")},
		{"the service is older than this CLI", ipc.DoctorReply{Events: 42}, nil},
	} {
		out, failed := runInstalledSection(tc.reply, tc.replyErr)
		lines := deliveryLines(t, out)
		if failed {
			t.Errorf("%s: it moved the exit code:\n%s", tc.name, out)
		}
		for _, label := range []string{"claude-code", "codex"} {
			got := lines[label]
			if !strings.HasPrefix(got, "unknown") {
				t.Errorf("%s: %s delivery line = %q, want it to start with unknown\n%s",
					tc.name, label, got, out)
			}
			if strings.Contains(got, "nothing, ever") {
				t.Errorf("%s: %s read an unanswerable question as zero: %q\n%s",
					tc.name, label, got, out)
			}
		}
	}
}

// TestAHostThatIsNotInstalledGetsNoDeliveryLine holds the one case where
// silence is the answer. An absent configuration file means that host is not on
// this machine, and saying it has delivered nothing would be a second finding
// about a host nobody installed.
func TestAHostThatIsNotInstalledGetsNoDeliveryLine(t *testing.T) {
	var out bytes.Buffer
	r := &report{w: &out}
	r.reportInstalled("bin", "bin/engramux.exe",
		[]string{host.RelayName, host.ServiceName},
		[]hostHooks{{label: "claude-code", wired: host.EventNames()}, {label: "codex", absent: true}},
		ipc.DoctorReply{}, nil)

	if lines := deliveryLines(t, out.String()); len(lines) != 1 || lines["claude-code"] == "" {
		t.Errorf("an absent host was given a delivery line:\n%s", out.String())
	}
}

// TestArrivedSumsOneHostAndIgnoresTheOthers pins the arithmetic the line
// reports. Counts add across a host's cells and the instant is the newest of
// them, and neither may take a value from a cell belonging to the other host.
func TestArrivedSumsOneHostAndIgnoresTheOthers(t *testing.T) {
	cells := []ipc.Cell{
		{Host: "claude-code", Count: 7, LastSeenMS: 300},
		{Host: "codex", Count: 5000, LastSeenMS: 999_999},
		{Host: "claude-code", Count: 3, LastSeenMS: 100},
	}
	count, last := arrived("claude-code", cells)
	if count != 10 || last != 300 {
		t.Errorf("arrived = (%d, %d), want (10, 300)", count, last)
	}
	if count, last := arrived("unknown", cells); count != 0 || last != 0 {
		t.Errorf("a host with no cells = (%d, %d), want (0, 0)", count, last)
	}
}
