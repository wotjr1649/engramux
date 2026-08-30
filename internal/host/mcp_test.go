package host

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	probeURL   = "http://127.0.0.1:8867/mcp"
	probeToken = "abcdef0123456789"
)

func probeEndpoint() *Endpoint { return &Endpoint{URL: probeURL, Token: probeToken} }

func splice(t *testing.T, text string, ep *Endpoint) string {
	t.Helper()
	out, err := SpliceCodex(text, ep)
	if err != nil {
		t.Fatalf("SpliceCodex: %v", err)
	}
	return out
}

// TestSpliceCodexLeavesTheRestOfTheFileAlone is the property this is a line
// splice for. config.toml is full of another product's settings and its
// comments, and a parse-and-re-emit through a TOML library would reformat all
// of it to write four lines.
func TestSpliceCodexLeavesTheRestOfTheFileAlone(t *testing.T) {
	const before = `# a comment the user wrote
model = "gpt-5.4"

[tui]
theme = "dark"   # trailing comment

[mcp_servers.other]
url = "http://127.0.0.1:1/mcp"
`
	got := splice(t, before, probeEndpoint())

	for _, keep := range []string{
		"# a comment the user wrote",
		`model = "gpt-5.4"`,
		"[tui]",
		`theme = "dark"   # trailing comment`,
		"[mcp_servers.other]",
		`url = "http://127.0.0.1:1/mcp"`,
	} {
		if !strings.Contains(got, keep) {
			t.Errorf("the splice lost %q\ngot:\n%s", keep, got)
		}
	}
	if !strings.Contains(got, "[mcp_servers.engramux]") {
		t.Errorf("the table was not written\ngot:\n%s", got)
	}
	if !strings.Contains(got, `http_headers = { Authorization = "Bearer `+probeToken+`" }`) {
		t.Errorf("the header line is not what Codex reads\ngot:\n%s", got)
	}
}

// TestSpliceCodexReplacesItsOwnTableAndStopsAtTheNextHeader is the splice's one
// piece of parsing: a TOML table runs from its header to the next line that
// starts one, so removing ours must not swallow whatever follows it.
func TestSpliceCodexReplacesItsOwnTableAndStopsAtTheNextHeader(t *testing.T) {
	before := `[mcp_servers.engramux]
url = "http://127.0.0.1:1/mcp"
http_headers = { Authorization = "Bearer stale" }

[mcp_servers.after]
url = "http://127.0.0.1:2/mcp"
`
	got := splice(t, before, probeEndpoint())

	if n := strings.Count(got, "[mcp_servers.engramux]"); n != 1 {
		t.Errorf("the table appears %d times, want 1 - the old one is removed before the new one is written\ngot:\n%s", n, got)
	}
	if strings.Contains(got, "Bearer stale") {
		t.Errorf("the stale token survived\ngot:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.after]") || !strings.Contains(got, `http://127.0.0.1:2/mcp`) {
		t.Errorf("the table after ours was swallowed\ngot:\n%s", got)
	}
}

// TestSpliceCodexIsIdempotent covers the re-run, which is the common case. The
// first run may normalise trailing blank lines and so report a change that is
// only whitespace; every run after that must change nothing.
func TestSpliceCodexIsIdempotent(t *testing.T) {
	once := splice(t, "model = \"gpt-5.4\"\n\n\n", probeEndpoint())
	twice := splice(t, once, probeEndpoint())
	if once != twice {
		t.Errorf("a second splice changed the file\nfirst:\n%q\nsecond:\n%q", once, twice)
	}
}

// TestSpliceCodexRemoves is the --remove half, spelled by passing no endpoint,
// so install and remove cannot drift apart.
func TestSpliceCodexRemoves(t *testing.T) {
	installed := splice(t, "model = \"gpt-5.4\"\n", probeEndpoint())
	removed := splice(t, installed, nil)

	if strings.Contains(removed, "engramux") {
		t.Errorf("the table survived removal\ngot:\n%s", removed)
	}
	if !strings.Contains(removed, `model = "gpt-5.4"`) {
		t.Errorf("removal took the rest of the file with it\ngot:\n%s", removed)
	}
	// Removing what is not there changes nothing at all, so a --remove run on
	// a machine that never installed does not rewrite the user's file.
	if again, err := SpliceCodex(removed, nil); err != nil || again != removed {
		t.Errorf("removing an absent table changed the file\n got: %q\nwant: %q (err %v)", again, removed, err)
	}
}

// TestSpliceCodexRefusesAValueThatWouldBreakOutOfItsString is the security half.
//
// The url and the token are written into a TOML basic string, so a value
// carrying a quote or a backslash would end that string early and the rest of
// it would be parsed as TOML. The token is a credential (spec 6.1), so the
// failure mode is not a malformed file - it is a credential landing somewhere
// this code did not decide.
func TestSpliceCodexRefusesAValueThatWouldBreakOutOfItsString(t *testing.T) {
	bs := string([]byte{92})
	for _, tc := range []struct{ name, url, token string }{
		{"a quote in the token", probeURL, `x" }` + "\n" + `evil = "yes`},
		{"a backslash in the token", probeURL, `x` + bs + `n`},
		{"a quote in the url", `http://x"`, probeToken},
		{"an empty token", probeURL, ""},
		{"an empty url", "", probeToken},
		{"a newline in the url", "http://x\ny", probeToken},
		{"a non-ASCII token", probeURL, "tokén"},
		{"a space in the token", probeURL, "two words"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := SpliceCodex("model = \"x\"\n", &Endpoint{URL: tc.url, Token: tc.token})
			if err == nil {
				t.Fatalf("SpliceCodex accepted it and wrote:\n%s", out)
			}
			if out != "" {
				t.Errorf("a refused splice returned text: %q", out)
			}
			if strings.Contains(err.Error(), tc.token) && tc.token != "" {
				t.Errorf("the error carries the token, which is a credential: %v", err)
			}
		})
	}
}

