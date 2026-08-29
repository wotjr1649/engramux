package mcpconf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
