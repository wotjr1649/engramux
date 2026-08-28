package spool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// idN is a valid UUID whose text sorts in n's order, so a test that writes
// idN(1), idN(2), idN(3) knows what order os.ReadDir hands them back in. The
// decimal digits %012d produces are all valid hex, and uuid.Validate is the
// only thing Write checks.
func idN(n int) string {
	return fmt.Sprintf("0192f0c0-0000-7000-8000-%012d", n)
}

// setBounds replaces all three bounds for one test and restores them after.
//
// All three, always. A test of one bound proves nothing unless the two bounds
// it is not about are visibly out of reach: "the write was refused" is not
// "the count bound refused it". Safe without a lock because no test in this
// package calls t.Parallel and go test runs with -p 1.
func setBounds(t *testing.T, records int, size int64, age time.Duration) {
	t.Helper()
	r, s, a := maxRecords, maxBytes, maxAge
	maxRecords, maxBytes, maxAge = records, size, age
	t.Cleanup(func() { maxRecords, maxBytes, maxAge = r, s, a })
}

// setBatch shrinks the drain's batch size and the pause between batches, so a
// cancellation test can reach the pause without waiting out the production
// one.
func setBatch(t *testing.T, batch int, pause time.Duration) {
	t.Helper()
	b, p := drainBatch, drainPause
	drainBatch, drainPause = batch, pause
	t.Cleanup(func() { drainBatch, drainPause = b, p })
}

// collector is an Ingest that records every envelope it was handed and answers
// Committed, except for the ids in poison - which fail the way a record the
// service cannot store fails.
type collector struct {
	seen []ipc.Envelope
	// poison maps an id to the failure it produces: true means an error,
	// false means a Rejected status with no error. Both are failures, and a
	// drain that checks only one of them loses the events answered the
	// other way.
	poison map[string]bool
	// before runs before each ingest, so a test can cancel a context from
	// inside the drain loop.
	before func(env ipc.Envelope)
}

func (c *collector) ingest(_ context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
	if c.before != nil {
		c.before(env)
	}
	c.seen = append(c.seen, env)
	withError, isPoison := c.poison[env.IngestID]
	switch {
	case isPoison && withError:
		return ipc.Rejected, fmt.Errorf("test: %s cannot be stored", env.IngestID)
	case isPoison:
		return ipc.Rejected, nil
	default:
		return ipc.Committed, nil
	}
}

// ids returns the ingest ids the collector was handed, in order.
func (c *collector) ids() []string {
	out := make([]string, 0, len(c.seen))
	for _, env := range c.seen {
		out = append(out, env.IngestID)
	}
	return out
}

// requireNames asserts dir holds exactly want, in sorted order.
func requireNames(t *testing.T, dir string, what string, want ...string) {
	t.Helper()
	got := entries(t, dir)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s: spool dir holds %q, want %q", what, got, want)
	}
}

// requireAbsent asserts name does not exist under dir, and that the reason is
// that it is not there rather than any other error.
func requireAbsent(t *testing.T, dir, name, what string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, name))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s: stat %s = %v, want fs.ErrNotExist", what, name, err)
	}
}

// requireBytes asserts the file at path holds exactly want.
func requireBytes(t *testing.T, path string, want []byte, what string) {
	t.Helper()
	//nolint:gosec // G304: a path this test built inside its own t.TempDir
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: read %s: %v", what, path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: %s holds %q, want %q", what, path, got, want)
	}
}

