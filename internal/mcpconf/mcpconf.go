// Package mcpconf owns spec 5.6's mcp.json: the endpoint the service published
// and the bearer token a client needs to reach it.
//
// # Why it is its own package
//
// Two binaries have to touch this file and neither can import the other's
// world. The service writes it (spec 5.9 gives it the port and the token, and
// gives the installer neither). `engramux doctor` reads it, and that binary is
// the hook relay as well as the CLI (spec 5.1).
//
// This paragraph used to end by saying the relay must not link the SQLite
// driver, and that everything here is encoding/json, net/url and os. **Measured
// 2026-09-04, both halves are false**: `go list -deps ./cmd/engramux` reports
// modernc.org/sqlite and goose, because cmd/engramux's own doctor.go and
// inject.go import internal/inject, which reaches internal/store. The relay is
// 8,703,488 B against the 3,862,528 B spec 7.1 records. That is a regression of
// its own and is carried as one; what it settles here is that a size argument
// cannot be why this package is separate, and must not be what licenses or
// refuses an import in it. The reason below can.
//
// # Nothing here reads the token back
//
// The only reader here is [URL], which decodes the endpoint and stops. There is
// no field for a token on the way in, so no caller of this package can be
// handed one - spec 6.1's rule about a secret, made structural rather than
// editorial.
//
// The service does read the token back: the token is sticky across a restart,
// or the static header in each host's configuration would be stale from the
// next logon onwards. That reader is in internal/mcpserver rather than here,
// and the difference is what keeps the property. `engramux doctor` reads this
// file and lives in the relay binary; that binary does not link
// internal/mcpserver, which pulls in the MCP SDK, so it cannot reach a token by
// any route rather than being trusted not to print one.
package mcpconf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wotjr1649/engramux/internal/winacl"
)

// Name is the file spec 5.6 assigns to the service, under the same directory as
// the database.
const Name = "mcp.json"

// Path is where [Name] lives under dir.
func Path(dir string) string { return filepath.Join(dir, Name) }

// tempInfix is what [Write]'s temporary file is named with, and the only reason
// it is spelled out rather than left as os.CreateTemp's usual `Name+".*"` is
// that [staleTemps] has to glob for it. `mcp.json.*` would also match a copy the
// user made by hand beside the real file, and a sweep that removes a credential
// must not be able to remove anything else. It is `internal/host`'s spelling on
// purpose: the two files are swept by the same policy.
const tempInfix = ".engramux-tmp-"

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

// staleTemps removes temporary files an earlier [Write] left behind when it was
// killed between its write and its rename. Best effort and silent: a leftover it
// cannot remove is not something this run should fail over, and publishing the
// endpoint is the caller's actual business.
//
// It is backlog 43, and the asymmetry that row records is worth keeping in view
// here rather than only there. `internal/host.writeAtomic` has had this sweep
// since it was written, and its temporary file carries a bearer token *inside
// another product's configuration*; this one carries the token on its own, two
// fields and nothing else, and had no sweep at all. Since the DACL landed
// (backlog 28) a leftover here is at least narrowed to SYSTEM, Administrators
// and this user - [winacl.Restrict] runs before the first write, so a file that
// exists at all has been through it - but narrowed is not absent, the token is
// sticky across restarts, and os.CreateTemp's random suffix means nothing will
// ever replace the copy.
//
// The two packages still do not share code, for the reason writeAtomic's own
// comment gives: what differs between them is policy, not sequence. This is the
// policy arriving on the side that needed it more.
func staleTemps(dir string) {
	matches, err := filepath.Glob(Path(dir) + tempInfix + "*")
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// Write publishes endpoint and token, replacing whatever was there.
//
// Temporary file, fsync, atomic rename - spec 5.6's rule for every file this
// product writes, and here it is what keeps a host from reading half a document
// while the service is starting.
//
// The DACL is what narrows it, and [winacl.Restrict] is called on the temporary
// file before one byte of the token is written to it: the file exists under the
// directory's inherited ACL only while it is empty. Backlog 28 and memory spec
// §8's second publication condition are what that closes for this file - the two
// host configuration files carry a copy of the same token and are not this
// product's to narrow, so `doctor` reports them instead.
//
// The 0o600 mode stays and does nothing here. On Windows Go's mode is derived
// from FILE_ATTRIBUTE_READONLY and cannot express a DACL, which is precisely why
// spec 7.1 measured this file at -rw-rw-rw- with every ACE inherited. It is set
// because it is the right mode wherever it is not advisory, and it is no longer
// what this function relies on.
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
	staleTemps(dir)

	f, err := os.CreateTemp(dir, Name+tempInfix+"*")
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
	// Before the first write, so the token never sits on disk under an ACL
	// this has not replaced. The rename below carries the DACL with the
	// file: os.Rename is MoveFileEx without MOVEFILE_COPY_ALLOWED, the
	// temporary file is in the destination's own directory, and a
	// same-volume move takes the security descriptor along.
	if err = winacl.Restrict(tmp); err != nil {
		return fmt.Errorf("mcpconf: %w", err)
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
