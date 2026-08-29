package ipc_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestGetEventRequestValidateNamesTheReason. Every refusal is the request being
// unanswerable, which is a different thing from the event not existing - that is
// a reply with a nil event. The errors are distinguishable because "you sent no
// id" and "you sent no project" are different things for a caller to fix.
func TestGetEventRequestValidateNamesTheReason(t *testing.T) {
	const project = `D:\work\repo`
	const id = "0192f0c0-0000-7000-8000-000000000001"

	for _, tc := range []struct {
		name string
		req  ipc.GetEventRequest
		want error
	}{
		{"both", ipc.GetEventRequest{ID: id, Project: project}, nil},
		{"no id", ipc.GetEventRequest{Project: project}, ipc.ErrNoEventID},
		{"no project", ipc.GetEventRequest{ID: id}, ipc.ErrNoProject},
		{"neither", ipc.GetEventRequest{}, ipc.ErrNoEventID},
		{
			"an id at the cap",
			ipc.GetEventRequest{ID: strings.Repeat("a", ipc.MaxEventIDBytes), Project: project},
			nil,
		},
		{
			"an id over the cap",
			ipc.GetEventRequest{ID: strings.Repeat("a", ipc.MaxEventIDBytes+1), Project: project},
			ipc.ErrEventIDLen,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.req.Validate(); !errors.Is(err, tc.want) {
				t.Errorf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestListSessionsLimitIsTheSearchLimit. The two share one pair of bounds
// deliberately (see [ipc.ListSessionsRequest.EffectiveLimit]), and this is what
// would catch them drifting apart into two numbers nobody kept in step.
func TestListSessionsLimitIsTheSearchLimit(t *testing.T) {
	for _, tc := range []struct {
		in      int
		want    int
		wantErr error
	}{
		{in: 0, want: ipc.DefaultSearchLimit},
		{in: 1, want: 1},
		{in: ipc.MaxSearchLimit, want: ipc.MaxSearchLimit},
		{in: ipc.MaxSearchLimit + 1, want: ipc.MaxSearchLimit},
		{in: -1, wantErr: ipc.ErrSearchLimit},
	} {
		got, err := ipc.ListSessionsRequest{Limit: tc.in}.EffectiveLimit()
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("EffectiveLimit(%d) error = %v, want %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("EffectiveLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestTheTwoNewRepliesRefuseWhatIsNotTheirs is what makes a nil event and an
// empty session list mean what they say. A rejected ACK - the document the
// service answers a request it will not serve - decodes into either struct
// without error and leaves both of those fields at their zero value, so Verify
// is the only thing standing between "the service refused" and "there is
// nothing there".
func TestTheTwoNewRepliesRefuseWhatIsNotTheirs(t *testing.T) {
	t.Run("get event", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			reply ipc.GetEventReply
			want  error
		}{
			{"its own", ipc.GetEventReply{Version: ipc.Version, Type: ipc.GetEvent}, nil},
			{"a wrong version", ipc.GetEventReply{Version: "v0", Type: ipc.GetEvent}, ipc.ErrGetEventVersion},
			{"a search reply", ipc.GetEventReply{Version: ipc.Version, Type: ipc.Search}, ipc.ErrGetEventType},
			{"an ack, which has neither field", ipc.GetEventReply{}, ipc.ErrGetEventVersion},
		} {
			if err := tc.reply.Verify(); !errors.Is(err, tc.want) {
				t.Errorf("%s: Verify = %v, want %v", tc.name, err, tc.want)
			}
		}
	})

	t.Run("list sessions", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			reply ipc.ListSessionsReply
			want  error
		}{
			{"its own", ipc.ListSessionsReply{Version: ipc.Version, Type: ipc.ListSessions}, nil},
			{"a wrong version", ipc.ListSessionsReply{Version: "v0", Type: ipc.ListSessions}, ipc.ErrListSessionsVersion},
			{"a status reply", ipc.ListSessionsReply{Version: ipc.Version, Type: ipc.Status}, ipc.ErrListSessionsType},
			{"an ack, which has neither field", ipc.ListSessionsReply{}, ipc.ErrListSessionsVersion},
		} {
			if err := tc.reply.Verify(); !errors.Is(err, tc.want) {
				t.Errorf("%s: Verify = %v, want %v", tc.name, err, tc.want)
			}
		}
	})
}
