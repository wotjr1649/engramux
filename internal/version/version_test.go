package version

import (
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/secret"
)

// TestTheLinkedValueWins is the whole of what -X buys.
func TestTheLinkedValueWins(t *testing.T) {
	t.Cleanup(func(prev string) func() { return func() { linked = prev } }(linked))
	linked = "0.4.2"
	if got := Product(); got != "0.4.2" {
		t.Errorf("Product() = %q, want the linked value %q", got, "0.4.2")
	}
}

// TestADevelopmentBuildIdentifiesItsCommit holds the fallback: a binary with no
// release version still says which commit it is, rather than answering "dev".
//
// The test binary is built from this repository, so it carries build info and
// the revision arm is the one that runs. A tree that is not a repository takes
// the bare-prefix arm, which is asserted through the shape rather than by
// arranging one.
func TestADevelopmentBuildIdentifiesItsCommit(t *testing.T) {
	t.Cleanup(func(prev string) func() { return func() { linked = prev } }(linked))
	linked = ""

	got := Product()
	if !strings.HasPrefix(got, devPrefix) {
		t.Fatalf("Product() = %q, want it to start with %q", got, devPrefix)
	}
	rev, _ := vcs()
	if rev == "" {
		t.Skip("this binary carries no vcs.revision, so there is no commit to identify")
	}
	if len(rev) != shaLen {
		t.Errorf("the revision is %d characters, want %d", len(rev), shaLen)
	}
	if !strings.Contains(got, rev) {
		t.Errorf("Product() = %q, want it to carry the revision %q", got, rev)
	}
}

// TestTheVersionSurvivesDoctorsMask is why [shaLen] is 12 and not 40.
//
// `doctor` prints every line through [secret.MaskString], and ClassOpaque
// matches forty or more characters of the base64 alphabet - which a full git
// SHA is. A version that came back redacted would be reported as a secret by
// the one command that exists to report it, and nothing else in this repository
// would notice.
func TestTheVersionSurvivesDoctorsMask(t *testing.T) {
	t.Cleanup(func(prev string) func() { return func() { linked = prev } }(linked))

	for _, v := range []string{"", "0.1.0", "1.2.3-rc.1"} {
		linked = v
		got := Product()
		if masked := secret.MaskString(got); masked != got {
			t.Errorf("Product() = %q, which doctor's own mask rewrites to %q", got, masked)
		}
	}
}
