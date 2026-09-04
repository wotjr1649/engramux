package host

import (
	"encoding/json/jsontext"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

// TestBackupsAreBoundedAndTheNewestSurvive is backlog 44.
//
// Every [Commit] left a timestamped copy and nothing removed any of them, so a
// machine that had been installed a few dozen times held a few dozen copies of
// ~/.codex/config.toml - a file that holds `Authorization = "Bearer <token>"`,
// in a directory people are asked to attach to bug reports.
//
// # Which three survive is the assertion, and a count alone would not be one
//
// A bound that keeps an arbitrary three passes a count check and destroys the
// thing the copies are for: the newest is the only one that can undo the write
// that just happened. So each write here leaves a copy with distinguishable
// contents, and the test names the three it expects by content.
//
// # The copies are stamped by hand, and that is what makes this about ordering
//
// A Windows file time moves in ticks of about 15.6 ms, so six writes in a loop
// share a modification time exactly and the retention order would fall to
// [savedCopies]' name tie-break - which is not what is under test, and which
// RFC3339Nano's trimmed fractional second makes unreliable at sub-second
// spacing anyway. Real installs are minutes apart. An hour apart is that,
// made exact.
func TestBackupsAreBoundedAndTheNewestSurvive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	seedRaw(t, path, "v0")

	// A neighbour that is not ours. The bound is scoped by backupInfix, and
	// a prune that removes a credential must not be able to remove anything
	// else.
	neighbour := path + ".bak"
	seedRaw(t, neighbour, "not a backup")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const writes = 6
	for i := 1; i <= writes; i++ {
		saved, err := Commit([]*Plan{{
			Path:  path,
			Label: "test-host",
			Text:  fmt.Appendf(nil, "v%d", i),
		}})
		if err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
		if len(saved) != 1 {
			t.Fatalf("commit %d reported %d backups, want 1", i, len(saved))
		}
		when := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(saved[0], when, when); err != nil {
			t.Fatalf("stamp the backup from commit %d: %v", i, err)
		}
	}

	if got := read(t, path); got != "v6" {
		t.Errorf("the destination holds %q, want the last write", got)
	}
	if got := read(t, neighbour); got != "not a backup" {
		t.Errorf("the prune reached a file that is not its own: %q", got)
	}

	found, err := savedCopies(path)
	if err != nil {
		t.Fatalf("list the copies: %v", err)
	}
	var got []string
	for _, c := range found {
		got = append(got, read(t, c.path))
	}
	// A copy taken before write i holds what write i-1 left, so the copy
	// from the last write holds "v5". Newest first.
	want := []string{"v5", "v4", "v3"}
	if !slices.Equal(got, want) {
		t.Fatalf("the surviving copies hold %v, want %v", got, want)
	}

	n, oldest, err := Backups(path)
	if err != nil {
		t.Fatalf("Backups: %v", err)
	}
	if n != backupKeep {
		t.Errorf("Backups counted %d, want %d", n, backupKeep)
	}
	// The fourth write is what left the "v3" copy, so base+4h is the answer
	// - taken from the stamps above rather than from savedCopies, or this
	// would be savedCopies agreeing with itself.
	if want := base.Add(4 * time.Hour); !oldest.Equal(want) {
		t.Errorf("Backups reported the oldest at %s, want %s", oldest.UTC(), want)
	}
}

// TestBackupsCountsNoneWithoutFailing. A destination nobody has written yet has
// no copies beside it, and that is an answer rather than an error - `doctor`
// asks this about a host file on every run.
func TestBackupsCountsNoneWithoutFailing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	seedRaw(t, path, "v0")

	n, oldest, err := Backups(path)
	if err != nil {
		t.Fatalf("Backups: %v", err)
	}
	if n != 0 {
		t.Errorf("Backups counted %d beside a file nothing has replaced, want 0", n)
	}
	if !oldest.IsZero() {
		t.Errorf("Backups reported an oldest of %s for no copies at all", oldest.UTC())
	}
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

// TestCommitStopsAtTheFirstFailureAndSaysWhatItDid holds Commit's own contract,
// which the test that used to stand here did not: that one never called Commit
// at all.
//
// Plans are written in order and the first failure stops the run - and what was
// done before it stopped comes back, because a caller told only "it failed"
// cannot say which files moved.
func TestCommitStopsAtTheFirstFailureAndSaysWhatItDid(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	seedRaw(t, first, "{}\n")
	// The second destination is a directory, so its backup cannot be read and
	// Commit stops there.
	if err := os.Mkdir(second, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	plans := []*Plan{
		{Path: first, Label: "first", Text: []byte(`{"a":1}` + "\n")},
		{Path: second, Label: "second", Text: []byte(`{"b":2}` + "\n")},
	}
	saved, err := Commit(plans)
	if err == nil {
		t.Fatal("Commit succeeded against a destination it could not back up")
	}
	if len(saved) != 1 {
		t.Errorf("Commit reported %d backups, want 1 - the one it took before it stopped: %v", len(saved), saved)
	}
	if got := read(t, first); got != `{"a":1}`+"\n" {
		t.Errorf("the first plan was not written before the second failed: %q", got)
	}
}

// TestPlanMergeRefusesAFileThatIsNotJSON is the read half of the two-file
// property: the failure is at planning time, before Commit is reached at all.
// The orchestration test is what holds that the OTHER file stays untouched -
// this one only holds that the refusal happens while reading.
//
// An earlier version of this test tried to hold both and held neither: it
// called PlanMerge on the bad file, discarded the good plan with a blank
// assignment, and then asserted the good file was unwritten - which no Commit
// implementation could ever have made false, because Commit was never called.
// A review found it.
func TestPlanMergeRefusesAFileThatIsNotJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	seedRaw(t, path, "{ this is not JSON")

	plan, err := PlanMerge(path, "test-host", twoEvents, probeEntry)
	if err == nil {
		t.Fatal("PlanMerge accepted a file that is not JSON; the whole point is that it refuses")
	}
	if plan != nil {
		t.Error("PlanMerge returned a plan alongside its error")
	}
	if got := read(t, path); got != "{ this is not JSON" {
		t.Errorf("PlanMerge wrote to the file it could not read: %q", got)
	}
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
