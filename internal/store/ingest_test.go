package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
)

// noUserRoot is an absolute cwd on a volume that does not exist, so the
// worktree walk finds nothing and the string carries no Windows user directory.
// Tests that assert an exact privacy_class need the second property: a temp
// directory is under C:\Users\ and would add secret.ClassUserPath to every
// payload that mentions it.
const noUserRoot = `Z:\fixture\workspace\fixture-project`

// ingestEnv builds the envelope a relay sends for one captured event.
func ingestEnv(id string, payload []byte) ipc.Envelope {
	return ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: id,
		Payload:  payload,
	}
}

// payloadOf marshals fields into the bytes a relay would forward. Tests that
// assert the bytes coming back out use a fixture or a literal instead; this is
// for the ones that need a value the test computed, such as a temp directory.
func payloadOf(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal the payload: %v", err)
	}
	return b
}

// requireCommitted asserts one Ingest returned no error and the exact status
// string "committed". Both halves matter: the status is what the relay checks
// (spec 5.3), and it is compared against the literal as well as the constant so
// that renaming the constant's *value* cannot quietly change the wire.
func requireCommitted(t *testing.T, status ipc.AckStatus, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: Ingest: %v", what, err)
	}
	if status != ipc.Committed || string(status) != "committed" {
		t.Fatalf("%s: status = %q, want %q", what, status, "committed")
	}
}

// eventRow is one events row, less the id the caller already has.
type eventRow struct {
	projectID        string
	sessionID        string
	host             string
	source           string
	eventName        string
	toolName         sql.NullString
	toolUseID        sql.NullString
	payload          string
	privacyClass     string
	redactionVersion int
	receivedAt       int64
}

// readEvent reads the event with id, failing the test when there is none.
func readEvent(t *testing.T, db *sql.DB, id string) eventRow {
	t.Helper()
	var r eventRow
	if err := db.QueryRowContext(t.Context(), `
		SELECT project_id, session_id, host, source, event_name, tool_name, tool_use_id,
		       payload, privacy_class, redaction_version, received_at
		FROM events WHERE id = ?`, id).
		Scan(&r.projectID, &r.sessionID, &r.host, &r.source, &r.eventName, &r.toolName,
			&r.toolUseID, &r.payload, &r.privacyClass, &r.redactionVersion, &r.receivedAt); err != nil {
		t.Fatalf("read event %q: %v", id, err)
	}
	return r
}

// requireCount asserts a table holds exactly n rows.
func requireCount(t *testing.T, db *sql.DB, table string, n int64) {
	t.Helper()
	if got := countRows(t, db, table); got != n {
		t.Fatalf("%s rows = %d, want %d", table, got, n)
	}
}

// requirePayload asserts the stored payload is byte-identical to want.
//
// The comparison is on bytes and not on a struct, because a payload that was
// unmarshalled and re-marshalled on the way through round-trips happily while
// reordering keys, dropping fields this build does not know about and
// re-encoding numbers. Phase 1's gate clause 1 is the byte comparison.
func requirePayload(t *testing.T, got string, want []byte, what string) {
	t.Helper()
	if !bytes.Equal([]byte(got), want) {
		t.Fatalf("%s: payload stored as %d bytes, want %d; first difference at %d\n got: %q\nwant: %q",
			what, len(got), len(want), firstDiff([]byte(got), want), got, want)
	}
}

