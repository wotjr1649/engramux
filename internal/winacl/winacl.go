// Package winacl narrows a file to the principals that have to reach it and
// reports what a file's DACL actually admits.
//
// # Why this is not internal/pipe's security descriptor
//
// That package builds an SDDL string for a named pipe and grants GENERIC_ALL.
// Neither half transfers, and the reason is not the one it looks like.
//
// GENERIC_ALL is 0x10000000 and FILE_ALL_ACCESS is 0x001F01FF, not one bit in
// common - so the obvious worry is that copying the pipe's mask would store a
// number no file request can use. **Measured 2026-09-04: it would not.**
// [windows.ACLFromEntries] wraps SetEntriesInAcl, which maps a generic mask to
// the object type's specific rights before the ACE is built: an entry written
// with GENERIC_READ reads back as 0x00120089, FILE_GENERIC_READ, and one written
// with GENERIC_ALL is indistinguishable from FILE_ALL_ACCESS through this path.
// A mutation swapping the constant here for GENERIC_ALL leaves every test green,
// and that is the mutation being equivalent rather than the test being fake -
// swapping it for GENERIC_READ turns three assertions and the read-back arm red.
//
// The mask is spelled out anyway, and the measurement is why rather than
// decoration: the mapping belongs to SetEntriesInAcl and not to the ACE, so a
// future caller that builds an ACL by hand or writes SDDL gets no mapping and no
// error - just a file its owner cannot open. Two object types want two masks,
// and the safe place to say so is where the mask is written.
//
// The SDDL string is not shared either, and that removes a hazard rather than
// duplicating one. internal/pipe has to reject a SID that is not decimal digits
// because it concatenates one into a string where "(", ")" and ";" delimit an
// ACE. Nothing here concatenates: the SID is parsed by
// [windows.StringToSid] and travels as a structure, so a value that could have
// closed an ACE and opened an allow-everyone one is a parse error instead.
package winacl

import (
	"fmt"
	"os/user"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileAllAccess is FILE_ALL_ACCESS: STANDARD_RIGHTS_REQUIRED, SYNCHRONIZE and
// every file-specific right. x/sys/windows does not define it, and GENERIC_ALL
// is not a substitute for the reason the package comment gives.
const fileAllAccess = 0x001F01FF

// Access is what [Describe] answers about one file's DACL.
//
// It is three counts and a flag rather than the ACEs themselves, because the
// one caller is `engramux doctor`, whose every line goes through a mask: an ACE
// list is account names, machine names and SIDs, which is exactly the shape that
// mask exists to keep out of a pasted diagnostic.
type Access struct {
	// Protected reports whether the DACL blocks inheritance. False means an
	// ACE granted on a parent directory reaches this file.
	Protected bool

	// Inherited is how many of the ACEs came from a parent rather than from
	// this file.
	Inherited int

	// Others is how many ACEs name a principal that is neither SYSTEM,
	// BUILTIN\Administrators nor the user this process runs as.
	//
	// A deny ACE for an outsider counts here too, which over-reports rather
	// than under-reports. Neither host writes one, and a finding that says
	// "look at this" about a file that is in fact narrower is the safe
	// direction to be wrong in.
	Others int
}

// Narrowed reports whether nothing beyond the three principals reaches the file.
func (a Access) Narrowed() bool { return a.Protected && a.Others == 0 }

// Restrict replaces path's DACL with an explicit, protected one granting full
// control to SYSTEM, to BUILTIN\Administrators and to the user this process runs
// as, and to nobody else.
//
// # Why Administrators is on the list when internal/pipe's DACL does not have it
//
// Because this DACL outlives the process, and a pipe's does not. The file it is
// written for is spec 5.9's only documented way to rotate the bearer token -
// delete it, restart, re-run the installer - so a DACL that can lock the owner
// out has removed the remedy along with the exposure. A user SID changes on a
// profile migration or a recreated domain account, and after one the protected
// DACL admits SYSTEM and a principal that no longer exists.
//
// It costs nothing to prevent: an administrator holds SeTakeOwnershipPrivilege
// and SeRestorePrivilege and reaches the file with or without an ACE, so the ACE
// buys a working `icacls` where its absence buys an inconvenience and the same
// access.
//
// What the protection does remove is real and was measured: on the machine spec
// 7.1 recorded, the inherited ACL granted read and execute to a machine-local
// group that another tool had created.
//
// # It is set before the first write, not after
//
// A caller creates the file, calls this, and only then writes. The file exists
// with the directory's inherited ACL for the length of one syscall and is empty
// for all of it, so there is no window in which the token is on disk under an
// ACL this has not replaced.
func Restrict(path string) error {
	sids, err := principals()
	if err != nil {
		return err
	}
	entries := make([]windows.EXPLICIT_ACCESS, 0, len(sids))
	for _, sid := range sids {
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: fileAllAccess,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		})
	}
	dacl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("winacl: build a DACL: %w", err)
	}

	// PROTECTED_DACL_SECURITY_INFORMATION is what sets SE_DACL_PROTECTED.
	// Without it the ACEs above are written and the inherited ones stay,
	// which reads as success and narrows nothing.
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("winacl: narrow %s: %w", path, err)
	}
	return nil
}

