package host

import (
	"encoding/json"
	"errors"
	"fmt"
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
	return out + "\n", nil
}

// codexTableHeader is the one line the splice recognises.
const codexTableHeader = "[mcp_servers." + MCPName + "]"