// TestIngestingTheSameEventTwiceLeavesOneRow is I-05, and the half of it that
// is easy to get wrong: the second ingest must ACK `committed`, not `rejected`.
// `rejected` means permanent loss - the relay will not spool an event the
// service rejected - so answering it to a duplicate throws away an event that
// was already safe. rev.2 shipped exactly that.
//
// "the second call did not error" is not the assertion. The row count is
// asserted too, and the second envelope carries a *different* payload under the
// same id, so an implementation that reached for ON CONFLICT DO UPDATE - which
// also does not error and also leaves one row - is caught by the stored bytes
// still being the first ones.
func TestIngestingTheSameEventTwiceLeavesOneRow(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	repo := repoDir(t)

	const id = "0198f0c1-0000-7000-8000-00000000d001"
	first := payloadOf(t, map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "s-duplicate",
		"cwd":             repo,
		"model":           "fixture-model",
		"tool_name":       "shell",
		"tool_use_id":     "call_duplicate",
	})
	second := payloadOf(t, map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "s-duplicate",
		"cwd":             repo,
		"model":           "fixture-model",
		"tool_name":       "a different tool",
		"tool_use_id":     "call_duplicate",
	})

	status, err := Ingest(ctx, db, ingestEnv(id, first), SourcePipe, upsertNow)
	requireCommitted(t, status, err, "first ingest")

	status, err = Ingest(ctx, db, ingestEnv(id, second), SourcePipe, upsertNow)
	requireCommitted(t, status, err, "second ingest")

	requireCount(t, db, "events", 1)
	requireCount(t, db, "sessions", 1)
	requireCount(t, db, "projects", 1)

	row := readEvent(t, db, id)
	requirePayload(t, row.payload, first, "after the duplicate")
	if row.toolName.String != "shell" {
		t.Fatalf("tool_name = %q, want %q - the duplicate overwrote the row",
			row.toolName.String, "shell")
	}
	if !row.toolUseID.Valid || row.toolUseID.String != "call_duplicate" {
		t.Fatalf("tool_use_id = %v, want %q", row.toolUseID, "call_duplicate")
	}
}

// TestIngestingDistinctIDsKeepsBothEvents is the converse, and it is what stops
// the test above from passing on an implementation that stores nothing at all
// after the first event: two byte-identical payloads under two ids are two
// rows, because the relay-minted id is the *only* idempotency key (I-05) and
// nothing derived from the payload narrows it.
func TestIngestingDistinctIDsKeepsBothEvents(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)

	payload := payloadOf(t, map[string]any{
		"hook_event_name": "SubagentStop",
		"session_id":      "s-identical",
		"cwd":             noUserRoot,
		"model":           "fixture-model",
	})
	for _, id := range []string{"ev-identical-1", "ev-identical-2"} {
		status, err := Ingest(ctx, db, ingestEnv(id, payload), SourcePipe, upsertNow)
		requireCommitted(t, status, err, "ingest "+id)
	}

	requireCount(t, db, "events", 2)
	requireCount(t, db, "sessions", 1)
}

// TestIngestRoundTripsEveryFixtureByteForByte is Phase 1 gate clause 1, run
// through the ingest path rather than through a hand-written INSERT: the bytes
// read back out of events.payload are compared against the fixture file's own
// bytes, so anything that unmarshals and re-marshals the payload on the way in
// or out fails here.
func TestIngestRoundTripsEveryFixtureByteForByte(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)

	for i, f := range fixtures.All() {
		want, err := f.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", f.File, err)
		}
		id := fmt.Sprintf("ev-fixture-%d", i)

		status, err := Ingest(ctx, db, ingestEnv(id, want), SourcePipe, upsertNow)
		requireCommitted(t, status, err, f.File)

		row := readEvent(t, db, id)
		requirePayload(t, row.payload, want, f.File)
		if row.host != f.Host {
			t.Fatalf("%s: host = %q, want %q", f.File, row.host, f.Host)
		}
		if row.eventName != f.Event {
			t.Fatalf("%s: event_name = %q, want %q", f.File, row.eventName, f.Event)
		}
	}

	requireCount(t, db, "events", int64(len(fixtures.All())))
}

