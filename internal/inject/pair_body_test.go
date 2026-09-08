package inject_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wotjr1649/engramux/internal/secret"
)

func pairedBody(payload []byte) string {
	if len(payload) > 1048576 {
		return ""
	}
	var fields struct {
		Body string `json:"last_assistant_message"`
	}
	if err := json.Unmarshal(secret.Mask(payload), &fields); err != nil {
		return ""
	}
	runes := []rune(fields.Body)
	if len(runes) > 240 {
		runes = runes[:240]
	}
	return string(runes)
}

func TestPairedBodyBoundary(t *testing.T) {
	long := strings.Repeat("한", 300)
	for _, tc := range []struct{ payload, want string }{
		{`{"last_assistant_message":"accepted retry count is seven"}`, "accepted retry count is seven"},
		{`{"prompt":"not an assistant answer"}`, ""},
		{`{"last_assistant_message":42}`, ""},
		{`{"last_assistant_message":null}`, ""},
		{`{"last_assistant_message":"broken"`, ""},
		{`{"last_assistant_message":"` + long + `"}`, strings.Repeat("한", 240)},
	} {
		got := pairedBody([]byte(tc.payload))
		if got != tc.want || !utf8.ValidString(got) {
			t.Fatalf("body bytes=%d want=%d", len(got), len(tc.want))
		}
	}
	// This is a generated sample, not a credential from a user or capture.
	sample := "ghp_" + strings.Repeat("a", 36)
	payload, err := json.Marshal(map[string]string{"last_assistant_message": sample})
	if err != nil {
		t.Fatal(err)
	}
	if got := pairedBody(payload); got != secret.MaskString(sample) {
		t.Fatal("body did not match whole-payload masking")
	}
	if got := pairedBody([]byte(`{"last_assistant_message":"` + strings.Repeat("x", 1048576) + `"}`)); got != "" {
		t.Fatal("oversized payload accepted")
	}
}
