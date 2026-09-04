package host

import (
	"encoding/json/jsontext"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// backupInfix is what a saved copy of a host configuration is named with. The
// destination's own name is kept in front of it so that the backup sorts beside
// the file it came from, and [savedCopies] can find every one of them.
const backupInfix = ".engramux-backup-"

// backupKeep is how many saved copies of one destination survive a [Commit],
// counting the one that Commit is about to take.
//
// It is backlog 44. Every Commit left a timestamped copy and nothing removed
// any of them, so the count grew without bound - and one of the files this
// backs up is ~/.codex/config.toml, which holds `Authorization = "Bearer
// <token>"`. Each copy taken after the token was written is another long-lived
// copy of it, under a name nothing replaces, in a directory people are asked to
// attach to bug reports.
//
// # Why this is a bound and not a sweep
//
// A backup here is meant to be recoverable, so removing all of them would take
// the remedy away with the exposure. [Plan]'s own comment is the evidence: the
// failure it describes leaves one host's configuration already replaced "with
// only a timestamped backup to recover it", and install.go prints every path
// [Commit] returns so that a person can go and use one. Same trade that put
// BUILTIN\Administrators on mcp.json's DACL - a narrowing that removes the only
// route back is not a narrowing worth having.
//
// # Why three
//
// It is a bound rather than a measurement, and what it has to be is finite and
// more than one. A copy is taken *before* each write, so the copy holding the
// last good configuration is one write older than the copy holding the bad one:
// keeping a single copy would mean a user who noticed one install too late had
// only the broken content left. Three survives noticing twice too late.
const backupKeep = 3

// # The paths here are variables, and gosec says so at three sites
//
// Every path this file opens is one the caller computed from the running user's
// own environment - a host configuration under their home directory, or that
// same path with a suffix appended. Spec 2 puts a single Windows user SID
// inside the trust boundary, and none of these paths arrives from a captured
// payload, a network, or an event: the untrusted input this product handles is
// hook payload TEXT, which never reaches a file name. So G304 and G703 have no
// abuse case here that is not already "the user can write their own files", and
// they are suppressed per site with this note as the reason rather than
// answered with a validation the caller would have to defeat itself.

// Plan is one host configuration file's pending write: what it is, and the
// exact bytes that will replace it. A nil *Plan means there is nothing to do,
// which is not an error.
//
// Planning and writing are two steps, and that is not tidiness. There are two
// files. An earlier installer rewrote one host's configuration completely
// before it had so much as parsed the other, so a syntax error in the second
// left the first already changed with only a timestamped backup to recover it.
// Planning both and then writing both cannot make two files atomic - nothing
// can - but it moves every failure that is about READING to before the first
// byte is written.
type Plan struct {
	Path  string
	Label string
	Text  []byte
}

// PlanMerge reads one host's configuration and works out what it should say,
// **without writing anything**.
//
// A file that is not there is skipped rather than created: a user with only one
// of the two hosts installed is an ordinary user, and writing a configuration
// for a host that is not present would leave a file its owner never made.
//
// A file whose merged form equals what it already holds returns no plan, so a
// re-run neither rewrites it nor leaves a backup for a write that changed
// nothing. A re-run with no rebuild in between is the common case.
func PlanMerge(path, label string, events []string, entryFor func(event string) jsontext.Value) (*Plan, error) {
	//nolint:gosec // G304: a caller-computed path inside the trust boundary; see the note above.
	before, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("host: read %s: %w", label, err)
	}

	after, err := MergeHooks(before, events, entryFor)
	if err != nil {
		return nil, fmt.Errorf("host: %s at %s: %w", label, path, err)
	}
	if string(after) == string(before) {
		return nil, nil
	}
	return &Plan{Path: path, Label: label, Text: after}, nil
}

