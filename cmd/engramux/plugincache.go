package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wotjr1649/engramux/internal/host"
)

// pluginName is what this product's plugin is called in Claude Code's cache. It
// is the `name` field of `.claude-plugin/plugin.json`, and the cache keys on it
// rather than on anything this binary can read out of itself.
const pluginName = "engramux"

// newestPlugin answers the newest version of this product sitting in Claude
// Code's plugin cache, and the directory holding it. Both are "" when there is
// nothing there, which is the ordinary state on a machine that has never
// installed the plugin.
//
// # The layout is measured rather than documented
//
// Read from the cache on the machine this was written on, 2026-09-07:
// <cache>/<marketplace>/<plugin>/<version>/ - one directory per version, kept
// after an update so that sessions still running against an old one keep
// working. So the marketplace directory is walked rather than named: it is
// whatever catalogue the user added this from, and a fork or a rename would key
// it differently. The plugin directory is named, because that name is ours.
//
// # A version directory only counts when both binaries are in it
//
// The line this feeds says "there is something newer, and `update --from` will
// take it", so a directory `update` would refuse is not an answer. That is not
// hypothetical: Claude Code keeps an old version's directory for about a
// fortnight, and a partially removed one would otherwise be reported as an
// available upgrade.
func newestPlugin(cacheRoot string) (dir, version string) {
	markets, err := os.ReadDir(cacheRoot)
	if err != nil {
		return "", ""
	}
	for _, m := range markets {
		if !m.IsDir() {
			continue
		}
		base := filepath.Join(cacheRoot, m.Name(), pluginName)
		versions, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, v := range versions {
			candidate := filepath.Join(base, v.Name())
			if !v.IsDir() || !carriesBothBinaries(candidate) {
				continue
			}
			if _, _, ok := parseVersion(v.Name()); !ok {
				continue
			}
			if version == "" || compareVersions(v.Name(), version) > 0 {
				dir, version = candidate, v.Name()
			}
		}
	}
	return dir, version
}

// carriesBothBinaries is what makes a directory something `update --from` can
// be pointed at: that command replaces the pair, so half of it is not an
// upgrade.
func carriesBothBinaries(dir string) bool {
	for _, name := range []string{host.RelayName, host.ServiceName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

// compareVersions orders two product versions, negative when a sorts first.
//
// # It is semantic versioning as far as this ever needs, and not further
//
// Three comparisons happen here and only three. A development build against a
// release: [Product]'s prefix is 0.0.0-dev, so the numeric part settles it and
// that is the reason the prefix is what it is. A release against a release:
// also the numeric part. And a release against a pre-release of the same
// numbers, where the specification's rule is that the release wins - which is
// the one case a plain string comparison would get backwards.
//
// What it does not implement is the specification's dot-by-dot ordering of two
// pre-release strings, where 1.0.0-alpha.2 sorts below 1.0.0-alpha.10 and a
// string comparison says the opposite. Nothing in this product has ever
// published a pre-release, the answer is a line in a report rather than a
// decision, and the fallback is at least deterministic. The day a pre-release
// ships is the day to write the rest of it.
func compareVersions(a, b string) int {
	an, ap, _ := parseVersion(a)
	bn, bp, _ := parseVersion(b)
	for i := range an {
		if an[i] != bn[i] {
			return an[i] - bn[i]
		}
	}
	switch {
	case ap == bp:
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	}
	return strings.Compare(ap, bp)
}

// parseVersion splits a version into its three numbers and its pre-release, and
// says whether it was a version at all.
//
// Build metadata - everything after a `+` - is dropped rather than compared,
// which is the specification's own rule and is what makes a development build's
// commit suffix invisible here. Anything that is not three dot-separated
// numbers is refused, so that a stray directory in the cache is skipped instead
// of being read as 0.0.0.
func parseVersion(v string) (nums [3]int, pre string, ok bool) {
	if plus := strings.IndexByte(v, '+'); plus >= 0 {
		v = v[:plus]
	}
	if dash := strings.IndexByte(v, '-'); dash >= 0 {
		v, pre = v[:dash], v[dash+1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nums, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return [3]int{}, "", false
		}
		nums[i] = n
	}
	return nums, pre, true
}