// TestReadEndpointReportsWhyItCannotBeUsed covers the three states the
// installer meets: not published yet, published, and present but unusable.
func TestReadEndpointReportsWhyItCannotBeUsed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	if _, err := ReadEndpoint(path); !errors.Is(err, ErrEndpointNotPublished) {
		t.Errorf("a missing mcp.json gave %v, want ErrEndpointNotPublished - it is the first-install "+
			"state and not a fault", err)
	}

	seedRaw(t, path, `{"url":"`+probeURL+`","token":"`+probeToken+`"}`)
	ep, err := ReadEndpoint(path)
	if err != nil {
		t.Fatalf("ReadEndpoint over a good file: %v", err)
	}
	if ep.URL != probeURL || ep.Token != probeToken {
		t.Errorf("ReadEndpoint = %+v, want the seeded values", ep)
	}

	// A file that does not parse. The error may not carry what it read: the
	// file holds a token, and a parser's message quotes the bytes it choked on.
	seedRaw(t, path, `{"url":"`+probeURL+`","token":"`+probeToken)
	_, err = ReadEndpoint(path)
	if err == nil {
		t.Fatal("ReadEndpoint accepted a truncated mcp.json")
	}
	if strings.Contains(err.Error(), probeToken) {
		t.Errorf("the parse error carries the token: %v", err)
	}

	seedRaw(t, path, `{"url":"`+probeURL+`","token":"two words"}`)
	if _, err := ReadEndpoint(path); err == nil {
		t.Error("ReadEndpoint accepted a token it will not pass on")
	}
}

// TestReadEndpointDoesNotWrite holds spec 5.9's assignment: the service binds
// the port, mints the token and writes mcp.json, and the installer reads it.
func TestReadEndpointDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	const body = `{"url":"` + probeURL + `","token":"` + probeToken + `"}`
	seedRaw(t, path, body)

	if _, err := ReadEndpoint(path); err != nil {
		t.Fatalf("ReadEndpoint: %v", err)
	}
	if got := read(t, path); got != body {
		t.Errorf("ReadEndpoint changed mcp.json\n got: %q\nwant: %q", got, body)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Errorf("ReadEndpoint left something beside mcp.json: %v (%v)", entries, err)
	}
}
