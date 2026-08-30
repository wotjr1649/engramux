package ipc

import (
	"os"
	"os/user"
	"strings"
	"testing"
)

// Example-format SIDs (Microsoft's own documentation shape), not any real
// machine's identity: PipeName must be pure and testable without being the
// current user.
const (
	testSID1 = "S-1-5-21-1111111111-2222222222-3333333333-1001"
	testSID2 = "S-1-5-21-1111111111-2222222222-3333333333-1002"
)

// TestPipeName_Deterministic: the same SID always derives the same name.
func TestPipeName_Deterministic(t *testing.T) {
	got1 := PipeName(testSID1)
	got2 := PipeName(testSID1)
	if got1 != got2 {
		t.Errorf("PipeName(sid) is not deterministic: %q != %q", got1, got2)
	}
}

// TestPipeName_DifferentSIDsDifferentNames: two different SIDs never
// collide.
func TestPipeName_DifferentSIDsDifferentNames(t *testing.T) {
	got1 := PipeName(testSID1)
	got2 := PipeName(testSID2)
	if got1 == got2 {
		t.Errorf("PipeName(sid1) == PipeName(sid2) == %q, want different names", got1)
	}
}

// TestPipeName_Shape asserts the fixed prefix and the local pipe namespace
// (spec 5.2), exactly, not by substring guesswork.
func TestPipeName_Shape(t *testing.T) {
	got := PipeName(testSID1)
	const wantPrefix = `\\.\pipe\engramux.v1`
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("PipeName(sid) = %q, want prefix %q", got, wantPrefix)
	}
}

// TestCurrentPipeName cross-checks the os/user-backed wrapper against the
// pure function, without ever hardcoding or printing this machine's actual
// SID anywhere.
func TestCurrentPipeName(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	want := PipeName(u.Uid)

	got, err := CurrentPipeName()
	if err != nil {
		t.Fatalf("CurrentPipeName: %v", err)
	}
	if got != want {
		// Neither name is printed and neither ever will be: both are derived
		// from this machine's SID, and one of them is what a real service
		// listens on. What a reader needs instead is the one thing that moves
		// the derivation. A leftover export in the calling shell fails this
		// assertion and nothing else here would say so - and whether it is set
		// says it without the value, which could be a real SID.
		_, override := os.LookupEnv(TestPipeSIDEnv)
		t.Errorf("CurrentPipeName() did not match PipeName(user.Current().Uid); %s set in this process: %v",
			TestPipeSIDEnv, override)
	}

	// The seam itself. Every test in this repository that listens on the
	// derived name is now standing on it, so it is asserted here rather than
	// inferred from those tests passing: they would also pass on a build that
	// ignored the variable and happened to run with no service up.
	const override = "engramux-test-current-pipe-name"
	t.Setenv(TestPipeSIDEnv, override)

	overridden, err := CurrentPipeName()
	if err != nil {
		t.Fatalf("CurrentPipeName under %s: %v", TestPipeSIDEnv, err)
	}
	// It stands in for the SID as the *input to the hash*, so the name keeps
	// the shape spec 5.2 fixes and only lands elsewhere in the namespace.
	if wantOverridden := PipeName(override); overridden != wantOverridden {
		t.Errorf("CurrentPipeName() under %s = %q, want PipeName(%q) = %q",
			TestPipeSIDEnv, overridden, override, wantOverridden)
	}
	if overridden == got {
		t.Errorf("CurrentPipeName() returned the same name with and without %s, "+
			"so the override is not reaching the derivation", TestPipeSIDEnv)
	}
	const wantPrefix = `\\.\pipe\engramux.v1`
	if !strings.HasPrefix(overridden, wantPrefix) {
		t.Errorf("CurrentPipeName() under %s = %q, want prefix %q", TestPipeSIDEnv, overridden, wantPrefix)
	}

	// Empty is not set. A stray empty value in an environment - a child
	// launched with the variable cleared rather than removed - must leave the
	// real derivation alone, or the service would listen where nothing dials.
	t.Setenv(TestPipeSIDEnv, "")
	empty, err := CurrentPipeName()
	if err != nil {
		t.Fatalf("CurrentPipeName under an empty %s: %v", TestPipeSIDEnv, err)
	}
	if empty != want {
		t.Errorf("an empty %s moved the pipe name off the real derivation", TestPipeSIDEnv)
	}
}
