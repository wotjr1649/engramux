package service

import (
	"context"
	"sync"
	"time"
)

// readDeadline bounds one read handler end to end: the wait for the gate below
// and every query it then runs.
//
// Nothing bounded a read before Phase 5. internal/pipe hands a handler the
// *service's root context*, which is cancelled at shutdown and never before, and
// SQLite's busy_timeout does not help - that governs lock contention, and spec
// 5.4 leaves no second connection to contend with. What actually waits is
// database/sql pool acquisition, which busy_timeout does not touch.
//
// # The number, and the wrong reason it was 2 s first
//
// It was set to 2 s to match internal/pipe's connection deadline, reasoning that
// past it the client had stopped waiting. **That reasoning was wrong, and it was
// wrong because of the reply-deadline fix in the same change**: the reply now
// takes a fresh deadline after the handler returns, so the connection's 2 s no
// longer bounds how long a client will wait for an answer. What bounds that is
// the CLI's own budget, which is 5 s.
//
// It was also wrong about the cost. **Measured live**, within half an hour of
// the install: `engramux status` was refused with
// `service: group events by cell: context deadline exceeded` on the first call
// after an idle period, against a 108 MB database. The same command five times
// immediately afterwards took 164-499 ms. The two scans a status runs are ~0.5 ms
// and ~8 ms warm at 12,000 events, so what blows the deadline is the page-cache
// I/O of reading the file, not the query - and that grows with the database.
//
// 4 s is what a legitimate cold read may take, with 1 s of the CLI's 5 s left for
// the reply. It is deliberately not a bound on ingest: abandoning a write already
// in flight is worse than answering late, and I-04 is why this product exists.
//
// # What it does not fix, stated rather than left to be found
//
// A cold read of a large database holds the one connection for its whole
// statement, which can exceed the relay's entire 800 ms post-dial budget (spec
// 5.3) on its own. The contention gate measures that warm, where it is 6.6 ms.
// Cold, it is whatever the disk takes. Nothing here changes it: the deadline
// bounds the wait, and what would bound the *work* is an index the events table
// does not have, which is a migration and a decision of its own.
//
// A var so a test can shrink it; nothing else writes to it.
var readDeadline = 4 * time.Second

// readGate orders reads against ingest on the one connection spec 5.4 allows.
//
// # What it is for
//
// MCP is the first non-human caller of that connection (spec 5.9), and a model
// does not wait politely. Two things follow, and they are separate properties
// with separate tests:
//
//   - **Read concurrency of one.** database/sql queues waiting acquisitions, so
//     N concurrent readers put up to N read statements in front of an arriving
//     ingest. With one read in flight there is at most one.
//   - **Ingest priority.** A read that has not started yet waits while any
//     ingest is pending, so a burst of reads cannot keep a relay out.
//
// The second is worth less than it looks once the first is in place, and saying
// so is better than letting somebody discover it: with one read in flight,
// database/sql already serves waiting acquisitions in arrival order, so an
// ingest that queued before a read is served before it either way. What priority
// adds is that the rule is *this package's* and is stated, rather than resting on
// the pool's queue order, which database/sql documents nowhere. The measured
// contention gate passes with either mechanism alone, which is exactly why each
// has a test of its own.
//
// # What it is not
//
// It does not preempt. A read already holding the gate runs to the end of its
// handler; what yields is a read that has not started. Preempting would mean
// abandoning a query mid-scan, and the thing that bounds a read that is already
// running is [readDeadline].
//
// It is also not a lock on the database. The connection is still acquired per
// statement by database/sql, and the checkpointer and the spool drain do not
// pass through here at all - they are background writers with their own pacing
// (spec 5.4), and giving the drain ingest priority would let a long replay
// starve every read.
//
// # The cost, stated
//
// Continuous ingest can starve a read. At the measured rate - 2 events/s across
// eight sessions, about 11 ms each (spec 7.1) - that is a 2% duty cycle, so a
// read gets in; a burst is what would not let it. That is the intended trade and
// [readDeadline] is what bounds it: a read that cannot start returns an error
// rather than waiting forever. I-04 is why the writer wins.
type readGate struct {
	mu   sync.Mutex
	cond *sync.Cond
	// reading is whether a read holds the gate.
	reading bool
	// ingests is how many ingests are pending - waiting for the connection
	// or running. A read may not start while it is above zero.
	ingests int
}

func newReadGate() *readGate {
	g := &readGate{}
	g.cond = sync.NewCond(&g.mu)
	return g
}

// enterIngest marks one ingest pending. It never blocks: an ingest waits for the
// connection and for nothing this type does.
func (g *readGate) enterIngest() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ingests++
}

// leaveIngest marks it done and wakes whatever is waiting.
func (g *readGate) leaveIngest() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ingests--
	g.cond.Broadcast()
}

// acquireRead waits until no read holds the gate and no ingest is pending, or
// until ctx is done.
//
// [context.AfterFunc] rather than a goroutine parked on ctx.Done(): sync.Cond
// cannot wait on a context, so something has to wake the waiter when the
// deadline passes, and a parked goroutine would be a leak for every read that
// finished normally. The callback takes the same mutex, which Wait has released
// while waiting, and stop releases the registration on every path.
func (g *readGate) acquireRead(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	stop := context.AfterFunc(ctx, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.cond.Broadcast()
	})
	defer stop()

	for g.reading || g.ingests > 0 {
		// Checked inside the loop rather than only before it: the wake
		// above is the only thing that distinguishes a deadline from a
		// spurious broadcast.
		if err := ctx.Err(); err != nil {
			return err
		}
		g.cond.Wait()
	}
	g.reading = true
	return nil
}

// pending is how many ingests are marked pending. It exists for the test that
// holds [handlers]' wiring: the ingest closure marking itself is not observable
// from timing - with read concurrency at one, an ingest waits about one read
// statement whether or not it is marked - so the count is the observable.
func (g *readGate) pending() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ingests
}

// releaseRead hands the gate back.
func (g *readGate) releaseRead() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reading = false
	g.cond.Broadcast()
}

// boundedRead runs one read handler under the gate and the deadline.
//
// It is generic over the reply document because the four read types differ in
// nothing else: same gate, same deadline, same order. A copy per type would be
// four places for one of them to be forgotten, which is exactly the shape of
// bug this exists to prevent.
//
// The deadline is applied before the gate is acquired, so the wait for the gate
// is inside it. A read that spends its whole budget queued behind ingest returns
// the same error as one that spends it querying, which is the honest answer: the
// caller asked for something that could not be delivered in time.
func boundedRead[T any](ctx context.Context, g *readGate, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, readDeadline)
	defer cancel()

	if err := g.acquireRead(ctx); err != nil {
		return zero, err
	}
	defer g.releaseRead()
	return fn(ctx)
}
