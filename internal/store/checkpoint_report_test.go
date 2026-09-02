package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestTheCheckpointerReportsEveryAttempt is backlog 31's seam in this
// package: the service learns how its checkpoints go only if the loop says
// so after each one. The timer trigger is what fires here - the threshold is
// out of reach - and a report with no error is what a healthy loop produces.
func TestTheCheckpointerReportsEveryAttempt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engramux.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	type report struct {
		at  time.Time
		err error
	}
	reports := make(chan report, 8)
	c := &Checkpointer{
		DB:        db,
		Path:      path,
		Threshold: 1 << 40,
		Interval:  20 * time.Millisecond,
		Poll:      5 * time.Millisecond,
		Report: func(at time.Time, err error) {
			select {
			case reports <- report{at, err}:
			default:
			}
		},
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	select {
	case r := <-reports:
		if r.err != nil {
			t.Errorf("the first checkpoint reported %v, want no error", r.err)
		}
		if r.at.IsZero() || time.Since(r.at) > time.Minute {
			t.Errorf("the report's instant is %v, want about now", r.at)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no checkpoint was reported within 5 s at a 20 ms interval")
	}
}
