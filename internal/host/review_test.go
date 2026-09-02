package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests in this file each hold a defect an independent security review
// found in the port, and each was reproduced before it was fixed. They are
// together rather than filed under the code they guard because what they have
// in common is how they were found, and because a reviewer coming back wants
// one place to check that the answers stuck.

// TestAUrlThatLooksLikeAFlagIsRefused holds the one that reported success for
// something that never happened.
//
// `--help` is printable ASCII, so safeValue passes it. As the url it becomes a
// positional argument to `claude`, which reads it as a flag, prints its usage
// and exits 0 - and the installer said it had registered. Measured, in that
// order.
func TestAUrlThatLooksLikeAFlagIsRefused(t *testing.T) {
	if !safeValue("--help") {
		t.Fatal("safeValue now refuses --help, so this test no longer measures what it was written for")
	}

	useStub(t)
	err := RegisterClaudeMCP(t.Context(), os.Args[0], &Endpoint{URL: "--help", Token: probeToken})
	if err == nil {
		t.Error("RegisterClaudeMCP accepted a url that claude reads as a flag")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	for _, bad := range []string{"--help", "http://example.com/mcp", "https://127.0.0.1/mcp", "ftp://127.0.0.1/"} {
		seedRaw(t, path, `{"url":"`+bad+`","token":"`+probeToken+`"}`)
		if _, err := ReadEndpoint(path); err == nil {
			t.Errorf("ReadEndpoint accepted %q", bad)
		}
	}
	// And the shape the service actually publishes still passes.
	seedRaw(t, path, `{"url":"`+probeURL+`","token":"`+probeToken+`"}`)
	if _, err := ReadEndpoint(path); err != nil {
		t.Errorf("ReadEndpoint refused the url the service publishes: %v", err)
	}
}

// TestAnEndpointDoesNotPrintItsToken is the structural answer to a seam that
// returns an error the installer prints verbatim. Any implementation writing
// fmt.Errorf("... %v", ep) would otherwise have put a credential in a report.
func TestAnEndpointDoesNotPrintItsToken(t *testing.T) {
	ep := &Endpoint{URL: probeURL, Token: probeToken}
	for _, got := range []string{
		fmt.Sprintf("%v", ep),
		// The verb is the point. staticcheck wants ep.String() here, which
		// would test the method and not what this test is about - that a
		// careless %s somewhere else in the product cannot spell the token.
		//nolint:staticcheck // S1025: see above.
		fmt.Sprintf("%s", ep),
		fmt.Sprintf("%v", fmt.Errorf("register %v: %w", ep, os.ErrPermission)),
	} {
		if strings.Contains(got, probeToken) {
			t.Errorf("an Endpoint printed its token: %s", got)
		}
		if !strings.Contains(got, probeURL) {
			t.Errorf("an Endpoint printed nothing useful either: %s", got)
		}
	}
	var nilEP *Endpoint
	if s := nilEP.String(); s == "" {
		t.Error("a nil Endpoint formats as nothing, which reads as a bug in the caller")
	}
}

// TestAMachineWithoutCodexIsNotAFailure holds the regression against the
// installer this replaces, which skipped a Codex configuration that is not
// there by name. Without it, every install on a Claude-Code-only machine
// printed a FAILED line indistinguishable from a real one.
func TestAMachineWithoutCodexIsNotAFailure(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true
	if err := os.Remove(tr.opt.CodexConfig); err != nil {
		t.Fatalf("remove: %v", err)
	}

	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	joined := strings.Join(report, "\n")
	if strings.Contains(joined, "FAILED") {
		t.Errorf("a machine with no Codex configuration reported a failure:\n%s", joined)
	}
	if !strings.Contains(joined, "skipped") {
		t.Errorf("the report does not say the file was skipped:\n%s", joined)
	}
	if _, err := os.Stat(tr.opt.CodexConfig); !os.IsNotExist(err) {
		t.Error("a Codex configuration was created for a machine that has no Codex")
	}
}

// TestEveryBackupIsReported keeps a credential copy from being made silently.
//
// A backup of a configuration that already held the bearer token is another
// copy of it, timestamped, under a name nothing will ever replace, in a
// directory people are asked to attach to bug reports. The installer this
// replaces printed every backup path and this one had stopped.
func TestEveryBackupIsReported(t *testing.T) {
	tr := newTree(t)
	tr.opt.Apply = true

	report, err := Install(t.Context(), tr.opt, tr.sys)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	joined := strings.Join(report, "\n")

	for _, path := range []string{tr.opt.ClaudePath, tr.opt.CodexHooks, tr.opt.CodexConfig} {
		backups, _ := filepath.Glob(path + backupInfix + "*")
		if len(backups) == 0 {
			t.Errorf("no backup was taken beside %s", filepath.Base(path))
			continue
		}
		for _, b := range backups {
			if !strings.Contains(joined, b) {
				t.Errorf("a backup was made and not reported: %s\nreport:\n%s", b, joined)
			}
		}
	}
}

// TestSpliceRefusesAHeaderItCannotSee covers the two shapes a line splice reads
// wrongly, and turns silent data loss into a refusal.
func TestSpliceRefusesAHeaderItCannotSee(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{
			// A trailing comment: the exact-line test misses it, so it
			// survives and the new table lands beside it. Two tables of one
			// name is a TOML parse error that takes the whole file with it.
			"a trailing comment on the header",
			"[mcp_servers." + MCPName + "] # installed by hand\nurl = \"http://127.0.0.1:1/mcp\"\n",
		},
		{
			// A header-shaped line inside a multi-line string, where the
			// splice would eat from there to the next bracket.
			"a header inside a multi-line string",
			"notes = \"\"\"\n[mcp_servers." + MCPName + "]\nkeep this\n\"\"\"\nmodel = \"x\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := SpliceCodex(tc.text, probeEndpoint())
			if err == nil {
				t.Fatalf("the splice accepted it and wrote:\n%s", out)
			}
			if out != "" {
				t.Errorf("a refused splice returned text: %q", out)
			}
			if !strings.Contains(err.Error(), "by hand") {
				t.Errorf("the error does not tell the user what to do: %v", err)
			}
		})
	}
}

// TestAKilledWriteLeavesNoTemporaryCopyBehind holds the sweep.
//
// os.CreateTemp names with a random suffix, so a run killed between the write
// and the rename leaves a file no later run would overwrite - and for the Codex
// configuration that file holds a bearer token. The installer this replaces
// named its temporary file after the process id, so the next run replaced it;
// this restores the property that naming gave away.
func TestAKilledWriteLeavesNoTemporaryCopyBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	seedRaw(t, path, "model = \"x\"\n")

	// What a killed run leaves: a temporary file beside the destination,
	// carrying a token, under a name nothing else will pick.
	orphan := path + ".engramux-tmp-987654321"
	seedRaw(t, orphan, `http_headers = { Authorization = "Bearer `+probeToken+`" }`)

	if err := writeAtomic(path, []byte("model = \"y\"\n")); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("a temporary file from an earlier killed run survived: %s", filepath.Base(orphan))
	}
	temps, _ := filepath.Glob(path + ".engramux-tmp-*")
	if len(temps) != 0 {
		t.Errorf("a temporary file outlived the write: %v", temps)
	}
	if got := read(t, path); got != "model = \"y\"\n" {
		t.Errorf("the destination is wrong after the sweep: %q", got)
	}
}
