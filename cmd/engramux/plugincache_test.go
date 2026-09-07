package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/host"
)

// plantPlugin writes one version directory into a cache root and answers where
// it put it. The binaries are named rather than defaulted, because "a version
// directory with only half the pair in it" is one of the cases under test.
func plantPlugin(t *testing.T, cacheRoot, market, version string, binaries ...string) string {
	t.Helper()
	dir := filepath.Join(cacheRoot, market, pluginName, version)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("plant %s: %v", dir, err)
	}
	for _, name := range binaries {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("not a binary"), 0o600); err != nil {
			t.Fatalf("plant %s: %v", name, err)
		}
	}
	return dir
}

func bothBinaries() []string { return []string{host.RelayName, host.ServiceName} }

// TestCompareVersionsIsNumericAndPutsAReleaseAboveItsPreRelease covers the
// three orderings [compareVersions] exists for, and one it would get wrong if
// it compared strings.
//
// 0.10.0 against 0.2.0 is the one worth having: a string comparison answers
// that 0.2.0 is newer, which would pin a user to an older build forever and
// look like nothing at all in a report.
func TestCompareVersionsIsNumericAndPutsAReleaseAboveItsPreRelease(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"0.10.0", "0.2.0", +1},
		{"0.2.0", "0.10.0", -1},
		{"1.0.0", "0.9.9", +1},
		{"0.1.0", "0.1.0", 0},
		{"0.0.0-dev+60339da107a9", "0.1.0", -1},
		{"0.1.0", "0.0.0-dev+60339da107a9", +1},
		{"1.0.0", "1.0.0-rc1", +1},
		{"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0+build", "1.0.0", 0},
	} {
		got := compareVersions(tc.a, tc.b)
		if (got > 0) != (tc.want > 0) || (got < 0) != (tc.want < 0) {
			t.Errorf("compareVersions(%q, %q) = %d, want sign %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestParseVersionRefusesWhatIsNotThreeNumbers is what keeps a stray directory
// out of the answer. Reading one as 0.0.0 would be worse than skipping it: it
// would be a candidate, and on a cache holding nothing else it would be *the*
// candidate.
func TestParseVersionRefusesWhatIsNotThreeNumbers(t *testing.T) {
	for _, bad := range []string{"", "1", "1.0", "1.0.0.0", "v1.0.0", "1.0.x", "01.0.0", "1.-1.0"} {
		if _, _, ok := parseVersion(bad); ok {
			t.Errorf("parseVersion(%q) accepted it", bad)
		}
	}
	nums, pre, ok := parseVersion("0.0.0-dev+60339da107a9")
	if !ok {
		t.Fatal("parseVersion refused a development version")
	}
	if nums != [3]int{0, 0, 0} || pre != "dev" {
		t.Errorf("parseVersion = %v, %q; want [0 0 0], \"dev\" - the build metadata is not part of the ordering", nums, pre)
	}
}

// TestNewestPluginTakesTheHighestCompleteVersionAcrossMarketplaces pins the
// layout this reads, which was measured off the machine's own cache rather than
// taken from a reference: <cache>/<marketplace>/<plugin>/<version>.
//
// The marketplace directory is walked and not named, so a fork or a rename
// still answers; 0.10.0 beside 0.2.0 is here as well as in the comparison test,
// because the ordering has to survive being called from the scan.
func TestNewestPluginTakesTheHighestCompleteVersionAcrossMarketplaces(t *testing.T) {
	root := t.TempDir()
	plantPlugin(t, root, "engramux", "0.2.0", bothBinaries()...)
	want := plantPlugin(t, root, "engramux", "0.10.0", bothBinaries()...)
	plantPlugin(t, root, "somebody-elses-fork", "0.9.0", bothBinaries()...)

	dir, version := newestPlugin(root)
	if version != "0.10.0" {
		t.Errorf("newestPlugin version = %q, want 0.10.0", version)
	}
	if dir != want {
		t.Errorf("newestPlugin dir = %q, want %q", dir, want)
	}
}

// TestNewestPluginSkipsWhatUpdateCouldNotUse. The line this feeds names
// `update --from`, so a directory that command would refuse is not an answer -
// and a half-removed old version is exactly what a cache that keeps versions
// for a fortnight will eventually hold.
func TestNewestPluginSkipsWhatUpdateCouldNotUse(t *testing.T) {
	root := t.TempDir()
	plantPlugin(t, root, "engramux", "1.0.0", host.RelayName)
	plantPlugin(t, root, "engramux", "0.9.0", host.ServiceName)
	plantPlugin(t, root, "engramux", "not-a-version", bothBinaries()...)
	want := plantPlugin(t, root, "engramux", "0.8.0", bothBinaries()...)

	dir, version := newestPlugin(root)
	if version != "0.8.0" {
		t.Errorf("newestPlugin version = %q, want 0.8.0 - the higher directories are incomplete", version)
	}
	if dir != want {
		t.Errorf("newestPlugin dir = %q, want %q", dir, want)
	}
}

// TestNewestPluginAnswersNothingForAMachineWithNoPlugin. An absent cache is the
// ordinary state and not a fault: most users of this product will never install
// the plugin at all.
func TestNewestPluginAnswersNothingForAMachineWithNoPlugin(t *testing.T) {
	for _, root := range []string{t.TempDir(), filepath.Join(t.TempDir(), "never-created")} {
		if dir, version := newestPlugin(root); dir != "" || version != "" {
			t.Errorf("newestPlugin(%q) = %q, %q; want two empty strings", root, dir, version)
		}
	}
}

// TestReportVersionsSaysWhetherTheCacheHoldsSomethingNewer is the half the scan
// cannot prove: that the three answers reach the report as three different
// lines, and that the one recommending a command names the directory that
// command takes.
func TestReportVersionsSaysWhetherTheCacheHoldsSomethingNewer(t *testing.T) {
	root := t.TempDir()
	newer := plantPlugin(t, root, "engramux", "0.2.0", bothBinaries()...)

	for _, tc := range []struct {
		name      string
		installed string
		cache     string
		want      string
	}{
		{"newer", "0.1.0", root, "0.2.0 in the plugin cache is newer - `engramux update --from " + newer + "`"},
		{"same", "0.2.0", root, "0.2.0 in the plugin cache, which is not newer than what is installed"},
		{"older", "0.3.0", root, "0.2.0 in the plugin cache, which is not newer than what is installed"},
		{"absent", "0.1.0", t.TempDir(), "nothing in Claude Code's plugin cache, so `update` reads a directory you point it at"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			// full, because the answer that matters carries a path and
			// the default masks it. What the mask does to it is
			// TestDoctorFieldsGoThroughTheMask's, not this test's.
			r := &report{w: &out, full: true}
			r.reportVersions(tc.installed, tc.installed, tc.cache)

			line := ""
			for _, l := range strings.Split(out.String(), "\n") {
				if strings.HasPrefix(strings.TrimSpace(l), "newest available") {
					line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "newest available"))
				}
			}
			if line != tc.want {
				t.Errorf("newest available = %q\nwant                %q", line, tc.want)
			}
			if r.failed {
				t.Error("a version comparison set failed, and doctor's exit code is M-6's stages")
			}
		})
	}
}
