package host

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// binaries is the pair every test here plans, named so that the role attached
// to each is what decides the advice rather than the file name.
var binaries = []Binary{
	{Name: "engramux.exe", Role: Relay},
	{Name: "engramux-service.exe", Role: Service},
}

func seedBin(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	return path
}

// TestPlanCopiesSkipsIdenticalBytes is the common case: a re-run with no
// rebuild in between. Rewriting a file with the bytes it already holds is still
// a write, and Windows refuses a write to a mapped image, so a copy that "does
// nothing" would fail on a machine where the service is up.
func TestPlanCopiesSkipsIdenticalBytes(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	for _, b := range binaries {
		seedBin(t, src, b.Name, "same bytes")
		seedBin(t, dest, b.Name, "same bytes")
	}

	plan, unchanged, err := PlanCopies(src, dest, binaries, true)
	if err != nil {
		t.Fatalf("PlanCopies: %v", err)
	}
	if len(plan) != 0 {
		t.Errorf("planned %d copies for identical files, want 0: %v", len(plan), plan)
	}
	if len(unchanged) != len(binaries) {
		t.Errorf("reported %d unchanged, want %d", len(unchanged), len(binaries))
	}
}

// TestPlanCopiesPlansWhatChanged covers both reasons a copy is needed: the
// bytes differ, and the destination is not there at all.
func TestPlanCopiesPlansWhatChanged(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	seedBin(t, src, binaries[0].Name, "new")
	seedBin(t, dest, binaries[0].Name, "old")
	seedBin(t, src, binaries[1].Name, "new") // no destination at all

	plan, unchanged, err := PlanCopies(src, dest, binaries, true)
	if err != nil {
		t.Fatalf("PlanCopies: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("planned %d copies, want 2: %v", len(plan), plan)
	}
	if len(unchanged) != 0 {
		t.Errorf("reported %v unchanged, want none", unchanged)
	}
	for i, b := range binaries {
		if plan[i].Src != filepath.Join(src, b.Name) || plan[i].Dest != filepath.Join(dest, b.Name) {
			t.Errorf("plan[%d] = %+v, want %s -> %s", i, plan[i],
				filepath.Join(src, b.Name), filepath.Join(dest, b.Name))
		}
	}
}

// TestPlanCopiesNamesAMissingSource fails before anything else, because a
// distribution missing one of its two binaries is not something to half-install.
func TestPlanCopiesNamesAMissingSource(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	seedBin(t, src, binaries[0].Name, "present")

	_, _, err := PlanCopies(src, dest, binaries, true)
	if err == nil {
		t.Fatal("PlanCopies accepted a source directory missing a binary")
	}
	if !strings.Contains(err.Error(), binaries[1].Name) {
		t.Errorf("the error does not name the missing file: %v", err)
	}
	if !strings.Contains(err.Error(), src) {
		t.Errorf("the error does not say where it looked: %v", err)
	}
}

// TestPlanCopiesRefusesBeforeTheFirstCopyWhenADestinationIsAMappedImage is the
// property the whole probe exists for, and it uses a real mapped image: the
// test binary itself, which Windows holds open against writes for as long as
// this test runs.
//
// The relay is planned first and its destination is writable, so an
// implementation that copied as it planned would already have written it by the
// time it reached the service. The assertion is that the returned plan is empty:
// nothing to copy, decided before the first copy.
func TestPlanCopiesRefusesBeforeTheFirstCopyWhenADestinationIsAMappedImage(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	dest := filepath.Dir(self)
	src := t.TempDir()

	// The destination directory is the test binary's own, so the second
	// binary's destination is a running image. The first is a name that is not
	// there, which is why it would be copied.
	bins := []Binary{
		{Name: "engramux-not-there.exe", Role: Relay},
		{Name: filepath.Base(self), Role: Service},
	}
	seedBin(t, src, bins[0].Name, "new")
	seedBin(t, src, bins[1].Name, "different from the running image")

	plan, _, err := PlanCopies(src, dest, bins, true)
	if err == nil {
		t.Fatal("PlanCopies did not refuse a destination that is a running image")
	}
	if len(plan) != 0 {
		t.Errorf("a refused run returned %d copies; nothing may be copied: %v", len(plan), plan)
	}

	var locked *LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("error is %T, want a *LockedError the caller can format: %v", err, err)
	}
	if !locked.MappedImage {
		t.Errorf("a running image was not classified as one: %v", locked.Err)
	}
	if locked.Role != Service {
		t.Errorf("Role = %v, want Service - the advice differs by role", locked.Role)
	}

	// The two halves of the advice, which are the reason this error is a type
	// and not a string: what happened, and what to do about it.
	msg := locked.Error()
	for _, want := range []string{"another process", "schtasks", "taskkill"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the diagnosis does not mention %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "/end /tn") && !strings.Contains(msg, "Git Bash") {
		t.Error("the message gives a schtasks line without the warning that Git Bash rewrites it")
	}
}

// TestPlanCopiesTellsAPermissionProblemFromALock is the distinction the advice
// turns on. Measured on this platform: a running image answers
// ERROR_SHARING_VIOLATION and a read-only file answers ERROR_ACCESS_DENIED, and
// errors.Is against fs.ErrPermission is false for the first - so the idiomatic
// check would not tell them apart.
func TestPlanCopiesTellsAPermissionProblemFromALock(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	bins := []Binary{{Name: "engramux.exe", Role: Relay}}
	seedBin(t, src, bins[0].Name, "new")
	readOnly := seedBin(t, dest, bins[0].Name, "old")
	if err := os.Chmod(readOnly, 0o400); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o600) })

	_, _, err := PlanCopies(src, dest, bins, true)
	if err == nil {
		t.Fatal("PlanCopies accepted a read-only destination")
	}
	var locked *LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("error is %T, want *LockedError: %v", err, err)
	}
	if locked.MappedImage {
		t.Errorf("a read-only file was classified as a mapped image: %v", locked.Err)
	}

	msg := locked.Error()
	if strings.Contains(msg, "wait") {
		t.Errorf("a permission problem was given advice to wait, which can never come true:\n%s", msg)
	}
	if !strings.Contains(msg, "read-only") && !strings.Contains(msg, "ACL") {
		t.Errorf("the diagnosis does not name the two causes it could be:\n%s", msg)
	}
}

// TestPlanCopiesDoesNotProbeWhenItIsNotGoingToCopy keeps a dry run useful on an
// installed machine. Probing asks for the write handle a copy would ask for, so
// probing during a dry run would make `install` without `--apply` fail every
// time the service is up - which is the state a dry run is most often used in.
func TestPlanCopiesDoesNotProbeWhenItIsNotGoingToCopy(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	src := t.TempDir()
	bins := []Binary{{Name: filepath.Base(self), Role: Service}}
	seedBin(t, src, bins[0].Name, "different from the running image")

	plan, _, err := PlanCopies(src, filepath.Dir(self), bins, false)
	if err != nil {
		t.Fatalf("a dry run refused a locked destination: %v", err)
	}
	if len(plan) != 1 {
		t.Errorf("a dry run planned %d copies, want 1 - it still says what it would do", len(plan))
	}
}
