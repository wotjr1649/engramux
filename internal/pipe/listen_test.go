package pipe

import (
	"context"
	"errors"
	"net"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/ipc/ipctest"
)

// currentSID is the SID every test builds its DACL from. A fabricated SID is
// not usable here: ConvertStringSecurityDescriptorToSecurityDescriptorW
// resolves the ACE's SID, so an invented one fails to parse and the test
// would prove nothing about the listener.
func currentSID(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	return u.Uid
}

// testSID is the stand-in this binary hashes its pipe names from: unique to
// the test, and to the process, so no other test here and no concurrently
// running copy of this binary derives the same name.
func testSID(t *testing.T) string {
	t.Helper()
	return ipctest.SID(t)
}

// uniquePipeName derives a pipe name no other test in this binary uses, and
// no concurrently running copy of this binary uses either.
func uniquePipeName(t *testing.T) string {
	t.Helper()
	return ipc.PipeName(testSID(t))
}

// useTestPipeName points ipc.CurrentPipeName - and so ListenCurrent, and any
// child this test launches with the environment it inherits - at the same
// name uniquePipeName would give. It is what lets a test exercise the derived
// name while a development service holds the real one.
func useTestPipeName(t *testing.T) {
	t.Helper()
	t.Setenv(ipc.TestPipeSIDEnv, testSID(t))
}

// listen opens a listener on a name unique to t and closes it when t ends.
func listen(t *testing.T) (net.Listener, string) {
	t.Helper()
	name := uniquePipeName(t)
	l, err := Listen(name, currentSID(t))
	if err != nil {
		t.Fatalf("Listen(%s): %v", name, err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})
	return l, name
}

// TestSecondListenerOnTheSameNameIsRefused is I-09's test. The singleton is
// not enforced by a mutex or a lock file - it is enforced by the fact that
// the second process to create a pipe with a given name loses, so this test
// is the whole enforcement mechanism.
//
// The assertion is deliberately not "an error came back". A typo in the
// name, a malformed SDDL and a refused name all return non-nil, and only one
// of them is I-09. Three separate things are asserted, each of which fails on
// its own if the mechanism is not what it claims:
//
//   - the error resolves to a *os.PathError naming the same path the first
//     listener took, so the failure is about that name and not another;
//   - the errno is exactly ERROR_ACCESS_DENIED, which is what the FILE_CREATE
//     disposition returns for an existing pipe. ERROR_INVALID_NAME (a
//     malformed name) and ERROR_ALREADY_EXISTS are both different values and
//     both fail this;
//   - the first listener is still usable afterwards, so the loser did not
//     disturb the winner.
func TestSecondListenerOnTheSameNameIsRefused(t *testing.T) {
	// The one test that reuses a name rather than minting a fresh one: it
	// opens ONE unique name TWICE, which is the condition under test.
	first, name := listen(t)

	second, err := Listen(name, currentSID(t))
	if err == nil {
		if cerr := second.Close(); cerr != nil {
			t.Errorf("close the second listener: %v", cerr)
		}
		t.Fatalf("Listen(%s) succeeded twice; I-09 requires exactly one winner", name)
	}

	// Logged, not just asserted: "the exact error a second listener gets" is
	// the sort of claim a document should not have to be trusted for.
	t.Logf("second Listen on %s: %v", name, err)

	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("second Listen error is %T (%v), want a *os.PathError", err, err)
	}
	if pathErr.Path != name {
		t.Errorf("refused path = %q, want %q; the error is about a different name", pathErr.Path, name)
	}
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		t.Errorf("second Listen errno = %v, want ERROR_ACCESS_DENIED (%d); "+
			"an error alone does not prove the name was taken",
			pathErr.Err, uintptr(syscall.ERROR_ACCESS_DENIED))
	}

	if first.Addr().String() != name {
		t.Errorf("first listener Addr = %q, want %q", first.Addr().String(), name)
	}
}

// TestDifferentNamesBothSucceed is the control for the test above. Without
// it, "the second Listen failed" could as easily mean Listen fails the second
// time it is called for any reason at all.
func TestDifferentNamesBothSucceed(t *testing.T) {
	sid := currentSID(t)
	for i := range 2 {
		name := ipc.PipeName(ipctest.SID(t) + "-" + strconv.Itoa(i))
		l, err := Listen(name, sid)
		if err != nil {
			t.Fatalf("Listen(%s): %v", name, err)
		}
		if err := l.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}
}

