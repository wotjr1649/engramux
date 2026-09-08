package inject_test

import (
	"encoding/json"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
)

func TestInjectionMatchesPunctuationLikeSearch(t *testing.T) {
	db := corpus(t, seeded{id: "answer", payload: []byte(`{"hook_event_name":"PostToolUse","session_id":"s","cwd":"/w","tool_response":{"stdout":"quartz uses three retries"}}`)})
	for _, prompt := range []string{"quartz", "quartz?", "quartz!", "quartz,"} {
		res, err := inject.Build(t.Context(), db, inject.Request{Prompt: prompt})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Events) != 1 || res.Events[0] != "answer" {
			t.Errorf("prompt %q lost its known content match", prompt)
		}
	}
}

// Supplied by an independent fixture-design agent before it saw either
// selector's outputs. This probe retains its prompts, decisions and expected
// answers; metadata distractors and future records are not ingested here.
// These are retrieval diagnostics, not task-success scores.
func TestMeasureIndependentSyntheticSelection(t *testing.T) {
	cases := []struct {
		prompt, earlier, later, answer string
		needed                         bool
	}{
		{"현재 prompt 이전에 이미 확정된 flaky network step의 retry policy를 고르세요. 미래 기록은 무시하세요. 정확히 다음 토큰 중 하나만 출력하세요: fixed-3, exponential-5", "The accepted decision before the current prompt was fixed backoff with a maximum of 3 attempts.", "Later review changed the recommendation to exponential backoff with a maximum of 5 attempts.", "fixed-3", true},
		{"현재 prompt 이전에 parser의 line-ending failure를 해결하기 위해 채택한 수정 방법을 고르세요. 미래 기록은 무시하세요. 정확히 다음 토큰 중 하나만 출력하세요: normalize-lf, trim-spaces", "Accepted fix before this prompt: normalize CRLF to LF at the input boundary.", "Later experiment proposed trimming trailing spaces while preserving CRLF.", "normalize-lf", true},
		{"현재 prompt 이전에 data migration에서 index rebuild를 언제 수행하기로 결정했는지 고르세요. 미래 기록은 무시하세요. 정확히 다음 토큰 중 하나만 출력하세요: post-migration, pre-migration", "Decision recorded before this prompt: rebuild the index after the migration commits.", "Later proposal moved the index rebuild before migration.", "post-migration", true},
		{"현재 prompt 이전에 cache collision을 고치기 위해 선택한 key 구성은 무엇이었나요? 미래 기록은 무시하세요. 정확히 다음 토큰 중 하나만 출력하세요: workspace-command, timestamp-only", "Accepted before this prompt: compose the cache key from the workspace identifier and command.", "Later suggestion: use timestamp-only keys.", "workspace-command", true},
		{"현재 prompt 자체에 check 결과가 successes=12, failures=0이라고 명시되어 있습니다. failures가 0이면 pass, 아니면 fail입니다. history를 사용하지 말고 정확히 다음 토큰 중 하나만 출력하세요: pass, fail", "A prior note discusses an unrelated benchmark.", "A later note incorrectly labels the current check fail because of an unrelated warning.", "pass", false},
		{"현재 prompt에 정확히 3 files changed라고 명시되어 있습니다. history를 사용하지 말고 정확히 다음 토큰 중 하나만 출력하세요: 3, 5", "The previous task concerned formatting only.", "A later note claims that 5 files changed.", "3", false},
		{"현재 prompt가 safe mode를 명시적으로 선택하고 error가 없다고 말합니다. history를 사용하지 말고 정확히 다음 토큰 중 하나만 출력하세요: safe, unsafe", "The previous run completed without producing a decision.", "A later note recommends unsafe mode for a different experiment.", "safe", false},
		{"현재 prompt가 deadline을 250ms로 지정합니다. history를 사용하지 말고 정확히 다음 토큰 중 하나만 출력하세요: 250ms, 500ms", "The previous timing check recorded no decision.", "A later note changes the deadline to 500ms for a separate experiment.", "250ms", false},
	}
	var needed, recovered, unnecessary int
	for _, tc := range cases {
		payload, err := json.Marshal(map[string]string{"last_assistant_message": tc.earlier, "cwd": "/w", "session_id": "s", "hook_event_name": "Stop"})
		if err != nil {
			t.Fatal(err)
		}
		// Future text is deliberately absent from the available prefix. The
		// temporal replay test separately proves the exclusion mechanism.
		db := corpus(t, seeded{id: "earlier", payload: payload})
		res, err := inject.Build(t.Context(), db, inject.Request{Prompt: tc.prompt})
		if err != nil {
			t.Fatal(err)
		}
		if tc.needed {
			needed++
			if contains(res.Events, "earlier") {
				recovered++
			}
		} else if res.Text != "" {
			unnecessary++
		}
	}
	t.Logf("synthetic history needed: %d/%d recovered; unnecessary injections: %d/%d", recovered, needed, unnecessary, len(cases)-needed)
	t.Log("NOT TASK SUCCESS: a retrieved decision still needs a separately observed agent answer")
}
