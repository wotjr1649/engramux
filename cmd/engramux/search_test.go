package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/pipe"
)

// serveSearch stands up the production pipe server on this test's pipe name
// with the search handler given, and returns a pointer to the request it was
// sent. The reply the CLI checks is therefore the reply the service actually
// produces, stamped fields and all, rather than a hand-rolled one.
func serveSearch(t *testing.T, h pipe.SearchFunc) *ipc.SearchRequest {
	t.Helper()
	l := listenRelayPipe(t)
	seen := &ipc.SearchRequest{}

	done := make(chan error, 1)
	go func() {
		done <- pipe.Serve(t.Context(), l, pipe.Handler{
			Ingest: func(context.Context, ipc.Envelope) (ipc.AckStatus, error) {
				return ipc.Rejected, nil
			},
			Search: func(ctx context.Context, req ipc.SearchRequest) (ipc.SearchReply, error) {
				*seen = req
				return h(ctx, req)
			},
		})
	}()
	t.Cleanup(func() {
		_ = l.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("pipe.Serve did not return within 10s of Close")
		}
	})
	return seen
}

// captureStdout runs f with os.Stdout redirected to a file and returns what it
// wrote. A file and not an os.Pipe: a pipe with nothing reading it blocks the
// writer once its buffer fills, which would turn a formatting mistake into a
// hung test.
//
// os.Stdout is process-wide, so nothing here may be parallel. Nothing here is:
// every test in this package moves the pipe name with t.Setenv.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout")
	file, err := os.Create(path) //nolint:gosec // G304: a path this test just built under t.TempDir
	if err != nil {
		t.Fatalf("create the capture file: %v", err)
	}

	previous := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = previous }()
	f()

	if err := file.Close(); err != nil {
		t.Fatalf("close the capture file: %v", err)
	}
	out, err := os.ReadFile(path) //nolint:gosec // G304: a path this test just made under t.TempDir
	if err != nil {
		t.Fatalf("read the capture file: %v", err)
	}
	return string(out)
}

// TestSearchWithNoServiceFails is I-08 at the command: there is no read-only
// fallback, so a service that is down is exit 1 and nothing on stdout - not an
// empty result, which is what a fallback that found no database would produce.
func TestSearchWithNoServiceFails(t *testing.T) {
	claimAFreePipeName(t)

	var code int
	out := captureStdout(t, func() { code = search([]string{"anything"}) })

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
}

// TestSearchARefusedRequestIsNotAnEmptyResult. A build that serves no Search
// answers a rejected ACK, which decodes into a SearchReply with no hits.
// ipc.SearchReply.Verify is the only thing between that and printing "no
// results" for a request the service would not serve.
func TestSearchARefusedRequestIsNotAnEmptyResult(t *testing.T) {
	l := listenRelayPipe(t)
	done := make(chan error, 1)
	go func() {
		done <- pipe.Serve(t.Context(), l, pipe.Handler{
			Ingest: func(context.Context, ipc.Envelope) (ipc.AckStatus, error) {
				return ipc.Rejected, nil
			},
		})
	}()
	t.Cleanup(func() {
		_ = l.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("pipe.Serve did not return within 10s of Close")
		}
	})

	var code int
	out := captureStdout(t, func() { code = search([]string{"anything"}) })

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
}

// TestSearchWithNoHitsSaysSo is the other side of the same line: a reply that
// verifies and holds nothing is a real answer and exit 0.
func TestSearchWithNoHitsSaysSo(t *testing.T) {
	seen := serveSearch(t, func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
		return ipc.SearchReply{}, nil
	})

	var code int
	out := captureStdout(t, func() { code = search([]string{"run-time", "budget"}) })

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if out != "no results\n" {
		t.Errorf("stdout = %q, want %q", out, "no results\n")
	}
	// The words a person typed are joined by single spaces and sent as one
	// query, uninterpreted.
	if seen.Query != "run-time budget" {
		t.Errorf("the service was asked for %q, want %q", seen.Query, "run-time budget")
	}
	// No limit is sent, so the service's default applies.
	if seen.Limit != 0 {
		t.Errorf("the request carried limit %d, want 0", seen.Limit)
	}
}

