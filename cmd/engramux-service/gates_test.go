package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/schedule"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/spool"
)

// ---------------------------------------------------------------------------
// Running the other binary
// ---------------------------------------------------------------------------

// relay runs one hook relay over stdin, with a LOCALAPPDATA of the caller's
// choosing, and asserts what I-03 promises: exit 0 and nothing on stdout.
func relay(t *testing.T, local string, stdin []byte) {
	t.Helper()
	useTestPipeName(t)
	var stdout, stderr bytes.Buffer
	//nolint:gosec // G204: relayBin is the binary TestMain built
	cmd := exec.CommandContext(t.Context(), relayBin)
	cmd.Env = append(os.Environ(), "LOCALAPPDATA="+local)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("the relay exited non-zero, which I-03 forbids: %v (stderr: %s)", err, stderr.Bytes())
	}
	if stdout.Len() != 0 {
		t.Fatalf("the relay wrote %q on stdout, want nothing (spec 4.5)", stdout.Bytes())
	}
	if stderr.Len() != 0 {
		t.Logf("relay stderr: %s", stderr.Bytes())
	}
}

// cliResult is one `engramux <command>` run.
type cliResult struct {
	exit   int
	stdout string
	stderr string
}

// cli runs the CLI half of the relay binary with this process's LOCALAPPDATA,
// which is the real one. Every command whose answer comes over the pipe is
// unaffected by that.
func cli(t *testing.T, args ...string) cliResult {
	t.Helper()
	return cliIn(t, os.Getenv("LOCALAPPDATA"), args...)
}

// cliIn runs the CLI half of the relay binary with a LOCALAPPDATA of the
// caller's choosing.
//
// `doctor` is what needs it. Its local half reads files rather than asking the
// service - the spool, the log, and spec 5.6's mcp.json - so a `doctor` given a
// different LOCALAPPDATA from the service it is dialing reports one
// installation's pipe beside another's directory. Every other command here
// answers from the pipe alone and does not care.
func cliIn(t *testing.T, local string, args ...string) cliResult {
	t.Helper()
	return cliInWith(t, local, nil, args...)
}

// cliInWith is [cliIn] with extra environment, which `doctor` needs and nothing
// else does: its installation section reads the two host configuration files,
// and those default to the ones under the developer's own home directory.
// Without an override this asserts against whatever that machine happens to
// have installed.
func cliInWith(t *testing.T, local string, env []string, args ...string) cliResult {
	t.Helper()
	// `status` and `doctor` dial the derived name; the CLI inherits this
	// process's environment, so it reaches the same service start did.
	useTestPipeName(t)
	var stdout, stderr bytes.Buffer
	//nolint:gosec // G204: relayBin is the binary TestMain built, args are the caller's literals
	cmd := exec.CommandContext(t.Context(), relayBin, args...)
	cmd.Env = append(append(os.Environ(), "LOCALAPPDATA="+local), env...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("run the cli: %v", err)
	}
	if cmd.ProcessState == nil {
		t.Fatalf("run the cli: no process state")
	}
	res := cliResult{exit: cmd.ProcessState.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
	t.Logf("engramux %v: exit=%d\nstdout: %s\nstderr: %s", args, res.exit, res.stdout, res.stderr)
	return res
}

// spoolDirOf is the spool directory a process given this LOCALAPPDATA writes to.
func spoolDirOf(local string) string { return filepath.Join(local, "engramux", "spool") }

// spooledID returns the id of the single record in dir, or fails.
func spooledID(t *testing.T, dir string) string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	if len(names) != 1 {
		t.Fatalf("%s holds %q, want exactly one record", dir, names)
	}
	return strings.TrimSuffix(filepath.Base(names[0]), ".json")
}

// event is one row of the events table.
type event struct {
	id        string
	source    string
	host      string
	eventName string
	payload   string
}

