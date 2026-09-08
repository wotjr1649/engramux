package inject_test

import (
	"strings"
	"testing"
)

// A position-only hypothesis, deliberately separate from the frozen v2 replay.
func positionSignal(prompt string) bool {
	words := strings.Fields(strings.ToLower(prompt))
	for i, word := range words {
		word = strings.Trim(word, ".,!?;:\"'()[]{}")
		if word == "continue" || word == "resume" {
			if i == 0 || (i == 1 && words[0] == "please") {
				return true
			}
			continue
		}
		if continuationSignal(word) {
			return true
		}
	}
	return false
}

func TestMeasureAdmissionPositionContrast(t *testing.T) {
	// Labels are synthetic prompt-level estimates, not owner judgements.
	// Ambiguous cases are measured separately rather than forced into yes/no.
	cases := []struct{ prompt, label string }{
		{"Explain the continue keyword.", "no"},
		{"Print exactly the word resume.", "no"},
		{"Do not continue the previous benchmark; start a fresh run.", "no"},
		{"Here is the complete schema: previous is an optional string. Explain the previous field using only this schema.", "no"},
		{"앞서 합의한 제한을 유지하고 다음 검증을 추가해.", "yes"},
		{"이어서 migration 검사를 적용하고 실패 원인을 기록해.", "yes"},
		{"Apply the recommendation we made earlier to the migration check.", "yes"},
		{"Continue with the API review.", "unknown"},
		{"그럼 확인해.", "unknown"},
		{"`추천대로`라는 단어가 이 명령문에서 어떤 역할을 하는지 설명해.", "no"},
	}
	for _, arm := range []struct {
		name   string
		signal func(string) bool
	}{{"v2", continuationSignal}, {"position", positionSignal}} {
		yes, no, unknown := 0, 0, 0
		for _, tc := range cases {
			if !arm.signal(tc.prompt) {
				continue
			}
			switch tc.label {
			case "yes":
				yes++
			case "no":
				no++
			case "unknown":
				unknown++
			}
		}
		t.Logf("%s: needed admitted=%d/3 self-contained admitted=%d/5 ambiguous admitted=%d/2", arm.name, yes, no, unknown)
	}
}
