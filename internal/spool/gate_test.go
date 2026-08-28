package spool

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
	"github.com/wotjr1649/engramux/internal/store"
)

// The ingest ids this run uses, one range per clause, so every row in the
// database says which clause wrote it.
const (
	gateFixtureID = 1 // ..4, one per fixture
	gateSecretID  = 5
	gateKilledID  = 6
)

// gateRowsBeforeTheKill is what clauses 1 and 4 together are expected to have
// written: one row per fixture, plus the one carrying the secret. Clause 3
// counts relative to it, and asserting the absolute number is what catches a
// clause that committed nothing and reported success.
const gateRowsBeforeTheKill = 5

// gateCWD is the working directory every payload this file builds claims. It is
// on a volume that does not exist, so nothing walks out of it, and it holds no
// Windows user directory - a path under one would add secret.ClassUserPath to
// every payload and make clause 4's exact privacy_class assertion untrue for a
// reason that has nothing to do with clause 4.
const gateCWD = `Z:\gate\workspace\gate-project`

// TestPhase1Gate is spec 8's Phase 1 gate: all four clauses, in one run,
// against one database this run creates from an empty directory.
//
// Four tests that pass in the same suite are four independent facts. The gate
// is one path: the database clause 3 kills a process over is the database
// clauses 1 and 4 wrote, and the foreign_key_check of clause 2 runs last, over
// every row all three of the others left behind. "It passes from clean" is then
// something this test observed rather than something a reader inferred.
//
// It lives in package spool because clause 3 kills a child that has to be a
// copy of the running test binary, and this package's TestMain is what turns a
// re-executed copy into that child. A gate in a package of its own would have
// to duplicate that harness, and the harness is the part with the trap in it
// (see killAfterCommit).
//
//	go test -p 1 -run TestPhase1Gate -v ./internal/spool/
func TestPhase1Gate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "engramux.db")
	spoolDir := filepath.Join(dir, "spool")

	// From nothing. Asserted rather than assumed: t.TempDir is documented to
	// be fresh, and this is the one property the whole run rests on.
	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("the gate's directory already holds %q, want it empty", got)
	}

	db := openMigrated(t, dbPath)

	runClause(t, "1 - the four fixtures round-trip byte for byte", func(t *testing.T) {
		gateFixturesRoundTrip(t, db)
	})
	runClause(t, "4 - a runtime-generated secret is tagged on ingest and absent from the log", func(t *testing.T) {
		gateSecretIsTaggedAndFilteredOnEgress(t, db)
	})

	if n := countEvents(t, db); n != gateRowsBeforeTheKill {
		t.Fatalf("events after clauses 1 and 4 = %d, want %d", n, gateRowsBeforeTheKill)
	}
	// The child takes the exclusive lock next, and this process is holding
	// it (I-07). Closing is part of the clause, not tidying.
	if err := db.Close(); err != nil {
		t.Fatalf("close the database before the kill: %v", err)
	}

	runClause(t, "3 - a kill between COMMIT and the ACK replays exactly once", func(t *testing.T) {
		gateKillReplaysExactlyOnce(t, dbPath, spoolDir)
	})

	// Reopened here rather than inside the clause, because clause 2 reads
	// what clause 3 left and clause 3 closes its own handle on the way out.
	db = openMigrated(t, dbPath)
	runClause(t, "2 - PRAGMA foreign_key_check returns empty", func(t *testing.T) {
		gateForeignKeyCheckIsEmpty(t, db)
	})
}

// runClause runs one gate clause as a named subtest and stops the run when it
// fails. Each clause reads the database the previous one wrote, so carrying on
// past a failure produces noise rather than a second finding.
func runClause(t *testing.T, name string, f func(t *testing.T)) {
	t.Helper()
	if !t.Run("clause "+name, f) {
		t.Fatalf("gate clause %q failed; the clauses after it were not run", name)
	}
}

// gateEnv is the envelope a relay sends for one captured event.
func gateEnv(id string, payload []byte) ipc.Envelope {
	return ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: id,
		Payload:  payload,
	}
}

// gateIngest stores one event and asserts the exact ACK status a relay accepts.
func gateIngest(t *testing.T, db *sql.DB, id string, payload []byte, what string) {
	t.Helper()
	status, err := store.Ingest(t.Context(), db, gateEnv(id, payload), store.SourcePipe, time.Now())
	if err != nil {
		t.Fatalf("%s: Ingest: %v", what, err)
	}
	if status != ipc.Committed || string(status) != "committed" {
		t.Fatalf("%s: Ingest answered %q, want %q", what, status, "committed")
	}
}

