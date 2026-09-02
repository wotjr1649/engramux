package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// MCPName is what the endpoint is registered as with both hosts.
const MCPName = "engramux"

// ErrEndpointNotPublished is returned when mcp.json is not there yet.
//
// It is the ordinary first-install state and not a fault: the service binds the
// port, mints the token and writes that file (spec 5.9), so on a machine where
// it has never run there is nothing to register. The installer says so and
// carries on with the rest.
var ErrEndpointNotPublished = errors.New("host: no MCP endpoint has been published yet")

// Endpoint is what mcp.json publishes.
type Endpoint struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// String masks the token, so that an Endpoint reaching a %v anywhere - a log
// line, a wrapped error, an installer's report - cannot spell it out.
//
// Structural rather than a rule for callers to remember. A review found that
// the System seam in install.go returns an error the installer prints verbatim,
// and any implementation writing fmt.Errorf("register %v", ep) would have put a
// credential in it. The type now refuses to.
func (e *Endpoint) String() string {
	if e == nil {
		return "<no endpoint>"
	}
	return e.URL + " (token withheld)"
}

// loopbackURL reports whether a url is the shape the service publishes, which
// is the only shape this product has any business registering.
//
// It is narrow on purpose, and the narrowness closes a real defect measured
// before it existed: a url of `--help` passes safeValue - every character of it
// is printable ASCII - and lands in the argument vector handed to `claude`,
// where it is read as a flag rather than as the positional url. Claude Code
// prints its usage, exits 0, and the installer reports a registration that
// never happened.
func loopbackURL(v string) bool {
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// safeValue reports whether a value may be written into a TOML basic string.
//
// Printable ASCII only, and no quote or backslash. The url and the token are
// written between quotes, so a value carrying either would end that string
// early and everything after it would be parsed as TOML - and the token is a
// credential (spec 6.1), so the failure is not a malformed file but a
// credential landing somewhere this code did not decide.
//
// It is deliberately narrower than "escape it properly". A url and a base32
// token have no business holding a quote, a backslash, a space or a non-ASCII
// rune, so refusing is a truer answer than escaping: a value that needs
// escaping here is a value something else got wrong.
func safeValue(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if r < 0x21 || r > 0x7e || r == '"' || r == '\\' {
			return false
		}
	}
	return true
}

// ReadEndpoint reads mcp.json. The installer only ever reads it: spec 5.9
// assigns the port, the token and this file to the service, because an
// installer that chose the port would be choosing it before anything bound it
// and one that minted the token would be minting one the service does not hold.
//
// No error from here may carry what the file held. It holds a token, and a JSON
// parser's message quotes the bytes it choked on - so a parse failure is
// reported as a parse failure and the underlying error is deliberately not
// wrapped.
func ReadEndpoint(path string) (*Endpoint, error) {
	//nolint:gosec // G304: a caller-computed path inside the trust boundary; see the note in write.go.
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrEndpointNotPublished
	}
	if err != nil {
		return nil, fmt.Errorf("host: read %s: %w", path, err)
	}

	var ep Endpoint
	if err := json.Unmarshal(body, &ep); err != nil {
		return nil, fmt.Errorf("host: %s does not parse as JSON", path)
	}
	if !safeValue(ep.URL) || !safeValue(ep.Token) {
		return nil, fmt.Errorf("host: %s holds a url or token this will not pass on", path)
	}
	if !loopbackURL(ep.URL) {
		return nil, fmt.Errorf("host: %s holds a url that is not a loopback http endpoint", path)
	}
	return &ep, nil
}

