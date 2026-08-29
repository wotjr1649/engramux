package ipc

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestSearchReplyVerify_Accepts is the one shape Verify lets through.
func TestSearchReplyVerify_Accepts(t *testing.T) {
	reply := SearchReply{Version: Version, Type: Search}
	if err := reply.Verify(); err != nil {
		t.Errorf("Verify() = %v, want nil", err)
	}
}

// TestSearchReplyVerify_Rejects covers the two failure modes with their own
// sentinels. The rejected-ACK row is the one that matters: it is the document
// the service answers when it will not serve a Search, and without the type
// discriminator it decodes into a SearchReply of zeroes that the CLI would
// print as "no results" - a wrong answer that looks like a right one.
func TestSearchReplyVerify_Rejects(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply SearchReply
		want  error
	}{
		{"a version this build does not speak", SearchReply{Version: "0", Type: Search}, ErrSearchVersion},
		{"a status reply", SearchReply{Version: Version, Type: Status}, ErrSearchType},
		{"a reply with no type at all", SearchReply{Version: Version}, ErrSearchType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.reply.Verify(); !errors.Is(err, tc.want) {
				t.Errorf("Verify() = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestARejectedAckIsNotASearchReply is the same check from the wire's side: an
// Ack's bytes decode into a SearchReply without error, and Verify is the only
// thing that tells them apart.
func TestARejectedAckIsNotASearchReply(t *testing.T) {
	raw, err := json.Marshal(Ack{Version: Version, Status: Rejected})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var reply SearchReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("an ACK did not decode as a SearchReply at all: %v", err)
	}
	if len(reply.Hits) != 0 {
		t.Fatalf("Hits = %v, want none", reply.Hits)
	}
	if err := reply.Verify(); !errors.Is(err, ErrSearchType) {
		t.Errorf("Verify() on a rejected ACK = %v, want ErrSearchType", err)
	}
}

// TestSearchRequestEffectiveLimit pins all four bands, by value.
func TestSearchRequestEffectiveLimit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		limit   int
		want    int
		wantErr error
	}{
		{name: "unset means the default", limit: 0, want: DefaultSearchLimit},
		{name: "a limit inside the cap is its own", limit: 7, want: 7},
		{name: "the cap itself", limit: MaxSearchLimit, want: MaxSearchLimit},
		{name: "over the cap is clamped", limit: MaxSearchLimit + 1, want: MaxSearchLimit},
		{name: "negative is refused", limit: -1, wantErr: ErrSearchLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SearchRequest{Query: "anything", Limit: tc.limit}.EffectiveLimit()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("EffectiveLimit() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if got != tc.want {
				t.Errorf("EffectiveLimit() = %d, want %d", got, tc.want)
			}
		})
	}
}
