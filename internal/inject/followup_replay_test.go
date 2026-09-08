package inject_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
)

func TestMeasureFollowupHoldout(t *testing.T) {
	arm := os.Getenv("ENGRAMUX_FOLLOWUP_ARM")
	if arm == "" {
		t.Skip("followup replay is opt-in")
	}
	if arm != "baseline" && arm != "literal" {
		t.Fatal("unknown frozen arm")
	}
	const dir = "../../.capture/selection-quality/followup-2026-09-08/holdout"
	data, err := os.ReadFile(dir + "/agent-labels.json")
	if err != nil {
		t.Fatal("read labels")
	}
	var labels struct {
		Source string
		Hash   string `json:"prompts_sha256"`
		Labels []struct{ ID, Label string }
	}
	if err := json.Unmarshal(data, &labels); err != nil || labels.Source != "agent" {
		t.Fatal("invalid agent labels")
	}
	prompts, err := os.ReadFile(dir + "/prompts.tsv")
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(prompts)) != labels.Hash {
		t.Fatal("prompt hash mismatch")
	}
	texts := map[string]string{}
	for _, line := range strings.Split(string(prompts), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) != 4 {
			t.Fatal("invalid prompt row")
		}
		texts[fields[0]] = fields[3]
	}
	const source = "../../.capture/session-resume-2026-09-08/engramux.db"
	uri, err := temporalSourceURI(source)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	type trigger struct {
		prompt m7Prompt
		stamp  int64
	}
	var triggers []trigger
	seen := map[string]bool{}
	for _, label := range labels.Labels {
		if seen[label.ID] || (label.Label != "yes" && label.Label != "no") || texts[label.ID] == "" {
			t.Fatal("invalid label identity")
		}
		seen[label.ID] = true
		var tr trigger
		var payload []byte
		if err := db.QueryRowContext(t.Context(), "SELECT received_at,project_id,payload FROM events WHERE id=? AND event_name='UserPromptSubmit'", label.ID).Scan(&tr.stamp, &tr.prompt.project, &payload); err != nil {
			t.Fatal("read trigger")
		}
		var fields struct {
			CWD string `json:"cwd"`
		}
		if err := json.Unmarshal(payload, &fields); err != nil {
			t.Fatal("decode trigger")
		}
		tr.prompt.id, tr.prompt.prompt, tr.prompt.cwd, tr.prompt.wanted = label.ID, texts[label.ID], fields.CWD, label.Label
		if m7Drifted(tr.prompt) {
			t.Fatal("project drift")
		}
		triggers = append(triggers, tr)
	}
	if len(triggers) != 33 || len(texts) != 33 {
		t.Fatal("incomplete holdout")
	}
	sort.SliceStable(triggers, func(i, j int) bool { return triggers[i].stamp < triggers[j].stamp })
	prefix, err := newTemporalReplay(t, source)
	if err != nil {
		t.Fatal(err)
	}
	var emitted, wanted, wantedEmitted, bytes, unwanted, blocks, deadlines int
	for _, tr := range triggers {
		if err := prefix.advance(t, tr.stamp); err != nil {
			t.Fatal(err)
		}
		res := m7Build(t, prefix.db, tr.prompt)
		if tr.prompt.wanted == "yes" {
			wanted++
		}
		if res.Reason == inject.ReasonDeadline {
			deadlines++
		}
		if res.Text == "" {
			continue
		}
		emitted++
		if tr.prompt.wanted == "yes" {
			wantedEmitted++
		}
		for _, b := range m7SplitBlocks(t, tr.prompt.id, res) {
			var stamp int64
			if b.kind != "event" {
				t.Fatal("current native memory entered replay")
			}
			if err := prefix.db.QueryRowContext(t.Context(), "SELECT received_at FROM events WHERE id=?", b.id).Scan(&stamp); err != nil || stamp >= tr.stamp {
				t.Fatal("nonhistorical block")
			}
			blocks++
			bytes += b.bytes
			if tr.prompt.wanted == "no" {
				unwanted += b.bytes
			}
		}
	}
	t.Logf("%s: prompts=%d emitted=%d wanted=%d wanted_emitted=%d blocks=%d bytes=%d unwanted=%d deadlines=%d", arm, len(triggers), emitted, wanted, wantedEmitted, blocks, bytes, unwanted, deadlines)
	t.Log("Agent holdout diagnostic only; no block relevance or task-success pass")
}
