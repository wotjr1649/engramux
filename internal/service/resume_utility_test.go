package service

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/search"
)

// An independent agent fixed these public synthetic tasks before retrieval.
// The measurement exports context, not a task-success verdict. Fresh solvers
// must supply that evidence without seeing the expected answers below.
func TestMeasureSessionResumeTaskContexts(t *testing.T) {
	if os.Getenv("ENGRAMUX_RESUME_SYNTHETIC") != "1" {
		t.Skip("opt-in synthetic task context export")
	}
	cases := []struct{ keys, prior, current, expected string }{
		{"retry_count and queue_name", `retry_count=7 and queue_name="amber-latch"`, "", `{"retry_count":7,"queue_name":"amber-latch"}`},
		{"batch_size and cache_mode", `batch_size=13 and cache_mode="copper-sieve"`, "", `{"batch_size":13,"cache_mode":"copper-sieve"}`},
		{"worker_limit and checkpoint_style", `worker_limit=3 and checkpoint_style="append-only"`, "", `{"worker_limit":3,"checkpoint_style":"append-only"}`},
		{"retry_count and queue_name", `retry_count=2 and queue_name="old-queue"`, `retry_count=9 and queue_name="current-queue"`, `{"retry_count":9,"queue_name":"current-queue"}`},
	}
	type task struct {
		ID                            int
		Prompt, FTS, Resume, Expected string
	}
	var tasks []task
	for i, tc := range cases {
		db, _, _ := twoProjects(t)
		const root = "C:/synthetic/resume-utility"
		session := fmt.Sprintf("20000000-0000-4000-8000-%012d", i+1)
		payload, err := json.Marshal(map[string]string{
			"cwd": root, "session_id": session, "hook_event_name": "Stop",
			"transcript_path":        "/synthetic/.claude/projects/session.jsonl",
			"last_assistant_message": "Stop: The accepted resume configuration for this session is " + tc.prior + ".",
		})
		if err != nil {
			t.Fatal(err)
		}
		ingestOne(t, db, "accepted", payload)
		p, err := project.FromArgument(root)
		if err != nil {
			t.Fatal(err)
		}
		hits, _, err := search.Search(t.Context(), db, session, p.ID, 100, search.MatchAny)
		if err != nil {
			t.Fatal(err)
		}
		fts := ""
		for _, hit := range hits {
			fts += hit.Excerpt + "\n"
		}
		res, err := sessionResume(t.Context(), db, ipc.SessionResumeRequest{Project: root, Host: "claude-code", HostSessionID: session})
		if err != nil || res.LatestReply == nil {
			t.Fatal("synthetic session read failed")
		}
		prompt := "Resume the work associated with session " + session + ". Return exactly one compact JSON object with the keys " + tc.keys + ", in that order, using the values accepted in that session's prior Stop message. Do not add explanation or extra keys."
		if tc.current != "" {
			prompt = "For session " + session + ", the current prompt itself gives the values to return: " + tc.current + ". Do not use history. Return exactly one compact JSON object with the keys " + tc.keys + ", in that order, using the values stated here. Do not add explanation or extra keys."
		}
		tasks = append(tasks, task{ID: i + 1, Prompt: prompt, FTS: fts, Resume: res.LatestReply.Body, Expected: tc.expected})
	}
	b, err := json.Marshal(tasks)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SYNTHETIC_TASK_CONTEXTS %s", b)
}