// Commit backs up and writes every plan, in order.
//
// The backup is taken immediately before its own write rather than for all
// files up front, so a run that fails on the second file has not left a backup
// beside a file nothing touched.
// It returns the backups it took, and the caller is expected to report them.
// That is not tidiness: a backup of a file that already held a bearer token is
// another copy of that token, under a timestamped name nothing will ever
// replace, in a directory people are asked to attach to bug reports. The
// installer this replaces printed every backup path; a review found that this
// one had stopped. How many of them a destination accumulates is [backupKeep],
// and `doctor` counts them through [Backups].
func Commit(plans []*Plan) ([]string, error) {
	var saved []string
	for _, plan := range plans {
		if plan == nil {
			continue
		}
		dest, err := backup(plan.Path)
		if err != nil {
			return saved, err
		}
		saved = append(saved, dest)
		if err := writeAtomic(plan.Path, plan.Text); err != nil {
			return saved, err
		}
	}
	return saved, nil
}

// staleTemps removes temporary files an earlier write left beside this
// destination.
//
// Best effort and silent: a file it cannot remove is one this run has no
// business failing over, and the write about to happen is the caller's actual
// business. What it is for is the credential case - a temporary copy of a
// configuration carrying a bearer token, under a random name nothing else will
// ever replace.
func staleTemps(path string) {
	matches, err := filepath.Glob(path + ".engramux-tmp-*")
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// savedCopy is one backup beside a destination: where it is, and when it was
// taken. The time comes from the directory scan rather than from the name, and
// that is not indifference - see [savedCopies].
type savedCopy struct {
	path string
	when time.Time
}

// savedCopies returns every backup beside path, newest first.
//
// # The order is by modification time and not by name, and the difference is
// # measurable
//
// The stamp in the name is [time.RFC3339Nano], which **trims trailing zeros**
// from the fractional second, so that field is variable width and a
// lexicographic sort of the names is not reliably chronological: `...-55-1Z`
// sorts after `...-55-12Z`, because `Z` is greater than `2`. Whole seconds are
// unaffected, so the wrongness is bounded and nothing here would have noticed
// it - which is exactly the kind of thing that survives into a retention policy
// and quietly keeps the wrong three files.
//
// The name is still the tie-break, because a Windows file time moves in ticks
// of about 15.6 ms by default and several copies written inside one tick share
// a modification time exactly. That is unreachable for real installs, which are
// minutes apart, and immediate for a test that calls [Commit] in a loop.
func savedCopies(path string) ([]savedCopy, error) {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("host: list %s: %w", dir, err)
	}
	prefix := filepath.Base(path) + backupInfix
	var found []savedCopy
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			// It went away between the scan and the stat, which is
			// another run of this product doing its own pruning.
			continue
		}
		found = append(found, savedCopy{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	slices.SortFunc(found, func(a, b savedCopy) int {
		if c := b.when.Compare(a.when); c != 0 {
			return c
		}
		return strings.Compare(b.path, a.path)
	})
	return found, nil
}

// Backups answers how many saved copies stand beside path and when the oldest
// was taken. A destination with none answers 0 and the zero time.
//
// # It answers a count and a time, and there is no way to ask it for a name
//
// `doctor` is the caller, every line of that command goes through a mask, and a
// backup's name is the destination's own path plus a stamp - the shape the mask
// exists to keep out of a diagnostic somebody pastes into a public issue. A
// function returning the names would leave "print only the count" as a rule the
// call site follows; this leaves it as something the package cannot be asked to
// break. It is the same trade internal/mcpconf's package comment makes about
// the token: no field for it on the way out, so no caller can be handed one.
func Backups(path string) (int, time.Time, error) {
	found, err := savedCopies(path)
	if err != nil || len(found) == 0 {
		return 0, time.Time{}, err
	}
	return len(found), found[len(found)-1].when, nil
}

// prune removes all but the newest keep backups beside path.
//
// Best effort and silent, like [staleTemps]: a copy it cannot remove is not
// something an install should fail over, and the write about to happen is the
// caller's actual business.
func prune(path string, keep int) {
	found, err := savedCopies(path)
	if err != nil {
		return
	}
	for _, c := range found[min(len(found), keep):] {
		_ = os.Remove(c.path)
	}
}

// backup copies a file beside itself under a timestamped name and returns that
// name. The stamp carries no colons, which a Windows path may not hold.
//
// The prune runs **before** the copy and keeps one fewer, rather than running
// after and keeping [backupKeep]. That is what makes the copy this run is about
// to take structurally ineligible for its own prune: it does not exist yet, so
// no ordering mistake can reach it. What an ordering mistake can still do is
// keep the wrong old copies, which costs recovery depth rather than the
// remedy.
func backup(path string) (string, error) {
	//nolint:gosec // G304: see the note above.
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("host: read %s to back it up: %w", path, err)
	}
	prune(path, backupKeep-1)
	stamp := strings.NewReplacer(":", "-", ".", "-").Replace(time.Now().UTC().Format(time.RFC3339Nano))
	dest := path + backupInfix + stamp
	//nolint:gosec // G703: dest is path plus a suffix this file appends; see the note above.
	if err := os.WriteFile(dest, body, 0o600); err != nil {
		return "", fmt.Errorf("host: write %s: %w", dest, err)
	}
	return dest, nil
}

