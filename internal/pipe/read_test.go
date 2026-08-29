package pipe

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// The two request types Phase 5 adds (spec 5.9) share [SearchFunc]'s contract
// exactly, so they are held to the same four properties Search is: the reply
// document is the request type's, the protocol fields are the server's, a build
// with no handler refuses rather than answering emptily, and a handler that
// failed cannot produce an answer that reads as a real one.
//
// The two functions below are written out rather than shared through a generic
// helper. The types differ in their request document, their reply document and
// their Verify, which is three of the four things each assertion is about; a
// helper parameterised on all three would be longer than the duplication and
// would hide which document a failure was about.

// TestGetEventRoutesToItsOwnReply is those four properties for GetEvent.
func TestGetEventRoutesToItsOwnReply(t *testing.T) {
	// Built with a wrong version and a wrong type on purpose, so "Serve
	// stamps them" is asserted rather than inherited from a handler that
	// happened to fill them in.
	want := ipc.GetEventReply{
		Version: "not the wire version",
		Type:    "not a get-event reply",
		Event: &ipc.EventDocument{
			ID:           "0192f0c0-0000-7000-8000-000000000001",
			Host:         "codex",
			EventName:    "PostToolUse",
			SessionID:    "codex:s",
			ReceivedAtMS: 1700000000123,
			PrivacyClass: "user-path",
			Payload:      json.RawMessage(`{"hook_event_name":"PostToolUse"}`),
			PayloadBytes: 33,
		},
	}

	t.Run("served", func(t *testing.T) {
		var seen ipc.GetEventRequest
		name, _ := startHandler(t, Handler{
			Ingest: (&recorder{status: ipc.Committed}).ingest,
			GetEvent: func(_ context.Context, req ipc.GetEventRequest) (ipc.GetEventReply, error) {
				seen = req
				return want, nil
			},
		})

		raw := exchangeRaw(t, name, request(t, ipc.Version, ipc.GetEvent, "",
			[]byte(`{"id":"an-id","project":"D:\\work\\repo"}`)))
		var got ipc.GetEventReply
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		if err := got.Verify(); err != nil {
			t.Fatalf("Verify: %v", err)
		}
		// reflect.DeepEqual because the payload is a json.RawMessage,
		// which is a slice and so not comparable with ==. That is the
		// whole reason this document cannot be compared field by field
		// the way a session below can.
		if got.Event == nil || !reflect.DeepEqual(*got.Event, *want.Event) {
			t.Errorf("the reply is not the handler's event\n got %+v\nwant %+v", got.Event, want.Event)
		}
		if seen.ID != "an-id" || seen.Project != `D:\work\repo` {
			t.Errorf("the handler saw %+v, want the id and project that were sent", seen)
		}
		// A caller verifying it as an ACK must not accept it: the
		// relay's delivery decision rests on ipc.Ack.Verify and stays
		// exactly that whatever else travels here.
		var ack ipc.Ack
		if err := json.Unmarshal(raw, &ack); err != nil {
			t.Fatalf("decode as an ack: %v", err)
		}
		if err := ack.Verify(""); err == nil {
			t.Error("a get-event reply verified as a committed ACK")
		}
	})

	t.Run("no handler", func(t *testing.T) {
		name, _ := startServer(t, (&recorder{status: ipc.Committed}).ingest)
		raw := exchangeRaw(t, name, request(t, ipc.Version, ipc.GetEvent, "", []byte(`{"id":"x","project":"D:\\w"}`)))
		requireRejected(t, raw)

		var reply ipc.GetEventReply
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("decode as a get-event reply: %v", err)
		}
		if err := reply.Verify(); !errors.Is(err, ipc.ErrGetEventType) {
			t.Errorf("GetEventReply.Verify = %v, want ErrGetEventType", err)
		}
	})

	t.Run("failing handler", func(t *testing.T) {
		name, _ := startHandler(t, Handler{
			Ingest: (&recorder{status: ipc.Committed}).ingest,
			GetEvent: func(context.Context, ipc.GetEventRequest) (ipc.GetEventReply, error) {
				return ipc.GetEventReply{}, errors.New("the project is not absolute")
			},
		})
		requireRejected(t, exchangeRaw(t, name, request(t, ipc.Version, ipc.GetEvent, "", []byte(`{"id":"x","project":"w"}`))))
	})

	t.Run("undecodable payload", func(t *testing.T) {
		called := false
		name, _ := startHandler(t, Handler{
			Ingest: (&recorder{status: ipc.Committed}).ingest,
			GetEvent: func(context.Context, ipc.GetEventRequest) (ipc.GetEventReply, error) {
				called = true
				return ipc.GetEventReply{}, nil
			},
		})
		// A JSON array is a valid envelope payload and not a request
		// document. Without the decode at the routing boundary it would
		// become a zero request instead of a refusal.
		requireRejected(t, exchangeRaw(t, name, request(t, ipc.Version, ipc.GetEvent, "", []byte(`[1,2,3]`))))
		if called {
			t.Error("the handler was called with a payload that did not decode")
		}
	})
}

