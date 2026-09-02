package host

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// MaxHostConfig bounds how much of a host configuration is read.
//
// ~/.claude.json is Claude Code's own state file rather than a static
// configuration - it holds per-project history alongside the MCP entries - so
// it is not a small file and nothing bounds how large it gets. 16 MiB is far
// past any observed size and is a bound rather than a budget: a file over it is
// reported as unreadable instead of being half-searched, because a substring
// that is not in the first 16 MiB is indistinguishable from one that is not
// there at all.
const MaxHostConfig = 16 << 20

// ReadCapped reads the file at path whole, and refuses one over cap bytes
// rather than reading part of it.
func ReadCapped(path string, cap int64) (string, error) {
	//nolint:gosec // G304: path is a host configuration this product resolved
	// from the home directory and constants. No part of it is input.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > cap {
		return "", fmt.Errorf("%s is %d bytes, over the %d this reads", path, info.Size(), cap)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// PointsAtEndpoint reports whether the host configuration at path names url,
// which is what "this host is registered against this service" means to both
// `doctor` and `install` (backlog 35): a substring search and not a parser,
// because the URL is a string this product publishes and no shape a host did
// not write can produce it by accident.
//
// An empty path, or a file that does not exist, is "no" and not an error - a
// host that has never been configured is a host to register. A file that
// cannot be read is an error, and the caller decides what to do about it.
func PointsAtEndpoint(path, url string) (bool, error) {
	if path == "" {
		return false, nil
	}
	text, err := ReadCapped(path, MaxHostConfig)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(text, url), nil
}
