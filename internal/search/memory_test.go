package search_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/memory"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// memoryDB opens a migrated database and fills memory_items from a synthesised
// Codex memory directory, which is how every test in this file gets a corpus:
// through the collector, so that what is searched is what the service would have
// written rather than rows a test invented.
func memoryDB(t *testing.T, sections ...string) *sql.DB {
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

	home := t.TempDir()
	writeFile(t, filepath.Join(home, "memories", "MEMORY.md"), "# index\n")
	writeFile(t, filepath.Join(home, "memories", "raw_memories.md"), strings.Join(sections, "\n"))

	c := &memory.Collector{CodexHome: home}
	rep, err := c.Collect(ctx, db, time.UnixMilli(1000))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if rep.Written == 0 {
		t.Fatal("the collector wrote nothing, so this test has no corpus")
	}
	return db
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", filepath.Base(path), err)
	}
}

// TestAMemorySearchFindsTheBlockAndNotItsNeighbour. The whole point of splitting
// at the format's delimiter is that a hit names one block; a file-grained reader
// would return the file and leave the reader to find the part that matched.
func TestAMemorySearchFindsTheBlockAndNotItsNeighbour(t *testing.T) {
	db := memoryDB(t,
		"## the pipe race\nthe second instance loses and exits\n",
		"## the covering index\nnine refusals in one hundred and forty-seven samples\n",
	)
	hits, total, err := search.SearchMemory(t.Context(), db, "refusals", nil, 10)
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if total != 1 || len(hits) != 1 {
		t.Fatalf("hits = %d, total = %d, want one of each", len(hits), total)
	}
	if !strings.Contains(hits[0].Excerpt, "nine refusals") {
		t.Fatalf("excerpt = %q, want the matching block's text", hits[0].Excerpt)
	}
	if strings.Contains(hits[0].Excerpt, "the second instance") {
		t.Fatalf("the excerpt reaches into the neighbouring block: %q", hits[0].Excerpt)
	}
	if hits[0].Host != memory.HostCodex || hits[0].ID == "" {
		t.Fatalf("hit = %+v, want a host and an id get_memory could be called with", hits[0])
	}
}

// TestAMemoryHitCarriesNoUserPath is the I-10 clause for this surface, and it is
// asserted here rather than left to the Phase 5 sweep because the sweep would
// catch it after the field had shipped. Both the path and the title go through
// the mask: a Claude Code note's title is its own name, and a Codex section's is
// its first line, either of which can be a path.
func TestAMemoryHitCarriesNoUserPath(t *testing.T) {
	db := memoryDB(t, "## a section\nthe error came from C:\\Users\\someone\\project\\main.go\n")
	hits, _, err := search.SearchMemory(t.Context(), db, "error", nil, 10)
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	for _, field := range []string{hits[0].Excerpt, hits[0].SourcePath, hits[0].Title} {
		if strings.Contains(field, "someone") {
			t.Fatalf("a reply field carries a user name: %q", field)
		}
	}
	if !strings.Contains(hits[0].Excerpt, "the error came from") {
		t.Fatalf("masking took the whole excerpt: %q", hits[0].Excerpt)
	}
}

// TestAScopedMemorySearchAsksAboutBothFormsOfAProject. Claude Code files memory
// under a directory key and Codex writes an absolute cwd, so a scope that asked
// about one of them would make the other host's memory unreachable through MCP.
func TestAScopedMemorySearchAsksAboutBothFormsOfAProject(t *testing.T) {
	db := memoryDB(t,
		"## mine\ncwd: D:\\work\\engramux\nthe answer is here\n",
		"## theirs\ncwd: D:\\work\\other\nthe answer is elsewhere\n",
	)
	keys := memory.ProjectKeys(`D:\work\engramux`)
	hits, total, err := search.SearchMemory(t.Context(), db, "answer", keys, 10)
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if total != 1 || len(hits) != 1 {
		t.Fatalf("hits = %d, total = %d, want the other project's block filtered out", len(hits), total)
	}
	if !strings.Contains(hits[0].Excerpt, "is here") {
		t.Fatalf("excerpt = %q, want the scoped project's block", hits[0].Excerpt)
	}

	all, allTotal, err := search.SearchMemory(t.Context(), db, "answer", nil, 10)
	if err != nil {
		t.Fatalf("unscoped SearchMemory: %v", err)
	}
	if allTotal != 2 || len(all) != 2 {
		t.Fatalf("unscoped hits = %d, total = %d, want both", len(all), allTotal)
	}
}

// TestAMemorySearchRefusesTheSameQueriesEventSearchDoes. The two surfaces share
// the query builder, so a token that is refused on one has to be refused on the
// other - otherwise the bound depends on which index a caller happened to hit.
func TestAMemorySearchRefusesTheSameQueriesEventSearchDoes(t *testing.T) {
	db := memoryDB(t, "## a\nsomething\n")
	if _, _, err := search.SearchMemory(t.Context(), db, "   ", nil, 10); err == nil {
		t.Fatal("an empty query was accepted")
	}
	if _, _, err := search.SearchMemory(t.Context(), db, strings.Repeat("x", 1<<12), nil, 10); err == nil {
		t.Fatal("an over-long token was accepted")
	}
}

// TestAMemorySearchReportsTheTotalBeyondItsLimit, on backlog 33's rule for the
// event surface: a caller that asked for ten and got ten has to be able to tell
// "ten" from "ten of two hundred".
func TestAMemorySearchReportsTheTotalBeyondItsLimit(t *testing.T) {
	var sections []string
	for i := range 5 {
		sections = append(sections, "## s"+string(rune('a'+i))+"\na recurring word\n")
	}
	db := memoryDB(t, sections...)
	hits, total, err := search.SearchMemory(t.Context(), db, "recurring", nil, 2)
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want the limit", len(hits))
	}
	if total != 5 {
		t.Fatalf("total = %d, want every match the limit cut", total)
	}
}
