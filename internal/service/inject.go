package service

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/ipc"
)

// injectFor answers an [ipc.Inject] request: the hook-time push path (memory
// spec rev.8, M-4).
//
// # The switch is not here
//
// Injection ships disabled, and what decides that is the relay: it reads the
// configuration and does not send this request at all when injection is off.
// The switch lives there rather than here for two reasons. A service-side
// switch would need a restart to change, since the service is a logon task; and
// a relay that never dials is a shorter path to zero bytes than one that dials
// and is told no. What this means for a reader of the log is that an entry
// below is proof the user turned injection on.
//
// # What is logged, and what is not
//
// §6's fifth mitigation asks for a switch and a way to see what was injected.
// This is the second half: one line per injection carrying the ids and the byte
// count, and one line per abstention carrying the reason.
//
// The excerpts are not logged and neither is the prompt. Both are corpus text,
// a log is a file, and I-10 governs what may be written to one - the same rule
// internal/pipe's search case states for the query. The ids are what makes an
// incident traceable, and they arrive already masked from internal/inject.
func injectFor(ctx context.Context, db *sql.DB, req ipc.InjectRequest) (ipc.InjectReply, error) {
	res, err := inject.Build(ctx, db, inject.Request{
		Prompt:    req.Prompt,
		Project:   req.Project,
		ExcludeID: req.ExcludeID,
	})
	if err != nil {
		return ipc.InjectReply{}, err
	}
	if res.Text == "" {
		slog.InfoContext(ctx, "engramux-service: injected nothing", "reason", res.Reason)
		return ipc.InjectReply{Reason: res.Reason}, nil
	}
	slog.InfoContext(ctx, "engramux-service: injected",
		"bytes", len(res.Text), "events", res.Events, "memory", res.Memory)
	return ipc.InjectReply{Context: res.Text}, nil
}
