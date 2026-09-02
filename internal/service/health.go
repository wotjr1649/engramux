package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/secret"
)

// health is what the status reply says about the service's own run that no
// count of rows can: how many errors it has logged since it started, and how
// its last checkpoint went (backlog 31).
//
// Spec 5.6 rev.4 put these in a `health.json` the service would write and
// `doctor` would read, because `doctor` is a separate process and cannot see
// this memory. Decided 2026-08-30: the reply carries them instead. No file
// means no failure mode where a stale file explains a live service, and the
// cost - a dead service reports nothing - is one `doctor` already pays for
// every other number in its service section.
type health struct {
	errors     atomic.Int64
	checkpoint atomic.Pointer[ipc.CheckpointResult]
}

func newHealth() *health { return &health{} }

// counting wraps inner so that every record at ERROR or above is counted on
// its way to it. It sits inside I-10's redacting handler, so it sees the
// record the file sees; what it counts is levels, not bytes, so the order
// does not matter and inside is where a handler that adds nothing belongs.
func (h *health) counting(inner slog.Handler) slog.Handler {
	return errorCounter{inner: inner, h: h}
}

type errorCounter struct {
	inner slog.Handler
	h     *health
}

func (c errorCounter) Enabled(ctx context.Context, l slog.Level) bool { return c.inner.Enabled(ctx, l) }

func (c errorCounter) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		c.h.errors.Add(1)
	}
	return c.inner.Handle(ctx, r)
}

// WithAttrs and WithGroup wrap what inner returns, so a derived logger still
// counts into the same health - slog derives handlers freely, and a counter
// that stopped at the first derivation would miss every record with a
// context attribute.
func (c errorCounter) WithAttrs(attrs []slog.Attr) slog.Handler {
	return errorCounter{inner: c.inner.WithAttrs(attrs), h: c.h}
}

func (c errorCounter) WithGroup(name string) slog.Handler {
	return errorCounter{inner: c.inner.WithGroup(name), h: c.h}
}

// recordCheckpoint is the [store.Checkpointer]'s report seam: one call per
// attempt, with the error or nil. The error's text goes through the mask,
// because it names the database path and the reply is an egress (I-10).
func (h *health) recordCheckpoint(at time.Time, err error) {
	r := &ipc.CheckpointResult{AtMS: at.UnixMilli()}
	if err != nil {
		r.Error = secret.MaskString(err.Error())
	}
	h.checkpoint.Store(r)
}

// snapshot is what [status] puts on the reply: the count so far and the last
// checkpoint result, or nil before the first attempt.
func (h *health) snapshot() (errors int64, last *ipc.CheckpointResult) {
	return h.errors.Load(), h.checkpoint.Load()
}
