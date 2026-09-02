package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestDoctorLocalNamesALeftoverPipeSIDOverride is backlog 6. A shell that still
// exports the test-only pipe SID sends every pipe read `doctor` makes to a pipe
// the installed service is not listening on, and a report that then said "not
// answering" without saying why would send a person to restart a service that
// is fine. The variable's name is printed and its value is not: the value is a
// SID, and the mask would eat it anyway.
func TestDoctorLocalNamesALeftoverPipeSIDOverride(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv(ipc.TestPipeSIDEnv, "S-1-5-21-1-2-3-1001")

	var out bytes.Buffer
	r := &report{w: &out}
	r.reportLocal()

	line := pipeNameLine(out.String())
	if line == "" {
		t.Fatalf("the local section does not name %s:\n%s", ipc.TestPipeSIDEnv, out.String())
	}
	if !strings.Contains(line, ipc.TestPipeSIDEnv) || !strings.Contains(line, "unset it") {
		t.Errorf("the pipe name line does not name the variable and the remedy: %q", line)
	}
	if strings.Contains(out.String(), "1001") {
		t.Errorf("the SID's value reached the report:\n%s", out.String())
	}
}

// TestDoctorLocalSaysNothingAboutThePipeSIDWhenItIsNotSet is the other half:
// the line is a finding, not a fixture, and an installed machine never sees it.
func TestDoctorLocalSaysNothingAboutThePipeSIDWhenItIsNotSet(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv(ipc.TestPipeSIDEnv, "")

	var out bytes.Buffer
	r := &report{w: &out}
	r.reportLocal()

	if line := pipeNameLine(out.String()); line != "" {
		t.Errorf("the local section reports a pipe name override that is not set: %q", line)
	}
}

// pipeNameLine returns the `pipe name` field of a report, or "" when there is
// none.
func pipeNameLine(report string) string {
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pipe name") {
			return line
		}
	}
	return ""
}
