package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoAt creates dir/.git as a *directory*, which is the shape an ordinary
// clone has. Every test that cares where the walk stops creates one, so the
// answer does not depend on whether some ancestor of the machine's temp
// directory happens to be a repository.
func repoAt(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Join(dir, ".git"), err)
	}
	return dir
}

// linkedWorktreeAt creates dir/.git as a *file* holding a `gitdir:` line, which
// is the shape git gives a linked worktree. No real git is involved: the file
// shape is the whole signal, and treating only the directory case as a worktree
// root is what this exists to catch.
func linkedWorktreeAt(t *testing.T, dir, gitdir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	marker := filepath.Join(dir, ".git")
	if err := os.WriteFile(marker, []byte("gitdir: "+gitdir+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", marker, err)
	}
	return dir
}

// TestIdentifyFoldsCaseAndTrailingSeparator is spec 6's requirement that
// identity survives drive-letter case and trailing-separator differences, plus
// the boundary this package picked beyond it: the fold is over the *whole*
// path, not the drive letter alone (see [Identify]). "whole path upper" is that
// boundary, and it fails if the fold is narrowed back to the volume name.
func TestIdentifyFoldsCaseAndTrailingSeparator(t *testing.T) {
	repo := repoAt(t, t.TempDir())
	vol := filepath.VolumeName(repo)
	rest := repo[len(vol):]

	want := Identify(repo)
	if want.Root != strings.ToLower(repo) {
		t.Fatalf("Identify(%q).Root = %q, want %q", repo, want.Root, strings.ToLower(repo))
	}
	if want.ID == "" {
		t.Fatalf("Identify(%q).ID is empty", repo)
	}

	spellings := map[string]string{
		"lower drive letter": strings.ToLower(vol) + rest,
		"upper drive letter": strings.ToUpper(vol) + rest,
		"trailing separator": repo + string(filepath.Separator),
		"forward slashes":    filepath.ToSlash(repo),
		"whole path upper":   strings.ToUpper(repo),
	}
	for name, spelling := range spellings {
		if got := Identify(spelling); got != want {
			t.Errorf("%s: Identify(%q) = %+v, want %+v", name, spelling, got, want)
		}
	}
}

// TestNestedPathResolvesToTheWorktreeRoot: an event fired from deep inside a
// worktree belongs to the same project as one fired from its root.
func TestNestedPathResolvesToTheWorktreeRoot(t *testing.T) {
	repo := repoAt(t, t.TempDir())
	nested := filepath.Join(repo, "internal", "store")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("create %s: %v", nested, err)
	}

	want := Identify(repo)
	if got := Identify(nested); got != want {
		t.Fatalf("Identify(%q) = %+v, want %+v", nested, got, want)
	}
}

// TestLinkedWorktreeResolvesToItself: a linked worktree's .git is a file
// pointing at the repository it was created from, and the worktree is still its
// own project. The worktree is placed *inside* the parent repository, so a
// walk that accepts only a .git directory finds the parent's and merges the two.
func TestLinkedWorktreeResolvesToItself(t *testing.T) {
	repo := repoAt(t, t.TempDir())
	worktree := linkedWorktreeAt(t,
		filepath.Join(repo, "wt"),
		filepath.Join(repo, ".git", "worktrees", "wt"))

	got := Identify(worktree)
	if got.Root != strings.ToLower(worktree) {
		t.Fatalf("Identify(%q).Root = %q, want %q", worktree, got.Root, strings.ToLower(worktree))
	}
	if parent := Identify(repo); got.ID == parent.ID {
		t.Fatalf("linked worktree and its parent repository share id %q", got.ID)
	}

	inside := filepath.Join(worktree, "cmd")
	if err := os.MkdirAll(inside, 0o750); err != nil {
		t.Fatalf("create %s: %v", inside, err)
	}
	if nested := Identify(inside); nested != got {
		t.Fatalf("Identify(%q) = %+v, want %+v", inside, nested, got)
	}
}

// TestNonRepositoryPathGetsItsOwnProject: a path in no repository is still a
// project (I-04 - an event is never dropped), it is its own root, and two of
// them do not collapse into one.
func TestNonRepositoryPathGetsItsOwnProject(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "alpha")
	b := filepath.Join(base, "beta")
	for _, dir := range []string{a, b} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	gotA, gotB := Identify(a), Identify(b)
	// If either of these fails with an ancestor directory as Root, something
	// above the machine's temp directory is a git repository.
	if gotA.Root != strings.ToLower(a) {
		t.Fatalf("Identify(%q).Root = %q, want %q", a, gotA.Root, strings.ToLower(a))
	}
	if gotB.Root != strings.ToLower(b) {
		t.Fatalf("Identify(%q).Root = %q, want %q", b, gotB.Root, strings.ToLower(b))
	}
	if gotA.ID == gotB.ID {
		t.Fatalf("%q and %q share id %q", a, b, gotA.ID)
	}
	if again := Identify(a); again != gotA {
		t.Fatalf("Identify(%q) = %+v on the second call, want %+v", a, again, gotA)
	}
}

// TestNameIsTheRootsLastElement pins the value that reaches projects.name,
// which is NOT NULL and has nothing else to fill it.
func TestNameIsTheRootsLastElement(t *testing.T) {
	repo := repoAt(t, filepath.Join(t.TempDir(), "Engramux"))
	if got := Identify(repo); got.Name != "engramux" {
		t.Fatalf("Identify(%q).Name = %q, want %q", repo, got.Name, "engramux")
	}
}

// TestNonAbsolutePathIsNeverWalked. The walk resolves relative elements against
// the *process's* working directory, and the process here is one long-lived
// service started by Task Scheduler. An unguarded walk would therefore give one
// payload two different project identities depending on where the service was
// launched from - and, worse, could attribute it to a real repository it has
// nothing to do with.
//
// t.Chdir puts the process inside a real repository, which is exactly the
// situation an unguarded walk absorbs silently. Every case below must come back
// as the cleaned, folded input and nothing else.
//
// The last two are the reason [filepath.IsAbs] is the test rather than "starts
// with a drive letter" or "starts with a separator": `D:work` names D:'s own
// current directory and `\work` names the current drive, so both are process
// state wearing an absolute path's clothes.
func TestNonAbsolutePathIsNeverWalked(t *testing.T) {
	t.Chdir(repoAt(t, filepath.Join(t.TempDir(), "launched-here")))

	for _, tc := range []struct{ in, want string }{
		{"", "."},
		{".", "."},
		{"..", ".."},
		{filepath.Join("nested", "dir"), filepath.Join("nested", "dir")},
		{"D:work", "d:work"},
		{`\work`, `\work`},
	} {
		want := strings.ToLower(tc.want)
		got := Identify(tc.in)
		if got.Root != want {
			t.Fatalf("Identify(%q).Root = %q, want %q", tc.in, got.Root, want)
		}
	}
}
