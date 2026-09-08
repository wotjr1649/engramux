package inject_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/wotjr1649/engramux/internal/secret"
)

// This is an availability audit, not a selector or a relevance gate. It never
// infers absence of an answer from absence of the latest same-session Stop.
func TestWriteTransferPredecessors(t *testing.T) {
	if os.Getenv("ENGRAMUX_TRANSFER_PREDECESSORS") != "1" {
		t.Skip("development predecessor audit is opt-in")
	}
	const dir = "../../.capture/selection-quality/transfer-2026-09-08"
	const source = "../../.capture/session-resume-2026-09-08/engramux.db"
	prompts, err := os.ReadFile(dir + "/development-prompts.json")
	if err != nil {
		t.Fatal("development prompts unavailable")
	}
	data, err := os.ReadFile(dir + "/development-agent-labels.json")
	if err != nil {
		t.Fatal("development labels unavailable")
	}
	var labels struct {
		Source string
		Hash   string `json:"prompts_sha256"`
		Labels []struct{ ID, Label string }
	}
	if json.Unmarshal(data, &labels) != nil || labels.Source != "agent" || labels.Hash != fmt.Sprintf("%x", sha256.Sum256(prompts)) || len(labels.Labels) != 23 {
		t.Fatal("development label binding mismatch")
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
	type result struct {
		PromptID, EventID, Body string
		Count                   int
		Truncated               bool
	}
	var output []result
	for _, label := range labels.Labels {
		if label.Label != "yes" {
			continue
		}
		r := result{PromptID: label.ID}
		var host, session, project string
		var cutoff int64
		if err := db.QueryRowContext(t.Context(), "SELECT host,session_id,project_id,received_at FROM events WHERE id=? AND event_name='UserPromptSubmit'", label.ID).Scan(&host, &session, &project, &cutoff); err != nil {
			t.Fatal("trigger missing")
		}
		if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM events WHERE host=? AND session_id=? AND project_id=? AND event_name='Stop' AND received_at>0 AND received_at<?", host, session, project, cutoff).Scan(&r.Count); err != nil {
			t.Fatal("predecessor count failed")
		}
		if r.Count > 0 {
			var payload []byte
			if err := db.QueryRowContext(t.Context(), `SELECT id,CASE WHEN length(CAST(payload AS BLOB))<=1048576 THEN payload ELSE NULL END FROM events WHERE host=? AND session_id=? AND project_id=? AND event_name='Stop' AND received_at>0 AND received_at<? ORDER BY received_at DESC,id DESC LIMIT 1`, host, session, project, cutoff).Scan(&r.EventID, &payload); err != nil {
				t.Fatal("predecessor missing")
			}
			var fields struct {
				Body string `json:"last_assistant_message"`
			}
			if len(payload) == 0 || json.Unmarshal(secret.Mask(payload), &fields) != nil {
				t.Fatal("predecessor body invalid or oversized")
			}
			runes := []rune(fields.Body)
			r.Truncated = len(runes) > 8000
			r.Body = string(runes[:min(len(runes), 8000)])
		}
		output = append(output, r)
	}
	if len(output) != 6 {
		t.Fatal("wanted count mismatch")
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(dir+"/development-predecessors-expanded.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("predecessor export exists or unavailable")
	}
	_, we := f.Write(encoded)
	ce := f.Close()
	if we != nil || ce != nil {
		t.Fatal("predecessor export failed")
	}
	for i, r := range output {
		t.Logf("wanted=%d prior_stops=%d body_bytes=%d truncated=%t", i+1, r.Count, len(r.Body), r.Truncated)
	}
}
