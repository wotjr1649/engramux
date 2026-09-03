package winacl_test

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/wotjr1649/engramux/internal/winacl"
)

// fileAllAccess is the mask every ACE [winacl.Restrict] writes must carry,
// spelled here rather than imported from the package under test: a constant
// shared with the implementation would move with it, and this test's whole job
// is to fail when it does.
//
// 0x001F01FF is FILE_ALL_ACCESS. Measured 2026-09-04, and it is not what the
// arithmetic suggests: writing GENERIC_ALL (0x10000000, sharing no bit with it)
// leaves this assertion green, because ACLFromEntries maps a generic mask to the
// object type's specific rights before the ACE exists. That mutation is
// equivalent rather than uncaught - GENERIC_READ instead reads back as
// 0x00120089 and turns this red, which is what says the assertion is live.
const fileAllAccess = 0x001F01FF

// TestRestrictWritesTheExactDACL reads the DACL back through a different API
// than the one that wrote it and pins all five properties separately.
//
// Five assertions and not one, because they fail independently and were each
// watched failing: unprotecting the DACL leaves every mask right and turns the
// count and the inheritance flag red, and a narrower mask leaves the count and
// the flag right and turns the mask red. A single "the DACL looks fine" check
// would pass on one of the two.
func TestRestrictWritesTheExactDACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restricted")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("create the file: %v", err)
	}
	if err := winacl.Restrict(path); err != nil {
		t.Fatalf("restrict: %v", err)
	}

	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("read the security descriptor back: %v", err)
	}

	control, _, err := sd.Control()
	if err != nil {
		t.Fatalf("read the control bits: %v", err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Error("SE_DACL_PROTECTED is not set, so an ACE granted on a parent " +
			"directory still reaches this file")
	}

	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("read the DACL: %v", err)
	}
	if dacl == nil {
		t.Fatal("the DACL is absent, which grants everyone everything")
	}

	want := wantedSIDs(t)
	if int(dacl.AceCount) != len(want) {
		t.Fatalf("the DACL has %d ACEs, want exactly %d - an extra one is an "+
			"inherited grant that survived", dacl.AceCount, len(want))
	}

	got := map[string]bool{}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatalf("read ACE %d: %v", i, err)
		}
		if ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			t.Errorf("ACE %d carries INHERITED_ACE", i)
		}
		if ace.Mask != fileAllAccess {
			t.Errorf("ACE %d has mask %#08x, want %#08x (FILE_ALL_ACCESS)",
				i, uint32(ace.Mask), uint32(fileAllAccess))
		}
		got[aceSID(ace).String()] = true
	}
	for _, sid := range want {
		if !got[sid] {
			t.Errorf("no ACE for %s", sid)
		}
	}
}

// TestRestrictLeavesTheFileReadable is the control arm, and it is the one that
// catches a mask that parses but grants nothing.
//
// Every assertion above is about what the DACL says. This one is about what
// Windows does with it, and the two come apart: the owner's implicit rights are
// READ_CONTROL and WRITE_DAC and include neither FILE_READ_DATA nor DELETE, so a
// DACL that names the owner in every ACE can still refuse both. Measured
// 2026-09-04 - a mask of FILE_GENERIC_READ leaves the ACEs, the count and the
// trustees exactly right and turns the rewrite below red.
func TestRestrictLeavesTheFileReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readable")
	body := []byte("the exact bytes this file holds")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("create the file: %v", err)
	}
	if err := winacl.Restrict(path); err != nil {
		t.Fatalf("restrict: %v", err)
	}

	back, err := os.ReadFile(path) //nolint:gosec // G304: a path this test made
	if err != nil {
		t.Fatalf("the owner cannot read a file restricted to the owner: %v", err)
	}
	if string(back) != string(body) {
		t.Errorf("read back %q, want %q", back, body)
	}

	// Rewriting matters separately: mcpconf renames over this file, which
	// needs DELETE on the source, and FILE_ALL_ACCESS carries it where a
	// read-only mask would not.
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Errorf("the owner cannot rewrite a file restricted to the owner: %v", err)
	}
}

// TestDescribeTellsAnInheritedFileFromANarrowedOne is what `doctor` reports on,
// so it is asserted on a file of each kind rather than on one.
//
// The inherited half is what makes it non-vacuous: a fresh file under the user's
// own temporary directory already carries an ACE for this user, so "there is an
// ACE for me" is true before Restrict runs and cannot distinguish the two.
func TestDescribeTellsAnInheritedFileFromANarrowedOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "described")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("create the file: %v", err)
	}

	before, err := winacl.Describe(path)
	if err != nil {
		t.Fatalf("describe the inherited file: %v", err)
	}
	if before.Protected {
		t.Error("a file nothing has narrowed reports Protected")
	}
	if before.Inherited == 0 {
		t.Error("a file nothing has narrowed reports no inherited ACE, so " +
			"Describe is not reading the flag")
	}

	if err := winacl.Restrict(path); err != nil {
		t.Fatalf("restrict: %v", err)
	}
	after, err := winacl.Describe(path)
	if err != nil {
		t.Fatalf("describe the narrowed file: %v", err)
	}
	if !after.Protected {
		t.Error("a narrowed file does not report Protected")
	}
	if after.Inherited != 0 {
		t.Errorf("a narrowed file reports %d inherited ACEs, want 0", after.Inherited)
	}
	if after.Others != 0 {
		t.Errorf("a narrowed file admits %d principals beyond SYSTEM, "+
			"Administrators and this user, want 0", after.Others)
	}
}

// TestDescribeReportsAMissingFileAsNotExist keeps `doctor` able to tell "this
// file is wide open" from "this file is not there", which are different findings
// with different remedies.
func TestDescribeReportsAMissingFileAsNotExist(t *testing.T) {
	_, err := winacl.Describe(filepath.Join(t.TempDir(), "absent"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("describing an absent file gave %v, want os.ErrNotExist", err)
	}
}

// wantedSIDs is the three trustees Restrict is specified to write, derived the
// same way a reader would check them by hand.
func wantedSIDs(t *testing.T) []string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	out := []string{u.Uid}
	for _, w := range []windows.WELL_KNOWN_SID_TYPE{
		windows.WinLocalSystemSid, windows.WinBuiltinAdministratorsSid,
	} {
		sid, err := windows.CreateWellKnownSid(w)
		if err != nil {
			t.Fatalf("well-known sid %d: %v", w, err)
		}
		out = append(out, sid.String())
	}
	return out
}

// aceSID is the trustee of one allow ACE. The SID begins at SidStart and runs
// to the end of the ACE, which is what makes the pointer arithmetic below the
// documented shape rather than a guess.
func aceSID(ace *windows.ACCESS_ALLOWED_ACE) *windows.SID {
	// G103 is suppressed with a reason rather than because the linter is
	// noisy. x/sys/windows models an ACE as a header, a mask and the first
	// DWORD of a trailing SID, and exposes no accessor for the rest of it, so
	// there is no route to the trustee that is not this one. What makes it
	// bounded: the pointer comes from [windows.GetAce], which the kernel
	// filled for an index below AceCount, and the offset is taken from the
	// field rather than counted in bytes, so it moves if the struct does.
	//nolint:gosec // G103: the documented ACE layout, offset taken from the field
	return (*windows.SID)(unsafe.Add(unsafe.Pointer(ace), unsafe.Offsetof(ace.SidStart)))
}
