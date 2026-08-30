// Package ipctest owns the pipe-name override every test suite in this
// repository shares.
//
// The pipe name is derived from the user SID (spec 5.2), so redirecting
// LOCALAPPDATA isolates nothing and two test binaries on one machine would
// otherwise meet on the name a development service is already holding.
// [ipc.TestPipeSIDEnv] is the seam that moves it, and what goes in it has to be
// unique to the test AND to the process: the test name alone collides between
// two copies of one binary, and the process id alone collides between two tests
// in one binary.
//
// It lives here because that value was spelled out six times across five
// packages, and a format only one of them changed is a relay and a service
// landing on different names - which loses nothing (the relay spools and the
// drain is by directory) but makes a test that expected the pipe fail for a
// reason no assertion names. AGENTS.md carries the row.
//
// Nothing shipped imports this package. That is deliberate and it is what
// `go list -deps` is for: [Use] calls t.Setenv, which is process-wide, so a
// test that moves the name cannot be parallel and cannot have a parallel
// ancestor.
package ipctest

import (
	"os"
	"strconv"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// SID returns the value that stands in for the user SID in this test's derived
// pipe name. It is unique to t and to this process.
func SID(t testing.TB) string {
	t.Helper()
	return "engramux-test-" + strconv.Itoa(os.Getpid()) + "-" + t.Name()
}

// Use points [ipc.CurrentPipeName] - and so every listener and dial derived
// from it, in this process and in every child that inherits the environment -
// at [SID].
func Use(t testing.TB) {
	t.Helper()
	t.Setenv(ipc.TestPipeSIDEnv, SID(t))
}