// TestListSessionsRoutesToItsOwnReply is the same four properties for
// ListSessions.
func TestListSessionsRoutesToItsOwnReply(t *testing.T) {
	want := ipc.ListSessionsReply{
		Version:     "not the wire version",
		Type:        "not a list-sessions reply",
		ProjectRoot: `d:\users\[redacted-user-path]\repo`,
		Sessions: []ipc.Session{
			{ID: "codex:s1", Host: "codex", HostSessionID: "s1", Status: "ended", CreatedAtMS: 1, EndedAtMS: 2},
			// The empty host session id is the shape a payload with
			// no session_id produces (I-04), and it has to survive
			// the wire as itself rather than as an omitted field.
			{ID: "unknown:", Host: "unknown", HostSessionID: "", Status: "active", CreatedAtMS: 3},
		},
	}

	t.Run("served", func(t *testing.T) {
		var seen ipc.ListSessionsRequest
		name, _ := startHandler(t, Handler{
			Ingest: (&recorder{status: ipc.Committed}).ingest,
			ListSessions: func(_ context.Context, req ipc.ListSessionsRequest) (ipc.ListSessionsReply, error) {
				seen = req
				return want, nil
			},
		})

		raw := exchangeRaw(t, name, request(t, ipc.Version, ipc.ListSessions, "",
			[]byte(`{"project":"D:\\work\\repo","limit":5}`)))
		var got ipc.ListSessionsReply
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		if err := got.Verify(); err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if got.ProjectRoot != want.ProjectRoot {
			t.Errorf("project_root = %q, want %q", got.ProjectRoot, want.ProjectRoot)
		}
		if len(got.Sessions) != len(want.Sessions) {
			t.Fatalf("%d sessions, want %d", len(got.Sessions), len(want.Sessions))
		}
		for i := range got.Sessions {
			if got.Sessions[i] != want.Sessions[i] {
				t.Errorf("session %d = %+v, want %+v", i, got.Sessions[i], want.Sessions[i])
			}
		}
		if seen.Project != `D:\work\repo` || seen.Limit != 5 {
			t.Errorf("the handler saw %+v, want the project and limit that were sent", seen)
		}
	})

	t.Run("no handler", func(t *testing.T) {
		name, _ := startServer(t, (&recorder{status: ipc.Committed}).ingest)
		raw := exchangeRaw(t, name, request(t, ipc.Version, ipc.ListSessions, "", []byte(`{"project":"D:\\w"}`)))
		requireRejected(t, raw)

		var reply ipc.ListSessionsReply
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("decode as a list-sessions reply: %v", err)
		}
		if err := reply.Verify(); !errors.Is(err, ipc.ErrListSessionsType) {
			t.Errorf("ListSessionsReply.Verify = %v, want ErrListSessionsType", err)
		}
	})

	t.Run("failing handler", func(t *testing.T) {
		name, _ := startHandler(t, Handler{
			Ingest: (&recorder{status: ipc.Committed}).ingest,
			ListSessions: func(context.Context, ipc.ListSessionsRequest) (ipc.ListSessionsReply, error) {
				return ipc.ListSessionsReply{}, errors.New("the project is a UNC path")
			},
		})
		requireRejected(t, exchangeRaw(t, name, request(t, ipc.Version, ipc.ListSessions, "", []byte(`{"project":"\\\\host\\share"}`))))
	})

	t.Run("undecodable payload", func(t *testing.T) {
		called := false
		name, _ := startHandler(t, Handler{
			Ingest: (&recorder{status: ipc.Committed}).ingest,
			ListSessions: func(context.Context, ipc.ListSessionsRequest) (ipc.ListSessionsReply, error) {
				called = true
				return ipc.ListSessionsReply{}, nil
			},
		})
		requireRejected(t, exchangeRaw(t, name, request(t, ipc.Version, ipc.ListSessions, "", []byte(`"a string"`))))
		if called {
			t.Error("the handler was called with a payload that did not decode")
		}
	})
}

// requireRejected holds that raw is a rejected ACK - the refusal every path
// that will not serve a request answers with, and the one ipc.Ack.Verify
// cannot accept.
func requireRejected(t *testing.T, raw []byte) {
	t.Helper()
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if ack.Status != ipc.Rejected {
		t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
	}
	if err := ack.Verify(""); !errors.Is(err, ipc.ErrAckRejected) {
		t.Errorf("Verify accepted the refusal: %v", err)
	}
}
