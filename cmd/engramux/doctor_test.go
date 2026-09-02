package main

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/schedule"
)

// writeHooks puts a host configuration holding one entry per event in it into
// path, built the way an install builds it.
func writeHooks(t *testing.T, path, relay string, events []string, build func(name, relay string) jsontext.Value) {
	t.Helper()
	merged, err := host.MergeHooks([]byte(`{}`), events,
		func(event string) jsontext.Value { return build(event, relay) })
	if err != nil {
		t.Fatalf("MergeHooks: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, merged, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestReadOneHostHooksSeesEveryEventAnInstallWrote is the whole point of the
// section: what the installer writes, `doctor` reads back as wired.
func TestReadOneHostHooksSeesEveryEventAnInstallWrote(t *testing.T) {
	dir := t.TempDir()
	relay := filepath.Join(dir, "bin", "engramux.exe")
	path := filepath.Join(dir, "settings.json")
	writeHooks(t, path, relay, host.EventNames(), host.ClaudeEntry)

	got := readOneHostHooks("claude-code", path, relay)
	if got.err != nil || got.absent {
		t.Fatalf("readOneHostHooks: err=%v absent=%v", got.err, got.absent)
	}
	if !slices.Equal(got.wired, host.EventNames()) {
		t.Errorf("wired = %v, want all %d events", got.wired, len(host.EventNames()))
	}
	if len(got.missing) != 0 || len(got.stale) != 0 {
		t.Errorf("missing = %v, stale = %v, want neither", got.missing, got.stale)
	}
}

// TestReadOneHostHooksSplitsMissingFromStale. Both send the user to the same
// command and they are not the same finding, and a report that collapsed them
// would say "9 of 11" to a machine where all eleven fire into a deleted binary.
func TestReadOneHostHooksSplitsMissingFromStale(t *testing.T) {
	dir := t.TempDir()
	relay := filepath.Join(dir, "bin", "engramux.exe")
	old := filepath.Join(dir, "somewhere-else", "engramux.exe")
	path := filepath.Join(dir, "settings.json")

	// Two events written against the relay that is installed, two against one
	// that is not, and the remaining seven not written at all.
	current, stale := host.EventNames()[:2], host.EventNames()[2:4]
	merged, err := host.MergeHooks([]byte(`{}`), current,
		func(event string) jsontext.Value { return host.ClaudeEntry(event, relay) })
	if err != nil {
		t.Fatalf("MergeHooks: %v", err)
	}
	merged, err = host.MergeHooks(merged, stale,
		func(event string) jsontext.Value { return host.ClaudeEntry(event, old) })
	if err != nil {
		t.Fatalf("MergeHooks: %v", err)
	}
	if err := os.WriteFile(path, merged, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readOneHostHooks("claude-code", path, relay)
	if !slices.Equal(got.wired, current) {
		t.Errorf("wired = %v, want %v", got.wired, current)
	}
	if !slices.Equal(got.stale, stale) {
		t.Errorf("stale = %v, want %v", got.stale, stale)
	}
	if want := host.EventNames()[4:]; !slices.Equal(got.missing, want) {
		t.Errorf("missing = %v, want %v", got.missing, want)
	}
	// And the sentence a person reads has to carry both numbers, or the
	// distinction the three lists make never reaches them.
	trouble := got.trouble()
	if !strings.Contains(trouble, "2 pointing somewhere else") || !strings.Contains(trouble, "7 missing") {
		t.Errorf("trouble() = %q, want both counts", trouble)
	}
}

// TestReadOneHostHooksTreatsAnAbsentFileAsAnAbsentHost. host.PlanMerge skips a
// file that is not there rather than creating one, so an absent configuration
// means that host is not installed - and a doctor that failed on it would make
// every machine with only one of the two hosts red forever.
func TestReadOneHostHooksTreatsAnAbsentFileAsAnAbsentHost(t *testing.T) {
	got := readOneHostHooks("codex", filepath.Join(t.TempDir(), "hooks.json"), "irrelevant")
	if !got.absent {
		t.Errorf("absent = false for a file that is not there (err=%v)", got.err)
	}
	if got.err != nil {
		t.Errorf("err = %v, want nil", got.err)
	}
}

// TestReadOneHostHooksReportsADocumentItCannotParse. Reporting no entries would
// send the user to an installer that is going to refuse the same file.
func TestReadOneHostHooksReportsADocumentItCannotParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"hooks":`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := readOneHostHooks("claude-code", path, "irrelevant")
	if got.err == nil {
		t.Fatal("readOneHostHooks accepted a truncated document")
	}
	if len(got.missing) != 0 {
		t.Errorf("missing = %v; an unreadable file is not eleven missing events", got.missing)
	}
}

// TestNothingIsInstalledNeedsEverySignAbsent. The stage answer replaces four
// red sections with one instruction, so it may only fire when the instruction
// is right - a machine with any sign of an installation needs to be told what
// is broken, not to install what it already has.
func TestNothingIsInstalledNeedsEverySignAbsent(t *testing.T) {
	absent := []hostHooks{{label: "claude-code", absent: true}, {label: "codex", absent: true}}
	wired := []hostHooks{{label: "claude-code", wired: []string{"Stop"}}, {label: "codex", absent: true}}
	stale := []hostHooks{{label: "claude-code", stale: []string{"Stop"}}, {label: "codex", absent: true}}
	unreadable := []hostHooks{{label: "claude-code", err: errors.New("boom")}, {label: "codex", absent: true}}

	for _, tc := range []struct {
		name    string
		taskErr error
		hooks   []hostHooks
		binary  []string
		want    bool
	}{
		{"a machine with nothing on it", schedule.ErrNotRegistered, absent, nil, true},
		{"a task is registered", nil, absent, nil, false},
		{"the task could not be read", errors.New("schtasks exploded"), absent, nil, false},
		{"a binary is in place", schedule.ErrNotRegistered, absent, []string{host.RelayName}, false},
		{"a host is wired", schedule.ErrNotRegistered, wired, nil, false},
		{"a host is stale", schedule.ErrNotRegistered, stale, nil, false},
		{"a host could not be read", schedule.ErrNotRegistered, unreadable, nil, false},
	} {
		if got := nothingIsInstalled(tc.taskErr, tc.hooks, tc.binary); got != tc.want {
			t.Errorf("%s: nothingIsInstalled = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestDoctorTellsAFreshMachineToInstall is the stage answer end to end: one
// instruction and the command that carries it out, and none of the four
// sections that used to be printed instead.
//
// The environment is redirected the way install_test.go redirects it, and the
// task name is the positional argument - so this never touches the name a real
// installation owns and never reads a developer's own settings.json.
func TestDoctorTellsAFreshMachineToInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "Local"))
	t.Setenv("ENGRAMUX_CLAUDE_SETTINGS", filepath.Join(dir, "claude", "settings.json"))
	t.Setenv("ENGRAMUX_CODEX_HOOKS", filepath.Join(dir, "codex", "hooks.json"))

	var out bytes.Buffer
	// A name schedule.Query will accept and nothing has ever registered, so
	// the answer is ErrNotRegistered rather than a rejected name - which is a
	// different branch and would leave this test asserting nothing.
	code := runDoctor(&out, []string{`\Engramux-Doctor-Test-` + filepath.Base(dir)})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	text := out.String()
	if !strings.Contains(text, "not installed") {
		t.Errorf("the report does not say it is not installed:\n%s", text)
	}
	if !strings.Contains(text, "install --apply") {
		t.Errorf("the report does not name the command that fixes it:\n%s", text)
	}
	// The sections it replaces. Their absence is the change: printing them
	// alongside the instruction would be the behaviour this removes.
	for _, section := range []string{"\nservice\n", "\nmcp", "\nlocal\n"} {
		if strings.Contains(text, section) {
			t.Errorf("the fresh-machine answer still prints %q:\n%s", section, text)
		}
	}

	// And it is masked, through runDoctor rather than through a report a test
	// built. Nothing else in this package reaches the flag parsing, so
	// without this a runDoctor that ignored the flag and always printed the
	// real values would pass every test here - measured, in a break-it pass
	// that only the gate one process out caught.
	if !strings.Contains(dir, `\Users\`) {
		t.Skipf("the temporary directory %q is not under a user profile, so masking it proves nothing", dir)
	}
	if strings.Contains(text, dir) {
		t.Errorf("the default printed an unmasked user path:\n%s", text)
	}
	if !strings.Contains(text, "[redacted-user-path]") {
		t.Errorf("nothing in the default output was masked:\n%s", text)
	}

	// --full is the only thing that prints it whole.
	var fullOut bytes.Buffer
	if code := runDoctor(&fullOut, []string{`\Engramux-Doctor-Test-` + filepath.Base(dir), "--full"}); code != 1 {
		t.Errorf("--full changed the exit code to %d, want 1", code)
	}
	if !strings.Contains(fullOut.String(), dir) {
		t.Error("--full did not print the real paths")
	}
}

// TestDoctorMasksTheDatabasePathAndTheSIDUnlessFullIsGiven.
//
// This is the output a person pastes into a public issue. The masking is
// checked on the section that is readable with nothing running, because the
// service's own numbers need a service and this must hold on a machine that
// has none.
func TestDoctorMasksTheDatabasePathAndTheSIDUnlessFullIsGiven(t *testing.T) {
	const userPath = `C:\Users\somebody\AppData\Local\engramux\engramux.db`

	masked := &report{w: &bytes.Buffer{}}
	if got := masked.mask(userPath); strings.Contains(got, "somebody") {
		t.Errorf("mask(%q) = %q, and the user name survived", userPath, got)
	} else if !strings.Contains(got, "redacted") {
		t.Errorf("mask(%q) = %q, which says nothing was removed", userPath, got)
	}

	full := &report{w: &bytes.Buffer{}, full: true}
	if got := full.mask(userPath); got != userPath {
		t.Errorf("with --full, mask(%q) = %q; the real value is what --full is for", userPath, got)
	}
}

// TestDoctorFieldsGoThroughTheMask is the half above cannot prove: that the
// masking is on the path every printed line takes, rather than on a helper
// nothing calls.
func TestDoctorFieldsGoThroughTheMask(t *testing.T) {
	var out bytes.Buffer
	r := &report{w: &out}
	r.line(`starting %s`, `C:\Users\somebody\bin`)
	r.field("database", `%s`, `C:\Users\somebody\engramux.db`)
	r.fail("principal", `%s`, `C:\Users\somebody\anything`)

	if strings.Contains(out.String(), "somebody") {
		t.Errorf("a user name reached the writer:\n%s", out.String())
	}
	if !r.failed {
		t.Error("fail did not set failed, so a finding would exit 0")
	}
	if n := strings.Count(out.String(), "[redacted-user-path]"); n != 3 {
		t.Errorf("%d of 3 lines were masked:\n%s", n, out.String())
	}
}

// TestDoctorFullPrintsTheRealValues, through the same writer, so that the flag
// is wired to the printing and not only to the helper.
func TestDoctorFullPrintsTheRealValues(t *testing.T) {
	var out bytes.Buffer
	r := &report{w: &out, full: true}
	r.field("database", `%s`, `C:\Users\somebody\engramux.db`)

	if !strings.Contains(out.String(), `C:\Users\somebody\engramux.db`) {
		t.Errorf("--full did not print the real path:\n%s", out.String())
	}
}

// TestReportInstalledFailsOnlyOnTheInstallationsOwnFaults pins what the new
// section contributes to the exit code, which is the one thing about it that
// can be wrong in both directions.
func TestReportInstalledFailsOnlyOnTheInstallationsOwnFaults(t *testing.T) {
	all := host.EventNames()
	for _, tc := range []struct {
		name  string
		hooks []hostHooks
		bins  []string
		want  bool
	}{
		{
			"both hosts complete",
			[]hostHooks{{label: "claude-code", wired: all}, {label: "codex", wired: all}},
			[]string{host.RelayName, host.ServiceName}, false,
		},
		{
			"one host absent, the other complete",
			[]hostHooks{{label: "claude-code", wired: all}, {label: "codex", absent: true}},
			[]string{host.RelayName, host.ServiceName}, false,
		},
		{
			"an event is missing",
			[]hostHooks{{label: "claude-code", wired: all[1:], missing: all[:1]}, {label: "codex", absent: true}},
			[]string{host.RelayName, host.ServiceName}, true,
		},
		{
			"every event points at a relay that moved",
			[]hostHooks{{label: "claude-code", stale: all}, {label: "codex", absent: true}},
			[]string{host.RelayName, host.ServiceName}, true,
		},
		{
			"the relay itself is not installed",
			[]hostHooks{{label: "claude-code", wired: all}, {label: "codex", absent: true}},
			[]string{host.ServiceName}, true,
		},
		{
			"the service binary is not installed",
			[]hostHooks{{label: "claude-code", wired: all}, {label: "codex", absent: true}},
			[]string{host.RelayName}, true,
		},
	} {
		r := &report{w: &bytes.Buffer{}}
		r.reportInstalled(`C:\Local\engramux\bin`, `C:\Local\engramux\bin\engramux.exe`, tc.bins, tc.hooks)
		if r.failed != tc.want {
			t.Errorf("%s: failed = %v, want %v", tc.name, r.failed, tc.want)
		}
	}
}

// TestReportMCPNeverDecidesTheExitCode is memory spec M-6's second change, and
// it is asserted rather than described because the old behaviour was one
// `return false` away and nothing else would have caught its return.
//
// The endpoint cannot be published here - LOCALAPPDATA points at an empty
// directory - which is precisely the state that used to make a capture-only
// installation red.
func TestReportMCPNeverDecidesTheExitCode(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	var out bytes.Buffer
	r := &report{w: &out}
	r.reportMCP(t.Context(), filepath.Join(t.TempDir(), "claude-state.json"), filepath.Join(t.TempDir(), "config.toml"))

	if r.failed {
		t.Errorf("the MCP section failed the run:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "NOT PUBLISHED") {
		t.Errorf("the missing endpoint was not reported at all:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "optional") {
		t.Errorf("the section does not say it is optional:\n%s", out.String())
	}
}
