package main

import (
	"os/exec"
	"strings"
	"testing"
)

// bannedDeps are the packages the relay binary must not link, by their module
// root. Both arrive together and from the same place, so naming one would let
// the other stand as evidence that the route reopened.
var bannedDeps = []string{
	"modernc.org/sqlite",
	"github.com/pressly/goose",
}

// suspectRoutes are the packages that reach [bannedDeps]. Naming them in the
// failure turns "something linked SQLite" into "this import did", which is the
// difference between a test that fires and a test somebody can act on.
var suspectRoutes = []string{
	"github.com/wotjr1649/engramux/internal/store",
	"github.com/wotjr1649/engramux/internal/search",
	"github.com/wotjr1649/engramux/internal/inject",
}

// TestTheRelayDoesNotLinkTheSQLiteDriver holds the 1.0 spec §7.1 invariant that
// this repository declared, repeated, and then violated for a month.
//
// # Why a dependency assertion and not a size assertion
//
// The invariant is about what the relay links, and a binary's size is a
// consequence of that plus the build flags - so a size assertion would go red
// on a `-ldflags` change and stay green on a driver arriving in a build that
// happened to strip more. `go list -deps` answers the actual question, is
// unaffected by how the binary is built, and names the package rather than a
// number.
//
// # What went wrong, so that this test's absence is legible
//
// The relay is spawned once per hook event, which is what made its size an
// invariant rather than a preference, and §5.9's rejection of an stdio proxy and
// `doctor`'s rejection of a `net/http` probe at +93.7% are both argued against
// the 3,862,528 B the spec records twice. Measured 2026-09-04 the binary was
// 8,703,488 B: `doctor.go` and `inject.go` imported internal/inject for four
// symbols - a switch file and two spec constants, none of which touches a
// database - and internal/inject reaches internal/search and internal/store,
// which carry the driver. `doctor.go`'s own comment explained that it used
// spool.Dir() rather than importing internal/store *because* that would link the
// driver, two hundred lines below an import that did.
//
// Nothing in this repository could see it. There is no CI, no size budget, and
// a human reading an import block cannot see three packages downstream. This is
// the check that can.
func TestTheRelayDoesNotLinkTheSQLiteDriver(t *testing.T) {
	// The module root, from this package's own directory. The context is
	// the test's own, so a `go list` that hangs is killed when the test
	// deadline passes rather than outliving the run.
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", "./cmd/engramux")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		// Not a skip. This test can only be reached by a toolchain that
		// just compiled it, so a `go` that cannot be run here is a
		// broken environment rather than an absent prerequisite, and a
		// skip would read as a pass in the one place it must not.
		t.Fatalf("go list -deps: %v", err)
	}

	var linked, routes []string
	for _, line := range strings.Split(string(out), "\n") {
		pkg := strings.TrimSpace(line)
		for _, banned := range bannedDeps {
			if pkg == banned || strings.HasPrefix(pkg, banned+"/") {
				linked = append(linked, pkg)
			}
		}
		for _, suspect := range suspectRoutes {
			if pkg == suspect {
				routes = append(routes, pkg)
			}
		}
	}

	if len(linked) > 0 {
		t.Errorf("the relay links %d package(s) it must not, starting with %s.\n"+
			"the route is one of these, which cmd/engramux imports: %s\n"+
			"this binary is spawned once per hook event; 1.0 spec §7.1 records its size at "+
			"3,862,528 B and argues two rejections against that figure",
			len(linked), linked[0], strings.Join(routes, ", "))
	}
}