// TestIngestStoresAnUnclassifiablePayloadAsUnknown is I-04's second clause: a
// payload host detection will not classify is stored as `unknown`, and that is
// not an error. Spooling cannot fix a payload that will never classify, so
// rejecting it would lose it for good.
func TestIngestStoresAnUnclassifiablePayloadAsUnknown(t *testing.T) {
	ctx := t.Context()

	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{
			// No prompt_id, effort, model or turn_id, and no
			// transcript_path to fall through to.
			name: "a well-formed payload with no host signal",
			payload: []byte(`{"hook_event_name":"Stop","session_id":"s-unknown","cwd":"` +
				`Z:\\fixture\\workspace\\fixture-project"}`),
		},
		{
			// Valid JSON, so it survived the envelope, but not an
			// object - so nothing can be read out of it at all.
			name:    "valid JSON that is not an object",
			payload: []byte(`["not","an","object"]`),
		},
		{
			name:    "an empty JSON object",
			payload: []byte(`{}`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := migrated(t)
			const id = "ev-unknown"

			status, err := Ingest(ctx, db, ingestEnv(id, tc.payload), SourcePipe, upsertNow)
			requireCommitted(t, status, err, tc.name)

			row := readEvent(t, db, id)
			if row.host != "unknown" {
				t.Fatalf("host = %q, want %q", row.host, "unknown")
			}
			requirePayload(t, row.payload, tc.payload, tc.name)

			// None of these payloads names a tool. tool_use_id pairs
			// PreToolUse with PostToolUse, so it has to be NULL and
			// not "": NULL never equals NULL, and an empty string
			// would pair every tool-less event with every other one.
			if row.toolName.Valid || row.toolUseID.Valid {
				t.Fatalf("tool_name = %v, tool_use_id = %v, want both NULL",
					row.toolName, row.toolUseID)
			}

			var sessionHost string
			if err := db.QueryRowContext(ctx,
				`SELECT host FROM sessions WHERE id = ?`, row.sessionID).Scan(&sessionHost); err != nil {
				t.Fatalf("read session %q: %v", row.sessionID, err)
			}
			if sessionHost != "unknown" {
				t.Fatalf("sessions.host = %q, want %q", sessionHost, "unknown")
			}
		})
	}
}

// TestIngestSetsSourceFromTheIngestPathNotThePayload. events.source says where
// the event entered the service, so it is set from the ingest path and never
// read out of what the relay sent - otherwise a spooled replay could claim to
// have arrived live, and the column would stop meaning anything.
//
// Both directions are asserted. A `pipe` literal hard-coded into the INSERT
// passes the first case and fails the second, and the payload also carries a
// "host" key so an implementation reading the payload for *either* column is
// caught.
func TestIngestSetsSourceFromTheIngestPathNotThePayload(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)

	payload := payloadOf(t, map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "s-source",
		"cwd":             noUserRoot,
		"model":           "fixture-model",
		"source":          "spool",
		"host":            "claude-code",
	})

	for _, tc := range []struct {
		id   string
		src  Source
		want string
	}{
		{"ev-source-pipe", SourcePipe, "pipe"},
		{"ev-source-spool", SourceSpool, "spool"},
	} {
		status, err := Ingest(ctx, db, ingestEnv(tc.id, payload), tc.src, upsertNow)
		requireCommitted(t, status, err, tc.id)

		row := readEvent(t, db, tc.id)
		if row.source != tc.want {
			t.Fatalf("%s: source = %q, want %q - the payload claimed %q",
				tc.id, row.source, tc.want, "spool")
		}
		if row.host != "codex" {
			t.Fatalf("%s: host = %q, want %q - the payload claimed %q",
				tc.id, row.host, "codex", "claude-code")
		}
	}
}

