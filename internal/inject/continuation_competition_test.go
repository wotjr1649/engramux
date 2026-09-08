package inject_test

import (
	"encoding/json"
	"testing"
)

// This diagnostic freezes a latest-reply counterexample, not a quality gate.
func TestMeasureContinuationCompetition(t *testing.T) {
	cases := []struct{ name, later, wanted string }{
		{"unrelated_latest", "The documentation screenshot now uses a blue background.", "decision"},
		{"revised_decision", "The tie policy was revised: equal weights now retain the larger Sequence at the exact cap.", "later"},
	}
	matched := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var docs []seeded
			for _, d := range []struct{ id, body string }{
				{"decision", "The SelectZorrenEvents tie policy retains smaller Sequence at the exact cap."},
				{"later", tc.later},
			} {
				payload, err := json.Marshal(map[string]string{
					"cwd": "/w", "session_id": "s", "hook_event_name": "Stop",
					"transcript_path":        "/synthetic/.claude/projects/s.jsonl",
					"last_assistant_message": d.body,
				})
				if err != nil {
					t.Fatal(err)
				}
				docs = append(docs, seeded{id: d.id, payload: payload})
			}
			db := corpus(t, docs...)
			if _, err := db.ExecContext(t.Context(), "UPDATE events SET received_at=CASE id WHEN 'decision' THEN 10 ELSE 11 END"); err != nil {
				t.Fatal(err)
			}
			p := m7Prompt{id: "current", cwd: "/w", prompt: "Continue SelectZorrenEvents using the earlier tie policy."}
			if err := db.QueryRowContext(t.Context(), "SELECT project_id,session_id FROM events WHERE id='decision'").Scan(&p.project, &p.session); err != nil {
				t.Fatal(err)
			}
			got := continuationBuild(t, db, p, 20)
			if len(got.Events) != 1 || got.Events[0] != "later" {
				t.Fatal("frozen latest-reply behavior changed; reassess diagnostic")
			}
			correct := got.Events[0] == tc.wanted
			if correct {
				matched++
			}
			t.Logf("selected=%s wanted=%s relevant=%v", got.Events[0], tc.wanted, correct)
		})
	}
	t.Logf("required decision recovered=%d/%d; process success is not a quality pass", matched, len(cases))
}
