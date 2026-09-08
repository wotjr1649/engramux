package inject_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/search"
)

func rareAnchorBuild(t *testing.T, db *sql.DB, p m7Prompt) inject.Result {
	t.Helper()
	start := time.Now()
	deadline := start.Add(inject.Budget)
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()
	late := func() bool { return ctx.Err() != nil || time.Now().After(deadline) }
	abstain := func() inject.Result { return inject.Result{Reason: inject.ReasonDeadline, Elapsed: time.Since(start)} }
	fields := strings.Fields(p.prompt)
	fields = fields[:min(len(fields), 32)]
	seen := map[string]bool{}
	best := ""
	var frequency int64
	for _, token := range fields {
		folded := strings.ToLower(token)
		if seen[folded] || len(inject.QueryFor(token)) == 0 {
			continue
		}
		seen[folded] = true
		_, count, err := search.Search(ctx, db, token, p.project, 1, search.MatchAll)
		if late() {
			return abstain()
		}
		if err != nil {
			if errors.Is(err, search.ErrEmptyQuery) || errors.Is(err, search.ErrNULQuery) || errors.Is(err, search.ErrTokenTooLong) || errors.Is(err, search.ErrTooManyTokens) {
				continue
			}
			t.Fatal("anchor frequency query failed")
		}
		if count > 0 && (best == "" || count < frequency) {
			best, frequency = token, count
		}
	}
	if best == "" {
		return inject.Result{Reason: inject.ReasonNoHits, Elapsed: time.Since(start)}
	}
	got, err := inject.Build(ctx, db, inject.Request{Prompt: best, Project: p.cwd, ExcludeID: p.id})
	if err != nil {
		t.Fatal("anchor selection failed")
	}
	if late() {
		return abstain()
	}
	got.Elapsed = time.Since(start)
	return got
}

func TestRareAnchorUsesAnAvailableTerm(t *testing.T) {
	seed := event("PostToolUse", "quartz accepted retry policy", "")
	db := corpus(t, seed)
	var project string
	if err := db.QueryRowContext(t.Context(), "SELECT project_id FROM events LIMIT 1").Scan(&project); err != nil {
		t.Fatal(err)
	}
	p := m7Prompt{id: "current", cwd: "/w", project: project, prompt: "unavailablelongword nonexistentvocabulary absentterminology quartz"}
	got := rareAnchorBuild(t, db, p)
	if len(got.Events) != 1 || got.Events[0] != seed.id {
		t.Fatal("available historical term was lost")
	}
}

func TestRareAnchorCanPreferMetadataOverAnswers(t *testing.T) {
	metadata := seeded{id: "metadata", payload: []byte(`{"cwd":"/w","session_id":"rare_nonce","hook_event_name":"SessionStart"}`)}
	db := corpus(t, metadata, event("PostToolUse", "quartz accepted repair A", ""), event("PostToolUse", "quartz accepted repair B", ""))
	var project string
	if err := db.QueryRowContext(t.Context(), "SELECT project_id FROM events WHERE id='metadata'").Scan(&project); err != nil {
		t.Fatal(err)
	}
	_, one, err := search.Search(t.Context(), db, "rare_nonce", project, 1, search.MatchAll)
	if err != nil {
		t.Fatal(err)
	}
	_, two, err := search.Search(t.Context(), db, "quartz", project, 1, search.MatchAll)
	if err != nil {
		t.Fatal(err)
	}
	if one != 1 || two != 2 {
		t.Fatal("frequency fixture does not distinguish total from limit")
	}
	got := rareAnchorBuild(t, db, m7Prompt{id: "current", cwd: "/w", project: project, prompt: "rare_nonce quartz"})
	if len(got.Events) != 1 || got.Events[0] != "metadata" {
		t.Fatal("rare metadata counterexample was not reproduced")
	}
	t.Log("candidate selected metadata instead of either answer; this is a quality counterexample")
}
