package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAServiceWithNoHostHomesIndexesNothing is the test that makes a forgotten
// redirection visible, and it exists because forgetting one has a consequence no
// other assertion here would catch: the memory directories belong to the two
// hosts and hold the user's own notes, so a child spawned with this process's
// real environment reads them into a test database.
//
// [childEnv] is the seam and every exec in this package goes through it. What
// this asserts is the end of that: a service started that way has an empty
// memory table, whatever is on the machine that ran the suite.
func TestAServiceWithNoHostHomesIndexesNothing(t *testing.T) {
	local := t.TempDir()
	s := start(t, local)
	s.stop(t)

	var n int64
	if err := s.openDB(t).QueryRowContext(t.Context(),
		`SELECT count(*) FROM memory_items`).Scan(&n); err != nil {
		t.Fatalf("count memory_items: %v", err)
	}
	if n != 0 {
		t.Fatalf("memory_items = %d rows, want 0 - the service read a host home the test did not redirect", n)
	}
}

// TestAServiceIndexesTheMemoryItIsPointedAt is the other half, and without it the
// test above passes on a collector that was never started at all.
//
// The files are synthesised here on the same rule the parser's tests are under:
// what a test may hold is the *shape* of a memory file, and never a copy of one.
func TestAServiceIndexesTheMemoryItIsPointedAt(t *testing.T) {
	local := t.TempDir()
	codex := filepath.Join(local, "no-codex-home")
	if err := os.MkdirAll(filepath.Join(codex, "memories"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, body := range map[string]string{
		"MEMORY.md":       "# index\n",
		"raw_memories.md": "## a section\nthe service has to reach this\n",
	} {
		if err := os.WriteFile(filepath.Join(codex, "memories", name), []byte(body), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	s := start(t, local)
	s.stop(t)

	var n int64
	if err := s.openDB(t).QueryRowContext(t.Context(),
		`SELECT count(*) FROM memory_fts WHERE memory_fts MATCH ?`, `"reach"`).Scan(&n); err != nil {
		t.Fatalf("match the indexed memory: %v", err)
	}
	if n != 1 {
		t.Fatalf("the index matched %d documents, want 1 - the collector did not run, or ran and indexed nothing", n)
	}
}