// TestWriteRefusesARecordOnceTheCountBoundIsReached is the first of spec 5.6's
// three bounds. The byte and age bounds are set far out of reach, so a refusal
// here can only be the count.
func TestWriteRefusesARecordOnceTheCountBoundIsReached(t *testing.T) {
	setBounds(t, 3, 1<<30, time.Hour)
	dir := t.TempDir()
	payload := []byte(`{"n":1}`)

	// Three fit. The third one succeeding is half the assertion: a bound
	// that fires one record early is as wrong as one that never fires.
	for i := 1; i <= 3; i++ {
		if err := Write(dir, idN(i), payload); err != nil {
			t.Fatalf("Write %d of 3: %v", i, err)
		}
	}

	err := Write(dir, idN(4), payload)
	if !errors.Is(err, ErrRecordBound) {
		t.Fatalf("Write past the count bound = %v, want ErrRecordBound", err)
	}
	if errors.Is(err, ErrByteBound) {
		t.Fatalf("Write past the count bound reported the byte bound too: %v", err)
	}
	requireAbsent(t, dir, idN(4)+ext, "the refused record")
	requireNames(t, dir, "after the refusal", idN(1)+ext, idN(2)+ext, idN(3)+ext)
}

// TestWriteRefusesARecordOnceTheByteBoundIsReached is the second bound, and it
// pins the boundary rather than the neighbourhood: the same one-byte record is
// refused at a cap of exactly the bytes on disk and accepted at one byte more.
func TestWriteRefusesARecordOnceTheByteBoundIsReached(t *testing.T) {
	payload := []byte(`{"n":123456}`)
	full := int64(2 * len(payload))
	setBounds(t, 1000, full, time.Hour)
	dir := t.TempDir()

	for i := 1; i <= 2; i++ {
		if err := Write(dir, idN(i), payload); err != nil {
			t.Fatalf("Write %d of 2: %v", i, err)
		}
	}

	// The spool now holds exactly the cap, so one more byte crosses it.
	err := Write(dir, idN(3), []byte("x"))
	if !errors.Is(err, ErrByteBound) {
		t.Fatalf("Write past the byte bound = %v, want ErrByteBound", err)
	}
	if errors.Is(err, ErrRecordBound) {
		t.Fatalf("Write past the byte bound reported the count bound too: %v", err)
	}
	requireAbsent(t, dir, idN(3)+ext, "the refused record")

	// One more byte of headroom and the identical write is accepted, which
	// is what makes the refusal above the bound and not the write.
	setBounds(t, 1000, full+1, time.Hour)
	if err := Write(dir, idN(3), []byte("x")); err != nil {
		t.Fatalf("Write with one byte of headroom: %v", err)
	}
	requireNames(t, dir, "after the accepted write", idN(1)+ext, idN(2)+ext, idN(3)+ext)
}

// TestARecordPastTheAgeBoundIsDropped is the third bound. Two records
// straddle it by one second each way, so the bound itself is the only thing
// that can tell them apart - not "one is newer than the other".
func TestARecordPastTheAgeBoundIsDropped(t *testing.T) {
	setBounds(t, 1000, 1<<30, time.Hour)
	dir := t.TempDir()
	stale, live := idN(1), idN(2)
	for _, id := range []string{stale, live} {
		if err := Write(dir, id, []byte(`{"n":1}`)); err != nil {
			t.Fatalf("Write %s: %v", id, err)
		}
	}
	backdate(t, filepath.Join(dir, stale+ext), maxAge+time.Second)
	backdate(t, filepath.Join(dir, live+ext), maxAge-time.Second)

	c := &collector{}
	d := &Drainer{Dir: dir, Ingest: c.ingest}
	n, err := d.Drain(t.Context())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if n != 1 {
		t.Fatalf("Drain replayed %d records, want 1", n)
	}
	if got := c.ids(); !slices.Equal(got, []string{live}) {
		t.Fatalf("the drain replayed %q, want only the record inside the age bound %q", got, live)
	}
	requireNames(t, dir, "after the drain")

	// Write sweeps too, because the relay is the only writer: a bound the
	// writer does not enforce is a bound the disk does not have.
	old := idN(3)
	if err := Write(dir, old, []byte(`{"n":1}`)); err != nil {
		t.Fatalf("Write %s: %v", old, err)
	}
	backdate(t, filepath.Join(dir, old+ext), maxAge+time.Second)
	if err := Write(dir, idN(4), []byte(`{"n":1}`)); err != nil {
		t.Fatalf("Write %s: %v", idN(4), err)
	}
	requireNames(t, dir, "after a write swept the stale record", idN(4)+ext)
}

