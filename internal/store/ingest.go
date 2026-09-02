package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/secret"
)

// Source is where an event entered the service, and the value events.source
// holds. It is set by whoever called [Ingest] and is never read out of the
// envelope, so nothing a relay sends can claim to have arrived somewhere it did
// not (spec 6).
//
// The column carries a CHECK for the same two values, so a third one fails the
// write rather than being stored. That is deliberately not a silent fallback:
// an unrecognised source means the caller is wrong about something, and the
// event belongs in the spool the relay still holds.
type Source string

const (
	// SourcePipe is an event delivered live over the named pipe.
	SourcePipe Source = "pipe"
	// SourceSpool is an event the drain replayed out of the relay's spool.
	SourceSpool Source = "spool"
)

// Ingest stores one captured event and returns the ACK status the caller must
// put in its [ipc.Ack]. The project row, the session row and the event row are
// written inside one transaction, on the one connection this package allows.
//
// Returning [ipc.AckStatus] rather than a status of this package's own is what
// keeps the value the relay checks and the value this function decided the same
// value. It also fixes the dependency direction: internal/ipc is imported here
// and cannot import this package back, so a pipe server that needs to reach the
// database takes a handler instead.
//
// # A duplicate is not an error
//
// env.IngestID is the relay-minted UUIDv7, and it *is* the idempotency key
// (I-05): idempotency is the INSERT on that primary key and nothing else. The
// same event ingested twice therefore leaves one row and answers
// [ipc.Committed] both times.
//
// [ipc.Rejected] is a delivery failure, not a drop: the relay spools a rejected
// send and retries it (I-04), and a record that keeps failing is quarantined
// rather than retried forever. So answering Rejected to a duplicate is not data
// loss - it is a lie. The row is already there, and the relay would spool,
// replay and finally quarantine an event that was safely stored all along.
// rev.2's related bug was on the other side of the wire: it accepted a Rejected
// ACK as success, so nothing spooled the event and it really was lost.
//
// The corollary is that this function trusts the id it is handed. Two events
// sent under one id are one event by definition, because the key is the only
// identity an event has; minting a replacement here for an empty or repeated id
// would turn a broken relay into duplicate rows instead of a caught bug.
//
// # What is read out of the payload, and what is not
//
// The payload decides the host (I-12), the session, the project and the event
// name. It does not decide events.source, and it cannot: that argument is the
// caller's.
//
// A payload that will not classify is stored with host `unknown` and is not an
// error (I-04) - spooling cannot fix a payload that will never classify. So is
// one that is not a JSON object at all: it survived the envelope, so it is
// valid JSON, and there is simply nothing in it to read. Every field below then
// comes back empty, which is the honest answer and still a row.
//
// # now
//
// now stamps every row this call writes, so one ingest is one instant.
// Timestamps are milliseconds since the Unix epoch and are not an ordering key:
// I-06 makes ordering partial, and Windows clock resolution is around 550 us
// against a busiest-session rate of 14.8 events/min, so a timestamp neither
// orders nor disambiguates.
func Ingest(ctx context.Context, db *sql.DB, env ipc.Envelope, src Source, now time.Time) (ipc.AckStatus, error) {
	// A payload that is not a JSON object leaves fields nil, which every
	// reader below treats as "absent". host.Detect answers "unknown" for it.
	var fields map[string]any
	_ = json.Unmarshal(env.Payload, &fields)

	h := host.Detect(fields)

	// Resolved before the transaction is opened, and this is the whole
	// reason both are hoisted up here rather than evaluated where they are
	// used. project.Identify walks the filesystem - one os.Lstat per path
	// component, up to the volume root - and takes no context, so nothing
	// can bound or cancel it. secret.Detect is the same shape and much
	// smaller: eight RE2 patterns over every string leaf.
	//
	// The DSN carries _txlock=immediate, so BeginTx takes the write lock the
	// moment it returns, and there is exactly one connection (spec 5.4).
	// Anything slow between BeginTx and Commit is therefore held by every
	// other ingest and by the drain, waiting in BeginTx for the same
	// connection - so one cwd that is slow to stat, a UNC path to a host
	// that is down or a mapped drive whose share has gone away, would stall
	// the entire service rather than one event. Neither call needs the
	// transaction, so neither is in it.
	//
	// A missing or non-absolute cwd is resolved by project.Identify without
	// consulting the service's own working directory - see its doc comment.
	p := project.Identify(field(fields, "cwd"))
	privacyClass := secret.Detect(env.Payload).String()
	// The third decode of the same bytes - the fields map above, Detect, and
	// this - and deliberately not shared with either. Detect walks a decoded
	// map, which loses document order, and Leaves needs it (see its doc). At
	// spec 7.4's payload sizes a third pass is microseconds, and it is not
	// to be optimised into one without a measurement saying it was worth it.
	leaves := Leaves(env.Payload)
	// The fourth decode, and hoisted for the same reason as the three above:
	// it walks the payload and it does not need the transaction. What it
	// costs is measured with the rest in BenchmarkIngest; what it would cost
	// inside the transaction is every other ingest waiting on the one
	// connection.
	derived := Derive(env.Payload)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ipc.Rejected, fmt.Errorf("store: begin the ingest transaction: %w", err)
	}
	// Every path rolls back. After a successful Commit this is a no-op
	// returning sql.ErrTxDone; on any path that is not, it is the only thing
	// standing between a failed ingest and a permanently wedged connection,
	// because there is exactly one connection and no second one to recover
	// on (spec 5.4).
	defer func() { _ = tx.Rollback() }()

	projectID, err := UpsertProject(ctx, tx, p, now)
	if err != nil {
		return ipc.Rejected, err
	}

	eventName := field(fields, "hook_event_name")
	// A payload with no session_id hands "" straight through, so every such
	// event for one host lands in that host's single bucket - one row with
	// host_session_id = '', not one row per event and not one row shared
	// across hosts. events.session_id is NOT NULL and I-04 forbids dropping
	// the event, so it needs *some* session; a bucket that says "no session
	// id" stays queryable and re-attributable, where a synthetic per-event
	// id would fill the sessions table with rows that look real.
	//
	// No capture in the 900-capture corpus lacks a session_id.
	sessionID, err := UpsertSession(ctx, tx, projectID, h, field(fields, "session_id"), eventName, now)
	if err != nil {
		return ipc.Rejected, err
	}

	// ON CONFLICT DO NOTHING is what makes a duplicate committed rather than
	// an error. DO UPDATE would also leave one row and would let a second,
	// different payload overwrite the first; INSERT OR REPLACE is banned
	// outright, because it deletes and reinserts, firing cascades and losing
	// the rowid.
	//
	// payload binds as a string and not as []byte: the column is TEXT under
	// a STRICT table, and modernc.org/sqlite binds []byte as a BLOB, which
	// STRICT then refuses. TEXT is byte-safe here - invalid UTF-8, lone
	// surrogate bytes and embedded NULs all round-trip byte-identical - and
	// the bytes go in exactly as they arrived, because Phase 1 gates on a
	// byte-for-byte round trip and nothing here re-marshals them. The one
	// normalisation the gate permits happens at the relay's stdin boundary,
	// before either delivery path exists.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO events (id, project_id, session_id, host, source, event_name,
		                    tool_name, tool_use_id, payload, leaves, privacy_class,
		                    redaction_version, received_at,
		                    derived_cmd, derived_paths, derived_output)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`,
		env.IngestID, projectID, sessionID, h, string(src), eventName,
		nullable(fields, "tool_name"), nullable(fields, "tool_use_id"),
		string(env.Payload), leaves, privacyClass,
		int64(secret.Version), now.UnixMilli(),
		derived.Cmd, derived.Paths, derived.Output); err != nil {
		return ipc.Rejected, fmt.Errorf("store: insert event %s: %w", env.IngestID, err)
	}

	if err := tx.Commit(); err != nil {
		return ipc.Rejected, fmt.Errorf("store: commit event %s: %w", env.IngestID, err)
	}
	return ipc.Committed, nil
}

// field returns fields[key] when it is a string, and "" for every other shape -
// absent, null, a number, an object. The columns these feed are NOT NULL and
// I-04 forbids dropping an event over one, so "" is the value a payload that
// did not say gets.
func field(fields map[string]any, key string) string {
	v, _ := fields[key].(string)
	return v
}

// nullable is [field] for the two columns that are nullable, returning nil -
// which binds NULL - rather than "".
//
// tool_use_id pairs PreToolUse with PostToolUse, and an empty string would pair
// every event that carries no tool with every other one. NULL never equals
// NULL, so it cannot.
func nullable(fields map[string]any, key string) any {
	if v := field(fields, key); v != "" {
		return v
	}
	return nil
}
