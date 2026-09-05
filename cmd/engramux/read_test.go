package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSessionsScopeDefaultsToEveryProject is backlog 47's decision at this
// surface: `engramux sessions` with no argument asks about every project, the
// way `engramux search` with no `--project` does, and the argument is what
// narrows it.
//
// The default is asserted as the empty string and not as "not the working
// directory", because those are not the same claim: a test run from a directory
// that is not a project root would pass the weaker one against the old
// behaviour. Empty is the wire's own word for every project
// ([ipc.ListSessionsRequest]).
//
// The narrowed path is asserted as absolute rather than as an exact string, for
// the reason [TestSearchScopeReadsTheFlagOnlyInFirstPosition] gives: the
// directory a test runs in is not this test's to know, and what the service
// requires is that the path is absolute at all.
func TestSessionsScopeDefaultsToEveryProject(t *testing.T) {
	t.Run("no argument", func(t *testing.T) {
		root, err := sessionsScope(nil)
		if err != nil {
			t.Fatalf("sessionsScope: %v", err)
		}
		if root != "" {
			t.Errorf("project = %q, want empty - the default is every project", root)
		}
	})

	t.Run("absolute", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "repo")
		root, err := sessionsScope([]string{abs})
		if err != nil {
			t.Fatalf("sessionsScope: %v", err)
		}
		if root != abs {
			t.Errorf("project = %q, want %q", root, abs)
		}
	})

	t.Run("relative", func(t *testing.T) {
		rel := filepath.Join("nested", "dir")
		root, err := sessionsScope([]string{rel})
		if err != nil {
			t.Fatalf("sessionsScope: %v", err)
		}
		if !filepath.IsAbs(root) {
			t.Errorf("project = %q, want an absolute path - the service refuses a relative one", root)
		}
		if !strings.HasSuffix(root, rel) {
			t.Errorf("project = %q, want it to end in the path that was given", root)
		}
	})
}