// TestSearchPrintsOneBlockPerHit pins the whole of the output by value.
//
// The excerpt carries a newline - internal/store joins a payload's leaves with
// one - and the assertion is that it does not reach the terminal as a line
// break: quoting is what keeps one hit to three lines, and it is the same
// quoting that keeps a terminal escape in a payload from being one.
func TestSearchPrintsOneBlockPerHit(t *testing.T) {
	const ms = 1700000000123
	hits := []ipc.SearchHit{
		{
			ID:           "0192f0c0-0000-7000-8000-000000000001",
			Host:         "codex",
			EventName:    "PostToolUse",
			ReceivedAtMS: ms,
			Excerpt:      "first leaf\nsecond leaf",
		},
		{
			// events.id is TEXT PRIMARY KEY with no shape
			// constraint and the routing boundary only requires it
			// to be non-empty, so a newline in one is storable.
			// Unquoted it would print as a fourth line that reads
			// like the start of another hit.
			ID:           "0192f0c0-0000-7000-8000-000000000002\nnot a second hit",
			Host:         "claude-code",
			EventName:    "",
			ReceivedAtMS: ms,
			Excerpt:      "",
		},
	}
	serveSearch(t, func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
		return ipc.SearchReply{Hits: hits, Total: 2}, nil
	})

	var code int
	out := captureStdout(t, func() { code = search([]string{"leaf"}) })

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	when := stamp(ms)
	want := "2 of 2 matches\n\n" +
		when + "  codex        " + `"PostToolUse"` + "\n" +
		`"0192f0c0-0000-7000-8000-000000000001"` + "\n" +
		`"first leaf\nsecond leaf"` + "\n\n" +
		when + "  claude-code  " + `""` + "\n" +
		`"0192f0c0-0000-7000-8000-000000000002\nnot a second hit"` + "\n" +
		`""` + "\n\n"
	if out != want {
		t.Errorf("stdout =\n%q\nwant\n%q", out, want)
	}
}

// TestSearchWithNoWordsIsRefused. An empty query is a usage error and not a
// round trip: the service would refuse it with ErrEmptyQuery, and there is no
// reason to ask.
func TestSearchWithNoWordsIsRefused(t *testing.T) {
	claimAFreePipeName(t)

	var code int
	out := captureStdout(t, func() { code = search(nil) })

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
}

// TestSearchScopeReadsTheFlagOnlyInFirstPosition holds the one thing this
// command interprets, and the boundary around it.
//
// The words are otherwise untouched: internal/search quotes every token before
// it reaches MATCH and 1.0 offers no query language, so a dash inside a query is
// a dash. That is why the flag is read in first position only, and why the same
// two words later in the list stay part of the query.
//
// The resolved path is asserted as absolute rather than as an exact string: the
// working directory a test runs in is not this test's to know, and what the
// service requires is that the path is absolute at all.
func TestSearchScopeReadsTheFlagOnlyInFirstPosition(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "repo")

	t.Run("absent", func(t *testing.T) {
		project, words, err := searchScope([]string{"run-time", "budget"})
		if err != nil {
			t.Fatalf("searchScope: %v", err)
		}
		if project != "" {
			t.Errorf("project = %q, want empty - the default is every project", project)
		}
		if !slices.Equal(words, []string{"run-time", "budget"}) {
			t.Errorf("words = %q, want the two that were typed", words)
		}
	})

	t.Run("first, absolute", func(t *testing.T) {
		project, words, err := searchScope([]string{"--project", abs, "run-time"})
		if err != nil {
			t.Fatalf("searchScope: %v", err)
		}
		if project != abs {
			t.Errorf("project = %q, want %q", project, abs)
		}
		if !slices.Equal(words, []string{"run-time"}) {
			t.Errorf("words = %q, want the query with the flag taken off", words)
		}
	})

	t.Run("first, relative", func(t *testing.T) {
		project, words, err := searchScope([]string{"--project", filepath.Join("nested", "dir"), "run-time"})
		if err != nil {
			t.Fatalf("searchScope: %v", err)
		}
		if !filepath.IsAbs(project) {
			t.Errorf("project = %q, want an absolute path - the service refuses a relative one", project)
		}
		if !strings.HasSuffix(project, filepath.Join("nested", "dir")) {
			t.Errorf("project = %q, want it to end in the path that was given", project)
		}
		if !slices.Equal(words, []string{"run-time"}) {
			t.Errorf("words = %q, want the query with the flag taken off", words)
		}
	})

	t.Run("not first, so it is query text", func(t *testing.T) {
		in := []string{"run-time", "--project", abs}
		project, words, err := searchScope(in)
		if err != nil {
			t.Fatalf("searchScope: %v", err)
		}
		if project != "" {
			t.Errorf("project = %q, want empty - the flag is read in first position only", project)
		}
		if !slices.Equal(words, in) {
			t.Errorf("words = %q, want every argument", words)
		}
	})

	t.Run("with no path", func(t *testing.T) {
		if _, _, err := searchScope([]string{"--project"}); err == nil {
			t.Error("a flag with no path was accepted")
		}
	})
}
