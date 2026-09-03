package inject_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/spool"
)

// The switch is off unless a file says otherwise, and every unreadable shape is
// off too. This is the whole of "ships disabled": the installer writes no such
// file, so a first install cannot have injection on.
func TestInjectionIsOffUntilAFileSaysOtherwise(t *testing.T) {
	for _, tc := range []struct {
		name    string
		write   string // "" means write no file at all
		enabled bool
	}{
		{"no file", "", false},
		{"empty", " ", false},
		{"not json", "enabled = true", false},
		{"json but not an object", `"enabled"`, false},
		{"the key is absent", `{}`, false},
		{"the key is false", `{"enabled":false}`, false},
		{"the key is a string", `{"enabled":"true"}`, false},
		{"the key is true", `{"enabled":true}`, true},
		{"true with other keys", `{"enabled":true,"cap":99}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv, so this test and its parents cannot be
			// parallel: the value is process-wide.
			dir := t.TempDir()
			t.Setenv("LOCALAPPDATA", dir)
			path, err := inject.ConfigPath()
			if err != nil {
				t.Fatalf("ConfigPath: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				t.Fatalf("make the data directory: %v", err)
			}
			if tc.write != "" {
				if err := os.WriteFile(path, []byte(tc.write), 0o600); err != nil {
					t.Fatalf("write the config: %v", err)
				}
			}
			if got := inject.Enabled(); got != tc.enabled {
				t.Errorf("Enabled() = %v with %s, want %v", got, tc.name, tc.enabled)
			}
		})
	}
}

// The switch lives beside the spool and the database, and this is what keeps
// the third copy of that derivation from drifting away from the other two - the
// same pin internal/service and internal/spool already have against each other.
// A switch in a directory nothing else uses is a switch nobody finds.
func TestTheConfigSitsBesideTheSpool(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir, err := inject.Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	sp, err := spool.Dir()
	if err != nil {
		t.Fatalf("spool.Dir: %v", err)
	}
	if got := filepath.Dir(sp); got != dir {
		t.Errorf("inject.Dir() = %q, want the spool's parent %q", dir, got)
	}
}
