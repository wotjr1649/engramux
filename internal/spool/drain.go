package spool

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// quarantineDir holds records the drain gave up on, under the spool directory.
// A subdirectory rather than a suffix, so one os.ReadDir listing separates
// "replay this" from "a human should look at this" without parsing names.
//
// Quarantined means kept, not deleted (I-04). The bounds below measure the
// records the drain will replay and deliberately do not measure this
// directory: counting it would let a poison record that nothing will ever
// ingest block every new event from being spooled.
//
// ponytail: quarantine grows without a bound of its own. A count on it, or a
// retention sweep, is the upgrade path if a real deployment ever fills it.
const quarantineDir = "quarantine"

// maxAttempts is how many times the drain replays one record before
// quarantining it. A record that fails this often is not going to succeed on
// the next pass either, and a drain that retries forever is a loop that never
// reaches the records behind it (spec 5.6).
const maxAttempts = 5

// The three bounds spec 5.6 puts on the spool. All three, because a bound on
// one of them is not a bound: 10,000 tiny records and one 64 MiB one both fill
// a disk, and neither is caught by a bound on the other.
//
// They are vars so a test can shrink them - a byte-bound test that has to
// write 64 MiB is a test nobody runs. Nothing else writes to them.
//
// The values, against §7.4's measured corpus (p50 1,182 B, p90 7,418 B, p99
// 32,936 B, max 171,764 B per payload; 14.8 events/min in the busiest session,
// about 2/s across eight):
//
//   - maxRecords 10,000 is about 11 hours of the busiest single session with
//     no service running at all, or 83 minutes of all eight. The gap spec 5.5's
//     upgrade actually produces - drain, stop, replace, start - is seconds.
//   - maxBytes 64 MiB is the same number as the DSN's journal_size_limit, and
//     at the p90 payload it holds about 9,000 records: for ordinary traffic the
//     count bound is the one that fires, and this one only binds when payloads
//     are far above the corpus. It is also 16x ipc.MaxFrameLen, so a maximum
//     size record is never unspoolable on an empty spool.
//   - maxAge 7 days is a judgement, not a measurement: a machine off over a
//     long weekend is three days, and a week after capture the session an event
//     belongs to is long gone.
var (
	maxRecords = 10000
	maxBytes   = int64(64 << 20)
	maxAge     = 7 * 24 * time.Hour
)

// drainBatch is how many records the drain replays before it yields, and
// drainPause is how long it yields for.
//
// The drain competes with live ingest for the one connection the service has
// (spec 5.4), so it must not hold it for an unbounded stretch. Each record is
// its own transaction, so the connection is already released between records;
// the pause is what stops a long spool from starving a relay that has 1 s in
// total (spec 5.3) and measures 1.04 ms at p95 for a whole round trip. Vars for
// the same reason as the bounds.
var (
	drainBatch = 64
	drainPause = 10 * time.Millisecond
)

// The bounds' errors. Each names exactly which bound refused the record, so a
// caller - and a test - can tell them apart with errors.Is.
var (
	// ErrRecordBound means the spool already holds the maximum number of
	// records.
	ErrRecordBound = errors.New("spool: the record bound is reached")

	// ErrByteBound means this record would take the spool past its byte
	// bound.
	ErrByteBound = errors.New("spool: the byte bound is reached")

	// ErrNoIngest means a Drainer was used without an Ingest.
	ErrNoIngest = errors.New("spool: the Drainer has no Ingest")
)

