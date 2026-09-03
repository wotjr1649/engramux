// Package version answers what build this is, which is a different question
// from what wire protocol it speaks.
//
// [ipc.Version] is a wire protocol version with exactly one consumer, the ack
// check, and it moves on a compatibility event. This one is the product's, it
// is semantic, and it moves on a release. Coupling them would raise the wire
// version because a document changed, and every relay and service pair a user
// had not restarted together would stop meeting (memory spec, M-7).
package version

import (
	"runtime/debug"
	"strings"
)

// linked is the release version, set at link time:
//
//	go build -ldflags "-X github.com/wotjr1649/engramux/internal/version.linked=0.1.0"
//
// # It is a var and not a const, and that is load-bearing
//
// `go tool link -X` sets a package-level string **variable**. A const, a
// mistyped import path, or an initializer that is not a plain constant all
// no-op with **no linker error at all** - and the obvious thing to copy here,
// ipc.Version, is a const. Nothing in this repository can catch a flag that
// silently did nothing: there is no CI, and a test cannot see release ldflags.
//
// What does catch it is the binary itself, and it is not `go version -m`.
// That does print the ldflags a build used, through `-s -w`, because build
// information is not a symbol table - but **measured 2026-09-04, `-trimpath`
// removes the `-ldflags` line from the recorded settings entirely.** Two builds
// of the same package, one with `-trimpath` and one without: the second records
// its `-X` and the first records `-trimpath=true` and no ldflags at all. The
// release line carries `-trimpath`, because that is what makes an artefact
// attributable to a commit, so that is the one build where this diagnostic is
// blind. What survives it is the vcs stamping [Product] falls back to.
//
// So: run the binary. `engramux doctor` prints this value, and reading it is
// the only check that cannot be satisfied by the command line you meant to type.
var linked string

// devPrefix is what a build with no release version calls itself. Semantic
// versioning's own rule: 0.0.0 sorts below every release, so a development
// binary never compares as newer than one.
const devPrefix = "0.0.0-dev"

// shaLen is how much of the revision a development version carries.
//
// Twelve, and the reason is not readability. `doctor` masks every line it
// prints through secret.MaskString, and ClassOpaque matches forty or more
// characters of base64 alphabet - which a forty-character git SHA is. A full
// revision would be reported as a redacted secret by the one command that
// exists to report it.
const shaLen = 12

// Product is the version of this build.
//
// A release build carries [linked]. Every other build derives one from the
// build information Go stamps into the binary itself, so a development binary
// still identifies the commit it came from rather than answering "dev" and
// leaving a reader to guess. An unstamped build - `go run`, or a source tree
// that is not a repository - answers the bare development prefix.
func Product() string {
	if linked != "" {
		return linked
	}
	rev, dirty := vcs()
	switch {
	case rev == "" && !dirty:
		return devPrefix
	case rev == "":
		return devPrefix + "+dirty"
	case dirty:
		return devPrefix + "+" + rev + ".dirty"
	default:
		return devPrefix + "+" + rev
	}
}

// vcs reads the revision and the dirty flag out of this binary's own build
// information, returning empty values when it carries none.
func vcs() (rev string, dirty bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) > shaLen {
		rev = rev[:shaLen]
	}
	return strings.ToLower(rev), dirty
}
