package main

import (
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/winacl"
)

// TestPermissionsTellsANarrowedFileFromAnInheritedOne is the doctor half of
// memory spec §8's second publication condition, and it is asserted on a file of
// each kind because one of the two is what every machine has today.
//
// The two verdicts have to differ, and that is the assertion the obvious version
// of this test misses: "the output mentions the file" is true of both, and a
// check that only looks at the narrowed case passes on a `permissions` that
// prints the same line unconditionally.
func TestPermissionsTellsANarrowedFileFromAnInheritedOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("create the file: %v", err)
	}

	inherited := permissionsLine(t, path)
	if !strings.Contains(inherited, "inherited DACL") {
		t.Errorf("an inherited file reported %q, want it to say so", inherited)
	}
	// This line is a finding rather than a fact, so it owes the reader what
	// is at stake and not only a verdict. Both inherited branches carry that
	// clause - one says the token is reachable, the other says the parent
	// directory is what decides - and a message trimmed to "inherited DACL"
	// would pass every other assertion here.
	//
	// This used to assert the line named backlog 28. That row closed on
	// 2026-09-04 and was deleted by the backlog's own rule, so the message
	// was pointing at a row that no longer exists - which is worse than
	// pointing nowhere, because a reader would go looking.
	if !strings.Contains(inherited, "token") && !strings.Contains(inherited, "parent directory") {
		t.Errorf("an inherited file reported %q, which is a verdict with no consequence attached", inherited)
	}

	if err := winacl.Restrict(path); err != nil {
		t.Fatalf("restrict: %v", err)
	}
	narrowed := permissionsLine(t, path)
	if !strings.Contains(narrowed, "narrowed") {
		t.Errorf("a narrowed file reported %q, want it to say so", narrowed)
	}
	if narrowed == inherited {
		t.Error("both files report the same line, so the verdict is not being computed")
	}
}

// TestPermissionsNamesNoPrincipal is a hard requirement rather than tidiness.
//
// This report is what a person pastes into a public issue, and a DACL is made of
// account names, machine names and SIDs. [report.mask] cannot be relied on for
// it: --full turns masking off, so the only safe form is one that has nothing to
// unmask. Both states are checked, because the finding branch is the one that
// would want to be helpful and name who can read the file.
func TestPermissionsNamesNoPrincipal(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("create the file: %v", err)
	}

	for _, stage := range []string{"inherited", "narrowed"} {
		if stage == "narrowed" {
			if err := winacl.Restrict(path); err != nil {
				t.Fatalf("restrict: %v", err)
			}
		}
		// --full, which is the harder case: it is the mode that stops
		// masking, so a line that leaks here leaks in the mode a user
		// reaches for when a masked report was not enough.
		var out bytes.Buffer
		r := &report{w: &out, full: true}
		r.permissions("codex file", path)
		got := out.String()

		if strings.Contains(got, u.Uid) {
			t.Errorf("%s: the line carries this user's SID", stage)
		}
		if strings.Contains(got, u.Username) {
			t.Errorf("%s: the line carries this user's account name", stage)
		}
		if strings.Contains(got, "S-1-") {
			t.Errorf("%s: the line carries a SID: %q", stage, got)
		}
	}
}

// TestPermissionsIsSilentAboutAFileThatIsNotThere keeps the caller's own "no
// configuration file at ..." from being followed by a second line saying the
// same thing in different words - two findings where there is one fact.
func TestPermissionsIsSilentAboutAFileThatIsNotThere(t *testing.T) {
	var out bytes.Buffer
	r := &report{w: &out}
	r.permissions("codex file", filepath.Join(t.TempDir(), "absent"))
	if out.Len() != 0 {
		t.Errorf("an absent file produced %q, want nothing", out.String())
	}
	if r.failed {
		t.Error("an absent file set the failure flag")
	}
}

// TestPermissionsDoesNotChangeTheExitCode pins the decision that an inherited
// DACL is a note and not a failure. Spec 5.9 accepts the exposure on the owner's
// machine and §8 carries it as a publication condition, so failing here would
// make every correct installation on every machine today report broken.
func TestPermissionsDoesNotChangeTheExitCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("create the file: %v", err)
	}
	var out bytes.Buffer
	r := &report{w: &out}
	r.permissions("codex file", path)
	if r.failed {
		t.Errorf("an inherited DACL set the failure flag; the line was %q", out.String())
	}
}

// permissionsLine runs one report and returns what it wrote.
func permissionsLine(t *testing.T, path string) string {
	t.Helper()
	var out bytes.Buffer
	r := &report{w: &out}
	r.permissions("codex file", path)
	if out.Len() == 0 {
		t.Fatalf("nothing was reported about %s", filepath.Base(path))
	}
	return out.String()
}