// events reads every row, oldest first by id.
func events(t *testing.T, db *sql.DB) []event {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT id, source, host, event_name, payload FROM events ORDER BY id`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close: %v", err)
		}
	}()
	var out []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.id, &e.source, &e.host, &e.eventName, &e.payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

// sessionEndFixture is the fixture the end-to-end tests send: exactly the bytes
// a host writes to the hook's stdin, trailing newline and all.
func sessionEndFixture(t *testing.T) (raw, want []byte) {
	t.Helper()
	raw, err := fixtures.Fixture{File: fixtures.CodexSessionEnd}.Bytes()
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	if raw[len(raw)-1] != '\n' {
		t.Fatalf("%s does not end in a newline, so the framing assertion below tests nothing",
			fixtures.CodexSessionEnd)
	}
	// want is written down rather than computed by calling the relay's own
	// trimFraming, so the expectation is not the implementation's opinion of
	// itself.
	return raw, raw[:len(raw)-1]
}

// ---------------------------------------------------------------------------
// Gate 1: end to end, for the first time
// ---------------------------------------------------------------------------

// TestARelayedEventLandsInTheDatabase is the whole product in one test: a hook
// event on a real relay's stdin, over the pipe, into the database a real
// service opened.
//
// Nothing before this task ran both halves in one process tree. Every earlier
// end-to-end test stood a pipe server up inside the test binary, which is the
// one arrangement that cannot catch a service that never listens, never
// migrates, or never wires the two together.
//
// The relay gets a LOCALAPPDATA of its own so that its spool is not the one the
// service drains: an empty spool then means the event was delivered, and not
// that something replayed it a moment later.
func TestARelayedEventLandsInTheDatabase(t *testing.T) {
	svc := start(t, t.TempDir())
	relayLocal := t.TempDir()
	raw, want := sessionEndFixture(t)

	relay(t, relayLocal, raw)

	// Delivered, therefore not spooled. A relay that failed to deliver also
	// exits 0 (I-03), so this is what tells the two apart.
	if names, err := filepath.Glob(filepath.Join(spoolDirOf(relayLocal), "*")); err != nil {
		t.Fatalf("glob the relay's spool: %v", err)
	} else if len(names) != 0 {
		t.Fatalf("the relay spooled %q instead of delivering it", names)
	}

	svc.stop(t)
	rows := events(t, svc.openDB(t))
	if len(rows) != 1 {
		t.Fatalf("the database holds %d events, want 1", len(rows))
	}
	got := rows[0]
	if got.source != "pipe" {
		t.Errorf("source = %q, want %q - it arrived some other way", got.source, "pipe")
	}
	if got.host != "codex" || got.eventName != "SessionEnd" {
		t.Errorf("(host, event_name) = (%q, %q), want (%q, %q)",
			got.host, got.eventName, "codex", "SessionEnd")
	}
	// Byte-identical. A payload that was unmarshalled and re-marshalled
	// anywhere on this path round-trips happily while reordering keys and
	// re-encoding numbers.
	if !bytes.Equal([]byte(got.payload), want) {
		t.Errorf("the stored payload is not the bytes the host wrote\n got (%d): %q\nwant (%d): %q",
			len(got.payload), got.payload, len(want), want)
	}
}

// ---------------------------------------------------------------------------
// Gate 3: the singleton is the pipe, not the database
// ---------------------------------------------------------------------------

// TestASecondInstanceIsRefusedByThePipe is the startup order's whole reason for
// being (I-09).
//
// Both instances are given the same directory, which is the real case: one
// Windows user, one set of paths. Both resources are therefore contested, and
// the assertion is about *which one* refuses. Opening the database first would
// also produce exactly one running service - and would say so with "database is
// locked", which is a true statement and a confusing way to tell a person that
// their service is already up.
func TestASecondInstanceIsRefusedByThePipe(t *testing.T) {
	local := t.TempDir()
	start(t, local)

	var out bytes.Buffer
	//nolint:gosec // G204: serviceBin is the binary TestMain built
	second := exec.CommandContext(t.Context(), serviceBin)
	second.Env = append(os.Environ(), "LOCALAPPDATA="+local)
	second.Stdout = &out
	second.Stderr = &out

	err := second.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("the second instance returned %v, want a non-zero exit - two services were running", err)
	}
	said := out.String()
	t.Logf("second instance: exit=%d\n%s", exitErr.ExitCode(), said)

	if !strings.Contains(said, `\\.\pipe\engramux.v1`) {
		t.Errorf("the refusal does not name the pipe:\n%s", said)
	}
	if strings.Contains(said, "database is locked") {
		t.Errorf("the refusal came from the database, not the pipe - the startup order is wrong:\n%s", said)
	}
}

// ---------------------------------------------------------------------------
// Gate 4: an event spooled while the service was down is drained
// ---------------------------------------------------------------------------

// TestAnEventSpooledWhileDownIsDrainedAtStartup is the other half of I-04. The
// relay saves what it cannot deliver; this is the half that gives it back.
//
// The relay and the service share one LOCALAPPDATA here, because that is the
// arrangement that makes the spool one directory rather than two - and a
// service draining a directory the relay does not write to is the failure this
// test exists to catch.
//
// The id is read off the spool file before the service starts and asserted
// against events.id afterwards. Two rows are impossible to tell from one
// replayed row without it: a drain that minted a fresh id would leave a
// perfectly good event under an id nothing can reconcile with the relay's
// (I-05).
func TestAnEventSpooledWhileDownIsDrainedAtStartup(t *testing.T) {
	local := t.TempDir()
	raw, want := sessionEndFixture(t)

	// Nothing is listening, so the relay spools.
	claimAFreePipeName(t)
	relay(t, local, raw)
	spooled := spooledID(t, spoolDirOf(local))

	svc := start(t, local)
	waitForEmptySpool(t, spoolDirOf(local))
	svc.stop(t)

	rows := events(t, svc.openDB(t))
	if len(rows) != 1 {
		t.Fatalf("the database holds %d events, want 1", len(rows))
	}
	got := rows[0]
	if got.id != spooled {
		t.Errorf("events.id = %q, want the id the relay minted %q - a re-minted id is a second row (I-05)",
			got.id, spooled)
	}
	if got.source != "spool" {
		t.Errorf("source = %q, want %q", got.source, "spool")
	}
	if !bytes.Equal([]byte(got.payload), want) {
		t.Errorf("the replayed payload is not the bytes the host wrote\n got: %q\nwant: %q", got.payload, want)
	}
}

// waitForEmptySpool waits until the drain has consumed every record.
func waitForEmptySpool(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		n, err := spool.Depth(dir)
		if err != nil {
			t.Fatalf("spool.Depth: %v", err)
		}
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still holds %d records after 20s: nothing drained them", dir, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// Gate 5: Status
// ---------------------------------------------------------------------------

// TestStatusReportsWhatIsActuallyThere is I-08 end to end: the numbers come
// over the pipe, because I-07 leaves no other way to read them.
//
// Every number is asserted against something this test put there, not against
// itself. The spool records are written directly rather than relayed, so that
// the count is one this test chose; they survive to be counted because the
// drain runs on a 30 s interval and this test finishes inside a second.
func TestStatusReportsWhatIsActuallyThere(t *testing.T) {
	local := t.TempDir()
	svc := start(t, local)

	const wantEvents = 3
	for i := range wantEvents {
		ingest(t, fmt.Sprintf("0192f0c0-0000-7000-8000-0000000005%02d", i),
			[]byte(`{"hook_event_name":"SessionEnd","session_id":"s","cwd":"Z:\\status\\project","model":"m"}`))
	}
	const wantSpool = 2
	for i := range wantSpool {
		id := fmt.Sprintf("0192f0c0-0000-7000-8000-0000000006%02d", i)
		if err := spool.Write(spoolDirOf(local), id, []byte(`{"hook_event_name":"Stop"}`), nil); err != nil {
			t.Fatalf("write a spool record: %v", err)
		}
	}

	var reply ipc.StatusReply
	if err := json.Unmarshal(send(t, request(t, ipc.Version, ipc.Status, "", nil)), &reply); err != nil {
		t.Fatalf("decode the status reply: %v", err)
	}
	if err := reply.Verify(); err != nil {
		t.Fatalf("the service did not answer a status reply: %v", err)
	}

	if reply.Events != wantEvents {
		t.Errorf("events = %d, want %d", reply.Events, wantEvents)
	}
	if reply.SpoolDepth != wantSpool {
		t.Errorf("spool_depth = %d, want %d", reply.SpoolDepth, wantSpool)
	}
	// The masked spelling of the file the service opened, not the file
	// (spec 5.9). Equality still pins which database it is: masking is a
	// function of the path, so the expected value is derived from the same
	// directory this test handed the service rather than from a literal.
	//
	// Under a temporary directory two classes fire, and both are the point.
	// ClassUserPath takes the profile name out of %LOCALAPPDATA%, which is
	// the one this clause exists for; ClassOpaque takes the test's own
	// directory name, which is 40-odd alphanumerics because t.TempDir names
	// it after the test.
	if want := maskedDatabasePath(local); reply.DatabasePath != want {
		t.Errorf("database_path = %q, want %q", reply.DatabasePath, want)
	}
	// Uptime is a real measurement, not a zero and not a start instant read
	// off the wrong clock. The upper bound is what catches the latter.
	if reply.UptimeMS <= 0 || reply.UptimeMS > time.Minute.Milliseconds() {
		t.Errorf("uptime_ms = %d, want a plausible age for a service this test just started", reply.UptimeMS)
	}

	// The CLI prints what the pipe said. The numbers are checked here as
	// well as above because the reply and the output are two different
	// things to get wrong.
	res := cli(t, "status")
	if res.exit != 0 {
		t.Fatalf("engramux status exited %d, want 0", res.exit)
	}
	for _, want := range []string{
		fmt.Sprintf("events      %d", wantEvents),
		fmt.Sprintf("spool       %d", wantSpool),
		// Backlog 31's two lines, as a freshly started service prints
		// them: nothing logged at ERROR, and no checkpoint yet.
		"errors      0",
		"checkpoint  none yet",
		maskedDatabasePath(local),
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("engramux status did not print %q:\n%s", want, res.stdout)
		}
	}

	svc.stop(t)

	// With the service down there is no read path at all (I-08), so this has
	// to fail rather than fall back to the database.
	down := cli(t, "status")
	if down.exit == 0 {
		t.Errorf("engramux status exited 0 with no service running:\n%s", down.stdout)
	}
	if !strings.Contains(down.stderr, `\\.\pipe\engramux.v1`) {
		t.Errorf("the failure does not say what could not be reached:\n%s", down.stderr)
	}
}

// TestTheCellBreakdownTravelsOverThePipe is the breakdown's half of I-08, at
// the process level: the counts are read out of a database no other process can
// open (I-07), so the pipe is the only thing that could have carried them.
//
// The expected counts are the events this test ingested, not a second query.
// Three hosts on purpose - `unknown` is reachable and is not an error (I-04),
// and it is the value most likely to be quietly dropped by a reader that
// assumes the two real hosts.
func TestTheCellBreakdownTravelsOverThePipe(t *testing.T) {
	local := t.TempDir()
	svc := start(t, local)

	// One payload per host, chosen so each takes a different branch of spec
	// 4.3: prompt_id is step 1, model is step 2, and the third carries
	// neither and no transcript_path, so it classifies as unknown.
	const cwd = `"cwd":"Z:\\cells\\project"`
	payloads := []struct {
		host, event string
		body        string
	}{
		{"claude-code", "PostToolUse", `{"hook_event_name":"PostToolUse","session_id":"cc",` + cwd + `,"prompt_id":"p1"}`},
		{"claude-code", "PostToolUse", `{"hook_event_name":"PostToolUse","session_id":"cc",` + cwd + `,"prompt_id":"p2"}`},
		{"codex", "SessionEnd", `{"hook_event_name":"SessionEnd","session_id":"cx",` + cwd + `,"model":"m"}`},
		{"unknown", "Stop", `{"hook_event_name":"Stop","session_id":"uk",` + cwd + `}`},
	}
	want := map[string]int64{}
	before := time.Now()
	for i, p := range payloads {
		ingest(t, fmt.Sprintf("0192f0c0-0000-7000-8000-0000000007%02d", i), []byte(p.body))
		want[p.host+" "+p.event]++
	}

	var reply ipc.StatusReply
	if err := json.Unmarshal(send(t, request(t, ipc.Version, ipc.Status, "", nil)), &reply); err != nil {
		t.Fatalf("decode the status reply: %v", err)
	}
	if err := reply.Verify(); err != nil {
		t.Fatalf("the service did not answer a status reply: %v", err)
	}

	got := map[string]int64{}
	for _, c := range reply.Cells {
		if c.Count == 0 {
			t.Errorf("cell %s/%q has count 0; an empty cell is absent, not zero", c.Host, c.EventName)
		}
		// The span brackets the ingests this test made, which is what
		// says these are the row timestamps and not, say, a clock read
		// when the reply was built.
		if c.FirstSeenMS > c.LastSeenMS ||
			c.FirstSeenMS < before.UnixMilli() || c.LastSeenMS > time.Now().UnixMilli() {
			t.Errorf("cell %s/%q spans %d..%d, which is not inside this test's run",
				c.Host, c.EventName, c.FirstSeenMS, c.LastSeenMS)
		}
		got[c.Host+" "+c.EventName] = c.Count
	}
	if !maps.Equal(got, want) {
		t.Errorf("the breakdown is not what this test ingested\n got %v\nwant %v", got, want)
	}

	// The CLI prints what the pipe said, and it is a second command sending
	// the same Status request - spec 5.2 fixes the request set at five types.
	res := cli(t, "cells")
	if res.exit != 0 {
		t.Fatalf("engramux cells exited %d, want 0", res.exit)
	}
	// Parsed by field rather than by column position, so this asserts what
	// was printed and not how it was padded.
	printed := map[string]string{}
	for _, line := range strings.Split(res.stdout, "\n")[1:] {
		if f := strings.Fields(line); len(f) >= 3 {
			printed[f[0]+" "+f[1]] = f[2]
		}
	}
	for cell, n := range want {
		host, event, _ := strings.Cut(cell, " ")
		// The event name is quoted on the way out, so that an event with
		// no name is visible rather than blank.
		key := host + ` "` + event + `"`
		if printed[key] != strconv.FormatInt(n, 10) {
			t.Errorf("engramux cells printed %q for %s, want %d:\n%s", printed[key], key, n, res.stdout)
		}
	}

	svc.stop(t)

	// With the service down there is no read path at all (I-08), so this
	// fails rather than falling back to the database - the same contract
	// `status` holds, because it is the same request.
	down := cli(t, "cells")
	if down.exit == 0 {
		t.Errorf("engramux cells exited 0 with no service running:\n%s", down.stdout)
	}
	if !strings.Contains(down.stderr, `\\.\pipe\engramux.v1`) {
		t.Errorf("the failure does not say what could not be reached:\n%s", down.stderr)
	}
}

// ---------------------------------------------------------------------------
// Gate 6: doctor's two halves, which do not need the same things
// ---------------------------------------------------------------------------

// scheduledProbe registers a scheduled task under a name no real install uses,
// and removes it when the test ends - including when the test fails partway
// through, which is the only reason this is a helper rather than four lines.
//
// The suffix is random rather than fixed: two runs overlapping would otherwise
// fight over one registration and the loser would delete the winner's task.
func scheduledProbe(t *testing.T, exe string) string {
	t.Helper()
	name := `\Engramux-test-` + rand.Text()
	t.Cleanup(func() {
		// Its own context: t.Context is already cancelled when cleanups
		// run, and a cleanup that cannot reach schtasks would leave a
		// scheduled task on the developer's machine.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := schedule.Unregister(ctx, name); err != nil {
			t.Errorf("unregister %s: %v - it is still on this machine", name, err)
		}
	})
	if err := schedule.Register(t.Context(), name, exe); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	return name
}

// installedTree makes local look like a finished installation and returns the
// environment that points the CLI at it: both binaries where `install` copies
// them, and both hosts' hook tables holding the eleven entries pointed at that
// relay.
//
// It exists because memory spec M-6 made `doctor` judge by stage. A temporary
// directory with a service running in it is, correctly, a machine with no
// installation on it - which is the right answer to that directory and the
// wrong question for a gate about whether `doctor`'s two halves are readable
// independently. This is what makes "everything in place" mean what the gate
// says it means.
//
// The binaries are copied rather than stubbed. An installation's are copies of
// exactly these two, so a fixture that wrote empty files would be asserting
// against a shape no install produces.
func installedTree(t *testing.T, local string) []string {
	t.Helper()

	bin := filepath.Join(local, "engramux", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatalf("create %s: %v", bin, err)
	}
	for _, src := range []string{relayBin, serviceBin} {
		body, err := os.ReadFile(src) //nolint:gosec // G304: a binary TestMain built.
		if err != nil {
			t.Fatalf("read %s: %v", src, err)
		}
		if err := os.WriteFile(filepath.Join(bin, filepath.Base(src)), body, 0o700); err != nil { //nolint:gosec // G302: an executable.
			t.Fatalf("copy %s: %v", src, err)
		}
	}

	relay := filepath.Join(bin, host.RelayName)
	env := make([]string, 0, 2)
	for _, h := range []struct {
		key, name string
		build     func(event, relay string) jsontext.Value
	}{
		{"ENGRAMUX_CLAUDE_SETTINGS", "settings.json", host.ClaudeEntry},
		{"ENGRAMUX_CODEX_HOOKS", "hooks.json", host.CodexEntry},
	} {
		merged, err := host.MergeHooks([]byte(`{}`), host.EventNames(),
			func(event string) jsontext.Value { return h.build(event, relay) })
		if err != nil {
			t.Fatalf("merge %s: %v", h.name, err)
		}
		path := filepath.Join(local, h.name)
		if err := os.WriteFile(path, merged, 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		env = append(env, h.key+"="+path)
	}
	return env
}

// principalSID is the SID a task registered by this test runs as. It is what
// schedule.Register writes, read from the same place, so a mismatch here means
// the registration really did name another account.
//
// It is never printed: the assertions using it report only that a value was or
// was not there.
func principalSID(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	return u.Uid
}

// maskedDatabasePath is the value a status reply carries for a service running
// out of local: spec 5.6's file, through spec 5.9's mask.
//
// It is derived rather than written out, so this holds on a machine whose
// profile is spelled differently and on one whose temporary directory is not
// under a profile at all. It also keeps a failure message free of the real
// path, which is what the unmasked expectation used to print.
func maskedDatabasePath(local string) string {
	return secret.MaskString(filepath.Join(local, "engramux", "engramux.db"))
}

// TestDoctorReadsTheTaskWhetherOrNotTheServiceIsUp is spec 8's Phase 3 [manual]
// gate turned into an [auto] one, and spec 10's first open question answered by
// the code rather than by a sentence.
//
// `doctor` has two halves with different availability: the task registration is
// a Windows query that needs no service at all, and the counts are only
// reachable over the pipe (I-07 leaves no other way to read them). So the
// service being down does not make the command useless, and this asserts both
// halves twice - once with the service up, once with it stopped.
//
// The settings are asserted by value: PT0S rather than "an ExecutionTimeLimit
// element exists", PT1M and 3 rather than "a restart policy". LeastPrivilege is
// the one value Windows never sends back - it normalises away an element equal
// to its default - so seeing it printed is the readback treating absence as the
// default rather than as a fault.
//
// It is pointed at a test-only registration. A test must never touch the name a
// real install owns, and `doctor` takes the name for exactly that reason.
func TestDoctorReadsTheTaskWhetherOrNotTheServiceIsUp(t *testing.T) {
	local := t.TempDir()
	env := installedTree(t, local)
	svc := start(t, local)
	name := scheduledProbe(t, serviceBin)

	// Everything spec 5.5 fixes, spelled as spec 5.5 spells it, plus the
	// principal: the interactive user's token, not SYSTEM and not elevated.
	//
	// serviceBin is not in this list: it is a path under the developer's own
	// profile, so the masked default rewrites it and only --full prints it
	// whole. It is asserted below, in both forms.
	registration := []string{
		name,
		"PT0S",
		"IgnoreNew",
		"3 times, one every PT1M",
		"InteractiveToken",
		"LeastPrivilege",
	}

	up := cliInWith(t, local, env, "doctor", name)
	if up.exit != 0 {
		t.Fatalf("engramux doctor exited %d with everything in place, want 0:\n%s\n%s", up.exit, up.stdout, up.stderr)
	}
	// The eleven entries in both hosts, checked against the installed relay -
	// memory spec M-6's third change and the one surface a working capture
	// actually depends on.
	//
	// The tokenizer line is asserted as the *comparison* and not as the
	// tokenizer string. What this command is for here is answering whether
	// the live index and the migration agree - goose does not checksum a
	// migration, so nothing else in the product could answer it at all.
	events := fmt.Sprintf("%d of %d events point at the installed relay", len(host.EventNames()), len(host.EventNames()))
	for _, want := range append(registration,
		"agrees with the migration",
		events,
		"this user",
		secret.MaskString(serviceBin),
	) {
		if !strings.Contains(up.stdout, want) {
			t.Errorf("engramux doctor did not report %q:\n%s", want, up.stdout)
		}
	}
	// The default is masked, and this is the output a person pastes into a
	// public issue: the real database path is spec 5.9's, handed to this
	// command and to nothing else, and the principal is a Windows SID.
	for _, leak := range []string{
		filepath.Join(local, "engramux", "engramux.db"),
		principalSID(t),
	} {
		if strings.Contains(up.stdout, leak) {
			t.Errorf("engramux doctor printed a value the default masks:\n%s", up.stdout)
		}
	}
	if !strings.Contains(up.stdout, maskedDatabasePath(local)) {
		t.Errorf("engramux doctor did not report the masked database path:\n%s", up.stdout)
	}

	// --full is what gets the real values, and it is the only thing that
	// does. Nothing here is printed on failure: that is what it is for.
	full := cliInWith(t, local, env, "doctor", name, "--full")
	if full.exit != 0 {
		t.Fatalf("engramux doctor --full exited %d, want 0", full.exit)
	}
	if !strings.Contains(full.stdout, filepath.Join(local, "engramux", "engramux.db")) {
		t.Error("engramux doctor --full did not print the real database path")
	}
	if !strings.Contains(full.stdout, principalSID(t)) {
		t.Error("engramux doctor --full did not print the task principal")
	}
	if !strings.Contains(full.stdout, serviceBin) {
		t.Error("engramux doctor --full did not print the registered command whole")
	}

	svc.stop(t)

	down := cliInWith(t, local, env, "doctor", name)
	if down.exit == 0 {
		t.Errorf("engramux doctor exited 0 with no service running:\n%s", down.stdout)
	}
	// Still reports the halves that never needed the service - the
	// registration, and the local state spec 5.5 added to this command
	// because a service that is down is when it is run. A command that gave
	// up at the first failure would print none of it.
	for _, want := range append(registration,
		secret.MaskString(serviceBin), "local", "this binary", "spool", "last log line") {
		if !strings.Contains(down.stdout, want) {
			t.Errorf("engramux doctor stopped reporting %q once the service was down:\n%s", want, down.stdout)
		}
	}
	// And says plainly what it could not read, by name.
	if !strings.Contains(down.stdout, `\\.\pipe\engramux.v1`) {
		t.Errorf("engramux doctor does not say what it could not reach:\n%s", down.stdout)
	}
}

// TestRegisterAndUnregisterFromTheCommandLine is provisioning at the level a
// user meets it, and gate 4 - unregister leaves nothing behind, and running it
// twice is not an error - one process out from the package that implements it.
//
// It is also the only thing that exercises how `register` finds the binary it
// registers. TestMain builds the two into one directory under the names they
// ship as, which is exactly the arrangement that lookup resolves against: a
// registration pointing anywhere else would be a task that fails silently at
// every logon.
func TestRegisterAndUnregisterFromTheCommandLine(t *testing.T) {
	name := `\Engramux-test-` + rand.Text()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := schedule.Unregister(ctx, name); err != nil {
			t.Errorf("unregister %s: %v - it is still on this machine", name, err)
		}
	})

	if res := cli(t, "register", name); res.exit != 0 {
		t.Fatalf("engramux register exited %d, want 0:\n%s", res.exit, res.stderr)
	}
	got, err := schedule.Query(t.Context(), name)
	if err != nil {
		t.Fatalf("query %s: %v", name, err)
	}
	if got.Command != serviceBin {
		t.Errorf("the registered command is %q, want the service binary beside the CLI %q", got.Command, serviceBin)
	}

	// Registering twice is spec 5.5's upgrade path - drain, stop, replace,
	// start - where the binary moves and the user runs this again.
	if res := cli(t, "register", name); res.exit != 0 {
		t.Errorf("engramux register over an existing registration exited %d, want 0:\n%s", res.exit, res.stderr)
	}

	if res := cli(t, "unregister", name); res.exit != 0 {
		t.Fatalf("engramux unregister exited %d, want 0:\n%s", res.exit, res.stderr)
	}
	if _, err := schedule.Query(t.Context(), name); !errors.Is(err, schedule.ErrNotRegistered) {
		t.Errorf("the task still answers a query after unregister: %v", err)
	}
	// Twice is not an error, so that nobody has to remember whether they
	// installed it. The cleanup above makes it three times.
	if res := cli(t, "unregister", name); res.exit != 0 {
		t.Errorf("engramux unregister a second time exited %d, want 0:\n%s", res.exit, res.stderr)
	}
}

// ---------------------------------------------------------------------------
// Gate 7: spec 8's Phase 3 [auto] gate - 30 concurrent starts leave one service
// ---------------------------------------------------------------------------

// TestThirtyConcurrentStartsLeaveOneService is I-09 at the process level.
//
// internal/pipe already measures ListenPipe's exclusivity at 20 rounds x 30
// processes, but that is the listener on its own. This is the service binary,
// which also opens the database exclusively (I-07) - so it is also the test
// that says *which* resource refuses the other 29. "database is locked" would
// produce exactly one survivor too, and would be a confusing way to tell a
// person their service is already running (spec 5.4, and the startup order in
// internal/service).
//
// All 30 are released from one barrier so that they genuinely overlap. A loop
// that started them one at a time would leave the first one holding the pipe
// before the second was created, and would prove nothing about a race - the
// spread below is logged and bounded so that a change back to that shape fails
// here rather than passing quietly.
func TestThirtyConcurrentStartsLeaveOneService(t *testing.T) {
	const n = 30
	local := t.TempDir()
	claimAFreePipeName(t)

	type instance struct {
		cmd *exec.Cmd
		out *bytes.Buffer
	}
	insts := make([]*instance, n)
	for i := range insts {
		var out bytes.Buffer
		//nolint:gosec // G204: serviceBin is the binary TestMain built
		cmd := exec.CommandContext(t.Context(), serviceBin)
		cmd.Env = append(os.Environ(), "LOCALAPPDATA="+local)
		cmd.Stdout, cmd.Stderr = &out, &out
		insts[i] = &instance{cmd: cmd, out: &out}
	}

	// One goroutine per instance, all parked on gate, so that the 30
	// CreateProcess calls are issued together rather than in sequence.
	var ready, launched sync.WaitGroup
	ready.Add(n)
	launched.Add(n)
	gate := make(chan struct{})
	startErr := make([]error, n)
	startedAt := make([]time.Time, n)
	for i, in := range insts {
		go func() {
			defer launched.Done()
			ready.Done()
			<-gate
			startErr[i] = in.cmd.Start()
			startedAt[i] = time.Now()
		}()
	}
	ready.Wait()
	close(gate)
	launched.Wait()

	for i := range insts {
		if startErr[i] != nil {
			t.Fatalf("instance %d did not start at all: %v", i, startErr[i])
		}
	}
	first, last := startedAt[0], startedAt[0]
	for _, at := range startedAt[1:] {
		if at.Before(first) {
			first = at
		}
		if at.After(last) {
			last = at
		}
	}
	spread := last.Sub(first)
	t.Logf("30 starts issued over %s", spread)
	if spread > 10*time.Second {
		t.Fatalf("the 30 starts were spread over %s, so they did not race and this test proves nothing", spread)
	}

	type exit struct {
		i   int
		err error
	}
	done := make(chan exit, n)
	for i, in := range insts {
		go func() { done <- exit{i, in.cmd.Wait()} }()
	}

	// Nothing survives this test. A leaked service is a process holding a
	// database and a pipe for the rest of the run, and since every test now
	// derives a pipe name of its own it would no longer announce itself by
	// failing the next one - so the cleanup is the only thing that catches it.
	consumed := 0
	t.Cleanup(func() {
		for _, in := range insts {
			if in.cmd.Process != nil {
				_ = in.cmd.Process.Kill()
			}
		}
		for consumed < n {
			select {
			case <-done:
				consumed++
			case <-time.After(60 * time.Second):
				t.Errorf("%d of %d instances never exited", n-consumed, n)
				return
			}
		}
	})

	losers := map[int]error{}
	deadline := time.After(120 * time.Second)
	for len(losers) < n-1 {
		select {
		case e := <-done:
			consumed++
			losers[e.i] = e.err
		case <-deadline:
			t.Fatalf("%d of the %d instances are still running: the singleton did not hold", n-len(losers), n)
		}
	}

	// One is not none. A build where every instance refused itself would
	// satisfy every assertion below and leave nothing capturing anything.
	select {
	case e := <-done:
		consumed++
		t.Fatalf("every instance exited, so no service is running (instance %d: %v)\n%s",
			e.i, e.err, insts[e.i].out.Bytes())
	case <-time.After(2 * time.Second):
	}
	if !servingOK(t) {
		t.Errorf("the surviving instance does not answer a Status request on %s - it is a process, not a service", pipeName(t))
	}

	for i, err := range losers {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Errorf("instance %d returned %v, want a non-zero exit", i, err)
			continue
		}
		// Reading the buffer is safe: this instance's Wait has returned,
		// so os/exec's copying goroutine has finished with it.
		said := insts[i].out.String()
		if !strings.Contains(said, `\\.\pipe\engramux.v1`) {
			t.Errorf("instance %d exited %d without naming the pipe:\n%s", i, exitErr.ExitCode(), said)
		}
		if strings.Contains(said, "database is locked") {
			t.Errorf("instance %d was refused by the database rather than the pipe:\n%s", i, said)
		}
	}
}

// TestAnUnknownCommandIsRefused. The relay path is the argument-free one, so a
// typo must not fall through to it and spool an event nobody sent.
func TestAnUnknownCommandIsRefused(t *testing.T) {
	res := cli(t, "statsu")
	if res.exit == 0 {
		t.Errorf("exit = 0 for an unknown command, want non-zero")
	}
	if !strings.Contains(res.stderr, "statsu") {
		t.Errorf("stderr does not name the command that was refused:\n%s", res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("stdout = %q, want nothing", res.stdout)
	}
}

// ---------------------------------------------------------------------------
// The other three request types
// ---------------------------------------------------------------------------

// TestTheUnimplementedRequestTypesAreRejected pins what this build does not do:
// a request type it does not serve is answered with a rejection rather than
// anything a caller could mistake for an empty result.
//
// The list used to hold Doctor, Search and Drain. Search was implemented in
// Phase 4 and Doctor in Phase 5, and Drain was withdrawn from spec 5.2 on
// 2026-08-30 (backlog 32) - so it is sent here as the string an old relay might
// still spell, which this build no longer knows as a type at all.
func TestTheUnimplementedRequestTypesAreRejected(t *testing.T) {
	start(t, t.TempDir())
	for _, typ := range []ipc.RequestType{"Drain"} {
		t.Run(string(typ), func(t *testing.T) {
			raw := send(t, request(t, ipc.Version, typ, "", nil))
			var ack ipc.Ack
			if err := json.Unmarshal(raw, &ack); err != nil {
				t.Fatalf("decode the reply: %v", err)
			}
			if ack.Status != ipc.Rejected {
				t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
			}
			// And it must not read as a status reply, or a client that
			// asked for one would print zeroes.
			var reply ipc.StatusReply
			if err := json.Unmarshal(raw, &reply); err != nil {
				t.Fatalf("decode the reply as a status reply: %v", err)
			}
			if err := reply.Verify(); !errors.Is(err, ipc.ErrStatusType) {
				t.Errorf("StatusReply.Verify = %v, want ErrStatusType", err)
			}
		})
	}
}
