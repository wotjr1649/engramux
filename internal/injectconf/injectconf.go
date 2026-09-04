// Package injectconf is everything about injection that the relay needs and
// that does not touch a database: the switch file, and the two numbers the spec
// fixes.
//
// # It exists because of what an import block cannot show you
//
// The relay is spawned once per hook event, which is what made its size an
// invariant rather than a preference (1.0 spec §7.1, which records 3,862,528 B
// twice and argues two rejections against that figure - §5.9's stdio proxy, and
// `doctor`'s net/http probe at +93.7%). Measured 2026-09-04 the binary was
// 8,703,488 B, and the reason was four symbols: `doctor.go` and `inject.go`
// reached into internal/inject for a switch file and two constants, none of
// which reads anything, and internal/inject reaches internal/search and
// internal/store, which carry modernc.org/sqlite and goose. Fourteen packages
// arrived to answer "is there a file called inject.json".
//
// Nothing about that is visible where it happens. `doctor.go`'s own comment
// explained that it used spool.Dir() rather than importing internal/store
// *because* that would link the driver - two hundred lines below an import that
// did. A leaf is what makes the coupling structural instead of a thing a
// reviewer has to hold three packages in their head to see, and
// `TestTheRelayDoesNotLinkTheSQLiteDriver` is what makes it stay one.
//
// # Why the two budgets are here and not in internal/inject
//
// They are not configuration - a user cannot set either, deliberately (M5,
// M-4), and a per-user cap would make gate M5 a measurement of one machine's
// setup. They are here because they are the half of the injection contract the
// relay holds: it clamps its own deadline with [Budget] and refuses a reply over
// [MaxBytes], and it does both without being able to build one. internal/inject
// names them again for its own code and its gates, which is safe in a way it was
// not before: the dependency test now fails the moment that alias is used to
// reach this package the long way round.
package injectconf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MaxBytes is gate M5's cap on one injection, fence included.
//
// It is a conversion and not a measurement (memory spec rev.8, M-4). Codex
// documents a default additionalContext limit of about 2,500 tokens, past which
// it spills the text to a file and gives the model a preview and a path; Claude
// Code documents no limit at all. So the hosts' documented budget is Codex's, it
// is the stricter of the two by virtue of existing, and 2 bytes per token is the
// conservative end for a corpus carrying Korean. §6's third mitigation - small -
// wants the error in this direction.
const MaxBytes = 5000

// Budget is the 500 ms M-4 gives one injection, and gate M10's subject.
//
// It comes out of the 1 s the relay already has (1.0 spec §5.3) rather than
// being added to it, so the product's own budget does not move because a
// feature was added inside it. Twice the worst of the five measured
// `engramux search` runs against the installed service over a 227,954,688 B
// database - 93, 113, 185, 245 and 251 ms - each of which is process start,
// pipe dial, search and reply. What is unverified is the tail: all five are
// warm, and every read-deadline failure the 1.0 spec §7.1 records was a cold
// read after an idle period against a smaller database. M10 measures that
// rather than assuming it.
const Budget = 500 * time.Millisecond

// ConfigName is the file that turns injection on. It sits in Engramux's own
// data directory beside the database and the spool.
//
// # Absent means off, and that is the whole of "ships disabled"
//
// M-4's row says injection is built and ships off, and nothing in the installer
// writes this file - so a first install has no switch to find and injects
// nothing. That is stronger than a default in code: a user who has never heard
// of this feature cannot have it on, and a user who wants it makes one file
// whose existence is the record of their consent.
//
// # Fail closed on every unreadable shape
//
// A file that will not open, will not parse, or parses to anything but
// `enabled: true` leaves injection off. Every mitigation in §6 is about a
// smaller window, so a configuration error must not be able to open one - and a
// hook that read a broken file and injected anyway would be the one failure the
// switch exists to prevent.
const ConfigName = "inject.json"

// Config is what [ConfigName] holds. One field, because the two numbers that
// could have been fields - [MaxBytes] and [Budget] - are the spec's and not the
// user's (M5, M-4): a per-user cap would make gate M5 a measurement of one
// machine's configuration.
type Config struct {
	Enabled bool `json:"enabled"`
}

// Dir is Engramux's data directory, the parent of the spool and the database.
//
// It is derived here rather than imported because the relay reaches this
// package and must not reach internal/service to learn where its own data
// lives. os.UserCacheDir returns %LocalAppData% on Windows, which is the
// derivation [github.com/wotjr1649/engramux/internal/spool.Dir] and
// [github.com/wotjr1649/engramux/internal/service.Dir] both make - and a test
// pins this against the spool's parent, because three copies of one derivation
// that disagree would put the switch in a directory nothing else uses.
func Dir() (string, error) {
	local, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("injectconf: locate the local application data directory: %w", err)
	}
	return filepath.Join(local, "engramux"), nil
}

// ConfigPath is where [Enabled] looks. `doctor` reports it, which is how a
// person finds out where to write the file.
func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigName), nil
}

// Enabled reports whether the user has turned injection on.
//
// It answers false for every failure and never an error, because there is
// nothing a caller could do with one: the relay's answer to "I could not read
// the switch" is the same as its answer to "the switch is off", and I-03 says
// it exits 0 either way.
func Enabled() bool {
	path, err := ConfigPath()
	if err != nil {
		return false
	}
	return enabledAt(path)
}

// enabledAt is [Enabled] against a named file, which is what a test can reach
// without moving the process's own environment.
func enabledAt(path string) bool {
	//nolint:gosec // G304: path comes from ConfigPath, not from a caller
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return false
	}
	return c.Enabled
}