// TestIngestTagsASecretAndKeepsTheOriginalBytes is I-10's first half: a secret
// is tagged on the way in, not destroyed. A row reading [redacted] would erase
// exactly the memory this product exists to keep, so the database holds the
// original and every egress filters on the tag - and the egress half is a later
// task.
//
// The clean case is asserted alongside, because a privacy_class assertion on
// its own passes on an implementation that writes one class into every row.
func TestIngestTagsASecretAndKeepsTheOriginalBytes(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	sample := secrettest.Of(secret.ClassAPIKey)

	for _, tc := range []struct {
		id      string
		command string
		want    secret.Set
	}{
		{"ev-secret", "echo " + sample.Value, secret.Set{sample.Class}},
		{"ev-clean", "echo nothing to see", nil},
	} {
		payload := payloadOf(t, map[string]any{
			"hook_event_name": "PostToolUse",
			"session_id":      "s-tagged",
			"cwd":             noUserRoot,
			"model":           "fixture-model",
			"tool_name":       "shell",
			"tool_input":      map[string]any{"command": tc.command},
		})

		status, err := Ingest(ctx, db, ingestEnv(tc.id, payload), SourcePipe, upsertNow)
		requireCommitted(t, status, err, tc.id)

		row := readEvent(t, db, tc.id)
		if got := secret.ParseSet(row.privacyClass); !slices.Equal(got, tc.want) {
			t.Fatalf("%s: privacy_class = %q -> %v, want %v", tc.id, row.privacyClass, got, tc.want)
		}
		if row.redactionVersion != secret.Version {
			t.Fatalf("%s: redaction_version = %d, want %d", tc.id, row.redactionVersion, secret.Version)
		}
		requirePayload(t, row.payload, payload, tc.id)
	}

	// Said again directly, because the byte comparison above would also hold
	// if the generator had produced something harmless: the secret itself is
	// in the row.
	if row := readEvent(t, db, "ev-secret"); !strings.Contains(row.payload, sample.Secret) {
		t.Fatalf("the stored payload no longer contains the generated secret: %q", row.payload)
	}
}

// TestIngestRollsBackEverythingWhenTheEventInsertFails. The project and the
// session are written before the event, so a failure on the event insert is the
// case where a missing transaction shows: without one, the run leaves a project
// and a session for an event that was never stored, and the relay - which got
// `rejected` and spooled - replays into a database that already half-remembers
// it.
//
// The failure is induced through the real code path by handing Ingest a source
// the column's CHECK refuses, which is also the only way to reach that CHECK
// from outside the package.
func TestIngestRollsBackEverythingWhenTheEventInsertFails(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)

	payload := payloadOf(t, map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "s-rollback",
		"cwd":             noUserRoot,
		"model":           "fixture-model",
	})

	status, err := Ingest(ctx, db, ingestEnv("ev-rollback", payload), Source("smuggled"), upsertNow)
	if status != ipc.Rejected || string(status) != "rejected" {
		t.Fatalf("status = %q, want %q", status, "rejected")
	}
	requireSQLiteCode(t, err, sqliteConstraintCheck, "Ingest with an out-of-domain source")

	requireCount(t, db, "events", 0)
	requireCount(t, db, "sessions", 0)
	requireCount(t, db, "projects", 0)

	// The pool holds exactly one connection, so a transaction that was left
	// open would wedge it permanently - every later BeginTx returning
	// "cannot start a transaction within a transaction", with no second
	// connection to recover on. The next ingest succeeding is what proves
	// the failed one rolled back rather than merely failing.
	next, err := Ingest(ctx, db, ingestEnv("ev-after-rollback", payload), SourcePipe, upsertNow)
	requireCommitted(t, next, err, "the ingest after a rolled-back one")
	requireCount(t, db, "events", 1)
}

// TestIngestStampsEveryRowWithOneInstant. One ingest is one instant: the
// project, the session and the event all carry the timestamp the caller passed,
// not three separate clock reads. Windows clock resolution is about 550 us and
// these three writes are microseconds apart, so an implementation calling
// time.Now three times would usually agree by accident - which is why the
// timestamp is a parameter and the assertion is against its exact value.
func TestIngestStampsEveryRowWithOneInstant(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)

	payload := payloadOf(t, map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "s-instant",
		"cwd":             noUserRoot,
		"model":           "fixture-model",
	})
	status, err := Ingest(ctx, db, ingestEnv("ev-instant", payload), SourcePipe, upsertNow)
	requireCommitted(t, status, err, "ingest")

	want := upsertNow.UnixMilli()
	for _, q := range []struct {
		what  string
		query string
	}{
		{"projects.created_at", `SELECT created_at FROM projects`},
		{"sessions.created_at", `SELECT created_at FROM sessions`},
		{"events.received_at", `SELECT received_at FROM events`},
	} {
		var got int64
		if err := db.QueryRowContext(ctx, q.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", q.what, err)
		}
		if got != want {
			t.Fatalf("%s = %d, want %d", q.what, got, want)
		}
	}
}

