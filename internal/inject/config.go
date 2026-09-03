package inject

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
// could have been fields - the byte cap and the budget - are the spec's and not
// the user's (M5, M-4): a per-user cap would make gate M5 a measurement of one
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
		return "", fmt.Errorf("inject: locate the local application data directory: %w", err)
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
