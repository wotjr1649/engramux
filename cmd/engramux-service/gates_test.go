package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/spool"
)

// ---------------------------------------------------------------------------
// Running the other binary
// ---------------------------------------------------------------------------

// relay runs one hook relay over stdin, with a LOCALAPPDATA of the caller's
// choosing, and asserts what I-03 promises: exit 0 and nothing on stdout.
func relay(t *testing.T, local string, stdin []byte) {
	t.Helper()
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

// cli runs the CLI half of the relay binary.
func cli(t *testing.T, args ...string) cliResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	//nolint:gosec // G204: relayBin is the binary TestMain built, args are the caller's literals
	cmd := exec.CommandContext(t.Context(), relayBin, args...)
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
	requirePipeFree(t)
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
	if want := filepath.Join(local, "engramux", "engramux.db"); reply.DatabasePath != want {
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
		fmt.Sprintf("events    %d", wantEvents),
		fmt.Sprintf("spool     %d", wantSpool),
		filepath.Join(local, "engramux", "engramux.db"),
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

// TestTheUnimplementedRequestTypesAreRejected pins what Phase 1 does not do.
// Doctor, Search and Drain are spec 5.2 types with no implementation, and the
// answer has to be a rejection rather than anything a caller could mistake for
// an empty result.
func TestTheUnimplementedRequestTypesAreRejected(t *testing.T) {
	start(t, t.TempDir())
	for _, typ := range []ipc.RequestType{ipc.Doctor, ipc.Search, ipc.Drain} {
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
