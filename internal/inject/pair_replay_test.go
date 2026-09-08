package inject_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/search"
)

func TestPairedReplayReplacesQuestionWithItsAnswer(t *testing.T) {
	makeEvent := func(id, kind, field, text string) seeded {
		payload, err := json.Marshal(map[string]string{"cwd": "/w", "session_id": "s", "hook_event_name": kind, "prompt_id": "turn", field: text})
		if err != nil {
			t.Fatal(err)
		}
		return seeded{id: id, payload: payload}
	}
	db := corpus(t, makeEvent("question", "UserPromptSubmit", "prompt", "quartz"), makeEvent("answer", "Stop", "last_assistant_message", "accepted retry count is seven"))
	if _, err := db.ExecContext(t.Context(), "UPDATE events SET received_at=10"); err != nil {
		t.Fatal(err)
	}
	var project string
	if err := db.QueryRowContext(t.Context(), "SELECT project_id FROM events WHERE id='question'").Scan(&project); err != nil {
		t.Fatal(err)
	}
	p := m7Prompt{id: "current", cwd: "/w", project: project, prompt: "quartz"}
	got := pairedBuild(t, db, p, 20)
	if len(got.Events) != 1 || got.Events[0] != "answer" || !strings.Contains(got.Text, "accepted retry count is seven") || len(got.Text) > inject.MaxBytes {
		t.Fatal("paired reply was not emitted exactly within cap")
	}
	if fenced, carries := m7Fenced(got.Text); !fenced || carries {
		t.Fatal("paired reply lost its fence")
	}
}

// Experimental event-only replacement arm. All scans and body work share one
// budget with the original selection; no offline index cost is hidden.
func pairedBuild(t *testing.T, db *sql.DB, p m7Prompt, cutoff int64) inject.Result {
	t.Helper()
	start := time.Now()
	deadline := start.Add(inject.Budget)
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()
	abstain := func() inject.Result { return inject.Result{Reason: inject.ReasonDeadline, Elapsed: time.Since(start)} }
	base, err := inject.Build(ctx, db, inject.Request{Prompt: p.prompt, Project: p.cwd, ExcludeID: p.id})
	if err != nil {
		t.Fatal("paired baseline failed")
	}
	if base.Text == "" {
		return base
	}
	rows, err := db.QueryContext(ctx, `SELECT id,host,session_id,project_id,event_name,received_at,
	CASE WHEN length(CAST(payload AS BLOB))<=1048576 THEN payload ELSE NULL END
	FROM events WHERE host IN ('claude-code','codex') AND event_name IN ('UserPromptSubmit','Stop') LIMIT 100001`)
	if err != nil {
		if ctx.Err() != nil {
			return abstain()
		}
		t.Fatal("pair scan failed")
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var records []pairRecord
	for rows.Next() {
		var r pairRecord
		var payload []byte
		if err := rows.Scan(&r.id, &r.host, &r.session, &r.project, &r.kind, &r.stamp, &payload); err != nil {
			t.Fatal("pair scan row failed")
		}
		r.key = pairKey(r.host, payload)
		records = append(records, r)
		if len(records) > 100000 {
			t.Fatal("pair scan bound exceeded")
		}
	}
	if err := rows.Err(); err != nil {
		if ctx.Err() != nil {
			return abstain()
		}
		t.Fatal("pair scan incomplete")
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	pairs := resolvePairs(records, cutoff)
	blocks := m7SplitBlocks(t, p.id, base)
	var hits []search.Hit
	seen := map[string]bool{}
	changed := false
	for i, b := range blocks {
		if b.kind != "event" {
			t.Fatal("paired arm is event-only")
		}
		id := b.id
		answer, ok := pairs[id]
		if ok {
			id = answer
		}
		if seen[id] {
			changed = true
			continue
		}
		var h search.Hit
		var payload []byte
		if err := db.QueryRowContext(ctx, `SELECT id,host,event_name,received_at,
		CASE WHEN length(CAST(payload AS BLOB))<=1048576 THEN payload ELSE NULL END
		FROM events WHERE id=? AND project_id=? AND received_at>0 AND received_at<?`, id, p.project, cutoff).Scan(&h.ID, &h.Host, &h.EventName, &h.ReceivedAtMS, &payload); err != nil {
			if ctx.Err() != nil {
				return abstain()
			}
			t.Fatal("paired scoped body missing")
		}
		if ok {
			h.Excerpt = pairedBody(payload)
			if h.Excerpt == "" {
				changed = true
				continue
			}
			changed = true
		} else {
			_, h.Excerpt, _ = strings.Cut(b.text, "\n")
			if i < len(blocks)-1 {
				h.Excerpt = strings.TrimSuffix(h.Excerpt, "\n")
			}
		}
		seen[id] = true
		hits = append(hits, h)
	}
	if ctx.Err() != nil || time.Now().After(deadline) {
		return abstain()
	}
	if !changed {
		return base
	}
	probe, err := inject.Fence("")
	if err != nil {
		t.Fatal(err)
	}
	body, events, mem := inject.Assemble(hits, nil, inject.MaxBytes-len(probe))
	if body == "" {
		return inject.Result{Reason: inject.ReasonNoRoom}
	}
	text, err := inject.Fence(body)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil || time.Now().After(deadline) {
		return abstain()
	}
	if len(text) > inject.MaxBytes {
		t.Fatal("paired output exceeded cap")
	}
	return inject.Result{Text: text, Events: events, Memory: mem, Elapsed: time.Since(start)}
}
