// Package project decides when two paths on disk are the same project.
//
// Spec 6 fixes the meaning and leaves the mechanism here: "Identity means: same
// repository, same worktree. It must survive drive-letter case and
// trailing-separator differences. The hash inputs are the code's choice; the
// meaning is not."
//
// A project is a row, not a process (I-01), and nothing in this package runs
// one: the worktree root is found by walking up for a .git entry, not by
// spawning git. That keeps a subprocess off the ingest critical path, and it
// removes an I-02 obligation - every child this project spawns has to carry the
// no-window creation flag, and a child that does not exist cannot get it wrong.
//
// The paths are Windows paths, and so are the rules: [path/filepath]'s
// separator and volume handling is what makes `D:/work/repo` and
// `D:\work\repo\` the same string, and case folding is only defensible because
// NTFS is case-insensitive.
package project

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// Project is one row of the projects table, less created_at.
type Project struct {
	// ID is the primary key: half a SHA-256 of Root, hex encoded. It is
	// derived rather than stored raw because ids travel - into logs, into
	// MCP responses - and an absolute path carries the user's name.
	ID string
	// Root is the normalised worktree root, and what projects.root holds.
	// The column is UNIQUE, so this string is the identity: two spellings
	// of one worktree that normalise differently do not merely get
	// different ids, they collide on the constraint.
	Root string
	// Name is Root's last element, for display. It is folded like the rest
	// of Root, so it is lower case.
	Name string
}

// Identify resolves path - a session's working directory, or anything inside it
// - to the project it belongs to. It never fails: a path in no repository is
// still a project, because I-04 does not allow an event to be dropped for
// wanting one.
//
// # A non-absolute path is taken literally
//
// Only an absolute path is walked. Every other shape - "", ".", a relative
// path, and the two Windows shapes that only look absolute (`D:work` names D:'s
// own current directory, `\work` names the current drive) - resolves against
// the *process's* working directory, and the process is one long-lived service
// started by Task Scheduler. Walking those would make a payload's project
// identity depend on where the service was launched from, and could attribute
// an event to a real repository it has nothing to do with. So they are cleaned,
// folded and returned as their own root, which no absolute path can collide
// with.
//
// Nothing in the 900-capture corpus produces one: every `cwd` observed is
// absolute. This is a guard on a trust boundary, not a path with traffic.
//
// # What the normalisation folds
//
// Everything [filepath.Clean] folds - trailing separators, forward slashes,
// "." and ".." elements - and then the case of the **whole path**, not the
// drive letter alone.
//
// Folding only the drive letter would satisfy spec 6 as written and still split
// one worktree in two: `D:\Work\Repo` and `D:\work\repo` are the same directory
// on NTFS, and the walk below reaches the same .git from either, so an identity
// that keeps them apart contradicts "same worktree".
//
// ponytail: the ceiling is a directory with per-directory case sensitivity
// enabled (`fsutil file setCaseSensitiveInfo`, which WSL sets), where two
// genuinely distinct directories fold together. The upgrade path is asking
// Windows for the canonical spelling - GetFinalPathNameByHandle - which costs a
// handle open per event on the ingest path, so it waits for a real report.
//
// What it does not fold: symlinks and junctions, 8.3 short names, and
// substituted drives. Each of those is a different string for the same
// directory and this package treats them as different projects.
func Identify(path string) Project {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		clean = worktreeRoot(clean)
	}
	root := strings.ToLower(clean)
	sum := sha256.Sum256([]byte(root))
	return Project{
		ID:   hex.EncodeToString(sum[:16]),
		Root: root,
		Name: filepath.Base(root),
	}
}

// worktreeRoot walks up from dir and returns the first directory holding a
// .git entry, or dir unchanged when there is none above it.
//
// A .git **file** counts as much as a .git directory. That is how git marks a
// linked worktree - the file holds a `gitdir:` line pointing back at the
// repository it was created from - and the line is deliberately not read: the
// worktree is its own project. Accepting only the directory case silently
// merges every linked worktree into its parent repository, or, when the
// worktree lives outside the repository, into "no project at all".
//
// [os.Lstat] rather than [os.Stat]: the marker's existence is the whole
// question, and a .git symlink that dangles still marks a root.
func worktreeRoot(dir string) string {
	for cur := dir; ; {
		if _, err := os.Lstat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return dir
		}
		cur = parent
	}
}