// backdate moves path's modification time back by age.
func backdate(t *testing.T, path string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("backdate %s: %v", path, err)
	}
}

// TestDrainReplaysEveryRecordUnderTheIDItWasWrittenUnder is I-05 on the drain
// side. The envelope is asserted field by field, and the id specifically: a
// drain that mints a fresh one turns the replay of an already-committed event
// into a second row.
func TestDrainReplaysEveryRecordUnderTheIDItWasWrittenUnder(t *testing.T) {
	dir := t.TempDir()
	want := map[string][]byte{}
	for i := 1; i <= 3; i++ {
		id := idN(i)
		// '<' and '&' are here on purpose: encoding/json HTML-escapes
		// them, so a drain that re-marshalled the payload rewrites these
		// bytes and the comparison below catches it.
		p := fmt.Appendf(nil, `{"n":%d,"cmd":"a < b && c"}`, i)
		if err := Write(dir, id, p); err != nil {
			t.Fatalf("Write %s: %v", id, err)
		}
		want[id] = p
	}

	c := &collector{}
	d := &Drainer{Dir: dir, Ingest: c.ingest}
	n, err := d.Drain(t.Context())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if n != 3 {
		t.Fatalf("Drain replayed %d records, want 3", n)
	}
	if got := c.ids(); !slices.Equal(got, []string{idN(1), idN(2), idN(3)}) {
		t.Fatalf("the drain replayed %q, want the three ids the records were written under", got)
	}
	for _, env := range c.seen {
		if env.Version != ipc.Version {
			t.Errorf("%s: envelope version %q, want %q", env.IngestID, env.Version, ipc.Version)
		}
		if env.Type != ipc.IngestEvent {
			t.Errorf("%s: envelope type %q, want %q", env.IngestID, env.Type, ipc.IngestEvent)
		}
		if !bytes.Equal(env.Payload, want[env.IngestID]) {
			t.Errorf("%s: payload %q, want %q", env.IngestID, env.Payload, want[env.IngestID])
		}
	}
	requireNames(t, dir, "after a clean drain")
}

