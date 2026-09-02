package memory

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/store"
)

// collected opens a migrated database under the test's own directory, on the
// rule every database in this suite is under: one per test, so nothing meets the
// running service's file (I-07).
func collected(t *testing.T) *sql.DB {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "engramux.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// codexHomeWith writes a Codex memory directory holding one index and one
// raw-memories file with the given sections, and returns the home.
func codexHomeWith(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	write(t, home, filepath.Join("memories", "MEMORY.md"), "# index\n", Source{})
	write(t, home, filepath.Join("memories", "raw_memories.md"), body, Source{})
	return home
}

// TestASecondPassReadsNothingWhenNothingMoved is the short-circuit, and it is
// the whole reason a ticker is affordable: on the machine this was written for
// both hosts' memory is switched off, so every pass after the first is this one.
func TestASecondPassReadsNothingWhenNothingMoved(t *testing.T) {
	db := collected(t)
	c := &Collector{CodexHome: codexHomeWith(t, "## a\nfirst\n\n## b\nsecond\n")}

	first, err := c.Collect(t.Context(), db, time.UnixMilli(1000))
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first.Files != 2 || first.Skipped != 0 || first.Items == 0 {
		t.Fatalf("first pass = %+v, want two files read and some items", first)
	}
	second, err := c.Collect(t.Context(), db, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if second.Skipped != 2 || second.Items != 0 || second.Written != 0 {
		t.Fatalf("second pass = %+v, want both files skipped and nothing written", second)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_items`); got != int64(first.Written) {
		t.Fatalf("rows = %d after a pass that wrote nothing, want %d", got, first.Written)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_items WHERE indexed_at = 2000`); got != 0 {
		t.Fatalf("%d rows were restamped by a pass that read nothing", got)
	}
}

// TestAChangedFileIsRewrittenAndItsOldBlocksGo. A block that was removed from a
// file has to leave the table with it - otherwise a search answers out of a
// document the host has already deleted, which is worse than not finding it.
func TestAChangedFileIsRewrittenAndItsOldBlocksGo(t *testing.T) {
	db := collected(t)
	home := codexHomeWith(t, "## a\nthe first section\n\n## b\nthe second section\n")
	c := &Collector{CodexHome: home}

	if _, err := c.Collect(t.Context(), db, time.UnixMilli(1000)); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_fts WHERE memory_fts MATCH ?`, `"second"`); got != 1 {
		t.Fatalf("the second section is not in the index: %d", got)
	}

	raw := filepath.Join(home, "memories", "raw_memories.md")
	if err := os.WriteFile(raw, []byte("## a\nthe first section, rewritten\n"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// The stamp is a modification time and a size, and a same-size rewrite in
	// the same second is exactly what the pair is for. Moving it explicitly
	// keeps the test about the sweep rather than about the clock.
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(raw, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	rep, err := c.Collect(t.Context(), db, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if rep.Skipped != 1 {
		t.Fatalf("skipped = %d, want the untouched index to be skipped and the rewritten file read", rep.Skipped)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_fts WHERE memory_fts MATCH ?`, `"second"`); got != 0 {
		t.Fatalf("the removed section is still in the index: %d rows", got)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_fts WHERE memory_fts MATCH ?`, `"rewritten"`); got != 1 {
		t.Fatalf("the rewritten section is not in the index: %d rows", got)
	}
}

// TestAVanishedFileTakesItsRowsWithIt. The other half of the same invariant: the
// table is what is on disk, and a file that is gone is not on disk.
func TestAVanishedFileTakesItsRowsWithIt(t *testing.T) {
	db := collected(t)
	home := codexHomeWith(t, "## a\nsomething\n")
	c := &Collector{CodexHome: home}

	if _, err := c.Collect(t.Context(), db, time.UnixMilli(1000)); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	before := count(t, db, `SELECT count(*) FROM memory_items`)
	if before == 0 {
		t.Fatal("nothing was indexed, so this test is about nothing")
	}
	if err := os.Remove(filepath.Join(home, "memories", "raw_memories.md")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	rep, err := c.Collect(t.Context(), db, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if rep.Removed == 0 {
		t.Fatalf("report = %+v, want the vanished file's rows counted", rep)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_items WHERE source_path LIKE '%raw_memories.md'`); got != 0 {
		t.Fatalf("%d rows of the vanished file survived", got)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_items`); got == 0 {
		t.Fatal("the index file's rows went too; only the vanished file's should have")
	}
}

// TestAWalkThatFindsNothingDoesNotWipeTheTable. Both homes reading as empty is
// what an uninstalled host looks like and also what a directory that could not
// be read for one tick looks like, and the two are not worth telling apart by
// deleting everything. Going stale is recoverable; going empty is not, because
// the next pass has nothing to compare against either.
func TestAWalkThatFindsNothingDoesNotWipeTheTable(t *testing.T) {
	db := collected(t)
	c := &Collector{CodexHome: codexHomeWith(t, "## a\nsomething\n")}
	if _, err := c.Collect(t.Context(), db, time.UnixMilli(1000)); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	before := count(t, db, `SELECT count(*) FROM memory_items`)

	c.CodexHome = filepath.Join(t.TempDir(), "gone")
	rep, err := c.Collect(t.Context(), db, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if rep.Files != 0 || rep.Removed != 0 {
		t.Fatalf("report = %+v, want a pass that found nothing to remove nothing", rep)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_items`); got != before {
		t.Fatalf("rows = %d after a pass that saw no files, want %d", got, before)
	}
}

// TestAMemoryItemIsTaggedLikeAnEvent. I-10 does not distinguish: what is stored
// is the original and the masking happens at egress, so the row has to carry the
// tag that says an egress has work to do. A memory note is full of paths and the
// user-identity class fired on 900 of 902 captures.
func TestAMemoryItemIsTaggedLikeAnEvent(t *testing.T) {
	db := collected(t)
	c := &Collector{CodexHome: codexHomeWith(t, "## a\nthe error came from C:\\Users\\someone\\project\\main.go\n")}
	if _, err := c.Collect(t.Context(), db, time.UnixMilli(1000)); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := count(t, db, `SELECT count(*) FROM memory_items WHERE privacy_class = ''`); got == 0 {
		t.Log("no untagged rows, which is fine")
	}
	if got := count(t, db, `SELECT count(*) FROM memory_items WHERE privacy_class != ''`); got == 0 {
		t.Fatal("a body carrying a user path was stored with no privacy class")
	}
	if got := count(t, db, `SELECT count(*) FROM memory_items WHERE redaction_version = 0`); got != 0 {
		t.Fatalf("%d rows carry no redaction version, so nothing can re-scan them against a later ruleset", got)
	}
}

// TestTheItemIdIsStableAcrossPasses. A search hit's id has to survive the tick
// that reads the same block again, because get_memory is a second call made with
// it. Deriving the id from the three columns 00004 makes unique is what
// guarantees that; minting one would break it every pass.
func TestTheItemIdIsStableAcrossPasses(t *testing.T) {
	db := collected(t)
	home := codexHomeWith(t, "## a\nsomething\n")
	c := &Collector{CodexHome: home}
	if _, err := c.Collect(t.Context(), db, time.UnixMilli(1000)); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	var first string
	if err := db.QueryRowContext(t.Context(),
		`SELECT id FROM memory_items WHERE entry_key = 'a'`).Scan(&first); err != nil {
		t.Fatalf("read the id: %v", err)
	}

	raw := filepath.Join(home, "memories", "raw_memories.md")
	if err := os.WriteFile(raw, []byte("## a\nsomething else entirely\n"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(raw, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if _, err := c.Collect(t.Context(), db, time.UnixMilli(2000)); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	var second string
	if err := db.QueryRowContext(t.Context(),
		`SELECT id FROM memory_items WHERE entry_key = 'a'`).Scan(&second); err != nil {
		t.Fatalf("read the id again: %v", err)
	}
	if first != second {
		t.Fatalf("id moved from %s to %s across a rewrite of the same block", first, second)
	}
	if ItemID(Item{Host: HostCodex, SourcePath: raw, EntryKey: "a"}) != first {
		t.Fatalf("the stored id is not the derivation of its own three columns")
	}
}

// TestCollectIsCancellable. The service calls this from a loop it shuts down,
// and a pass that ignored its context would hold the single connection past the
// point everything else has stopped.
func TestCollectIsCancellable(t *testing.T) {
	db := collected(t)
	c := &Collector{CodexHome: codexHomeWith(t, "## a\nsomething\n")}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Collect(ctx, db, time.UnixMilli(1000)); err == nil {
		t.Fatal("Collect returned no error against a cancelled context")
	}
}
