package mcpconf

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wotjr1649/engramux/internal/winacl"
)

// TestWriteAndReadBackTheEndpoint. [URL] is the whole read side of this
// package, and the round trip is what the sticky port rests on: the next start
// reuses the port this one published (spec 5.9).
func TestWriteAndReadBackTheEndpoint(t *testing.T) {
	dir := t.TempDir()
	const endpoint = "http://127.0.0.1:52341/mcp"

	if err := Write(dir, endpoint, "TOKENTOKENTOKENTOKENTOKEN1"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := URL(dir)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != endpoint {
		t.Fatalf("the endpoint came back as %q", got)
	}
	if p := Port(got); p != 52341 {
		t.Fatalf("the port came back as %d", p)
	}
}

// TestTheDocumentCarriesTheTokenForTheInstaller. Nothing in this package can
// hand a caller a token - [URL] decodes the endpoint and stops - and that is a
// property of the package, not of the file. The installer reads this file to
// write a host configuration, so the token has to be in it, and this is the
// only place that shape is pinned.
func TestTheDocumentCarriesTheTokenForTheInstaller(t *testing.T) {
	dir := t.TempDir()
	const token = "TOKENTOKENTOKENTOKENTOKEN1"

	if err := Write(dir, "http://127.0.0.1:1/mcp", token); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatalf("read %s: %v", Name, err)
	}
	var doc struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode %s: %v", Name, err)
	}
	if doc.Token != token {
		// The value is not printed on a mismatch, because a mismatch
		// means it is not the one this test minted.
		t.Fatal("the document does not carry the token that was written")
	}
}

// TestWriteLeavesNoTemporaryFile. The temporary file carries the token, so a
// leftover is a second copy of a secret under a name nothing will ever replace
// - which is a sharper reason than the usual one for the same rule (spec 5.6).
func TestWriteLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()

	for range 3 {
		if err := Write(dir, "http://127.0.0.1:1/mcp", "TOKENTOKENTOKENTOKENTOKEN1"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	if len(entries) != 1 || entries[0].Name() != Name {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the directory holds %v, want just %s", names, Name)
	}
}

// TestAMissingFileIsNotAFailure. A service that has never started, and one
// whose bind failed, both leave no file - and the caller's next move is the
// same as for a file naming a port that is now taken: bind port 0.
func TestAMissingFileIsNotAFailure(t *testing.T) {
	got, err := URL(filepath.Join(t.TempDir(), "not-a-directory"))
	if err != nil {
		t.Fatalf("read a missing file: %v", err)
	}
	if got != "" {
		t.Fatalf("a missing file answered %q", got)
	}
}

// TestPortAnswersZeroForEverythingItCannotUse. Zero is "let Windows choose",
// and every shape below has to reach it rather than an error: a hand-edited
// file is a state the service starts through.
//
// The out-of-range cases are not theoretical. [strconv.Atoi] accepts 99999
// happily, and a bind of it would fail at run time with a message about a port
// nobody chose.
func TestPortAnswersZeroForEverythingItCannotUse(t *testing.T) {
	for _, in := range []string{
		"",
		"not a url",
		"http://127.0.0.1/mcp",
		"http://127.0.0.1:0/mcp",
		"http://127.0.0.1:99999/mcp",
		"http://127.0.0.1:-1/mcp",
		"http://127.0.0.1:http/mcp",
	} {
		if p := Port(in); p != 0 {
			t.Errorf("Port(%q) = %d, want 0", in, p)
		}
	}
}

// TestWriteNarrowsTheFileItPublishes is backlog 28's first half, asserted on the
// file Write actually leaves behind rather than on the temporary one it narrowed.
//
// That distinction is the test: Restrict runs on the temporary file, and what a
// host reads is the renamed one. os.Rename is MoveFileEx, which carries a
// security descriptor across a same-volume move and would not across volumes -
// so this is the assertion that fails if the temporary file ever stops being
// created in the destination's own directory.
func TestWriteNarrowsTheFileItPublishes(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "http://127.0.0.1:1/mcp", "not-a-real-token"); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := winacl.Describe(Path(dir))
	if err != nil {
		t.Fatalf("describe the published file: %v", err)
	}
	if !got.Protected {
		t.Error("the published file does not block inheritance, so the DACL " +
			"did not survive the rename")
	}
	if got.Inherited != 0 {
		t.Errorf("the published file carries %d inherited ACEs, want 0", got.Inherited)
	}
	if got.Others != 0 {
		t.Errorf("the published file admits %d principals beyond SYSTEM, "+
			"Administrators and this user, want 0 - it holds a bearer token", got.Others)
	}
	if !got.Narrowed() {
		t.Error("Narrowed() disagrees with the three fields above")
	}
}

// TestWriteSweepsWhatAKilledRunLeft is backlog 43, and it is the path
// TestWriteLeavesNoTemporaryFile above does not reach: that one asserts a write
// that finished cleans up after itself, and this one asserts a write cleans up
// after a run that never finished at all.
//
// Two leftovers rather than one, because os.CreateTemp's random suffix is the
// whole reason a sweep is needed. The installer this product replaces named its
// temporary file after the process id, so a later run overwrote it; a random
// suffix means every killed run leaves its own file under a name nothing will
// ever replace, and each of them holds the token verbatim.
//
// The `.bak` neighbour is not padding. It is what says the sweep is bounded by
// [tempInfix] rather than by `mcp.json.*`, which would also remove a copy the
// user made beside the file - a sweep that removes a credential must not be able
// to remove anything else.
func TestWriteSweepsWhatAKilledRunLeft(t *testing.T) {
	dir := t.TempDir()
	const (
		endpoint = "http://127.0.0.1:1/mcp"
		token    = "TOKENTOKENTOKENTOKENTOKEN1"
	)
	body := []byte(`{"url":"` + endpoint + `","token":"` + token + `"}`)

	var killed []string
	for range 2 {
		f, err := os.CreateTemp(dir, Name+tempInfix+"*")
		if err != nil {
			t.Fatalf("stage a leftover: %v", err)
		}
		if _, err := f.Write(body); err != nil {
			t.Fatalf("stage a leftover: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("stage a leftover: %v", err)
		}
		killed = append(killed, f.Name())
	}
	keep := Path(dir) + ".bak"
	if err := os.WriteFile(keep, body, 0o600); err != nil {
		t.Fatalf("stage the neighbour: %v", err)
	}

	if err := Write(dir, endpoint, token); err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, p := range killed {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a temporary file holding a token survived the write: Stat answered %v", err)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("the sweep removed a file that is not its own: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	if want := []string{Name, Name + ".bak"}; !slices.Equal(names, want) {
		t.Fatalf("the directory holds %v, want %v", names, want)
	}
}