// TestDrainIgnoresAPartiallyWrittenRecord: a write in progress is staged under
// a name the drain's listing cannot pick up (spec 5.6), so a half-written
// record is never replayed as if it were whole.
func TestDrainIgnoresAPartiallyWrittenRecord(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, idN(1), []byte(`{"n":1}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	partial := filepath.Join(dir, ".partial-2130517550")
	half := []byte(`{"n":2`)
	if err := os.WriteFile(partial, half, 0o600); err != nil {
		t.Fatalf("write the partial record: %v", err)
	}

	c := &collector{}
	d := &Drainer{Dir: dir, Ingest: c.ingest}
	n, err := d.Drain(t.Context())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if n != 1 {
		t.Fatalf("Drain replayed %d records, want 1", n)
	}
	if got := c.ids(); !slices.Equal(got, []string{idN(1)}) {
		t.Fatalf("the drain replayed %q, want only the complete record", got)
	}
	// Left alone, not consumed: the write that staged it may still be
	// running, and it is not the drain's to delete.
	requireBytes(t, partial, half, "the partial record after the drain")
}

// TestWriteNeverExposesAPartialRecordUnderTheFinalName is the atomic-rename
// half of spec 5.6, and the only assertion that can tell a staged write from
// an in-place one. A poller watches the final name for the whole of a 4 MiB
// write: with the rename, the name either does not exist or holds every byte;
// written in place it exists at zero bytes for the duration.
func TestWriteNeverExposesAPartialRecordUnderTheFinalName(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, idN(1)+ext)
	payload := bytes.Repeat([]byte("x"), 4<<20)

	stop := make(chan struct{})
	bad := make(chan int64, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if fi, err := os.Stat(final); err == nil && fi.Size() != int64(len(payload)) {
				bad <- fi.Size()
				return
			}
			select {
			case <-stop:
				return
			default:
			}
		}
	}()

	if err := Write(dir, idN(1), payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	close(stop)
	wg.Wait()

	select {
	case n := <-bad:
		t.Fatalf("%s was visible holding %d bytes mid-write, want %d bytes or nothing at all",
			idN(1)+ext, n, len(payload))
	default:
	}
	requireBytes(t, final, payload, "the completed record")
}

// TestDrainQuarantinesAPoisonRecordAndKeepsMakingProgress asserts both halves
// of the quarantine, because a quarantine that stalls the queue is not a fix:
//
//   - the record is retried exactly maxAttempts times and then moved aside,
//     still holding its bytes - quarantined means kept, not deleted;
//   - every other record drains in the same pass, including the pass that does
//     the quarantining.
func TestDrainQuarantinesAPoisonRecordAndKeepsMakingProgress(t *testing.T) {
	dir := t.TempDir()
	poison := idN(1)
	poisonBytes := []byte(`{"poison":true}`)
	if err := Write(dir, poison, poisonBytes); err != nil {
		t.Fatalf("Write the poison record: %v", err)
	}
	// Numbered above the poison record so os.ReadDir hands them back
	// behind it: "the records behind it" is the half that a drain aborting
	// on the first failure would lose.
	for i := 2; i <= 4; i++ {
		if err := Write(dir, idN(i), []byte(`{"n":1}`)); err != nil {
			t.Fatalf("Write %s: %v", idN(i), err)
		}
	}

	c := &collector{poison: map[string]bool{poison: true}}
	d := &Drainer{Dir: dir, Ingest: c.ingest}
	quarantined := filepath.Join(dir, quarantineDir, poison+ext)

	// Pass 1 replays all four: the poison record fails and the three
	// behind it are stored anyway.
	n, err := d.Drain(t.Context())
	if err != nil {
		t.Fatalf("Drain 1: %v", err)
	}
	if n != 3 {
		t.Fatalf("Drain 1 replayed %d records, want 3 - the three behind the poison record", n)
	}
	requireNames(t, dir, "after pass 1", poison+ext)

	// Passes 2..maxAttempts-1 keep retrying it and keep it where it is.
	for pass := 2; pass < maxAttempts; pass++ {
		if n, err := d.Drain(t.Context()); err != nil || n != 0 {
			t.Fatalf("Drain %d = (%d, %v), want (0, nil)", pass, n, err)
		}
		requireNames(t, dir, fmt.Sprintf("after pass %d", pass), poison+ext)
		requireAbsent(t, dir, filepath.Join(quarantineDir, poison+ext),
			fmt.Sprintf("the poison record after only %d attempts", pass))
	}

	// The pass that quarantines still makes progress on a record behind it.
	fresh := idN(5)
	if err := Write(dir, fresh, []byte(`{"n":5}`)); err != nil {
		t.Fatalf("Write %s: %v", fresh, err)
	}
	n, err = d.Drain(t.Context())
	if err != nil {
		t.Fatalf("Drain %d: %v", maxAttempts, err)
	}
	if n != 1 {
		t.Fatalf("the quarantining pass replayed %d records, want 1", n)
	}
	if got := len(c.seen); got != maxAttempts+4 {
		t.Fatalf("ingest was called %d times, want %d", got, maxAttempts+4)
	}
	requireNames(t, dir, "after the quarantining pass", quarantineDir)
	requireBytes(t, quarantined, poisonBytes, "the quarantined record")

	// And it is never replayed again: quarantined is out of the drain's
	// listing, so the loop cannot spin on it.
	seenBefore := len(c.seen)
	if n, err := d.Drain(t.Context()); err != nil || n != 0 {
		t.Fatalf("Drain after the quarantine = (%d, %v), want (0, nil)", n, err)
	}
	if len(c.seen) != seenBefore {
		t.Fatalf("ingest was called %d more times after the quarantine, want 0", len(c.seen)-seenBefore)
	}
	requireBytes(t, quarantined, poisonBytes, "the quarantined record after a later drain")
}

// TestDrainKeepsARecordTheServiceRejectedWithoutAnError closes the other half
// of "failed": ipc.Rejected with a nil error is a delivery failure exactly
// like an error is (I-04), and a drain that only checks err would delete a
// record that was never stored.
func TestDrainKeepsARecordTheServiceRejectedWithoutAnError(t *testing.T) {
	dir := t.TempDir()
	id := idN(1)
	if err := Write(dir, id, []byte(`{"n":1}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	c := &collector{poison: map[string]bool{id: false}}
	d := &Drainer{Dir: dir, Ingest: c.ingest}
	n, err := d.Drain(t.Context())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if n != 0 {
		t.Fatalf("Drain replayed %d records, want 0", n)
	}
	requireNames(t, dir, "after a rejected replay", id+ext)
}

// TestDrainStopsOnContextCancellationAndLeavesTheRestIntact. The drain holds
// the service's single connection (spec 5.4), so a shutdown has to reach it
// inside a batch, not only between two of them - and everything it did not get
// to must still be on disk, un-quarantined.
func TestDrainStopsOnContextCancellationAndLeavesTheRestIntact(t *testing.T) {
	for name, batch := range map[string]int{
		"inside a batch":  64,
		"between batches": 1,
	} {
		t.Run(name, func(t *testing.T) {
			setBatch(t, batch, 50*time.Millisecond)
			dir := t.TempDir()
			for i := 1; i <= 4; i++ {
				if err := Write(dir, idN(i), []byte(`{"n":1}`)); err != nil {
					t.Fatalf("Write %s: %v", idN(i), err)
				}
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c := &collector{before: func(env ipc.Envelope) {
				if env.IngestID == idN(1) {
					cancel()
				}
			}}
			d := &Drainer{Dir: dir, Ingest: c.ingest}

			n, err := d.Drain(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Drain after a cancel = %v, want context.Canceled", err)
			}
			if n != 1 {
				t.Fatalf("Drain replayed %d records before the cancel took effect, want 1", n)
			}
			requireNames(t, dir, "after the cancel", idN(2)+ext, idN(3)+ext, idN(4)+ext)
			requireAbsent(t, dir, quarantineDir, "the quarantine directory after a cancel")
		})
	}
}

// TestDrainYieldsBetweenBatches is the other half of a bounded batch. The
// drain competes with live ingest for the one connection (spec 5.4), and
// without the pause a long spool runs the whole way through without a relay
// inside its 1 s budget getting a turn.
//
// A lower bound on elapsed time is the only shape of this assertion that
// cannot be flaky: a Go timer never fires early, so the test can only go red
// when the pause is not there. The cancellation test above does not cover
// this - with the per-record check in place it stops the drain either way.
func TestDrainYieldsBetweenBatches(t *testing.T) {
	const pause = 40 * time.Millisecond
	setBatch(t, 1, pause)
	dir := t.TempDir()
	for i := 1; i <= 4; i++ {
		if err := Write(dir, idN(i), []byte(`{"n":1}`)); err != nil {
			t.Fatalf("Write %s: %v", idN(i), err)
		}
	}

	c := &collector{}
	d := &Drainer{Dir: dir, Ingest: c.ingest}
	start := time.Now()
	n, err := d.Drain(t.Context())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if n != 4 {
		t.Fatalf("Drain replayed %d records, want 4", n)
	}
	// Four records at a batch size of one is three batch boundaries.
	if want := 3 * pause; elapsed < want {
		t.Fatalf("draining 4 records at a batch size of 1 took %v, want at least %v - it never paused between batches",
			elapsed, want)
	}
}
