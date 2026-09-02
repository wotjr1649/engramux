package main

import (
	"context"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestSearchSaysHowManyMatched is backlog 33 at the terminal: the reply's
// total is printed against the number of hits shown, so a person can tell a
// list that is everything from a list that is the first twenty of a thousand.
func TestSearchSaysHowManyMatched(t *testing.T) {
	serveSearch(t, func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
		return ipc.SearchReply{
			Hits:  []ipc.SearchHit{{ID: "0192f0c0-0000-7000-8000-000000000001", Host: "codex"}},
			Total: 137,
		}, nil
	})

	var code int
	out := captureStdout(t, func() { code = search([]string{"leaf"}) })

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.HasPrefix(out, "1 of 137 matches\n\n") {
		t.Errorf("stdout does not open with the count:\n%q", out)
	}
}
