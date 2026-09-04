package main

import (
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/host"
)

// seedBackups leaves n saved copies beside a host configuration by replacing it
// n times, which is the only way they are ever made.
func seedBackups(t *testing.T, n int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("create the file: %v", err)
	}
	for i := 1; i <= n; i++ {
		if _, err := host.Commit([]*host.Plan{{
			Path:  path,
			Label: "codex",
			Text:  []byte(strings.Repeat("v", i+1)),
		}}); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}
	return path
}

// backupsLine runs one report and returns what it wrote.
func backupsLine(t *testing.T, path string, full bool) string {
	t.Helper()
	var out bytes.Buffer
	r := &report{w: &out, full: full}
	r.backups("codex backups", path)
	if r.failed {
		t.Error("counting backups set the failure flag; a working installation has them")
	}
	return out.String()
}

// TestBackupsReportsTheCountItWasGiven is backlog 44's reporting half.
//
// The count is the whole point of the line: the number on the owner's machine
// was `[unverified]` when the row was filed, because a credential-directory
// guard refused an agent's shell the glob that would have answered it - and this
// is the route to the number that does not ask anyone to run one.
func TestBackupsReportsTheCountItWasGiven(t *testing.T) {
	got := backupsLine(t, seedBackups(t, 2), false)
	if !strings.Contains(got, "2 kept") {
		t.Errorf("two saved copies reported %q, want the count", got)
	}
	// A line that printed a fixed number would pass the check above.
	if one := backupsLine(t, seedBackups(t, 1), false); !strings.Contains(one, "1 kept") {
		t.Errorf("one saved copy reported %q, want the count", one)
	}
}

// TestBackupsIsSilentWhenThereAreNone. Claude Code's own state file is one this
// product never writes, so it has no copies and reaches zero by itself - and a
// finding that fires on every machine is a finding nobody reads.
func TestBackupsIsSilentWhenThereAreNone(t *testing.T) {
	if got := backupsLine(t, seedBackups(t, 0), false); got != "" {
		t.Errorf("a file nothing has replaced produced %q, want nothing", got)
	}
}

// TestBackupsNamesNoFile is the same hard requirement [report.permissions]
// carries, for the same reason: a backup's name is this file's own path plus a
// stamp, and `--full` turns masking off, so the only safe form is one with
// nothing to unmask.
//
// It is checked on the line rather than trusted to [host.Backups]' signature,
// because the signature is what makes it true today and this is what notices
// when a later change hands the caller a name to be helpful with.
func TestBackupsNamesNoFile(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	path := seedBackups(t, 2)

	// --full is the harder case: it is the mode that stops masking.
	got := backupsLine(t, path, true)
	if got == "" {
		t.Fatal("nothing was reported about a file with two saved copies")
	}
	for _, bad := range []struct{ what, s string }{
		{"a path separator", `\`},
		{"a path separator", "/"},
		{"the destination's name", filepath.Base(path)},
		{"the backup infix", ".engramux-backup-"},
		{"this user's account name", u.Username},
		{"this user's SID", u.Uid},
	} {
		if strings.Contains(got, bad.s) {
			t.Errorf("the line carries %s: %q", bad.what, got)
		}
	}
}
