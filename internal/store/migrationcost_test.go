package store

import (
	"os"
	"testing"
	"time"
)

// migrationCostDB names a database file to migrate and report on, and is empty
// on every run but the one somebody asks for.
//
// This is the throwaway harness the 1.0 spec §7.1's `00002` row describes, kept
// in the tree this time rather than deleted with its copies: what it does is
// open a database, run [Migrate], and print sizes and a duration. It gates
// nothing and asserts nothing about the figures, because there is nothing to
// assert - a migration's cost over one real installation on one disk is an
// observation, and §7.1 is where an observation lives.
//
// It skips without the variable, so an ordinary run never sees it.
//
// **Point it at a copy, never at the live database.** The service holds that
// file exclusively (I-07) and this would fail to open it - but on a stopped
// service it would succeed, and then the figure would have been bought by
// migrating the thing being measured.
const migrationCostDB = "ENGRAMUX_MIGRATION_COST_DB"

func TestMigrationCostOverARealDatabase(t *testing.T) {
	path := os.Getenv(migrationCostDB)
	if path == "" {
		t.Skipf("set %s to a *copy* of a real database to measure a migration over it", migrationCostDB)
	}

	before := fileSizes(t, path)
	t.Logf("before: db %d B, wal %d B", before.db, before.wal)

	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)

	p, err := provider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	from, err := p.GetDBVersion(t.Context())
	if err != nil {
		t.Fatalf("GetDBVersion: %v", err)
	}

	var events int64
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM events`).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}

	start := time.Now()
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	took := time.Since(start)

	to, err := p.GetDBVersion(t.Context())
	if err != nil {
		t.Fatalf("GetDBVersion after: %v", err)
	}
	peak := fileSizes(t, path)

	// The three derived columns against the payload they were cut from, which
	// is the ratio §7.1 records for `leaves` and the one that says whether a
	// column doubled the file or barely moved it.
	var payload, cmd, paths, output int64
	if err := db.QueryRowContext(t.Context(), `
		SELECT sum(length(payload)), sum(length(derived_cmd)),
		       sum(length(derived_paths)), sum(length(derived_output))
		FROM events`).Scan(&payload, &cmd, &paths, &output); err != nil {
		t.Fatalf("measure the columns: %v", err)
	}
	var withCmd, withPaths, withOutput int64
	if err := db.QueryRowContext(t.Context(), `
		SELECT sum(derived_cmd <> ''), sum(derived_paths <> ''), sum(derived_output <> '')
		FROM events`).Scan(&withCmd, &withPaths, &withOutput); err != nil {
		t.Fatalf("count the non-empty columns: %v", err)
	}

	// Integrity, on the same four checks §7.1's `00002` row reports, because a
	// fast migration that broke the index is not a cheaper migration.
	for _, check := range []struct{ name, stmt string }{
		{"integrity_check", `PRAGMA integrity_check`},
		{"foreign_key_check", `PRAGMA foreign_key_check`},
		{"fts integrity-check", `INSERT INTO events_fts(events_fts) VALUES('integrity-check')`},
	} {
		if _, err := db.ExecContext(t.Context(), check.stmt); err != nil {
			t.Errorf("%s after the migration: %v", check.name, err)
		}
	}

	t.Logf("migrated %d -> %d over %d events in %s", from, to, events, took)
	t.Logf("file: %d B -> %d B (%+d), wal %d B -> %d B", before.db, peak.db, peak.db-before.db, before.wal, peak.wal)
	t.Logf("columns: payload %d B; derived_cmd %d B on %d rows, derived_paths %d B on %d rows, derived_output %d B on %d rows",
		payload, cmd, withCmd, paths, withPaths, output, withOutput)
	t.Logf("derived total %d B, which is %.1f%% of the payload", cmd+paths+output,
		100*float64(cmd+paths+output)/float64(payload))
}

type dbSizes struct{ db, wal int64 }

func fileSizes(t *testing.T, path string) dbSizes {
	t.Helper()
	return dbSizes{db: fileSize(t, path), wal: fileSize(t, path+"-wal")}
}