// Describe reads path's DACL. A missing file answers an error satisfying
// errors.Is(err, os.ErrNotExist), which is what lets a caller tell a file that
// is not there from one that is wide open.
func Describe(path string) (Access, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return Access{}, fmt.Errorf("winacl: read the DACL of %s: %w", path, err)
	}
	control, _, err := sd.Control()
	if err != nil {
		return Access{}, fmt.Errorf("winacl: read the control bits of %s: %w", path, err)
	}
	out := Access{Protected: control&windows.SE_DACL_PROTECTED != 0}

	dacl, _, err := sd.DACL()
	if err != nil {
		return Access{}, fmt.Errorf("winacl: read the DACL of %s: %w", path, err)
	}
	if dacl == nil {
		// A NULL DACL is not an empty one: it grants everyone
		// everything. Counting zero others would report it as narrow.
		return Access{}, fmt.Errorf("winacl: %s has no DACL, which grants full access to everyone", path)
	}

	known, err := principals()
	if err != nil {
		return Access{}, err
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return Access{}, fmt.Errorf("winacl: read an ACE of %s: %w", path, err)
		}
		if ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			out.Inherited++
		}
		if !slicesContainsSID(known, aceSID(ace)) {
			out.Others++
		}
	}
	return out, nil
}

// principals is the three trustees, in the order [Restrict] writes them.
func principals() ([]*windows.SID, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("winacl: current user: %w", err)
	}
	// StringToSid is the validation. A SID this cannot parse is refused
	// here rather than reaching an ACE.
	self, err := windows.StringToSid(u.Uid)
	if err != nil {
		return nil, fmt.Errorf("winacl: parse the current user's SID: %w", err)
	}
	out := []*windows.SID{self}
	for _, w := range []windows.WELL_KNOWN_SID_TYPE{
		windows.WinLocalSystemSid, windows.WinBuiltinAdministratorsSid,
	} {
		sid, err := windows.CreateWellKnownSid(w)
		if err != nil {
			return nil, fmt.Errorf("winacl: well-known SID %d: %w", w, err)
		}
		out = append(out, sid)
	}
	return out, nil
}

// slicesContainsSID is Equals over a slice. windows.SID has no comparable form,
// so slices.Contains cannot be used on one.
func slicesContainsSID(known []*windows.SID, sid *windows.SID) bool {
	for _, k := range known {
		if k.Equals(sid) {
			return true
		}
	}
	return false
}

// aceSID is the trustee of one ACE. The SID begins at SidStart and runs to the
// end of the ACE - the documented layout of every ACE type that names one, which
// is why the offset is taken from the field rather than counted in bytes.
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