// Drainer replays spooled records back into the service.
//
// The zero value is not usable; Dir and Ingest are both required. It keeps the
// per-record failure counts that decide when a record is quarantined, so the
// same Drainer has to be reused across passes - a fresh one each time would
// retry a poison record forever. One Drainer is used by one goroutine: the
// counts are a plain map and nothing here locks.
//
// ponytail: the counts are in memory, so a service restart gives every record
// its attempts back. Persisting them means a sidecar file per record or a
// table; the upgrade path is there if a poison record ever survives a restart
// often enough to matter.
type Drainer struct {
	// Dir is the spool directory, the same one the relay's Write writes to.
	Dir string

	// Ingest stores one replayed event and answers what the service would
	// have ACKed. The service's implementation calls store.Ingest with
	// store.SourceSpool.
	//
	// It is a field rather than an import because internal/spool is linked
	// into the relay, which is a process per hook event (spec 5.1): reaching
	// for internal/store here would put SQLite in a binary that never opens
	// a database. It is deliberately not internal/pipe's IngestFunc for the
	// same reason in the other direction - that package pulls in go-winio.
	Ingest func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error)

	// Log is where this Drainer's records go. Optional; nil discards them.
	//
	// The service passes its own logger, which is the one wrapped in
	// secret.NewLogHandler (I-10). See [logger] for why nil is not
	// slog.Default().
	Log *slog.Logger

	// failures counts consecutive failed replays per record id.
	failures map[string]int
}

// discard is the handler a nil logger resolves to.
var discard = slog.New(slog.DiscardHandler)

// logger returns l, or a logger that throws records away - never
// [slog.Default].
//
// Falling back to the package default is precisely what this package must not
// do, and the reason is which binaries link it. internal/spool is linked into
// the relay as well as the service (see [Drainer.Ingest]), and the relay
// installs no handler at all: slog.Default() there is the unfiltered
// package-default handler, so a record built from a payload would leave through
// it with none of I-10's masking. The values logged today are a UUID and two
// durations and none of them is payload-derived - but the channel is what is
// being closed, not one line's values.
//
// So a caller that wants its records kept passes a logger, and a caller that
// passes nil has said it does not want them. Silence is the safe default here
// in a way that "wherever the process happens to point slog" is not.
func logger(l *slog.Logger) *slog.Logger {
	if l == nil {
		return discard
	}
	return l
}

// Drain replays every record in the spool through Ingest and removes each one
// the service commits. It returns the number of records the service stored.
//
// It is a bounded batch and it honours cancellation. The context is checked
// before every record, not only at a batch boundary, so a shutdown that starts
// mid-batch is not held up by the rest of it; a cancelled drain leaves every
// record it did not reach exactly where it was, and spends none of their
// attempts.
//
// A record the service will not store is left in place and counted, and after
// maxAttempts it is quarantined - the drain does not stop on it, because a
// poison record that stalls the queue is worse than the poison record.
// Filesystem failures do stop it: they are not one record's problem.
func (d *Drainer) Drain(ctx context.Context) (int, error) {
	if d.Ingest == nil {
		return 0, ErrNoIngest
	}
	if d.failures == nil {
		d.failures = make(map[string]int)
	}

	ids, _, err := scan(d.Dir, time.Now(), d.Log)
	if err != nil {
		return 0, err
	}

	done := 0
	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		if i > 0 && i%drainBatch == 0 {
			if err := yield(ctx); err != nil {
				return done, err
			}
		}
		stored, err := d.replay(ctx, id)
		if err != nil {
			return done, err
		}
		if stored {
			done++
		}
	}
	return done, nil
}

