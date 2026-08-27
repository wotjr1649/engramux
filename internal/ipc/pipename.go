package ipc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/user"
)

// pipeNamespace is the Windows local named-pipe namespace (spec 5.2).
const pipeNamespace = `\\.\pipe\`

// pipePrefix identifies the wire protocol, spelled exactly as spec 5.2
// spells it.
const pipePrefix = "engramux.v1"

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
func CurrentPipeName() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("ipc: current user: %w", err)
	}
	return PipeName(u.Uid), nil
}
