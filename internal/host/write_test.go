package host

import (
	"encoding/json/jsontext"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// planFor is the plan these tests make: the probe entry, over the two-event
// set, so that what is under test is the writing and not the merge.
func planFor(t *testing.T, path string) *Plan {
	t.Helper()
	plan, err := PlanMerge(path, "test-host", twoEvents, probeEntry)
	if err != nil {
		t.Fatalf("PlanMerge(%s): %v", filepath.Base(path), err)
	}
	return plan
}

func seedRaw(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed %s: %v", filepath.Base(path), err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	//nolint:gosec // G304: a path this test built under t.TempDir.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	return string(b)
}

// leftovers returns every file in dir that is neither the seeded file nor a
// backup of it, which is how a temp file that outlived its write is caught.
func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") || strings.Contains(name, backupInfix) {
			continue
		}
		out = append(out, name)
	}
	return out
}

// TestCommitWritesAndBacksUp is the ordinary path.
func TestCommitWritesAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	const before = "{\"model\":\"opus\"}\n"
	seedRaw(t, path, before)

	plan := planFor(t, path)
	if plan == nil {
		t.Fatal("PlanMerge returned no plan for a file that has no hooks in it yet")
	}
	if read(t, path) != before {
		t.Error("PlanMerge wrote to the file; planning reads and decides, it does not write")
	}

	if _, err := Commit([]*Plan{plan}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	after := read(t, path)
	if !strings.Contains(after, "engramux.exe") {
		t.Errorf("the merged document was not written\ngot:\n%s", after)
	}
	if !strings.Contains(after, `"model": "opus"`) {
		t.Errorf("the document the user had was lost\ngot:\n%s", after)
	}

	// The backup holds what was there before, byte for byte. A backup that
	// holds the NEW bytes is the failure this asserts against, and it is what
	// a copy taken after the write would produce.
	backups, err := filepath.Glob(path + backupInfix + "*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("want exactly one backup beside %s, found %v (%v)", filepath.Base(path), backups, err)
	}
	if got := read(t, backups[0]); got != before {
		t.Errorf("the backup holds the wrong bytes\n got: %q\nwant: %q", got, before)
	}
	if l := leftovers(t, dir); len(l) != 0 {
		t.Errorf("a temporary file outlived the write: %v", l)
	}
}

// TestCommitLeavesNothingWhenOneFileCannotBeRead is backlog 25's second half,
// and the reason planning and writing are two steps.
//
// The previous installer rewrote one host's configuration completely before it
// had so much as parsed the other, so a syntax error in the second left the
// first already changed with only a timestamped backup to recover it. Planning
// both and then writing both cannot make two files atomic - nothing can - but
// it moves every failure that is about READING to before the first byte is
// written.
func TestCommitLeavesNothingWhenOneFileCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "settings.json")
	bad := filepath.Join(dir, "hooks.json")
	const goodBefore = "{\"model\":\"opus\"}\n"
	seedRaw(t, good, goodBefore)
	seedRaw(t, bad, "{ this is not JSON")

	goodPlan := planFor(t, good)

	if _, err := PlanMerge(bad, "test-host", twoEvents, probeEntry); err == nil {
		t.Fatal("PlanMerge accepted a file that is not JSON; the whole point is that it refuses")
	}

	// The caller stops on the first planning error and never reaches Commit,
	// which is what leaves the good file untouched. Asserted on the file
	// rather than on the caller, because that is the property.
	if got := read(t, good); got != goodBefore {
		t.Errorf("the readable file was written even though the other could not be read\n got: %q\nwant: %q",
			got, goodBefore)
	}
	if backups, _ := filepath.Glob(good + backupInfix + "*"); len(backups) != 0 {
		t.Errorf("a backup was taken for a write that never happened: %v", backups)
	}
	_ = goodPlan
}

