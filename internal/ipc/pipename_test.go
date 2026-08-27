package ipc

import (
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
		t.Errorf("CurrentPipeName() did not match PipeName(user.Current().Uid)")
	}
}