// writeAtomic writes through a temporary file and a rename, which spec 5.6
// requires of every file this product writes on a user's behalf - the two host
// configurations included.
//
// A direct write truncates first. If it then fails - a full disk, a scanner
// holding the file, the process dying - what is left on disk is a truncated
// JSON document, and the host that reads it next has no hook configuration at
// all rather than the one it started with. The temporary file takes that risk
// and the rename is the only step that touches the destination.
//
// Sync before rename is not decoration: a rename that lands before the data
// does leaves a file that is present, correctly named, and empty.
//
// The temporary file is created **beside the destination** and not in the
// system temporary directory, because a rename is atomic only within a volume
// and the two are not reliably on the same one.
//
// # This path carries the bearer token, and an earlier version of this comment
// # said it did not
//
// It said mcp.json carries a token and this one replaces a file whose owner is
// the user. That stopped being true the moment install.go routed the Codex
// configuration through here, because that file holds
// `Authorization = "Bearer <token>"`. A review found the comment before anyone
// found a bug from it, which is the only reason it is worth writing down twice.
//
// What follows from it: the temporary file below is a second copy of a
// credential while it exists, so it is removed on every failure path and named
// so that a run killed between the write and the rename leaves at most one -
// [staleTemps] sweeps whatever an earlier kill left. os.CreateTemp opens at
// 0600, which is Go's default rather than this code's decision and is advisory
// on Windows anyway (spec 7.1).
//
// # Why this is not mcpconf.Write
//
// internal/mcpconf writes mcp.json through the same shape and internal/spool
// writes a record through a third. They are not shared because what differs is
// not the sequence but the policy, and 2026-09-04 widened the gap rather than
// closing it: mcp.json's writer now narrows its file to an explicit DACL
// (backlog 28), and this one must not - these are the hosts' own configuration
// files, and re-permissioning another product's file is not this product's to
// do. `doctor` reports them instead. A helper taking DACL, temp-naming and
// cleanup policy as parameters would be longer than either caller.
//
// The asymmetry runs the other way too, and that half is a finding rather than
// a design: [staleTemps] lives here and mcpconf has no equivalent, while the
// temporary file that holds a *raw* token is mcpconf's. A run killed between
// its write and its rename leaves a copy that nothing sweeps.
func writeAtomic(path string, text []byte) (err error) {
	// Whatever an earlier run left when it was killed between the write and
	// the rename. The installer this replaces used a temporary name carrying
	// the process id, so a later run overwrote it; os.CreateTemp's random
	// suffix means every killed run leaves its own file, and one of them can
	// hold a bearer token. Sweeping first restores the property the naming
	// gave away.
	staleTemps(path)

	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".engramux-tmp-*")
	if err != nil {
		return fmt.Errorf("host: create a temporary file beside %s: %w", path, err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if _, err = f.Write(text); err != nil {
		return fmt.Errorf("host: write %s: %w", tmp, err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("host: sync %s: %w", tmp, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("host: close %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("host: rename %s onto %s: %w", tmp, path, err)
	}
	return nil
}
