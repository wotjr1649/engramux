package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/store"
)

func sessionResume(ctx context.Context, db *sql.DB, req ipc.SessionResumeRequest) (ipc.SessionResumeReply, error) {
	var out ipc.SessionResumeReply
	if err := req.Validate(); err != nil {
		return out, err
	}
	p, err := project.FromArgument(req.Project)
	if err != nil {
		return out, err
	}
	events, err := store.LatestSessionEvents(ctx, db, p.ID, req.Host, req.HostSessionID, ipc.MaxEventPayloadBytes, ipc.MaxEventIDBytes)
	if err != nil {
		return out, err
	}
	for _, e := range events {
		field, dest := "prompt", &out.LatestPrompt
		if e.EventName == "Stop" {
			field, dest = "last_assistant_message", &out.LatestReply
		}
		msg := &ipc.ResumeMessage{EventID: secret.MaskString(e.ID), ReceivedAtMS: e.ReceivedAtMS, Truncated: e.Omitted}
		if msg.EventID != e.ID {
			msg.EventID = ""
		}
		if !e.Omitted {
			var fields map[string]json.RawMessage
			if json.Unmarshal(secret.Mask(e.Payload), &fields) == nil {
				var body string
				if json.Unmarshal(fields[field], &body) == nil {
					msg.Body = body
				}
			}
			if len(msg.Body) > ipc.ResumeBodyBytes {
				n := ipc.ResumeBodyBytes
				for !utf8.RuneStart(msg.Body[n]) {
					n--
				}
				msg.Body, msg.Truncated = msg.Body[:n], true
			}
		}
		*dest = msg
	}
	// Masking is CPU work outside database/sql's cancellation. Do not return
	// bodies after the shared read deadline, including Windows timer lag.
	if err := ctx.Err(); err != nil {
		return ipc.SessionResumeReply{}, err
	}
	if deadline, ok := ctx.Deadline(); ok && time.Now().After(deadline) {
		return ipc.SessionResumeReply{}, context.DeadlineExceeded
	}
	return out, nil
}
