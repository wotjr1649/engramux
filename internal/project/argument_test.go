package project

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestFromArgumentRefusesWhatIdentifyWouldHaveTakenLiterally is the trust
// boundary spec 5.9 puts around a caller-supplied project path.
//
// [Identify] never fails, because I-04 does not allow an event to be dropped
// for wanting a project, so every shape it cannot walk it takes literally as
// its own root. That is right on the ingest path and wrong for an argument: a
// caller that sent a relative path would get a project nothing was ever
// ingested into, silently and with no error, and a caller that sent a UNC path
// would have the service stat a host that may be down (spec 5.4 leaves one
// connection, so a stall there is the whole service).
//
// So the two shapes are refused here rather than absorbed. The refusals carry
// distinguishable errors, because "not absolute" and "UNC" are different things
// for a caller to fix.
func TestFromArgumentRefusesWhatIdentifyWouldHaveTakenLiterally(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want error
	}{
		{"", ErrNotAbsolute},
		{".", ErrNotAbsolute},
		{"..", ErrNotAbsolute},
		{filepath.Join("nested", "dir"), ErrNotAbsolute},
		// The two Windows shapes that only look absolute: `D:work`
		// names D:'s own current directory and `\work` names the
		// current drive. Both resolve against the service's working
		// directory, which is a long-lived process Task Scheduler
		// started.
		{"D:work", ErrNotAbsolute},
		{`\work`, ErrNotAbsolute},
		// UNC, in both spellings filepath.Clean accepts.
		{`\\host\share\dev`, ErrUNC},
		{"//host/share/dev", ErrUNC},
		{`\\host\share`, ErrUNC},
	} {
		t.Run(strconv.Quote(tc.in), func(t *testing.T) {
			p, err := FromArgument(tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("FromArgument(%q) error = %v, want %v", tc.in, err, tc.want)
			}
			if p != (Project{}) {
				t.Errorf("FromArgument(%q) returned a project alongside its error", tc.in)
			}
		})
	}
}

// TestFromArgumentAgreesWithIdentify is what makes the whole thing usable: a
// path it accepts must resolve to the project ingest stored events under, or a
// scoped read would answer about a project that has nothing in it.
func TestFromArgumentAgreesWithIdentify(t *testing.T) {
	repo := repoAt(t, t.TempDir())
	nested := filepath.Join(repo, "internal", "pkg")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("create %s: %v", nested, err)
	}

	for _, in := range []string{repo, nested, strings.ToUpper(repo), repo + string(filepath.Separator)} {
		got, err := FromArgument(in)
		if err != nil {
			t.Fatalf("FromArgument(%q): %v", in, err)
		}
		if want := Identify(in); got != want {
			t.Errorf("FromArgument(%q) = %+v, Identify gives %+v", in, got, want)
		}
	}
}

// TestTheWalkUpIsBounded holds the bound on how many directories one call may
// stat.
//
// Each step is an os.Lstat on a path the caller chose, and nothing can cancel
// one: [Identify] takes no context. The bound is what turns "as many stats as
// the path is deep" into a fixed number, and it is deliberately far above any
// real repository - the assertion below has to build a tree deeper than any
// checkout to reach it.
//
// Past the bound the walk stops and the path becomes its own root, which is
// exactly what an unrooted path already does. It is not an error: rejecting a
// deep path would refuse a real project, and the divergence needs a worktree
// root more than the bound above the queried directory, which no checkout has.
func TestTheWalkUpIsBounded(t *testing.T) {
	root := repoAt(t, t.TempDir())

	// One directory per step, so the depth below root is exactly the index.
	deep := root
	for i := range maxWalkSteps + 1 {
		deep = filepath.Join(deep, "d"+strconv.Itoa(i))
	}
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatalf("create the deep tree: %v", err)
	}

	// The last directory the walk can still reach: maxWalkSteps steps up
	// from it is root itself.
	inside := filepath.Dir(deep)
	if got, want := mustFromArgument(t, inside).Root, strings.ToLower(root); got != want {
		t.Errorf("a path %d directories below the root resolved to %q, want the root %q",
			maxWalkSteps, got, want)
	}
	// One deeper, and the walk runs out before it arrives.
	if got, want := mustFromArgument(t, deep).Root, strings.ToLower(deep); got != want {
		t.Errorf("a path %d directories below the root resolved to %q, want itself %q",
			maxWalkSteps+1, got, want)
	}
}

func mustFromArgument(t *testing.T, path string) Project {
	t.Helper()
	p, err := FromArgument(path)
	if err != nil {
		t.Fatalf("FromArgument(%q): %v", path, err)
	}
	return p
}
