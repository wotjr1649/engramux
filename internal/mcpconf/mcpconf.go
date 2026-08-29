// Package mcpconf owns spec 5.6's mcp.json: the endpoint the service published
// and the bearer token a client needs to reach it.
//
// # Why it is its own package
//
// Two binaries have to touch this file and neither can import the other's
// world. The service writes it (spec 5.9 gives it the port and the token, and
// gives the installer neither). `engramux doctor` reads it - and that binary is
// the hook relay as well as the CLI (spec 5.1), so it cannot import
// internal/service, which links the SQLite driver through internal/store and
// put 4 MiB into a process spawned once per hook event. Everything here is
// encoding/json, net/url and os.
//
// # Nothing here reads the token back
//
// The service mints a token per start and writes it; it never needs to read
// one. The installer reads the file to write a host configuration, and the
// installer is not this package. So the only reader here is [URL], which
// decodes the endpoint and stops - the token is not a value callers are asked
// not to print, it is a value they cannot obtain. That is spec 6.1's rule about
// a secret made structural rather than editorial.
package mcpconf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

// Name is the file spec 5.6 assigns to the service, under the same directory as
// the database.
const Name = "mcp.json"

// Path is where [Name] lives under dir.
func Path(dir string) string { return filepath.Join(dir, Name) }

// URL is the endpoint the last service start published, or "" with a nil error
// when the file does not exist - which is what a service that never started, or
// one that could not bind, leaves behind.
//
// It decodes the endpoint alone. The document also holds the bearer token, and
// the struct below has no field for it, so no caller of this package can be
// handed one.
func URL(dir string) (string, error) {
	b, err := os.ReadFile(Path(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("mcpconf: read %s: %w", Path(dir), err)
	}
	var doc struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return "", fmt.Errorf("mcpconf: decode %s: %w", Path(dir), err)
	}
	return doc.URL, nil
}

// Port is the TCP port endpoint names, or 0 when endpoint is empty or names
// none. It is what makes the port sticky (spec 5.9): the next start reuses it,
// and falls back to letting Windows choose when that bind fails.
//
// An endpoint this cannot parse answers 0 rather than an error, because the
// caller's next move is the same either way - bind port 0 - and a stale or
// hand-edited file is a state the service starts through, not one it stops for.
func Port(endpoint string) int {
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil || p < 1 || p > 65535 {
		return 0
	}
	return p
}

// Write publishes endpoint and token, replacing whatever was there.
//
// Temporary file, fsync, atomic rename - spec 5.6's rule for every file this
// product writes, and here it is what keeps a host from reading half a document
// while the service is starting.
//
// The 0o600 mode is advisory on Windows: the file inherits the directory's ACL
// and Go's mode does not narrow it. Spec 5.9 records what that leaves resting on
// the bearer token. It is set anyway because it costs nothing and is the right
// mode wherever it is not advisory.
func Write(dir, endpoint, token string) error {
	b, err := json.Marshal(struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}{URL: endpoint, Token: token})
	if err != nil {
		// Unreachable for two strings, and still not swallowed: the
		// token would otherwise be missing with nothing said.
		return fmt.Errorf("mcpconf: encode %s: %w", Name, err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mcpconf: create %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, Name+".*")
	if err != nil {
		return fmt.Errorf("mcpconf: create a temp %s in %s: %w", Name, dir, err)
	}
	tmp := f.Name()
	// Every failure below removes the temp file. It carries the token, so a
	// leftover is not merely untidy - it is a second copy of a secret under
	// a name nothing will ever replace.
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if err = f.Chmod(0o600); err != nil {
		return fmt.Errorf("mcpconf: restrict %s: %w", tmp, err)
	}
	if _, err = f.Write(b); err != nil {
		return fmt.Errorf("mcpconf: write %s: %w", tmp, err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("mcpconf: sync %s: %w", tmp, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("mcpconf: close %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, Path(dir)); err != nil {
		return fmt.Errorf("mcpconf: rename %s: %w", tmp, err)
	}
	return nil
}
