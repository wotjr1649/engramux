package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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
	if err := os.MkdirAll(filepath.Join(codex, "memories"), 0o750); err != nil {
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
	waitUntilIndexed(t, s)
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

// / waitUntilIndexed blocks until the service has written its first memory pass.
//
// [start] waits for a Status reply, which the collector does not gate: it runs
// on its own goroutine and the first pass is concurrent with the service
// becoming answerable. A test that asserted straight after start was asserting
// against a race it usually won - it lost one once the pass got slower, which is
// the only warning that kind of test gives.
//
// The log line is the signal rather than a poll of the database, because the
// service holds that file exclusively while it runs (I-07).
func waitUntilIndexed(t *testing.T, s *running) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if bytes.Contains(s.logFile(t), []byte("indexed native memory")) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the service did not index native memory within 30s")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// memoryIDLine finds the id the CLI printed for a memory hit: a quoted 32-hex
// run, which is what memory.ItemID mints.
var memoryIDLine = regexp.MustCompile(`"([0-9a-f]{32})"`)

// TestTheCLIShowsNativeMemoryAndReadsOneBack is the whole of M-2 decision 9 end
// to end, through the two binaries a person actually runs: one search reaches
// both lists, and a memory hit's id round-trips to `engramux memory`.
//
// It is here rather than in cmd/engramux because only this package has a running
// service to answer, and the id has to survive the pipe for the round trip to
// mean anything.
func TestTheCLIShowsNativeMemoryAndReadsOneBack(t *testing.T) {
	local := t.TempDir()
	codex := filepath.Join(local, "no-codex-home")
	if err := os.MkdirAll(filepath.Join(codex, "memories"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, body := range map[string]string{
		"MEMORY.md":       "# index\n",
		"raw_memories.md": "## a section\nthe covering index came out of the soak\n",
	} {
		if err := os.WriteFile(filepath.Join(codex, "memories", name), []byte(body), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	waitUntilIndexed(t, start(t, local))

	found := cliIn(t, local, "search", "covering")
	if found.exit != 0 {
		t.Fatalf("search exited %d: %s", found.exit, found.stderr)
	}
	if !strings.Contains(found.stdout, "native memory matches") {
		t.Fatalf("the search printed no native memory list:\n%s", found.stdout)
	}
	m := memoryIDLine.FindStringSubmatch(found.stdout)
	if m == nil {
		t.Fatalf("the search printed no memory id:\n%s", found.stdout)
	}

	read := cliIn(t, local, "memory", m[1])
	if read.exit != 0 {
		t.Fatalf("memory exited %d: %s\n%s", read.exit, read.stderr, read.stdout)
	}
	if !strings.Contains(read.stdout, "the covering index came out of the soak") {
		t.Fatalf("`engramux memory` did not print the body:\n%s", read.stdout)
	}
	// The id the search printed has to be the one the read answers to, or the
	// round trip this test is about is passing on a coincidence.
	if !strings.Contains(read.stdout, m[1]) {
		t.Fatalf("the item read back carries a different id than the hit:\n%s", read.stdout)
	}
}