// storedEvent is what the gate reads back out of events.
type storedEvent struct {
	payload          string
	privacyClass     string
	redactionVersion int64
}

// readStored reads one event, failing the test when there is no such row.
func readStored(t *testing.T, db *sql.DB, id string) storedEvent {
	t.Helper()
	var e storedEvent
	if err := db.QueryRowContext(t.Context(),
		`SELECT payload, privacy_class, redaction_version FROM events WHERE id = ?`, id).
		Scan(&e.payload, &e.privacyClass, &e.redactionVersion); err != nil {
		t.Fatalf("read event %s: %v", id, err)
	}
	return e
}

// gateFixturesRoundTrip is clause 1. Every fixture goes in through the ingest
// path and comes back out of events.payload, and the comparison is on bytes:
// anything that unmarshalled and re-marshalled the payload on the way through
// round-trips happily while reordering keys, dropping fields this build does
// not know about and re-encoding numbers.
func gateFixturesRoundTrip(t *testing.T, db *sql.DB) {
	all := fixtures.All()
	if len(all) != 4 {
		t.Fatalf("fixtures.All returned %d fixtures, want the 4 spec 8 names", len(all))
	}
	for i, f := range all {
		want, err := f.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", f.File, err)
		}
		id := idN(gateFixtureID + i)
		gateIngest(t, db, id, want, f.File)

		if got := readStored(t, db, id).payload; !bytes.Equal([]byte(got), want) {
			t.Fatalf("%s: stored %d bytes, want %d\n got: %q\nwant: %q",
				f.File, len(got), len(want), got, want)
		}
	}
}

