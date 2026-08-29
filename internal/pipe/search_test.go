package pipe

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// searchRequest builds a Search frame carrying payload as its request document.
func searchRequest(t *testing.T, payload string) []byte {
	t.Helper()
	return request(t, ipc.Version, ipc.Search, "", []byte(payload))
}

// TestSearchIsAnsweredWithASearchReply. The request document reaches the
// handler as it was sent, the handler's hits come back unchanged, and the two
// protocol fields are the server's: the reply below is built with a wrong
// version and a wrong type on purpose, so "Serve stamps them" is asserted
// rather than inherited from a handler that filled them in correctly.
func TestSearchIsAnsweredWithASearchReply(t *testing.T) {
	want := ipc.SearchReply{
		Version: "not the wire version",
		Type:    "not a search reply",
		Hits: []ipc.SearchHit{
			{
				ID:           "0192f0c0-0000-7000-8000-000000000001",
				Host:         "claude-code",
				EventName:    "PostToolUse",
				ReceivedAtMS: 1700000000123,
				Excerpt:      "the excerpt, with a [redacted-api-key] in it",
			},
			// The empty event name is the shape a payload with no
			// hook_event_name produces, and it has to survive the
			// wire as itself rather than as an omitted field.
			{ID: "0192f0c0-0000-7000-8000-000000000002", Host: "unknown", EventName: "", ReceivedAtMS: 1},
		},
	}

	var seen ipc.SearchRequest
	name, _ := startHandler(t, Handler{
		Ingest: (&recorder{status: ipc.Committed}).ingest,
		Search: func(_ context.Context, req ipc.SearchRequest) (ipc.SearchReply, error) {
			seen = req
			return want, nil
		},
	})

	raw := exchangeRaw(t, name, searchRequest(t, `{"query":"run-time budget","limit":7}`))
	var got ipc.SearchReply
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if err := got.Verify(); err != nil {
		t.Fatalf("Verify: %v (reply = %q)", err, raw)
	}
	if !slices.Equal(got.Hits, want.Hits) {
		t.Errorf("the reply is not the handler's hits\n got %+v\nwant %+v", got.Hits, want.Hits)
	}
	if seen.Query != "run-time budget" || seen.Limit != 7 {
		t.Errorf("the handler saw %+v, want the query and limit that were sent", seen)
	}
}

// TestASearchReplyIsNotAnAck is the other half of choosing a reply document per
// request type: a caller that verifies it as an ACK must not be able to accept
// it. ipc.Ack.Verify's three-way check is what the relay's delivery decision
// rests on (spec 5.3), and it stays exactly that whatever else travels here.
func TestASearchReplyIsNotAnAck(t *testing.T) {
	name, _ := startHandler(t, Handler{
		Ingest: (&recorder{status: ipc.Committed}).ingest,
		Search: func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
			return ipc.SearchReply{Hits: []ipc.SearchHit{{ID: "x"}}}, nil
		},
	})

	raw := exchangeRaw(t, name, searchRequest(t, `{"query":"anything"}`))
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if err := ack.Verify(""); err == nil {
		t.Errorf("a search reply verified as a committed ACK: %q", raw)
	}
}

// TestSearchWithoutAHandlerIsRejected. A build that does not serve Search
// refuses it exactly as the types nothing implements are refused, and the
// refusal is an ACK - which is not a search reply, so a client cannot read an
// empty hit list out of it and print "no results".
func TestSearchWithoutAHandlerIsRejected(t *testing.T) {
	name, _ := startServer(t, (&recorder{status: ipc.Committed}).ingest)

	raw := exchangeRaw(t, name, searchRequest(t, `{"query":"anything"}`))
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if ack.Status != ipc.Rejected {
		t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
	}
	var reply ipc.SearchReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("decode as a search reply: %v", err)
	}
	if err := reply.Verify(); !errors.Is(err, ipc.ErrSearchType) {
		t.Errorf("SearchReply.Verify = %v, want ErrSearchType", err)
	}
}

// TestAFailingSearchHandlerIsRejected. Half a search is worse than none: an
// empty hit list is a real answer - nothing matched - so a handler that could
// not answer must produce a refusal and not a reply that reads as one.
func TestAFailingSearchHandlerIsRejected(t *testing.T) {
	boom := errors.New("the query has too many tokens")
	name, _ := startHandler(t, Handler{
		Ingest: (&recorder{status: ipc.Committed}).ingest,
		Search: func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
			return ipc.SearchReply{Hits: []ipc.SearchHit{{ID: "leaked"}}}, boom
		},
	})

	raw := exchangeRaw(t, name, searchRequest(t, `{"query":"anything"}`))
	var reply ipc.SearchReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if err := reply.Verify(); err == nil {
		t.Fatalf("a failed search was answered with an acceptable search reply: %q", raw)
	}
	if len(reply.Hits) != 0 {
		t.Errorf("the failed handler's hits reached the wire: %q", raw)
	}
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("decode as an ack: %v", err)
	}
	if ack.Status != ipc.Rejected {
		t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
	}
}

// TestASearchPayloadThatDoesNotDecodeIsRejected. The envelope is well formed -
// validate accepts the type - and the document inside it is not, which is a
// case the envelope check cannot see and this is the only place that can.
// The handler must not be called with a zero request: an empty query would be
// refused downstream, but a limit of 0 would quietly mean the default.
func TestASearchPayloadThatDoesNotDecodeIsRejected(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
	}{
		{"a payload that is not an object", `"just a string"`},
		{"a limit that is not a number", `{"query":"x","limit":"7"}`},
		{"an array where the document should be", `[{"query":"x"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := 0
			name, _ := startHandler(t, Handler{
				Ingest: (&recorder{status: ipc.Committed}).ingest,
				Search: func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
					called++
					return ipc.SearchReply{}, nil
				},
			})

			ack := exchange(t, name, searchRequest(t, tc.payload))
			if ack.Status != ipc.Rejected {
				t.Errorf("status = %q, want %q", ack.Status, ipc.Rejected)
			}
			if called != 0 {
				t.Errorf("the handler was called %d times for a payload that does not decode", called)
			}
		})
	}
}
