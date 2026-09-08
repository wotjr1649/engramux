package inject_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
)

func TestMeasureTransferDevelopment(t *testing.T) {
	if os.Getenv("ENGRAMUX_TRANSFER_REPLAY") != "1" {
		t.Skip("transfer development replay is opt-in")
	}
	const dir = "../../.capture/selection-quality/transfer-2026-09-08"
	const source = "../../.capture/session-resume-2026-09-08/engramux.db"
	data, err := os.ReadFile(dir + "/development-prompts.json")
	if err != nil {
		t.Fatal(err)
	}
	var prompts []struct{ ID, Text string }
	if err := json.Unmarshal(data, &prompts); err != nil {
		t.Fatal(err)
	}
	labelData, err := os.ReadFile(dir + "/development-agent-labels.json")
	if err != nil {
		t.Fatal(err)
	}
	var labels struct {
		Source string
		Hash   string `json:"prompts_sha256"`
		Labels []struct{ ID, Label string }
	}
	if err := json.Unmarshal(labelData, &labels); err != nil || labels.Source != "agent" || labels.Hash != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal("invalid label provenance or prompt hash")
	}
	byID := map[string]string{}
	for _, p := range prompts {
		if byID[p.ID] != "" {
			t.Fatal("duplicate prompt")
		}
		byID[p.ID] = p.Text
	}
	if len(prompts) != 23 || len(labels.Labels) != 23 {
		t.Fatal("incomplete development set")
	}
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
		p     m7Prompt
		stamp int64
	}
	var triggers []trigger
	seen := map[string]bool{}
	for _, l := range labels.Labels {
		if seen[l.ID] || byID[l.ID] == "" || (l.Label != "yes" && l.Label != "no") {
			t.Fatal("invalid prompt label")
		}
		seen[l.ID] = true
		var tr trigger
		var payload []byte
		if err := db.QueryRowContext(t.Context(), "SELECT project_id,session_id,received_at,payload FROM events WHERE id=? AND event_name='UserPromptSubmit'", l.ID).Scan(&tr.p.project, &tr.p.session, &tr.stamp, &payload); err != nil {
			t.Fatal("trigger missing")
		}
		var fields struct {
			CWD string `json:"cwd"`
		}
		if err := json.Unmarshal(payload, &fields); err != nil {
			t.Fatal("trigger malformed")
		}
		tr.p.id, tr.p.prompt, tr.p.cwd, tr.p.wanted = l.ID, byID[l.ID], fields.CWD, l.Label
		if m7Drifted(tr.p) {
			t.Fatal("development project identity drift")
		}
		triggers = append(triggers, tr)
	}
	sort.SliceStable(triggers, func(i, j int) bool { return triggers[i].stamp < triggers[j].stamp })
	prefix, err := newTemporalReplay(t, source)
	if err != nil {
		t.Fatal(err)
	}
	type block struct {
		ID, Text string
		Bytes    int
	}
	type result struct {
		Arm, PromptID, Session, Prompt, Wanted, Reason string
		Blocks                                         []block
	}
	var output []result
	type totals struct{ emitted, wanted, wantedEmitted, blocks, bytes, unwanted, deadlines int }
	counts := map[string]*totals{"baseline": {}, "paired": {}}
	for _, tr := range triggers {
		if err := prefix.advance(t, tr.stamp); err != nil {
			t.Fatal(err)
		}
		for _, arm := range []string{"baseline", "paired"} {
			var got inject.Result
			if arm == "baseline" {
				got = m7Build(t, prefix.db, tr.p)
			} else {
				got = pairedBuild(t, prefix.db, tr.p, tr.stamp)
			}
			r := result{Arm: arm, PromptID: tr.p.id, Session: tr.p.session, Prompt: tr.p.prompt, Wanted: tr.p.wanted, Reason: got.Reason}
			c := counts[arm]
			if tr.p.wanted == "yes" {
				c.wanted++
			}
			if got.Reason == inject.ReasonDeadline {
				c.deadlines++
			}
			if got.Text != "" {
				c.emitted++
				if tr.p.wanted == "yes" {
					c.wantedEmitted++
				}
			} else {
				output = append(output, r)
				continue
			}
			for _, b := range m7SplitBlocks(t, tr.p.id, got) {
				var stamp int64
				if b.kind != "event" {
					t.Fatal("nonhistorical native memory entered replay")
				}
				if err := prefix.db.QueryRowContext(t.Context(), "SELECT received_at FROM events WHERE id=? AND project_id=?", b.id, tr.p.project).Scan(&stamp); err != nil || stamp >= tr.stamp {
					t.Fatal("block scope or cutoff violation")
				}
				r.Blocks = append(r.Blocks, block{b.id, b.text, b.bytes})
				c.blocks++
				c.bytes += b.bytes
				if tr.p.wanted == "no" {
					c.unwanted += b.bytes
				}
			}
			output = append(output, r)
		}
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	f, err := root.OpenFile("development-replay.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("replay export exists or cannot be created")
	}
	_, we := f.Write(encoded)
	ce := f.Close()
	if we != nil || ce != nil {
		t.Fatal("replay export failed")
	}
	for _, arm := range []string{"baseline", "paired"} {
		c := counts[arm]
		t.Logf("%s: prompts=23 emitted=%d wanted=%d wanted_emitted=%d blocks=%d bytes=%d unwanted=%d deadlines=%d", arm, c.emitted, c.wanted, c.wantedEmitted, c.blocks, c.bytes, c.unwanted, c.deadlines)
	}
	t.Log("development only; block relevance unjudged; holdout was not read")
}
