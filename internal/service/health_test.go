package service

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStatusCountsErrorsAndCarriesTheLastCheckpoint is backlog 31: the two
// numbers spec 5.6 once assigned to a health.json ride on the status reply
// instead. The counter counts ERROR and above through a derived logger too,
// which is how the service's own code logs; the checkpoint result is the
// instant and the error, masked, or no error - and before any attempt there
// is no result rather than a zero one.
func TestStatusCountsErrorsAndCarriesTheLastCheckpoint(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))
	h := newHealth()

	before, err := status(t.Context(), db, filepath.Join(dir, dbName), filepath.Join(dir, spoolDir), time.Now(), h)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if before.Errors != 0 || before.LastCheckpoint != nil {
		t.Errorf("a fresh service reports errors %d and checkpoint %+v, want 0 and none", before.Errors, before.LastCheckpoint)
	}

	log := slog.New(h.counting(slog.NewTextHandler(io.Discard, nil)))
	log.Error("one")
	log.Warn("not counted")
	log.With("k", "v").WithGroup("g").Error("two, through a derived logger")
	log.Info("not counted either")

	at := time.Date(2026, 9, 2, 6, 30, 0, 0, time.UTC)
	h.recordCheckpoint(at, nil)

	reply, err := status(t.Context(), db, filepath.Join(dir, dbName), filepath.Join(dir, spoolDir), time.Now(), h)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if reply.Errors != 2 {
		t.Errorf("errors = %d, want 2 - ERROR and above, whatever logger they came through", reply.Errors)
	}
	if reply.LastCheckpoint == nil {
		t.Fatal("the checkpoint result is missing after an attempt")
	}
	if reply.LastCheckpoint.AtMS != at.UnixMilli() || reply.LastCheckpoint.Error != "" {
		t.Errorf("last checkpoint = %+v, want at %d with no error", *reply.LastCheckpoint, at.UnixMilli())
	}

	sep := string(os.PathSeparator)
	h.recordCheckpoint(at.Add(time.Minute), errors.New("store: checkpoint the WAL beside C:"+sep+"Users"+sep+"someone"+sep+"engramux.db: busy"))
	failed, err := status(t.Context(), db, filepath.Join(dir, dbName), filepath.Join(dir, spoolDir), time.Now(), h)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if failed.LastCheckpoint == nil || !strings.Contains(failed.LastCheckpoint.Error, "busy") {
		t.Errorf("a failed checkpoint's error is not on the reply: %+v", failed.LastCheckpoint)
	}
	if strings.Contains(failed.LastCheckpoint.Error, "someone") {
		t.Errorf("the checkpoint error carries the user name out of the path: %q", failed.LastCheckpoint.Error)
	}
}
