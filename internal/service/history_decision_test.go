package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

func TestMeasureHistoryDecisionContexts(t *testing.T) {
	if os.Getenv("ENGRAMUX_HISTORY_DECISIONS") != "1" {
		t.Skip("synthetic decision context export is opt-in")
	}
	data, err := os.ReadFile("../../docs/evidence/history-decision-tasks-2026-09-08.json")
	if err != nil {
		t.Fatal(err)
	}
	var design struct {
		Choices []string
		Cases   []struct {
			ID, Question string
			Records      []struct {
				ID, Body string
				Received int64
			}
		}
	}
	if json.Unmarshal(data, &design) != nil || len(design.Cases) != 3 {
		t.Fatal("invalid design")
	}
	type task struct {
		ID, Question string
		Choices      []string
		Latest       *ipc.ResumeMessage
		History      []ipc.ResumeMessage
	}
	var out []task
	for _, tc := range design.Cases {
		db, a, _ := twoProjects(t)
		for _, r := range tc.Records {
			payload, err := json.Marshal(map[string]string{"cwd": a.root, "session_id": a.session, "hook_event_name": "Stop", "last_assistant_message": r.Body, "transcript_path": "/synthetic/.claude/projects/session.jsonl"})
			if err != nil {
				t.Fatal(err)
			}
			ingestOne(t, db, r.ID, payload)
			if _, err := db.ExecContext(t.Context(), "UPDATE events SET received_at=? WHERE id=?", r.Received, r.ID); err != nil {
				t.Fatal(err)
			}
		}
		var storedSession string
		if err := db.QueryRowContext(t.Context(), "SELECT session_id FROM events WHERE id=?", tc.Records[0].ID).Scan(&storedSession); err != nil {
			t.Fatal(err)
		}
		res, err := sessionResume(t.Context(), db, ipc.SessionResumeRequest{Project: a.root, Host: "claude-code", HostSessionID: a.session})
		if err != nil || res.LatestReply == nil {
			t.Fatal("latest read failed")
		}
		history := boundedHistory(t, db, a.root, "claude-code", storedSession, 200)
		if len(history) != 3 {
			t.Fatal("synthetic history incomplete")
		}
		out = append(out, task{tc.ID, tc.Question, design.Choices, res.LatestReply, history})
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PUBLIC_DECISION_CONTEXTS %s", encoded)
}
