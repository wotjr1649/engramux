package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Where a bare `update` reads from, now that there is a delivery channel.
//
// # Why this changed
//
// The plugin is the channel: Claude Code fetches the release archive, checks it
// against the SHA-256 the marketplace entry names, and unpacks it into its
// cache. Before this, `engramux update` still had to be handed that directory
// by hand - so `/plugin update` moved bytes into a place nothing read, and the
// service kept running the build it already had. The owner's words for it were
// that there is then no reason to update the plugin at all, and they are right:
// a delivery channel that delivers somewhere nothing looks is not one.
//
// `newestPlugin` already knew how to find it, and already refused a version
// directory that does not carry both binaries - Claude Code keeps an old
// version for about a fortnight, so a partially removed one must not be
// offered. What was missing was the wiring.
//
// # What is deliberately not here
//
// There is no downgrade guard. `host.PlanCopies` compares bytes, so installing
// the same version is already a no-op that prints "nothing to replace", and an
// older cache version is visible in the line this prints before it acts and
// recoverable with an explicit `--from`. Guarding it properly means asking the
// running service its version over IPC before stopping it, which is more
// machinery than a visible, reversible copy warrants.
// ponytail: no version comparison, add the IPC one if a real downgrade bites.
func TestABareUpdateTakesTheNewestPluginInTheCache(t *testing.T) {
	cache := t.TempDir()
	plantPlugin(t, cache, "engramux", "0.1.0", bothBinaries()...)
	want := plantPlugin(t, cache, "engramux", "0.2.0", bothBinaries()...)

	dir, note, ok := updateFrom("", cache)
	if !ok {
		t.Fatal("a populated plugin cache was not read as a source")
	}
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	// The version has to be in what the user sees, because it is the only
	// thing that makes an unintended downgrade visible before it happens.
	if !strings.Contains(note, "0.2.0") {
		t.Errorf("the note does not name the version it chose:\n%s", note)
	}
}

// TestAnExplicitFromOutranksTheCache. The owner's own loop is building into
// `dist/` and updating from it, which is routinely *newer* than the released
// plugin. A resolver that preferred the cache would undo that build every time.
func TestAnExplicitFromOutranksTheCache(t *testing.T) {
	cache := t.TempDir()
	plantPlugin(t, cache, "engramux", "9.9.9", bothBinaries()...)

	dir, note, ok := updateFrom(`D:\build`, cache)
	if !ok {
		t.Fatal("an explicit --from was refused")
	}
	if dir != `D:\build` {
		t.Errorf("dir = %q, want %q", dir, `D:\build`)
	}
	if note != "" {
		t.Errorf("an explicit --from should say nothing extra; it said %q", note)
	}
}

// TestNoFromAndNoPluginIsRefused. The state of a machine that installed by hand
// and never added the plugin. It has to be a refusal rather than a copy from
// somewhere invented - the empty string as a source plans a copy out of this
// process's working directory.
func TestNoFromAndNoPluginIsRefused(t *testing.T) {
	if dir, _, ok := updateFrom("", t.TempDir()); ok {
		t.Errorf("an empty cache was read as a source: %q", dir)
	}
	// A cache directory that does not exist at all is the same answer, and
	// it is the commoner shape: no plugin has ever been installed.
	if dir, _, ok := updateFrom("", filepath.Join(t.TempDir(), "absent")); ok {
		t.Errorf("a missing cache was read as a source: %q", dir)
	}
}

// TestAHalfPresentVersionIsNotOffered. `newestPlugin` owns this rule and has its
// own tests; it is asserted again through this seam because the wiring is what
// is new, and a resolver that called something else would pass every test above.
func TestAHalfPresentVersionIsNotOffered(t *testing.T) {
	cache := t.TempDir()
	want := plantPlugin(t, cache, "engramux", "0.1.0", bothBinaries()...)
	plantPlugin(t, cache, "engramux", "0.3.0", "engramux.exe")

	dir, _, ok := updateFrom("", cache)
	if !ok {
		t.Fatal("the complete version was not found")
	}
	if dir != want {
		t.Errorf("dir = %q, want the complete 0.1.0 at %q", dir, want)
	}
}

// TestTheBareUpdateMessageNamesTheChannelThatNowExists replaces the test that
// forbade the words "release", "archive" and "download".
//
// That one said in its own comment that it is deleted rather than relaxed on the
// day a release exists, because telling a reader to download an archive is then
// correct and a test forbidding it would be pinning a fact that had moved.
// 0.1.0 was published on 2026-09-09, so the day arrived and the test went.
//
// What replaces it holds the same shape of invariant against the new fact: the
// message has to name the route that exists rather than leave the reader with
// only a directory they are supposed to already have.
func TestTheBareUpdateMessageNamesTheChannelThatNowExists(t *testing.T) {
	help := strings.Join(updateNoSourceHelp, "\n")
	for _, want := range []string{"plugin", "--from <directory>"} {
		if !strings.Contains(help, want) {
			t.Errorf("the bare-update message does not mention %q:\n%s", want, help)
		}
	}
}