// TestPlanMergeReportsNothingToDo keeps a re-run from rewriting a file it does
// not need to change, and from leaving a backup for a write that changed
// nothing. A re-run with no rebuild in between is the common case.
func TestPlanMergeReportsNothingToDo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	seedRaw(t, path, "{}\n")

	if _, err := Commit([]*Plan{planFor(t, path)}); err != nil {
		t.Fatalf("first Commit: %v", err)
	}
	installed := read(t, path)

	second, err := PlanMerge(path, "test-host", twoEvents, probeEntry)
	if err != nil {
		t.Fatalf("second PlanMerge: %v", err)
	}
	if second != nil {
		t.Errorf("a second plan over an unchanged file is not nil, so a re-run rewrites it\nplan text:\n%s",
			second.Text)
	}
	if read(t, path) != installed {
		t.Error("the file changed between the two plans")
	}
	if backups, _ := filepath.Glob(path + backupInfix + "*"); len(backups) != 1 {
		t.Errorf("want the one backup from the first write, found %d", len(backups))
	}
}

// TestPlanMergeSkipsAFileThatIsNotThere covers a host that is not installed on
// this machine. It is not an error: a user with only one of the two hosts is an
// ordinary user.
func TestPlanMergeSkipsAFileThatIsNotThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	plan, err := PlanMerge(path, "test-host", twoEvents, probeEntry)
	if err != nil {
		t.Fatalf("PlanMerge over a missing file: %v", err)
	}
	if plan != nil {
		t.Errorf("a missing host configuration produced a plan, so the installer would create one")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("PlanMerge created %s", filepath.Base(path))
	}
}

// TestWriteAtomicRemovesItsTemporaryFileWhenTheRenameFails asserts the failure
// path leaves nothing behind.
//
// It calls [writeAtomic] rather than [Commit], and that is the whole reason it
// works. The first version of this test went through Commit with a directory
// standing in for the destination, and passed against an implementation that
// leaked the temporary file - because Commit backs up first, reading the
// destination fails on a directory, and the write was never reached. It was
// asserting that no temporary file existed in a run that created none. The
// mutation that exposed it was deleting the cleanup, which changed nothing.
func TestWriteAtomicRemovesItsTemporaryFileWhenTheRenameFails(t *testing.T) {
	dir := t.TempDir()
	// A directory as the destination: the temporary file is created beside it
	// and written, and only the rename fails, which is the step under test.
	dest := filepath.Join(dir, "settings.json")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := writeAtomic(dest, []byte("{}\n")); err == nil {
		t.Fatal("writeAtomic renamed a file onto a directory")
	}
	if l := leftovers(t, dir); len(l) != 0 {
		t.Errorf("a temporary file outlived a failed write: %v", l)
	}
}

// TestCommitWritesNothingWhenTheBackupFails keeps the order of the two steps
// observable: the backup is first, so a destination that cannot be read leaves
// the write unattempted rather than half done.
func TestCommitWritesNothingWhenTheBackupFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	seedRaw(t, path, "{}\n")
	plan := planFor(t, path)

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := Commit([]*Plan{plan}); err == nil {
		t.Fatal("Commit succeeded against a destination it could not back up")
	}
	if l := leftovers(t, dir); len(l) != 0 {
		t.Errorf("something was left behind: %v", l)
	}
}

// TestTheEntryPointsAreWhatTheCallerNeeds keeps the exported shape honest: a
// plan carries the bytes it will write, so a caller can report what it is about
// to do without writing it.
func TestTheEntryPointsAreWhatTheCallerNeeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	seedRaw(t, path, "{}\n")

	plan := planFor(t, path)
	if plan.Path != path {
		t.Errorf("Plan.Path = %q, want %q", plan.Path, path)
	}
	if plan.Label != "test-host" {
		t.Errorf("Plan.Label = %q, want %q", plan.Label, "test-host")
	}
	if !jsontext.Value(plan.Text).IsValid() {
		t.Errorf("Plan.Text is not valid JSON:\n%s", plan.Text)
	}
}
