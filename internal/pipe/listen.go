// Package pipe is the service side of Engramux's named pipe: it creates the
// listener that makes the service a singleton (I-09) and runs the accept loop
// that turns a frame into an ACK.
//
// It is deliberately not part of internal/ipc. internal/store imports ipc -
// store.Ingest takes an ipc.Envelope and answers with an ipc.AckStatus - so a
// go-winio listener living in ipc would pull the transport into the database
// package's dependency graph and mix the wire format with the thing that
// carries it. ipc stays pure data over an io.Reader and an io.Writer; this
// package owns the pipe.
//
// The same dependency direction is why [Serve] takes an [IngestFunc] rather
// than a database handle: ipc cannot import store, so the service supplies the
// seam between them.
package pipe

import (
	"errors"
	"fmt"
	"net"
	"os/user"
	"strings"

	"github.com/Microsoft/go-winio"

	"github.com/wotjr1649/engramux/internal/ipc"
)

var errOwnerSID = errors.New("pipe: owner sid is not a decimal SID string")

// securityDescriptor returns the pipe's DACL in SDDL form (spec 5.2): full
// control to SYSTEM and to ownerSID, with inheritance blocked.
//
// Reading it clause by clause:
//
//   - D: opens the DACL. There is no O:, G: or S: clause, because the spec
//     asks for a DACL and nothing else; an absent clause leaves the object's
//     default rather than asserting one this design has not thought about.
//   - P is SE_DACL_PROTECTED. It blocks inheritance, so an ACE granted on a
//     container above the pipe cannot widen this one.
//   - (A;;GA;;;SY) allows GENERIC_ALL - full control on a named pipe - to SY,
//     the well-known alias for NT AUTHORITY\SYSTEM.
//   - (A;;GA;;;<ownerSID>) allows the same to the user the service runs as,
//     which is the single SID spec 2 puts inside the trust boundary.
//
// Nothing is denied explicitly, and that is not an omission: a DACL that lists
// two allow ACEs and stops denies everyone else by construction. Adding
// (D;;GA;;;WD) would be worse than redundant - WD is Everyone, which contains
// SYSTEM, and a deny ACE placed ahead of the allow ACEs would win over both.
func securityDescriptor(ownerSID string) string {
	return "D:P(A;;GA;;;SY)(A;;GA;;;" + ownerSID + ")"
}

// isDecimalSID reports whether sid is a SID in the decimal string form
// os/user hands back on Windows: "S-1-" followed by digits and hyphens.
//
// This is not a courtesy check on an argument that is always right.
// [securityDescriptor] concatenates sid into an SDDL string, where "(", ")"
// and ";" are the ACE delimiters, so a sid carrying any of them could close
// the ACE it sits in and append one of its own - an allow-everyone ACE is 12
// characters. Restricting the charset removes that shape entirely instead of
// blacklisting the three characters that make it reachable today.
//
// The hexadecimal identifier-authority form ("S-1-0x...", for an authority
// above 2^32) is refused too. No account SID uses it, and refusing a SID this
// package has never seen is better than concatenating it.
func isDecimalSID(sid string) bool {
	rest, ok := strings.CutPrefix(sid, "S-1-")
	if !ok || rest == "" {
		return false
	}
	for _, r := range rest {
		if r != '-' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// Listen creates the named pipe at name, with the DACL for ownerSID, and
// returns a listener on it.
//
// This is where the singleton is enforced (I-09), and it is enforced by
// Windows rather than by anything in this repository: go-winio creates the
// first pipe instance with the FILE_CREATE disposition, whose contract is
// that the object must not already exist. A second call on a name a live
// listener already holds fails with ERROR_ACCESS_DENIED wrapped in an
// *os.PathError naming that path. No mutex, no lock file, and no discovery
// file - and no per-start nonce either, which is what rev.2 got wrong: thirty
// concurrent starts would have minted thirty distinct names and all thirty
// would have won.
//
// The pipe is left in byte mode. Message mode would impose a second framing
// on top of the length-prefixed frames ipc already defines, and the only
// behaviour it adds is CloseWrite, which nothing here uses.
//
// Listen is synchronous: when it returns, the pipe exists and a client may
// dial. That is what makes the listener/dial race a non-problem rather than
// something callers sleep through - [Serve] is a separate call, and a client
// that dials between the two waits in ERROR_PIPE_BUSY, which is the one
// condition winio.DialPipeContext retries.
func Listen(name, ownerSID string) (net.Listener, error) {
	if !isDecimalSID(ownerSID) {
		return nil, fmt.Errorf("%w: %.64q", errOwnerSID, ownerSID)
	}

	l, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: securityDescriptor(ownerSID),
	})
	if err != nil {
		return nil, fmt.Errorf("pipe: listen: %w", err)
	}
	return l, nil
}

// ListenCurrent is [Listen] on the pipe name and the DACL of the user this
// process runs as - the one listener the service ever creates.
//
// The name comes from ipc.CurrentPipeName rather than from a second copy of
// the derivation, because that is the function the relay dials with: two
// copies of one rule are two things to move, and a listener on a name nothing
// dials is invisible until an event is lost. ipc.TestPipeSIDEnv moves both
// ends at once for exactly that reason.
//
// The DACL is derived separately, from os/user.Current().Uid - on Windows the
// user's SID - and never from the name. So the ACE protecting the pipe is the
// real user's under every environment, and the override reaches nothing
// spec 5.2 puts inside the trust boundary.
func ListenCurrent() (net.Listener, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("pipe: current user: %w", err)
	}
	name, err := ipc.CurrentPipeName()
	if err != nil {
		// Wrapped rather than returned bare, so the message names the
		// layer the way the one above it does. Unreachable today - the
		// only failure inside is user.Current, which has already
		// succeeded here - which is why it is consistency and not a fix
		// (backlog 5).
		return nil, fmt.Errorf("pipe: derive the pipe name: %w", err)
	}
	return Listen(name, u.Uid)
}