// SpliceCodex rewrites config.toml's [mcp_servers.engramux] table and leaves
// the rest of the file alone. A nil endpoint removes the table, so install and
// remove are one path.
//
// # Why a line splice and not a TOML round trip
//
// config.toml is full of another product's settings and the comments its owner
// wrote. Parsing and re-emitting would reformat all of it, and drop every
// comment in it, to write three lines - and it would mean a TOML library this
// module does not have and would not otherwise need. The table header is
// unambiguous: a TOML table runs from its header to the next line that starts
// one, so removing our own table and appending it again is the whole operation.
//
// It normalises trailing blank lines, so the very first run may report a change
// that is only whitespace. Every run after that is idempotent, because the
// previous run left the file in exactly this shape.
func SpliceCodex(text string, ep *Endpoint) (string, error) {
	if ep != nil && (!safeValue(ep.URL) || !safeValue(ep.Token)) {
		// Checked here as well as in ReadEndpoint, because this is the
		// function that does the writing and the check is what stops a value
		// closing the string it is written into. The message names neither
		// value.
		return "", fmt.Errorf("host: the endpoint's url or token cannot be written into %s", codexTableHeader)
	}

	// Removing what is not there changes nothing at all, so a remove run on a
	// machine that never installed does not rewrite the user's file.
	if ep == nil && !strings.Contains(text, codexTableHeader) {
		return text, nil
	}

	// A header inside a multi-line string is an exact line, so the loop below
	// enters the table there and eats forward to the next bracket - taking
	// whatever was between with it, and leaving the string unterminated. The
	// count guard at the end cannot see this one, because nothing it swallowed
	// was a second header; that was tried and measured.
	//
	// What does see it is parity: if an odd number of multi-line delimiters
	// opens before our header, the header is inside a string. Refusing there
	// is the honest answer for a splice that reads lines.
	if i := headerIndex(text); i >= 0 {
		for _, delim := range []string{`"""`, "'''"} {
			if strings.Count(text[:i], delim)%2 == 1 {
				return "", fmt.Errorf("host: %s appears inside a multi-line string in this file, "+
					"where a line splice cannot tell it from a table header. it needs fixing by hand",
					codexTableHeader)
			}
		}
	}

	var kept []string
	inTable := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == codexTableHeader {
			inTable = true
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "[") {
			inTable = false
		}
		if !inTable {
			kept = append(kept, line)
		}
	}

	out := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if ep != nil {
		// http_headers is a documented map of static header values Codex
		// checks before its OAuth fallback engages, so a static Authorization
		// needs no environment variable. The inline `bearer_token` field is a
		// different thing and is rejected by Codex, and `bearer_token_env_var`
		// is the other documented route and would need a variable the service
		// has no way to set in a host's environment.
		out += "\n\n" + codexTableHeader + "\n" +
			`url = "` + ep.URL + `"` + "\n" +
			`http_headers = { Authorization = "Bearer ` + ep.Token + `" }`
	}
	out += "\n"

	// One header, and no more. A header carrying a trailing comment does not
	// match the exact-line test above, so it survives the removal and the new
	// table is appended beside it - two tables with the same name, which is a
	// TOML parse error that takes the whole of Codex's configuration with it.
	// The same count catches the other shape a line splice cannot read: a
	// header-looking line inside a multi-line string, where the splice eats
	// from there to the next bracket and silently loses whatever was between.
	// Refusing is the answer rather than adding a TOML parser: it turns silent
	// data loss into a message a person can act on.
	want := 0
	if ep != nil {
		want = 1
	}
	if n := strings.Count(out, codexTableHeader); n != want {
		return "", fmt.Errorf("host: %s appears %d times after the splice, want %d - this file has "+
			"one somewhere a line splice cannot read, such as a trailing comment on the header or a "+
			"header-shaped line inside a multi-line string. it needs fixing by hand",
			codexTableHeader, n, want)
	}
	return out, nil
}

// headerIndex is the byte offset of the first line that is exactly our table
// header, or -1. It is the same test the splice loop makes, so the parity check
// above asks about the line the loop would actually act on.
func headerIndex(text string) int {
	offset := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == codexTableHeader {
			return offset
		}
		offset += len(line) + 1
	}
	return -1
}

// codexTableHeader is the one line the splice recognises.
const codexTableHeader = "[mcp_servers." + MCPName + "]"
