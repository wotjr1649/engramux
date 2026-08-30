package host

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// errSharingViolation is Windows' ERROR_SHARING_VIOLATION, which is what an
// open-for-write against a mapped image answers.
//
// It is written as a number because neither `syscall` nor anything already in
// this module's require list names it, and promoting golang.org/x/sys to a
// direct dependency for one constant buys nothing.
//
// **[verified] on this platform**, by opening the running test binary for write
// and a 0400 file for write in the same run: the image answers
// ERROR_SHARING_VIOLATION and the read-only file answers ERROR_ACCESS_DENIED.
// Worth knowing because `errors.Is(err, fs.ErrPermission)` is **false** for the
// first, so the idiomatic check does not tell the two apart.
// TestPlanCopiesTellsAPermissionProblemFromALock holds both ends.
const errSharingViolation = syscall.Errno(32)

// Role is which of the two binaries a destination is, and it exists because the
// advice for a locked one differs entirely.
type Role int

const (
	// Relay is the per-event binary. It lives for as long as one hook takes,
	// so a lock on it clears on its own - and stopping the service is the
	// wrong advice, because the service is not what holds it.
	Relay Role = iota
	// Service is the resident binary. A lock on it almost always means it is
	// running, and the user has to stop it.
	Service
)

func (r Role) String() string {
	if r == Service {
		return "service"
	}
	return "relay"
}

// Binary is one file to install, by name in both directories.
type Binary struct {
	Name string
	Role Role
}

// CopyPlan is one copy to make.
type CopyPlan struct{ Src, Dest string }

// LockedError is a destination that exists and cannot be written.
//
// It is a type rather than a formatted string because the caller needs the
// classification, not only the prose: MappedImage decides whether waiting is
// advice that can come true, and Role decides whether stopping the service is
// advice at all.
type LockedError struct {
	Path string
	Role Role
	Err  error
	// MappedImage is true when the open failed because something has the file
	// mapped as a running image, and false when it failed for any other
	// reason - a read-only attribute, or an ACL.
	MappedImage bool
}

func (e *LockedError) Unwrap() error { return e.Err }

func (e *LockedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "cannot write %s: %v\n", e.Path, e.Err)

	// Nothing here looks for a process, so nothing here may claim one is
	// running. The error is the only measured fact; which file it is decides
	// which cause is likelier.
	if e.MappedImage {
		b.WriteString("that is a sharing violation: another process has that image mapped, " +
			"and Windows locks a mapped image against writes.\n")
	} else {
		b.WriteString("that is not a lock: the file is read-only, or an ACL denies the write. " +
			"no delay clears it.\n")
	}

	switch {
	case e.Role == Service && e.MappedImage:
		b.WriteString("for this file the usual cause is that the engramux service is running.\n")
		b.WriteString("stop it yourself, then run this again. run the line in cmd or PowerShell -\n")
		b.WriteString("Git Bash rewrites /end and /tn into paths and the command fails there:\n")
		b.WriteString("  schtasks /end /tn \"\\Engramux\"        # registered to start at logon\n")
		b.WriteString("  taskkill /f /im engramux-service.exe   # started by hand\n")
		b.WriteString("stopping it loses nothing: it is a hard kill, so the WAL keeps whatever was\n")
		b.WriteString("committed but not checkpointed, and the next start recovers from it.\n")
	case e.Role == Service:
		b.WriteString("check this file's read-only attribute, its ACL, and whether antivirus has\n")
		b.WriteString("quarantined it. stopping the service will not help.\n")
	case e.MappedImage:
		b.WriteString("for this file that is a hook firing right now - the relay runs only for as\n")
		b.WriteString("long as one event takes. try again in a moment.\n")
		b.WriteString("do not stop the service for this one; the service is not what holds it.\n")
	default:
		b.WriteString("check this file's read-only attribute, its ACL, and whether antivirus has\n")
		b.WriteString("quarantined it. do not stop the service; it is not what holds this file.\n")
	}
	b.WriteString("nothing was copied.")
	return b.String()
}

// PlanCopies decides which binaries need copying, and refuses before the first
// copy when a destination that already exists cannot be written.
//
// # What the refusal buys, at its real size
//
// Windows locks the image of a running process against writes, and the service
// is meant to be resident - so on an installed machine the one destination that
// normally cannot be written is one this has to overwrite. Both are therefore
// decided before either is copied. Copying the relay and only then failing on
// the service is what leaves a new relay, an old service and no hook
// configuration at all: the state that makes this confusing rather than merely
// annoying.
//
// The guarantee is exactly one thing: **no half-install from a lock or a
// permission failure on a destination that already exists.** A destination that
// does not exist yet is never probed, and nothing here can stop the service
// being started, or a scanner grabbing the file, between the probe and the copy.
//
// Identical bytes are not copied at all. A re-run with no rebuild in between is
// the common case, and rewriting a file with the bytes it already holds is
// still a write, which the lock still refuses.
//
// probe is false for a dry run, and that is not laziness. The probe asks for
// the same write handle a copy would ask for, so probing during a dry run would
// make a dry run fail every time the service is up - which is the state a dry
// run is most often used in.
func PlanCopies(srcDir, destDir string, bins []Binary, probe bool) (plan []CopyPlan, unchanged []string, err error) {
	for _, b := range bins {
		src := filepath.Join(srcDir, b.Name)
		dest := filepath.Join(destDir, b.Name)

		//nolint:gosec // G304: a path built from the caller's own directories; see the note in write.go.
		want, err := os.ReadFile(src)
		if err != nil {
			return nil, nil, fmt.Errorf("host: %s is missing from %s: %w", b.Name, srcDir, err)
		}

		//nolint:gosec // G304: see above.
		have, readErr := os.ReadFile(dest)
		switch {
		case readErr == nil && bytes.Equal(have, want):
			unchanged = append(unchanged, dest)
			continue
		case readErr != nil && !os.IsNotExist(readErr):
			return nil, nil, fmt.Errorf("host: read %s: %w", dest, readErr)
		case readErr == nil && probe:
			if err := probeWritable(dest, b.Role); err != nil {
				return nil, nil, err
			}
		}
		plan = append(plan, CopyPlan{Src: src, Dest: dest})
	}
	return plan, unchanged, nil
}

// probeWritable asks for the write handle a copy would ask for, and asks
// without truncating anything: O_RDWR rather than a trial copy.
func probeWritable(dest string, role Role) error {
	//nolint:gosec // G304: see the note in write.go.
	f, err := os.OpenFile(dest, os.O_RDWR, 0)
	if err == nil {
		return f.Close()
	}
	return &LockedError{
		Path:        dest,
		Role:        role,
		Err:         err,
		MappedImage: errors.Is(err, errSharingViolation),
	}
}
