package main

import (
	"context"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestSearchMarksAnEventNameThatWasCut is backlog 17 at the terminal. Two
// cuts can shorten a name before a person sees it - the service's bound on the
// wire, which the hit says with a flag, and this command's own 64-rune display
// width - and both have to leave a mark, or a shortened name reads as the name.
// The mark is outside the quotes, so it cannot be mistaken for the name's own
// last character.
func TestSearchMarksAnEventNameThatWasCut(t *testing.T) {
	long := strings.Repeat("n", 70)
	serveSearch(t, func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
		return ipc.SearchReply{
			Hits: []ipc.SearchHit{
				{ID: "1", Host: "codex", EventName: long},
				{ID: "2", Host: "codex", EventName: "Short", EventNameTruncated: true},
				{ID: "3", Host: "codex", EventName: "Whole"},
			},
			Total: 3,
		}, nil
	})

	out := captureStdout(t, func() { search([]string{"n"}) })

	for _, want := range []string{
		`"` + strings.Repeat("n", 64) + `"…` + "\n",
		`"Short"…` + "\n",
		`"Whole"` + "\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `"Whole"…`) {
		t.Errorf("a whole name was marked as cut:\n%s", out)
	}
}
