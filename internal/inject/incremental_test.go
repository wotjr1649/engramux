package inject_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// TestMeasureIncrementalHistory exports actual frozen-selector output for public tasks.
// Success means the experiment ran, not that injection improved task completion.
func TestMeasureIncrementalHistory(t *testing.T) {
	if os.Getenv("ENGRAMUX_INCREMENTAL_HISTORY") != "1" {
		t.Skip("public task export is opt-in")
	}
	data, err := os.ReadFile("../../docs/evidence/incremental-tasks-2026-09-08.json")
	if err != nil {
		t.Fatal(err)
	}
	var design struct {
		Cases []struct{ ID, Prompt, History string }
	}
	if err := json.Unmarshal(data, &design); err != nil {
		t.Fatal(err)
	}
	if len(design.Cases) != 4 {
		t.Fatal("expected four frozen public tasks")
	}
	type result struct {
		ID     string   `json:"id"`
		Text   string   `json:"text"`
		Events []string `json:"events"`
		Reason string   `json:"reason"`
		SHA256 string   `json:"sha256"`
	}
	var results []result
	seen := map[string]bool{}
	for _, c := range design.Cases {
		if c.ID == "" || c.Prompt == "" || c.History == "" || seen[c.ID] {
			t.Fatal("invalid or duplicate task")
		}
		seen[c.ID] = true
		payload, err := json.Marshal(map[string]string{
			"cwd": "/w", "session_id": "s", "hook_event_name": "Stop",
			"transcript_path":        "/synthetic/.claude/projects/s.jsonl",
			"last_assistant_message": c.History,
		})
		if err != nil {
			t.Fatal(err)
		}
		db := corpus(t, seeded{id: c.ID, payload: payload})
		if _, err := db.ExecContext(t.Context(), "UPDATE events SET received_at=10"); err != nil {
			t.Fatal(err)
		}
		p := m7Prompt{id: "current", cwd: "/w", prompt: c.Prompt}
		if err := db.QueryRowContext(t.Context(), "SELECT project_id,session_id FROM events WHERE id=?", c.ID).Scan(&p.project, &p.session); err != nil {
			t.Fatal(err)
		}
		got := continuationBuild(t, db, p, 20)
		results = append(results, result{c.ID, got.Text, got.Events, got.Reason, fmt.Sprintf("%x", sha256.Sum256([]byte(got.Text)))})
		t.Logf("task=%s selected=%d bytes=%d reason=%s", c.ID, len(got.Events), len(got.Text), got.Reason)
	}
	out, err := json.MarshalIndent(struct {
		DesignSHA256 string   `json:"design_sha256"`
		Results      []result `json:"results"`
	}{fmt.Sprintf("%x", sha256.Sum256(data)), results}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	const path = "../../docs/evidence/incremental-output-2026-09-08.json"
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.Write(append(out, '\n'))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("export failed: write=%v close=%v", writeErr, closeErr)
	}
}
