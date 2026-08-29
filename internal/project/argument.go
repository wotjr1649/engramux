package project

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Errors [FromArgument] returns, distinguishable with [errors.Is]. They are
// separate because they are different things for a caller to fix: one is a path
// that has to be spelled absolutely, the other is a location this service will
// not reach at all.
var (
	ErrNotAbsolute = errors.New("project: the path is not absolute")
	ErrUNC         = errors.New("project: a UNC path is refused")
)

// FromArgument resolves a project path that came from a caller rather than from
// a captured payload (spec 5.9).
//
// [Identify] never fails, because I-04 does not allow an event to be dropped
// for wanting a project: every shape it cannot walk becomes its own root. That
// is right on the ingest path and wrong here. An argument is a question, and
// answering a question about a project that does not exist - which is what a
// relative path silently becomes - is worse than refusing it.
//
// # What is refused, and why each one
//
//   - Not absolute. "", ".", a relative path, and the two Windows shapes that
//     only look absolute (`D:work` names D:'s own current directory, `\work`
//     names the current drive) all resolve against the *process's* working
//     directory, and the process is one long-lived service Task Scheduler
//     started. Nothing a caller sends may depend on that.
//   - UNC. The walk is a stat per ancestor and nothing can cancel one, so a
//     path to a host that is down or a share that has gone away parks the walk
//     for as long as Windows takes to give up. Spec 5.4 leaves exactly one
//     connection, so that is not one slow request - it is the service.
//
// A mapped drive letter can stall the same way and is not refused, because it
// is indistinguishable from a local one without a call this package does not
// make. What bounds it is [maxWalkSteps] rather than a rejection.
//
// What it does not do is check that the project exists. A path with no rows
// resolves normally and reads answer nothing, which is the honest shape: a
// project is created by ingest, so "no such project" and "no events yet" are
// the same state.
func FromArgument(path string) (Project, error) {
	clean := filepath.Clean(path)
	// Clean normalises `//host/share` to `\\host\share`, so this is checked
	// after it rather than on the input. VolumeName is what names a UNC
	// share; a `\\` prefix alone would also catch the extended-length and
	// device prefixes, which are a different question nobody has asked.
	if strings.HasPrefix(clean, `\\`) {
		return Project{}, fmt.Errorf("%w: %.128q", ErrUNC, path)
	}
	if !filepath.IsAbs(clean) {
		return Project{}, fmt.Errorf("%w: %.128q", ErrNotAbsolute, path)
	}
	return identify(clean, maxWalkSteps+1), nil
}

// maxWalkSteps bounds how many ancestors [FromArgument] may stat.
//
// Each step is one [os.Lstat] on a path a caller chose, and nothing can cancel
// one - the walk takes no context. Without a bound the number of stats is
// however deep the caller made the path, and spec 5.4 leaves one connection, so
// a request that stalls is the service that stalls.
//
// # Why the bound is here and not in [Identify]
//
// It would look tidier in the shared walk, and it would buy almost nothing
// there. The stall internal/store guards against on the ingest path is a *slow*
// stat - a UNC host that is down, a mapped drive whose share has gone away -
// which costs tens of seconds per level, so a bound of 32 turns an unbounded
// stall into a very long one and fixes nothing. What fixes that is resolving the
// project outside the write transaction, which internal/store does and has a
// test for. Bounding Identify would also change stored project identity for a
// path deeper than the bound below its own repository root, which is a schema
// consequence for no gain.
//
// 32 is far above any checkout: a repository root sits 0 to about 5 directories
// above the working directory a hook reports, and the test that measures this
// bound has to build a tree on purpose to reach it.
//
// Past the bound the walk stops and the path becomes its own root, which is what
// an unrooted path already does. It is not an error: rejecting a deep path would
// refuse a real project, and diverging from the unbounded answer needs a worktree
// root more than 32 directories above the one being asked about.
const maxWalkSteps = 32
