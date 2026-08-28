package ipc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
)

// pipeNamespace is the Windows local named-pipe namespace (spec 5.2).
const pipeNamespace = `\\.\pipe\`

// pipePrefix identifies the wire protocol, spelled exactly as spec 5.2
// spells it.
const pipePrefix = "engramux.v1"

// TestPipeSIDEnv names the environment variable that stands in for the user
// SID when [CurrentPipeName] derives a name.
//
// It exists because the pipe name is derived from the SID and nothing else
// (spec 5.2), so a development service holding the real name locks every test
// that listens on it out of the machine it is developed on. A test sets this
// to a value of its own before it starts anything, and the binaries it
// launches inherit it, so both ends of the dial move together.
//
// What it is not:
//
//   - Not a security boundary. The DACL is still built from the real SID
//     (internal/pipe), the override never reaches it, and a process running as
//     this user is inside spec 2's trust boundary already - a name it chooses
//     for itself grants it nothing it did not have.
//   - Not a supported configuration. It is read here and nowhere else, no
//     shipped path sets it, and it is documented to no user.
//   - Not set in production. An empty value is the same as unset, so a child
//     that clears it rather than removing it still gets the real name.
const TestPipeSIDEnv = "ENGRAMUX_TEST_PIPE_SID"

// PipeName derives the fixed pipe name for a Windows user SID (spec 5.2):
// pipePrefix, then a hash of sid, under the local pipe namespace. It is
// pure and deterministic — the same sid always yields the same name, two
// different sids always yield different names — and takes a plain string so
// it is testable without being the current user.
//
// The name is fixed on purpose (spec 5.2): no per-start nonce, no discovery
// file. SHA-256 is used for even distribution and collision resistance
// across arbitrary SIDs, not for secrecy — the SID is not a secret, and the
// pipe name is visible to every process on the machine by design; the
// DACL granted when the pipe is listened on is what protects it, not the
// name being hard to guess.
func PipeName(sid string) string {
	sum := sha256.Sum256([]byte(sid))
	return pipeNamespace + pipePrefix + "-" + hex.EncodeToString(sum[:])
}

// CurrentPipeName returns PipeName for the current process's user. On
// Windows, os/user.Current().Uid is the user's SID.
//
// [TestPipeSIDEnv], when it is set to something, stands in for that SID as the
// input to the hash: the name keeps the shape spec 5.2 fixes and lands in a
// namespace no live service holds. Tests set it; nothing else does.
func CurrentPipeName() (string, error) {
	if sid := os.Getenv(TestPipeSIDEnv); sid != "" {
		return PipeName(sid), nil
	}
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("ipc: current user: %w", err)
	}
	return PipeName(u.Uid), nil
}