// replay sends one record to the service and decides what happens to the file.
func (d *Drainer) replay(ctx context.Context, id string) (bool, error) {
	path := filepath.Join(d.Dir, id+ext)
	//nolint:gosec // G304: d.Dir joined with a name scan has already checked
	// is a canonical UUID, so the path cannot climb out of the spool
	// directory.
	payload, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		// Another relay's age sweep took it between the listing and
		// here. There is nothing to replay and nothing to report.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("spool: read %s: %w", path, err)
	}

	// The id is the name of the file this payload came out of, and it is
	// passed through untouched. Minting a fresh one here is the single bug
	// that turns replaying an event the service already committed into a
	// second row: events.id is the idempotency key (I-05), so the same id
	// hits ON CONFLICT DO NOTHING and a new one does not.
	status, err := d.Ingest(ctx, ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: id,
		Payload:  payload,
	})
	if err == nil && status == ipc.Committed {
		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("spool: remove the replayed record %s: %w", path, err)
		}
		delete(d.failures, id)
		return true, nil
	}

	// A cancelled context fails the ingest for reasons that have nothing to
	// do with the record, so it does not spend one of the record's
	// attempts.
	if cerr := ctx.Err(); cerr != nil {
		return false, cerr
	}

	// ipc.Rejected with no error counts exactly like an error does. It is a
	// delivery failure, not a drop (I-04), and a drain that only looked at
	// err would delete a record the service never stored.
	d.failures[id]++
	logger(d.Log).Warn("spool: a replayed record was not stored",
		"id", id, "status", status, "attempt", d.failures[id], "error", err)
	if d.failures[id] < maxAttempts {
		return false, nil
	}
	return false, d.quarantine(id)
}

// quarantine moves a record out of the drain's listing and keeps it.
func (d *Drainer) quarantine(id string) error {
	dir := filepath.Join(d.Dir, quarantineDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("spool: create %s: %w", dir, err)
	}
	if err := os.Rename(filepath.Join(d.Dir, id+ext), filepath.Join(dir, id+ext)); err != nil {
		return fmt.Errorf("spool: quarantine %s: %w", id, err)
	}
	delete(d.failures, id)
	logger(d.Log).Warn("spool: quarantined a record the service would not store",
		"id", id, "attempts", maxAttempts, "dir", quarantineDir)
	return nil
}

// yield pauses between batches, and returns the context's error rather than
// finishing the pause when the caller has cancelled.
func yield(ctx context.Context) error {
	t := time.NewTimer(drainPause)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// scan lists the ids of the live records in dir and the bytes they occupy,
// removing any record past maxAge on the way.
//
// The age bound is enforced here rather than in one caller because both
// callers need it: the relay's Write, so that a spool nobody has drained for a
// week does not refuse new events on behalf of records that will never be
// replayed, and the drain, so that a record past the bound is not replayed
// after all. It is the one bound that deletes, and it says so in the log -
// I-04 forbids dropping an event silently, not dropping one at a bound the
// design chose.
//
// Anything that is not <uuid>.json is skipped and left where it is: the temp
// file a Write is staged in, the quarantine subdirectory, and anything a human
// put there. Write is the only thing that produces a record, and a name it
// could not have produced is not one.
//
// os.ReadDir sorts by name, and a UUIDv7's text sorts in the order it was
// minted, so records come back oldest first. That is a convenience, not a
// promise: I-06 makes ordering partial.
func scan(dir string, now time.Time, log *slog.Logger) ([]string, int64, error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("spool: read %s: %w", dir, err)
	}

	ids := make([]string, 0, len(des))
	var total int64
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		id, ok := recordID(de.Name())
		if !ok {
			continue
		}
		info, err := de.Info()
		if err != nil {
			// It was there for the listing and is not there now, so
			// it is not a record this pass has to account for.
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			if err := os.Remove(filepath.Join(dir, de.Name())); err != nil {
				return nil, 0, fmt.Errorf("spool: drop the expired record %s: %w", id, err)
			}
			// This is the one log line in this package the *relay*
			// reaches: scan runs inside Write. It goes to the logger
			// the caller injected and to no other, which is the whole
			// point of the parameter - the relay installs no handler,
			// so reaching slog.Default() there would be reaching an
			// unfiltered one (I-10, and see [logger]).
			//
			// The relay passes nil, so on that path the drop is not
			// reported. It is the service that sweeps in practice: it
			// scans on every drain pass, and the relay scans only when
			// it has an event to spool. Nothing is silently dropped
			// that the service would not itself have reported.
			logger(log).Warn("spool: dropped a record past the age bound",
				"id", id, "age", now.Sub(info.ModTime()).Round(time.Second), "bound", maxAge)
			continue
		}
		ids = append(ids, id)
		total += info.Size()
	}
	return ids, total, nil
}
