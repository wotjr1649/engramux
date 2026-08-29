package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// The three mechanisms spec 5.9 puts around the single connection are asserted
// one at a time here, against the gate itself, because the composed property -
// TestPhase5GateAReaderDoesNotPushIngestPastItsBudget - passes with any two of
// them and would not say which one had gone.

// TestOnlyOneReadHoldsTheGate is the read-concurrency mechanism.
//
// database/sql queues waiting acquisitions, so N concurrent readers put up to N
// read statements in front of an arriving ingest. This is what makes it one.
func TestOnlyOneReadHoldsTheGate(t *testing.T) {
	g := newReadGate()
	if err := g.acquireRead(t.Context()); err != nil {
		t.Fatalf("the first read could not start: %v", err)
	}

	second := make(chan error, 1)
	go func() { second <- g.acquireRead(t.Context()) }()

	// It must still be waiting. A poll would be a race in the other
	// direction; a short wait that the second read is *expected* to lose is
	// the assertion.
	select {
	case err := <-second:
		t.Fatalf("a second read started while the first held the gate (err %v)", err)
	case <-time.After(100 * time.Millisecond):
	}

	g.releaseRead()
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("the second read failed once the gate was free: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second read did not start after the first released the gate")
	}
	g.releaseRead()
}

// TestAPendingIngestGoesBeforeAQueuedRead is the priority mechanism.
//
// It is the half that is about I-04: a burst of reads must not be able to keep
// a relay out. A read that has not started yet waits while any ingest is
// pending; a read that is already running is not preempted, which is
// [readGate]'s stated boundary.
func TestAPendingIngestGoesBeforeAQueuedRead(t *testing.T) {
	g := newReadGate()
	g.enterIngest()

	// Nothing holds the gate - no read is running - and the read still must
	// not start. That is the whole property: it is yielding to the ingest
	// and not to another read.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := g.acquireRead(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a read started with an ingest pending: %v", err)
	}

	started := make(chan error, 1)
	go func() { started <- g.acquireRead(t.Context()) }()
	select {
	case err := <-started:
		t.Fatalf("a read started while the ingest was still pending (err %v)", err)
	case <-time.After(100 * time.Millisecond):
	}

	g.leaveIngest()
	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("the read failed once the ingest finished: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the read did not start after the ingest finished")
	}
	g.releaseRead()
}

// TestAReadIsBoundedByTheQueryDeadline is the deadline mechanism, and it covers
// both halves of what the deadline bounds: the wait for the gate, and the work
// after it.
//
// The error is context.DeadlineExceeded in both cases and deliberately so - the
// caller asked for something that could not be delivered in time, and which side
// of the gate the time went on is not the caller's business.
func TestAReadIsBoundedByTheQueryDeadline(t *testing.T) {
	restore := readDeadline
	readDeadline = 100 * time.Millisecond
	t.Cleanup(func() { readDeadline = restore })

	// The outer context is the backstop, and it is deliberately far longer
	// than readDeadline: a build where readDeadline does not bound anything
	// would otherwise hang until `go test` gave up on the whole package,
	// which is a detection but a useless one. With the backstop the same
	// break fails in a few seconds and says which deadline fired, because
	// the elapsed time is asserted alongside the error.
	const backstop = 5 * time.Second

	t.Run("waiting for the gate", func(t *testing.T) {
		g := newReadGate()
		g.enterIngest()
		t.Cleanup(g.leaveIngest)

		ctx, cancel := context.WithTimeout(t.Context(), backstop)
		defer cancel()

		ran := false
		start := time.Now()
		_, err := boundedRead(ctx, g, func(context.Context) (int, error) {
			ran = true
			return 1, nil
		})
		requireReadDeadlineFired(t, time.Since(start), err)
		if ran {
			t.Error("the handler ran even though the gate was never acquired")
		}
	})

	t.Run("inside the handler", func(t *testing.T) {
		g := newReadGate()
		ctx, cancel := context.WithTimeout(t.Context(), backstop)
		defer cancel()

		start := time.Now()
		_, err := boundedRead(ctx, g, func(ctx context.Context) (int, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		})
		requireReadDeadlineFired(t, time.Since(start), err)
		// And the gate is handed back, so the deadline does not leave a
		// read wedged in front of every later one.
		if err := g.acquireRead(t.Context()); err != nil {
			t.Fatalf("the gate was not released by the timed-out read: %v", err)
		}
		g.releaseRead()
	})
}

