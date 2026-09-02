package service

import (
	"context"
	"database/sql"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/memory"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/search"
)

// getMemory answers a [ipc.GetMemory] request (memory spec rev.2, M-2 decision
// 9), and is the fourth place I-10 has to hold.
//
// It is [getEvent]'s shape with one difference, and the difference is deliberate:
// an empty project is every project here, where for an event it is refused. A
// memory item may belong to no project this database has a row for - Codex's
// memory is global and Claude Code's is filed under a directory key that is not
// a path - so requiring one would make those items unreachable through MCP. On
// this machine that is 155 of 303 items.
//
// The masking is [search.GetMemoryItem]'s, which owns the whole memory read
// path. What this function adds is the bound and the wire shape.
func getMemory(ctx context.Context, db *sql.DB, req ipc.GetMemoryRequest) (ipc.GetMemoryReply, error) {
	if err := req.Validate(); err != nil {
		return ipc.GetMemoryReply{}, err
	}
	var keys []string
	if req.Project != "" {
		p, err := project.FromArgument(req.Project)
		if err != nil {
			return ipc.GetMemoryReply{}, err
		}
		keys = memory.ProjectKeys(p.Root)
	}

	it, err := search.GetMemoryItem(ctx, db, req.ID, keys)
	if err != nil {
		return ipc.GetMemoryReply{}, err
	}
	if it == nil {
		// No such item *in that scope*, which is the same answer as no
		// such item anywhere - the same rule store.GetEvent follows.
		return ipc.GetMemoryReply{}, nil
	}

	doc := ipc.MemoryDocument{
		ID:             it.ID,
		Host:           it.Host,
		Kind:           it.Kind,
		SourcePath:     it.SourcePath,
		EntryKey:       it.EntryKey,
		ProjectPath:    it.ProjectPath,
		Title:          it.Title,
		HostModifiedMS: it.HostModifiedMS,
		PrivacyClass:   it.PrivacyClass,
		BodyBytes:      len(it.Body),
	}
	// Over the bound the body is left out rather than cut, on [getEvent]'s
	// rule: BodyBytes is set either way, so "too large" is distinguishable
	// from "no such item", which answers a nil Item instead.
	if len(it.Body) <= ipc.MaxMemoryBodyBytes {
		doc.Body = it.Body
	}
	return ipc.GetMemoryReply{Item: &doc}, nil
}