// TestSecurityDescriptorIsTheSpecDACL pins the exact SDDL string, because
// every character in it is a decision and none of them is observable from a
// listener that merely opened.
func TestSecurityDescriptorIsTheSpecDACL(t *testing.T) {
	const sid = "S-1-5-21-1111111111-2222222222-3333333333-1001"
	const want = "D:P(A;;GA;;;SY)(A;;GA;;;S-1-5-21-1111111111-2222222222-3333333333-1001)"
	if got := securityDescriptor(sid); got != want {
		t.Errorf("securityDescriptor(%q) =\n got %q\nwant %q", sid, got, want)
	}
}

// TestTheDACLIsActuallyApplied proves the SDDL reaches the pipe, which
// TestSecurityDescriptorIsTheSpecDACL above cannot: that one pins the string
// securityDescriptor builds, and it stays green if Listen never passes it to
// winio at all. Deleting the SecurityDescriptor field was measured to leave
// every other test in this package passing.
//
// The lever is a DACL that grants the current user nothing. Listening as
// SYSTEM alone makes both ACEs (A;;GA;;;SY), so a dial from this process -
// which is not SYSTEM - has to be refused by the DACL. With the descriptor
// dropped, go-winio falls back to RtlDefaultNpAcl, the same user connects,
// and this test fails.
//
// It reads nothing back, so it needs no security API beyond what go-winio
// already exposes: the observable is whether a dial succeeds.
func TestTheDACLIsActuallyApplied(t *testing.T) {
	const systemSID = "S-1-5-18"
	if currentSID(t) == systemSID {
		t.Skip("running as SYSTEM, which this test needs to be locked out of")
	}

	name := uniquePipeName(t)
	l, err := Listen(name, systemSID)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	go func() {
		if c, err := l.Accept(); err == nil {
			_ = c.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, name)
	if err == nil {
		if cerr := conn.Close(); cerr != nil {
			t.Errorf("close conn: %v", cerr)
		}
		t.Fatal("dialed a pipe whose DACL grants only SYSTEM; the SDDL is not reaching winio")
	}
	t.Logf("dial against a SYSTEM-only DACL: %v", err)
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		t.Errorf("dial failed with %v, want ERROR_ACCESS_DENIED (%d); "+
			"any other failure means this test is not measuring the DACL",
			err, uintptr(syscall.ERROR_ACCESS_DENIED))
	}
}

// TestOwnerSIDIsRefusedUnlessItIsASID guards the concatenation in
// securityDescriptor. The first case is the one that matters: it is a
// syntactically valid SID followed by a complete extra allow-everyone ACE,
// which is what SDDL injection looks like.
func TestOwnerSIDIsRefusedUnlessItIsASID(t *testing.T) {
	for _, sid := range []string{
		"S-1-5-18)(A;;GA;;;WD",
		"S-1-5-21-1;;",
		"S-1-0x20-1",
		"BA",
		"S-1-",
		"",
		"s-1-5-18",
	} {
		t.Run(sid, func(t *testing.T) {
			l, err := Listen(uniquePipeName(t), sid)
			if err == nil {
				if cerr := l.Close(); cerr != nil {
					t.Errorf("close: %v", cerr)
				}
				t.Fatalf("Listen accepted owner sid %q", sid)
			}
			if !errors.Is(err, errOwnerSID) {
				t.Errorf("Listen(%q) error = %v, want errOwnerSID", sid, err)
			}
		})
	}
}

// TestOwnerSIDAcceptsARealSID is the control for the test above: the refusal
// list means nothing if the accept list is empty.
func TestOwnerSIDAcceptsARealSID(t *testing.T) {
	sid := currentSID(t)
	if !strings.HasPrefix(sid, "S-1-") {
		t.Fatalf("user.Current().Uid = %q, which is not a SID; the rest of this package assumes it is", sid)
	}
	if !isDecimalSID(sid) {
		t.Errorf("isDecimalSID(%q) = false, want true", sid)
	}
}

// TestListenCurrentUsesTheDerivedName ties the two halves of the singleton
// together: the name ListenCurrent takes has to be the name ipc sends a relay
// to, or the service listens where nothing dials.
//
// It used to skip when a development service held the real name, which made
// the one assertion binding the two halves together the one assertion that
// did not run on the machine this is developed on. Under the override both
// halves move and neither is hardcoded, so the test measures the same thing
// it always claimed to and now measures it every run.
func TestListenCurrentUsesTheDerivedName(t *testing.T) {
	useTestPipeName(t)

	want, err := ipc.CurrentPipeName()
	if err != nil {
		t.Fatalf("ipc.CurrentPipeName: %v", err)
	}

	l, err := ListenCurrent()
	if err != nil {
		t.Fatalf("ListenCurrent: %v", err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()

	if got := l.Addr().String(); got != want {
		t.Errorf("ListenCurrent listens on %q, want ipc.CurrentPipeName() = %q", got, want)
	}
}