// requireReadDeadlineFired holds that the read ended on readDeadline and not on
// the backstop above it. Both produce context.DeadlineExceeded, so the error
// alone cannot tell them apart and the elapsed time is what does.
func requireReadDeadlineFired(t *testing.T, took time.Duration, err error) {
	t.Helper()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("boundedRead = %v, want a deadline", err)
	}
	if limit := 10 * readDeadline; took > limit {
		t.Fatalf("the read took %s, over %s: readDeadline is not what bounded it", took, limit)
	}
}

// TestTheIngestHandlerMarksItselfPending holds the wiring rather than the gate.
//
// It exists because a deliberate break found a hole: taking enterIngest and
// leaveIngest out of [handlers]' ingest closure left every other test in this
// package green. The three mechanism tests drive the gate directly, and the
// contention gate passes on read concurrency alone - with one read in flight, an
// ingest waits about one read statement whether or not it is marked. So nothing
// was holding the production wiring, and this is that.
//
// # Freezing an ingest mid-flight
//
// The test opens a transaction of its own, which takes the one connection spec
// 5.4 allows. store.Ingest's own BeginTx then blocks, so the ingest is stopped
// *after* the closure marked it pending and before it could unmark it - which is
// the window the assertion needs and the only way to hold it open.
func TestTheIngestHandlerMarksItselfPending(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))
	gate := newReadGate()
	h := handlers(db, filepath.Join(dir, dbName), filepath.Join(dir, spoolDir), time.Now(), gate)

	if n := gate.pending(); n != 0 {
		t.Fatalf("%d ingests pending before anything ran", n)
	}

	// Takes the single connection and holds it.
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("hold the connection: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := h.Ingest(t.Context(), ipc.Envelope{
			Version: ipc.Version, Type: ipc.IngestEvent,
			IngestID: "8f1c2a10-0000-7000-8000-00000000c001",
			Payload:  []byte(`{"hook_event_name":"Stop","session_id":"wiring"}`),
		})
		done <- err
	}()

	// Polled rather than slept on: the goroutine has to be scheduled and
	// reach the closure, and a fixed sleep would either be a flake or be
	// longer than it needs to be.
	deadline := time.Now().Add(10 * time.Second)
	for gate.pending() == 0 {
		if time.Now().After(deadline) {
			_ = tx.Rollback()
			t.Fatal("the ingest handler never marked itself pending: " +
				"a read would queue in front of a relay")
		}
		time.Sleep(time.Millisecond)
	}

	// And it is unmarked once the connection is free and the ingest lands.
	if err := tx.Commit(); err != nil {
		t.Fatalf("release the connection: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n := gate.pending(); n != 0 {
		t.Errorf("%d ingests still pending after the ingest returned - a read would never start", n)
	}
}

// cliBudget is what one CLI request is allowed end to end, restated here because
// internal/service does not import cmd/engramux. A change to that budget has to
// reach this line by hand, which is why the number is written out.
const cliBudget = 5 * time.Second

// refusedStatusDeadline is the value of readDeadline that refused a real
// `engramux status` on the installed service, against a 108 MB database, on the
// first call after an idle period. It is here as a floor, not as a target.
const refusedStatusDeadline = 2 * time.Second

// TestTheReadDeadlineSitsBetweenAColdReadAndTheClientsPatience is the guard the
// regression this file's readDeadline comment describes did not have.
//
// A deadline shorter than a legitimate read turns a working command into a
// refusal, which is what 2 s did. A deadline longer than the client's own budget
// bounds nothing a client will wait for, and leaves the reply no room. Neither
// end is a matter of taste, and both are numbers, so this holds them.
//
// It cannot assert the cold cost itself - a cold page cache is not reproducible
// on demand, which is exactly why the first value was a guess. What it can do is
// stop the number moving back below the one that was measured failing.
func TestTheReadDeadlineSitsBetweenAColdReadAndTheClientsPatience(t *testing.T) {
	if readDeadline <= refusedStatusDeadline {
		t.Errorf("readDeadline is %s, which is not more than the %s that refused a real status "+
			"against a 108 MB database on a cold cache", readDeadline, refusedStatusDeadline)
	}
	if readDeadline >= cliBudget {
		t.Errorf("readDeadline is %s, which leaves the reply nothing of the CLI's %s budget: "+
			"a read that outlasts the client bounds nothing the client will wait for",
			readDeadline, cliBudget)
	}
}
