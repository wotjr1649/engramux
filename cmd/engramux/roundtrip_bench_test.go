package main

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/pipe"
	"github.com/wotjr1649/engramux/internal/store"
)

// This file is spec 5.3's harness. The p50/p95 round trip that section quotes
// lost the code that produced it, so from here the number and the thing that
// measures it live together.
//
// It is in the root module rather than under docs/evidence/ because those
// modules are nested and outside ./..., so they cannot import internal/ - and
// this measurement is worth nothing unless it runs the real relay against the
// real server against a real database.
//
// Reproduce:
//
//	go test -p 1 -run '^$' -bench BenchmarkRelayRoundTrip -benchtime 300x ./cmd/engramux/
//	go test -p 1 -run '^$' -bench BenchmarkRelayDialWhileBusy -benchtime 300x ./cmd/engramux/
//
// One at a time: both listen on the relay's single fixed pipe name, so a
// -bench pattern matching both makes the second wait for the first to tear
// down.

// benchServer stands up the production pipe server on the relay's real pipe
// name, backed by a real SQLite database with the real migrations applied and
// the real store.Ingest behind it. It returns the pool so a benchmark can
// check that the work it timed actually happened.
//
// The database is what makes this mean anything. A named-pipe round trip on
// its own is tens of microseconds; the rest is the commit under
// synchronous=FULL (spec 5.4), and the relay's budget has to cover that.
func benchServer(b *testing.B) *sql.DB {
	b.Helper()

	// pipe.Serve logs a warning for every connection that closes without
	// sending a frame, which BenchmarkRelayDialWhileBusy does once per
	// iteration. Hundreds of those lines are noise around the number, and
	// every real failure here is a b.Fatalf rather than a log line.
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	b.Cleanup(func() { slog.SetDefault(previous) })

	ctx := b.Context()
	db, err := store.Open(ctx, filepath.Join(b.TempDir(), "engramux.db"))
	if err != nil {
		b.Fatalf("store.Open: %v", err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Errorf("close db: %v", err)
		}
	})
	if err := store.Migrate(ctx, db); err != nil {
		b.Fatalf("store.Migrate: %v", err)
	}

	name := relayPipeName(b)
	l, err := pipe.Listen(name, currentSID(b))
	if err != nil {
		b.Fatalf("Listen(%s): %v\nSomething else holds this benchmark's pipe - another copy of this binary, or a leaked listener.", name, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- pipe.Serve(ctx, l, pipe.Handler{
			Ingest: func(ctx context.Context, env ipc.Envelope) (ipc.AckStatus, error) {
				return store.Ingest(ctx, db, env, store.SourcePipe, time.Now())
			},
		})
	}()
	b.Cleanup(func() {
		_ = l.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			b.Error("pipe.Serve did not return within 10s of Close")
		}
	})
	return db
}

// report sorts the samples and reports the percentiles spec 5.3 quotes. A
// benchmark's own ns/op is a mean, and a mean is the wrong statistic for a
// budget: what the budget has to cover is the tail.
func report(b *testing.B, samples []time.Duration) {
	b.Helper()
	if len(samples) == 0 {
		b.Fatal("no samples")
	}
	slices.Sort(samples)
	pick := func(p float64) time.Duration {
		i := min(int(p*float64(len(samples))), len(samples)-1)
		return samples[i]
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

	b.ReportMetric(ms(pick(0.50)), "ms_p50")
	b.ReportMetric(ms(pick(0.95)), "ms_p95")
	b.ReportMetric(ms(samples[len(samples)-1]), "ms_max")
	b.ReportMetric(float64(len(samples)), "samples")
}

// BenchmarkRelayRoundTrip is the headline number: one call to the relay's own
// deliver per iteration - dial, frame out, ingest, ACK back, ACK verified -
// against the real server.
//
// Every iteration mints a fresh id, because a repeated id is a duplicate and
// store.Ingest's ON CONFLICT DO NOTHING would make every iteration after the
// first cheaper than a real one. The row count at the end is what says the
// timed work happened: a round trip that stored nothing would be very fast and
// would look like an improvement.
func BenchmarkRelayRoundTrip(b *testing.B) {
	db := benchServer(b)

	raw, err := fixtures.Fixture{File: fixtures.CodexSessionEnd}.Bytes()
	if err != nil {
		b.Fatalf("fixture: %v", err)
	}

	samples := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for range b.N {
		id, err := uuid.NewV7()
		if err != nil {
			b.Fatalf("uuid.NewV7: %v", err)
		}
		start := time.Now()
		if err := deliver(start, id.String(), raw); err != nil {
			b.Fatalf("deliver: %v", err)
		}
		samples = append(samples, time.Since(start))
	}
	b.StopTimer()

	var rows int
	if err := db.QueryRowContext(b.Context(), `SELECT count(*) FROM events`).Scan(&rows); err != nil {
		b.Fatalf("count events: %v", err)
	}
	if rows != b.N {
		b.Fatalf("events table holds %d rows after %d iterations: the round trip did not store what it timed", rows, b.N)
	}

	report(b, samples)
}

// BenchmarkRelayDialWhileBusy measures the one dial cost that is not
// negligible, and it is a property of go-winio rather than of this code.
//
// go-winio's listener creates the next pipe instance only when Accept is
// called, so between a client connecting and the accept loop coming back
// round there is a window in which the pipe exists but has no free instance.
// A dial landing in that window gets ERROR_PIPE_BUSY, and tryDialPipe's answer
// to that is an unconditional 10 ms sleep before it tries again (go-winio
// v0.6.2, pipe.go:229). There is no backoff and no shorter first retry.
//
// This loop dials and closes with nothing in between, so it lands in that
// window nearly every time - which is why it is a separate benchmark and not
// a component of the one above: BenchmarkRelayRoundTrip's samples show no
// retries at all, because the ACK exchange is longer than the window.
//
// What it bounds is the cost of two relays arriving back to back. Spec 5.1
// puts expected concurrent relays at 0.28 even at a hundred sessions, so this
// is the rare case, and 10 ms against the 200 ms dial budget (spec 5.3) leaves
// room for 19 more.
func BenchmarkRelayDialWhileBusy(b *testing.B) {
	benchServer(b)
	name := relayPipeName(b)

	samples := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for range b.N {
		ctx, cancel := context.WithTimeout(b.Context(), dialBudget)
		start := time.Now()
		conn, err := dial(ctx, name)
		samples = append(samples, time.Since(start))
		if err != nil {
			cancel()
			b.Fatalf("dial: %v", err)
		}
		_ = conn.Close()
		cancel()
	}
	b.StopTimer()
	report(b, samples)
}
