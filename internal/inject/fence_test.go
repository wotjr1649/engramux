package inject_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
)

// fencePattern is the shape [inject.Fence] promises, spelled out here rather
// than imported: a test that reads the package's own constants would go green
// on a delimiter that changed to something a model cannot see.
var fencePattern = regexp.MustCompile(`(?s)\A(.*)\n<engramux-data ([A-Z2-7]+)>\n(.*)\n</engramux-data ([A-Z2-7]+)>\z`)

func TestFenceWrapsTheBodyInAMatchedNoncePair(t *testing.T) {
	const body = "an excerpt\nand another"
	out, err := inject.Fence(body)
	if err != nil {
		t.Fatalf("Fence: %v", err)
	}
	m := fencePattern.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("Fence produced text that is not a fenced document: %q", out)
	}
	if m[2] != m[4] {
		t.Errorf("the open nonce is %q and the close nonce is %q, want one nonce", m[2], m[4])
	}
	if m[3] != body {
		t.Errorf("the fenced body is %q, want %q", m[3], body)
	}
	// The lead line is outside the fence on purpose: inside it would be
	// indistinguishable from an instruction the corpus carried.
	if !strings.Contains(m[1], "not instructions") {
		t.Errorf("the lead line does not say the content is not instructions: %q", m[1])
	}
	if strings.Contains(m[1], m[2]) {
		t.Errorf("the lead line carries the nonce, so a reader cannot tell the marker from the prose")
	}
}

// A body that already carries the nonce about to be minted is refused rather
// than fenced. This is gate M9's invariant at the one input crypto/rand will
// not produce on purpose, which is why the mint is a parameter here.
func TestFenceRefusesABodyThatCarriesEveryMintedNonce(t *testing.T) {
	const collides = "AAAAAAAAAAAAAAAAAAAAAAAAAA"
	mints := 0
	_, err := inject.FenceWith("before "+collides+" after", func() string {
		mints++
		return collides
	})
	if !errors.Is(err, inject.ErrFenceCollision) {
		t.Fatalf("FenceWith returned %v, want ErrFenceCollision", err)
	}
	// Every attempt was spent, and the answer is an error rather than a
	// fence the body can close.
	if mints != 3 {
		t.Errorf("the mint was called %d times, want one per attempt", mints)
	}
}

// A mint that collides once and then does not still produces a usable fence:
// the check retries rather than giving up on the first attempt.
func TestFenceRetriesPastACollidingNonce(t *testing.T) {
	const collides = "AAAAAAAAAAAAAAAAAAAAAAAAAA"
	const clean = "BBBBBBBBBBBBBBBBBBBBBBBBBB"
	n := 0
	out, err := inject.FenceWith("before "+collides+" after", func() string {
		n++
		if n == 1 {
			return collides
		}
		return clean
	})
	if err != nil {
		t.Fatalf("FenceWith: %v", err)
	}
	m := fencePattern.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("FenceWith produced text that is not a fenced document: %q", out)
	}
	if m[2] != clean {
		t.Errorf("the fence uses the nonce %q, want the second mint %q", m[2], clean)
	}
	if strings.Contains(m[3], m[2]) {
		t.Errorf("the fenced body carries the nonce, so it can close its own fence")
	}
}

// Two injections do not share a nonce. A fixed marker is one an attacker can
// write into a page the agent fetched weeks ago; this is what makes the close
// marker unforgeable by anything already in the corpus.
func TestFenceMintsANewNoncePerInjection(t *testing.T) {
	seen := map[string]bool{}
	for i := range 100 {
		out, err := inject.Fence("body")
		if err != nil {
			t.Fatalf("Fence: %v", err)
		}
		nonce := fencePattern.FindStringSubmatch(out)[2]
		if seen[nonce] {
			t.Fatalf("injection %d reused the nonce %q", i, nonce)
		}
		seen[nonce] = true
		if len(nonce) < 26 {
			t.Fatalf("the nonce is %d characters, too few for 128 bits of base32", len(nonce))
		}
	}
}

// A body with no trailing newline still gets the close marker on its own line,
// so the marker cannot be glued to the last excerpt's text.
func TestFenceClosesOnItsOwnLine(t *testing.T) {
	for _, body := range []string{"no newline", "one newline\n", "two\n\n"} {
		out, err := inject.Fence(body)
		if err != nil {
			t.Fatalf("Fence(%q): %v", body, err)
		}
		lines := strings.Split(out, "\n")
		last := lines[len(lines)-1]
		if !strings.HasPrefix(last, "</engramux-data ") {
			t.Errorf("Fence(%q) ends with %q, want the close marker alone on the last line", body, last)
		}
	}
}
