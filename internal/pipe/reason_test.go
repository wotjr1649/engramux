package pipe

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestARefusedRequestCarriesAMaskedReason is backlog 27 on the wire. A
// handler's error becomes the rejected Ack's reason, masked on the way - the
// error names the database path, and the path names the user - and a request
// the routing boundary itself refuses says why as well. The reason is what a
// CLI prints and what a relay logs, so it goes through the same mask every
// other egress does (I-10).
func TestARefusedRequestCarriesAMaskedReason(t *testing.T) {
	sep := string(os.PathSeparator)
	dbPath := "C:" + sep + "Users" + sep + "someone" + sep + "AppData" + sep + "Local" + sep + "engramux" + sep + "engramux.db"
	name, _ := startHandler(t, Handler{
		Ingest: (&recorder{status: ipc.Committed}).ingest,
		Search: func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
			return ipc.SearchReply{}, errors.New("service: open " + dbPath + ": refused by the read gate")
		},
	})

	ack := exchange(t, name, request(t, ipc.Version, ipc.Search, "", []byte(`{"query":"anything"}`)))
	if ack.Status != ipc.Rejected {
		t.Fatalf("status = %q, want %q", ack.Status, ipc.Rejected)
	}
	if !strings.Contains(ack.Reason, "refused by the read gate") {
		t.Errorf("the reason does not carry the handler's error: %q", ack.Reason)
	}
	if strings.Contains(ack.Reason, "someone") {
		t.Errorf("the reason carries the user name out of the path: %q", ack.Reason)
	}

	// The routing boundary's own refusals say why too: a version this build
	// does not speak is the one a stale relay most needs to hear about.
	stale := exchange(t, name, request(t, "0", ipc.Status, "", []byte(`null`)))
	if stale.Status != ipc.Rejected {
		t.Fatalf("stale version: status = %q, want %q", stale.Status, ipc.Rejected)
	}
	if !strings.Contains(stale.Reason, "version") {
		t.Errorf("a version mismatch is refused without saying so: %q", stale.Reason)
	}
}
