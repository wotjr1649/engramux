package service

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/store"
)

// getEvent answers a [ipc.GetEvent] request (spec 5.2, 5.9) and is the third
// place I-10 has to hold.
//
// The shape is [searchEvents]'s: internal/store hands back what is stored, this
// function decides what leaves. The difference is what it hands out - a whole
// payload rather than a window of one - which is why the bound below is a
// measured number rather than a chosen one.
//
// The project is resolved before the query and refused if it is a shape that
// must not be walked, so nothing a caller sends can put the single connection
// behind a stat of a host that is down (spec 5.9). It is resolved with
// [project.FromArgument] and not [project.Identify]: Identify never fails,
// which is right for ingest and wrong for a question.
func getEvent(ctx context.Context, db *sql.DB, req ipc.GetEventRequest) (ipc.GetEventReply, error) {
	if err := req.Validate(); err != nil {
		return ipc.GetEventReply{}, err
	}
	p, err := project.FromArgument(req.Project)
	if err != nil {
		return ipc.GetEventReply{}, err
	}

	e, err := store.GetEvent(ctx, db, req.ID, p.ID)
	if err != nil {
		return ipc.GetEventReply{}, err
	}
	if e == nil {
		// No such event *in that project*, which is the same answer as
		// no such event anywhere. See store.GetEvent for why the two
		// are not distinguished.
		return ipc.GetEventReply{}, nil
	}

	masked := secret.Mask(e.Payload)
	doc := ipc.EventDocument{
		ID:   e.ID,
		Host: e.Host,
		// The same untrusted column a hit carries, masked and then
		// bounded for the same reasons - see [searchEvents].
		EventName:    truncateRunes(secret.MaskString(e.EventName), maxEventNameRunes),
		SessionID:    secret.MaskString(e.SessionID),
		ReceivedAtMS: e.ReceivedAtMS,
		PrivacyClass: e.PrivacyClass,
		PayloadBytes: len(masked),
	}
	// Over the bound the payload is left out rather than cut. PayloadBytes
	// is set either way, so "too large" is distinguishable from "no such
	// event", which answers a nil Event instead.
	if len(masked) <= ipc.MaxEventPayloadBytes {
		doc.Payload = asJSON(masked)
	}
	return ipc.GetEventReply{Event: &doc}, nil
}

// asJSON returns b as a JSON value, whatever b is.
//
// A masked payload is valid JSON for every payload the relay can send -
// encodeEnvelope refuses one that is not a JSON document, and internal/secret
// re-encodes what it decoded - and it is spliced in verbatim in that case, so a
// document is not escaped into a string and roughly doubled.
//
// The other branch is not dead code waiting for a caller: internal/secret falls
// back to scanning text when a payload does not parse, and a payload can reach
// the database through the spool without passing encodeEnvelope. Wrapping it as
// a JSON string is what keeps the reply decodable whatever the row holds.
func asJSON(b []byte) json.RawMessage {
	if json.Valid(b) {
		return b
	}
	s, err := json.Marshal(string(b))
	if err != nil {
		// Unreachable: Marshal of a string fails only on invalid UTF-8,
		// which it replaces rather than refuses. A JSON null is still a
		// value a decoder accepts, and PayloadBytes still says how much
		// there was.
		return json.RawMessage("null")
	}
	return s
}

// listSessions answers a [ipc.ListSessions] request (spec 5.2, 5.9).
//
// projects.root is the field this exists to mask: it is a normalised worktree
// root, which is the exact shape internal/secret's user-path class matches in
// 900 of 902 captures (spec 6.1).
//
// The root comes from resolving the caller's own argument rather than from a
// SELECT on projects. It is the same string either way - internal/store stores
// what project.Identify normalised - and this way a project with no rows yet
// answers "no sessions" instead of "no such project". Those are the same state:
// a project is created by ingest.
//
// ponytail: a session id is untrusted width, because host_session_id is
// whatever a payload said, and nothing truncates it. The ceiling is
// ipc.MaxFrameLen, at which point WriteFrame refuses and the caller reports a
// failed read - the same ceiling the cell breakdown has. Truncating is what an
// event name gets and an id must not: a shortened name is still readable and a
// shortened id is not an id.
func listSessions(ctx context.Context, db *sql.DB, req ipc.ListSessionsRequest) (ipc.ListSessionsReply, error) {
	if req.Project == "" {
		return ipc.ListSessionsReply{}, ipc.ErrNoProject
	}
	limit, err := req.EffectiveLimit()
	if err != nil {
		return ipc.ListSessionsReply{}, err
	}
	p, err := project.FromArgument(req.Project)
	if err != nil {
		return ipc.ListSessionsReply{}, err
	}

	rows, err := store.Sessions(ctx, db, p.ID, limit)
	if err != nil {
		return ipc.ListSessionsReply{}, err
	}
	out := make([]ipc.Session, len(rows))
	for i, s := range rows {
		out[i] = ipc.Session{
			ID:            secret.MaskString(s.ID),
			Host:          s.Host,
			HostSessionID: secret.MaskString(s.HostSessionID),
			Status:        s.Status,
			CreatedAtMS:   s.CreatedAtMS,
			EndedAtMS:     s.EndedAtMS,
		}
	}
	return ipc.ListSessionsReply{ProjectRoot: secret.MaskString(p.Root), Sessions: out}, nil
}