// TestIngestWithoutASessionIDBucketsByHost pins the decision this task owns for
// a payload carrying no session_id: the empty string is passed through, so
// every such event for one host lands in that host's single "no session id"
// bucket. The event itself is stored, which is the part I-04 requires; the
// bucket is per host and stays queryable by its empty host_session_id.
//
// Nothing in the 900-capture corpus reaches this: every capture carries a
// session_id. The column is NOT NULL, so the decision could not be left open.
func TestIngestWithoutASessionIDBucketsByHost(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)

	codex := payloadOf(t, map[string]any{
		"hook_event_name": "SubagentStop",
		"cwd":             noUserRoot,
		"model":           "fixture-model",
	})
	claude := payloadOf(t, map[string]any{
		"hook_event_name": "SubagentStop",
		"cwd":             noUserRoot,
		"prompt_id":       "00000000-0000-4000-8000-0000000000f1",
	})

	for _, tc := range []struct {
		id      string
		payload []byte
		want    string
	}{
		{"ev-nosession-1", codex, "codex:"},
		{"ev-nosession-2", codex, "codex:"},
		{"ev-nosession-3", claude, "claude-code:"},
	} {
		status, err := Ingest(ctx, db, ingestEnv(tc.id, tc.payload), SourcePipe, upsertNow)
		requireCommitted(t, status, err, tc.id)

		if row := readEvent(t, db, tc.id); row.sessionID != tc.want {
			t.Fatalf("%s: session_id = %q, want %q", tc.id, row.sessionID, tc.want)
		}
	}

	// Three events, none dropped (I-04), in two buckets - one per host, not
	// one for all of them and not one each.
	requireCount(t, db, "events", 3)
	requireCount(t, db, "sessions", 2)

	var hostSessionID string
	if err := db.QueryRowContext(ctx,
		`SELECT host_session_id FROM sessions WHERE id = 'codex:'`).Scan(&hostSessionID); err != nil {
		t.Fatalf("read the codex bucket: %v", err)
	}
	if hostSessionID != "" {
		t.Fatalf("host_session_id = %q, want %q", hostSessionID, "")
	}
}

// TestIngestWithoutAnAbsoluteCwdIgnoresTheServiceWorkingDirectory pins the
// other decision this task owns. project.Identify walks up looking for .git,
// and for a non-absolute path that walk resolves against the *process's*
// working directory - one long-lived service started by Task Scheduler - so
// identity would depend on where the service was launched from, and could
// attribute the event to a real repository it has nothing to do with.
//
// t.Chdir puts the test process inside a real repository, which is exactly what
// an unguarded walk absorbs. The event is still stored either way, because I-04
// does not allow it to be dropped for wanting a usable cwd; it gets a root of
// its own that no absolute path can collide with.
func TestIngestWithoutAnAbsoluteCwdIgnoresTheServiceWorkingDirectory(t *testing.T) {
	ctx := t.Context()

	for _, tc := range []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{
			// A walk from "." cannot climb, so this case is the same
			// with the guard and without it. It is here for the
			// decision, not for the guard.
			name:   "no cwd at all",
			fields: map[string]any{"hook_event_name": "Stop", "session_id": "s-nocwd"},
			want:   ".",
		},
		{
			// The discriminating case. Unguarded, the walk starts one
			// directory above the service and climbs straight back
			// into the repository the service is sitting in.
			name:   "a relative cwd",
			fields: map[string]any{"hook_event_name": "Stop", "session_id": "s-relcwd", "cwd": ".."},
			want:   "..",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := migrated(t)
			t.Chdir(repoDir(t))
			const id = "ev-cwd"

			status, err := Ingest(ctx, db, ingestEnv(id, payloadOf(t, tc.fields)), SourcePipe, upsertNow)
			requireCommitted(t, status, err, tc.name)

			requireCount(t, db, "events", 1)
			requireCount(t, db, "projects", 1)

			projectID, root, _ := projectRow(t, db)
			if root != tc.want {
				t.Fatalf("projects.root = %q, want %q - the walk reached the service's own directory",
					root, tc.want)
			}
			if row := readEvent(t, db, id); row.projectID != projectID {
				t.Fatalf("events.project_id = %q, want %q", row.projectID, projectID)
			}
		})
	}
}