// gateSecretIsTaggedAndFilteredOnEgress is clause 4, and I-10 whole: the row
// keeps the secret and carries the tag, and the log - Phase 1's only egress -
// does not.
//
// It has to assert both halves, because each one alone passes on the design the
// other forbids. Absence from the log alone is also what erasing the secret
// from the database produces, and that is the design spec 6.1 rejects: a row
// reading [redacted] destroys exactly the memory this product exists to keep.
// The row still holding it alone is also what a redactor that never ran
// produces. The placeholder's presence is neither assertion - it is the
// redactor's footprint, not the secret's absence - so the secret's own bytes
// are what is searched for.
//
// The third assertion is that the log still parses, at both levels: the line
// itself, and the payload carried inside it as one string. A token pattern
// widened to \S+ swallows the closing quote and brace and breaks the inner
// document while the line around it still parses.
func gateSecretIsTaggedAndFilteredOnEgress(t *testing.T, db *sql.DB) {
	// Generated in memory, this run. Nothing credential-shaped is committed
	// to this repository: origin is public, a known-bad file trips push
	// protection, and a deliberate one is indistinguishable from a leak.
	sample := secrettest.Of(secret.ClassAPIKey)
	const sessionID = "0192f0c0-0000-7000-8000-00000000e5e5"
	payload := []byte(`{"hook_event_name":"PreToolUse","session_id":"` + sessionID +
		`","cwd":"Z:\\gate\\workspace\\gate-project","tool_name":"Bash",` +
		`"tool_input":{"command":"echo ` + sample.Value + `"}}`)

	// The secret is in the bytes about to be ingested, and the payload is a
	// document. Neither is interesting on its own; without them every
	// assertion below is about a payload that never carried a secret.
	if !bytes.Contains(payload, []byte(sample.Secret)) {
		t.Fatalf("the generated secret is not in the payload; the test is testing nothing:\n%s", payload)
	}
	if !json.Valid(payload) {
		t.Fatalf("the gate's payload is not valid JSON:\n%s", payload)
	}

	id := idN(gateSecretID)
	gateIngest(t, db, id, payload, "the payload carrying a secret")
	row := readStored(t, db, id)

	// Tagged. Both columns are asserted against the constant and against the
	// literal the column is expected to hold, so that changing a constant's
	// value cannot quietly change what is stored.
	if secret.Class(row.privacyClass) != secret.ClassAPIKey || row.privacyClass != "api-key" {
		t.Fatalf("privacy_class = %q, want %q", row.privacyClass, "api-key")
	}
	if int(row.redactionVersion) != secret.Version || row.redactionVersion != 1 {
		t.Fatalf("redaction_version = %d, want %d", row.redactionVersion, secret.Version)
	}

	// Not destroyed. The narrow assertion comes first because it is the one
	// I-10 is about; the byte comparison after it is the stronger statement
	// and the less informative failure.
	if !strings.Contains(row.payload, sample.Secret) {
		t.Fatalf("the stored row no longer contains the secret - it was erased rather than tagged:\n%s", row.payload)
	}
	if !bytes.Equal([]byte(row.payload), payload) {
		t.Fatalf("the stored payload is not the bytes that were ingested\n got: %q\nwant: %q", row.payload, payload)
	}

	// The egress. What is logged is the row as it was read back, which is
	// the shape spec 6.1 describes: a tagged row put through an egress.
	var buf bytes.Buffer
	slog.New(secret.NewLogHandler(slog.NewJSONHandler(&buf, nil))).
		Warn("gate: an event the service would not store",
			"id", id, "privacy_class", row.privacyClass, "payload", row.payload)
	out := buf.Bytes()

	if bytes.Contains(out, []byte(sample.Secret)) {
		t.Fatalf("the secret reached the log:\n%s", out)
	}
	var line map[string]any
	if err := json.Unmarshal(out, &line); err != nil {
		t.Fatalf("the log line does not parse as JSON: %v\n%s", err, out)
	}
	if got := line["msg"]; got != "gate: an event the service would not store" {
		t.Fatalf("msg = %v, want the message the service logged", got)
	}
	if got := line["privacy_class"]; got != "api-key" {
		t.Fatalf("privacy_class in the log = %v, want %q - the tag is what a reader needs", got, "api-key")
	}
	carried, ok := line["payload"].(string)
	if !ok {
		t.Fatalf("the log line carries no payload string:\n%s", out)
	}

	var doc struct {
		Event     string `json:"hook_event_name"`
		Session   string `json:"session_id"`
		CWD       string `json:"cwd"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal([]byte(carried), &doc); err != nil {
		t.Fatalf("the masked payload no longer parses as JSON: %v\n%s", err, carried)
	}
	// Filtered, not emptied. A handler that dropped the attribute, or masked
	// the whole document, passes the absence assertion above.
	if doc.Event != "PreToolUse" || doc.Session != sessionID || doc.CWD != gateCWD {
		t.Fatalf("the masked payload lost context it should keep: %+v", doc)
	}
	if doc.ToolInput.Command != "echo [redacted-api-key]" {
		t.Fatalf("command = %q, want %q", doc.ToolInput.Command, "echo [redacted-api-key]")
	}
}

// gateKillReplaysExactlyOnce is clause 3, against the database clauses 1 and 4
// have already written to:
//
//  1. a child process ingests one event with store.SourcePipe under a minted id;
//  2. its COMMIT returns, and it says so;
//  3. it is killed with TerminateProcess before any ACK is written;
//  4. the relay's post-dial budget expires with no valid ACK, so it spools the
//     event under the same id (I-05);
//  5. a fresh process opens the same database, which it can because the
//     exclusive lock does not survive process death;
//  6. the drain replays the spooled record;
//  7. the event exists exactly once.
//
// Two of the assertions pass on their own for the wrong reason. One extra row
// after the drain is also what a lost commit followed by a successful replay
// looks like, so the row is asserted before the drain runs; and it is also what
// a drain that replayed nothing looks like, so the record is asserted consumed
// and the ingest asserted to have been called with the original id.
func gateKillReplaysExactlyOnce(t *testing.T, dbPath, spoolDir string) {
	id := idN(gateKilledID)
	payload, err := fixtures.Fixture{File: fixtures.ClaudePostToolUseObject}.Bytes()
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}

	// Steps 1-3.
	killAfterCommit(t, dbPath, id)

	// Step 4.
	if err := Write(spoolDir, id, payload); err != nil {
		t.Fatalf("spool the undelivered event: %v", err)
	}
	requireNames(t, spoolDir, "after the relay spooled the event", id+ext)

	// Step 5. This handle is closed when the clause ends, so the caller can
	// reopen for clause 2.
	db := openMigrated(t, dbPath)

	// The committed row survived the kill, and so did everything clauses 1
	// and 4 wrote.
	if n := countEvents(t, db); n != gateRowsBeforeTheKill+1 {
		t.Fatalf("events after the kill = %d, want %d - the committed row did not survive",
			n, gateRowsBeforeTheKill+1)
	}
	source, receivedAt := eventRow(t, db, id)
	if source != string(store.SourcePipe) {
		t.Fatalf("the committed row's source = %q, want %q", source, store.SourcePipe)
	}

	// Step 6.
	c := &collector{}
	d := &Drainer{Dir: spoolDir, Ingest: func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
		if _, err := c.ingest(ctx, env); err != nil {
			return ipc.Rejected, err
		}
		return store.Ingest(ctx, db, env, store.SourceSpool, time.Now())
	}}
	n, err := d.Drain(t.Context())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// Step 7, and the ways one row can be the right count for the wrong
	// reason.
	if n != 1 {
		t.Fatalf("Drain replayed %d records, want 1 - nothing was replayed, so nothing was tested", n)
	}
	if got := c.ids(); !slices.Equal(got, []string{id}) {
		t.Fatalf("the drain replayed %q, want the id the relay minted %q - a re-minted id is a second row", got, id)
	}
	requireNames(t, spoolDir, "after the drain consumed the record")
	requireAbsent(t, spoolDir, quarantineDir, "the quarantine directory")

	if total := countEvents(t, db); total != gateRowsBeforeTheKill+1 {
		t.Fatalf("events after the replay = %d, want exactly %d", total, gateRowsBeforeTheKill+1)
	}
	gotSource, gotReceived := eventRow(t, db, id)
	if gotSource != string(store.SourcePipe) || gotReceived != receivedAt {
		t.Fatalf("after the replay the row is (source %q, received_at %d), want the committed row (%q, %d) untouched",
			gotSource, gotReceived, source, receivedAt)
	}
}

// gateForeignKeyCheckIsEmpty is clause 2, run last so that it covers every row
// this run wrote, by all three of the paths that wrote one: live ingest, a
// process that was killed mid-flight, and the drain.
//
// An empty result is what a sound schema looks like and equally what a check
// that never ran looks like - a misspelled pragma returns no rows and no error.
// So the clean result is asserted first, then a violation is built deliberately
// and the same query is asserted to name it, and then the database is put back.
func gateForeignKeyCheckIsEmpty(t *testing.T, db *sql.DB) {
	if got := gateForeignKeyViolations(t, db); len(got) != 0 {
		t.Fatalf("foreign_key_check = %q, want no rows", got)
	}

	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("PRAGMA foreign_keys = OFF: %v", err)
	}
	var enforcing int64
	if err := db.QueryRowContext(t.Context(), `PRAGMA foreign_keys`).Scan(&enforcing); err != nil {
		t.Fatalf("PRAGMA foreign_keys readback: %v", err)
	}
	if enforcing != 0 {
		t.Fatalf("PRAGMA foreign_keys = %d after turning it off, want 0", enforcing)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO events (id, project_id, session_id, host, source, event_name,
		                    payload, privacy_class, redaction_version, received_at)
		VALUES ('gate-dangling', 'no-such-project', 'no-such-session', 'codex', 'pipe',
		        'PostToolUse', '{}', '', 1, 1000)`); err != nil {
		t.Fatalf("INSERT a dangling event: %v", err)
	}

	// fkid indexes PRAGMA foreign_key_list, which SQLite reports in reverse
	// declaration order: session_id is declared after project_id, so it is 0.
	want := []string{
		"{table:events parent:sessions fkid:0}",
		"{table:events parent:projects fkid:1}",
	}
	if got := gateForeignKeyViolations(t, db); !slices.Equal(got, want) {
		t.Fatalf("foreign_key_check over a dangling row = %q, want %q - the clean result above proves nothing",
			got, want)
	}

	if _, err := db.ExecContext(t.Context(), `DELETE FROM events WHERE id = 'gate-dangling'`); err != nil {
		t.Fatalf("DELETE the dangling event: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("PRAGMA foreign_keys = ON: %v", err)
	}
	if got := gateForeignKeyViolations(t, db); len(got) != 0 {
		t.Fatalf("foreign_key_check after removing the dangling row = %q, want no rows", got)
	}
	if n := countEvents(t, db); n != gateRowsBeforeTheKill+1 {
		t.Fatalf("events at the end of the gate = %d, want %d", n, gateRowsBeforeTheKill+1)
	}
}

// gateForeignKeyViolations runs the clause's pragma and renders every row it
// reported. The rowid is left out: it is the only field that changes with how
// many rows the run wrote before it.
func gateForeignKeyViolations(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_check: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close: %v", err)
		}
	}()
	var got []string
	for rows.Next() {
		var table, parent string
		var rowid, fkid int64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		got = append(got, fmt.Sprintf("{table:%s parent:%s fkid:%d}", table, parent, fkid))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return got
}
