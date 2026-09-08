package secret

import (
	"os"
	"strings"
	"testing"
)

// This public synthetic probe measures each existing rule independently.
// It changes no pattern, masking boundary or production call path.
func TestDiagnoseRuleCost(t *testing.T) {
	if os.Getenv("ENGRAMUX_RULE_COST") != "1" {
		t.Skip("rule cost probe is opt-in")
	}
	text := strings.Repeat(" w12345", 1200)[:8192]
	for _, r := range rules {
		failed := false
		result := testing.Benchmark(func(b *testing.B) {
			defer func() { failed = failed || b.Failed() }()
			for range b.N {
				if len(r.re.FindAllStringSubmatchIndex(text, -1)) != 0 {
					b.Fatal("nonsecret probe unexpectedly matched")
				}
			}
		})
		if failed || result.N == 0 {
			t.Fatal("rule benchmark did not run")
		}
		t.Logf("class=%s bytes=8192 ns/op=%d", r.class, result.NsPerOp())
	}
}
