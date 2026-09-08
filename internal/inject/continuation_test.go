package inject_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/secret"
)

var continuationUUID = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func TestContinuationCueDoesNotMatchAgentNouns(t *testing.T) {
	if continuationSignal("서브에이전트와 에이전트로 검토한다") {
		t.Fatal("agent noun mistaken for prior-reference cue")
	}
	if !continuationSignal("이전에 정한 기준을 확인한다") {
		t.Fatal("prior-reference inflection lost")
	}
}

func continuationSignal(prompt string) bool {
	for _, word := range strings.Fields(strings.ToLower(prompt)) {
		word = strings.Trim(word, ".,!?;:\"'()[]{}")
		for _, cue := range []string{"이전", "앞서", "이어서", "추천대로", "그럼"} {
			if strings.HasPrefix(word, cue) {
				return true
			}
		}
		switch word {
		case "previous", "earlier", "recall", "resume", "continue":
			return true
		}
	}
	return false
}

func continuationBuild(t *testing.T, db *sql.DB, p m7Prompt, cutoff int64) inject.Result {
	t.Helper()
	start := time.Now()
	deadline := start.Add(inject.Budget)
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()
	late := func() bool { return ctx.Err() != nil || time.Now().After(deadline) }
	abstain := func(reason string) inject.Result { return inject.Result{Reason: reason, Elapsed: time.Since(start)} }
	if !continuationSignal(p.prompt) {
		return abstain("reference_no_signal")
	}
	if continuationUUID.MatchString(p.prompt) {
		return abstain("reference_named_scope_unresolved")
	}
	var host string
	if err := db.QueryRowContext(ctx, "SELECT host FROM sessions WHERE id=?", p.session).Scan(&host); err != nil {
		if late() {
			return abstain(inject.ReasonDeadline)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return abstain(inject.ReasonNoHits)
		}
		t.Fatal("reference session lookup failed")
	}
	if host != "claude-code" && host != "codex" {
		return abstain(inject.ReasonNoHits)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE project_id=? AND session_id=? AND host=? AND event_name='Stop' AND received_at>0 AND received_at<?", p.project, p.session, host, cutoff).Scan(&count); err != nil {
		if late() {
			return abstain(inject.ReasonDeadline)
		}
		t.Fatal("reference count failed")
	}
	if count == 0 {
		return abstain(inject.ReasonNoHits)
	}
	if count > 200 {
		return abstain(inject.ReasonTooBroad)
	}
	var h search.Hit
	var payload []byte
	if err := db.QueryRowContext(ctx, `SELECT id,host,event_name,received_at,CASE WHEN length(CAST(payload AS BLOB))<=1048576 THEN payload ELSE NULL END FROM events WHERE project_id=? AND session_id=? AND host=? AND event_name='Stop' AND received_at>0 AND received_at<? ORDER BY received_at DESC,rowid DESC LIMIT 1`, p.project, p.session, host, cutoff).Scan(&h.ID, &h.Host, &h.EventName, &h.ReceivedAtMS, &payload); err != nil {
		if late() {
			return abstain(inject.ReasonDeadline)
		}
		t.Fatal("reference body lookup failed")
	}
	if len(payload) == 0 || len(h.ID) > 64 || h.ID == p.id || secret.MaskString(h.ID) != h.ID {
		return abstain(inject.ReasonNoHits)
	}
	var fields struct {
		Body string `json:"last_assistant_message"`
	}
	if json.Unmarshal(secret.Mask(payload), &fields) != nil {
		return abstain(inject.ReasonNoHits)
	}
	runes := []rune(fields.Body)
	if len(runes) > 480 {
		h.Excerpt = string(runes[:240]) + "\n[... middle omitted ...]\n" + string(runes[len(runes)-240:])
	} else {
		h.Excerpt = fields.Body
	}
	if h.Excerpt == "" {
		return abstain(inject.ReasonNoHits)
	}
	probe, err := inject.Fence("")
	if err != nil {
		t.Fatal(err)
	}
	body, events, mem := inject.Assemble([]search.Hit{h}, nil, inject.MaxBytes-len(probe))
	if body == "" {
		return abstain(inject.ReasonNoRoom)
	}
	text, err := inject.Fence(body)
	if err != nil {
		t.Fatal(err)
	}
	if late() {
		return abstain(inject.ReasonDeadline)
	}
	if len(text) > inject.MaxBytes {
		t.Fatal("reference output exceeds budget")
	}
	return inject.Result{Text: text, Events: events, Memory: mem, Elapsed: time.Since(start)}
}

func TestContinuationRetrievesAnEarlierAnswer(t *testing.T) {
	db := corpus(t, seeded{id: "answer", payload: []byte(`{"cwd":"/w","session_id":"s","transcript_path":"/synthetic/.claude/projects/s.jsonl","hook_event_name":"Stop","last_assistant_message":"accepted retry count is seven"}`)})
	if _, err := db.ExecContext(t.Context(), "UPDATE events SET received_at=10"); err != nil {
		t.Fatal(err)
	}
	p := m7Prompt{id: "current", cwd: "/w", prompt: "continue"}
	if err := db.QueryRowContext(t.Context(), "SELECT project_id,session_id FROM events WHERE id='answer'").Scan(&p.project, &p.session); err != nil {
		t.Fatal(err)
	}
	got := continuationBuild(t, db, p, 20)
	if len(got.Events) != 1 || got.Events[0] != "answer" {
		t.Fatal("prior answer not recovered")
	}
	if got := continuationBuild(t, db, p, 10); got.Text != "" {
		t.Fatal("equal-time event admitted")
	}
	wrong := p
	wrong.project = "absent-project"
	if got := continuationBuild(t, db, wrong, 20); got.Text != "" {
		t.Fatal("project boundary crossed")
	}
	wrong = p
	wrong.session = "absent-session"
	if got := continuationBuild(t, db, wrong, 20); got.Text != "" {
		t.Fatal("session boundary crossed")
	}
	wrong = p
	wrong.prompt = "continue session 10000000-0000-4000-8000-000000000001"
	if got := continuationBuild(t, db, wrong, 20); got.Text != "" {
		t.Fatal("named scope silently replaced")
	}
	// Counterexample, not an admission-quality pass: the cue is a language keyword.
	wrong = p
	wrong.prompt = "Explain the continue keyword"
	if got := continuationBuild(t, db, wrong, 20); len(got.Events) != 1 {
		t.Fatal("lexical false-positive counterexample changed")
	}
}