// deepWalkComponents is how many path components deepWalkCWD carries. The walk
// costs one os.Lstat per component and every one of them fails, so the cost is
// real work with no setup: 4,000 measured 77 ms on the machine this was written
// on, where 1,000 measured 5.8 ms.
const deepWalkComponents = 4000

// deepWalkCWD is an absolute cwd on a volume that does not exist, nested deeply
// enough that resolving it costs measurable filesystem work.
//
// It is the cheap, deterministic stand-in for the trigger that matters in
// production and cannot be reproduced here: a cwd that is slow to stat because
// it is a UNC path to a host that is down, or a mapped drive whose share has
// gone away. Those cost tens of seconds per level; this costs microseconds per
// level and gets there by having a lot of levels.
var deepWalkCWD = `Z:\` + strings.Repeat(`w\`, deepWalkComponents) + "leaf"

// TestIngestResolvesTheProjectOutsideTheTransaction is the availability
// property behind spec 5.4's single connection: while the write transaction is
// open, nothing that is not SQL runs.
//
// project.Identify walks the filesystem and takes no context, so it can neither
// be bounded nor cancelled. Called from inside the transaction it holds the
// service's *only* connection for its whole duration - every other ingest and
// the drain wait behind it in db.BeginTx - so one cwd on a dead UNC path stalls
// the whole service, and every relay that arrives during the stall times out
// and spools. Called before BeginTx it stalls one event. secret.Detect is the
// same shape and much smaller, and is hoisted with it.
//
// The assertion is a ratio and not a duration, so it means the same thing on a
// slow machine as on a fast one. N ingests of one slow-to-resolve cwd cost *at
// least* N walks when the walk is inside the transaction, because the single
// connection serialises them end to end and nothing can overlap; when it is
// outside, they overlap by however much the machine allows. Three quarters of N
// walks is below the first floor and above what the second needs on any machine
// that can run two of these walks at once - measured here at 8 walks serialised
// against 2.7 overlapped.
func TestIngestResolvesTheProjectOutsideTheTransaction(t *testing.T) {
	const concurrency = 8
	ctx := t.Context()
	db := migrated(t)

	// The premise, measured rather than assumed: a walk too cheap to
	// measure makes both orderings look alike and the assertion vacuous.
	start := time.Now()
	project.Identify(deepWalkCWD)
	walk := time.Since(start)
	t.Logf("one project.Identify over %d components = %s", deepWalkComponents, walk)
	if walk < 20*time.Millisecond {
		t.Fatalf("resolving the cwd took %s, too little to tell the two orderings apart; "+
			"raise deepWalkComponents until it costs at least 20ms on this machine", walk)
	}

	payload := payloadOf(t, map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "s-deep-walk",
		"cwd":             deepWalkCWD,
	})

	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	start = time.Now()
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("0198f0c1-0000-7000-8000-00000000e%03d", i)
			status, err := Ingest(ctx, db, ingestEnv(id, payload), SourcePipe, upsertNow)
			if err == nil && status != ipc.Committed {
				err = fmt.Errorf("status = %q, want %q", status, ipc.Committed)
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent ingest %d: %v", i, err)
		}
	}
	// Every event still landed, so the overlap above is not a shortcut
	// through work that did not happen.
	requireCount(t, db, "events", concurrency)
	requireCount(t, db, "projects", 1)

	if limit := walk * concurrency * 3 / 4; elapsed >= limit {
		t.Fatalf("%d concurrent ingests took %s, want under %s (three quarters of %d serialised %s walks): "+
			"the cwd is resolved inside the transaction, so the one connection is held for the walk",
			concurrency, elapsed, limit, concurrency, walk)
	}
	t.Logf("%d concurrent ingests = %s, against %s serialised", concurrency, elapsed, walk*concurrency)
}
