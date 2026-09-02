// Package mcpserver is spec 5.9's MCP surface, as the memory spec rev.2 widened it: the five tools, and the
// Streamable HTTP transport on 127.0.0.1 that carries them.
//
// # It answers with the same closures the pipe does
//
// The tools take an [pipe.Handler] - the exact struct internal/service builds
// once in `handlers` and hands to [pipe.Serve] - and call its functions
// directly. Nothing here opens a database, builds a query, or masks a field.
// Three things follow, and each is the reason for the choice:
//
//   - The read gate is shared. A tool call takes the same query deadline, the
//     same read concurrency of one, and the same ingest priority a pipe read
//     takes, because it is the same closure. A second path to the database
//     would be a second thing to gate, and spec 5.9's contention clause would
//     be measuring the surface nobody used.
//   - The reply documents are internal/ipc's. Spec 8's egress clause sweeps the
//     marshalled reply with the detector rather than naming fields, so a field
//     added to one of those structs is covered here the moment it is on the
//     wire. A set of MCP-only output structs would have escaped that gate.
//   - A refusal carries its reason. Backlog 27 is that a refused pipe request
//     answers a bare rejected [ipc.Ack] with no field for a reason, so a caller
//     learns only that it was refused. That is a property of the wire, and this
//     surface is not on the wire: the handler's own error is in hand, and it is
//     what the tool returns. The row stays open because the pipe still has the
//     problem; the tool surface, which is what raised it, does not.
//
// # Every error is masked on the way out
//
// A tool error is as much an egress as a reply (I-10). The errors these
// handlers return are the product's own bounded strings, and the one that
// carries a path carries the path the caller itself sent - but "the caller sent
// it" is a claim about today's error set, not a property. [secret.MaskString]
// over the text makes spec 8's "no MCP-facing field carries a user path" hold
// for the refusal path too, and it costs the caller nothing it does not
// already know.
package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/pipe"
	"github.com/wotjr1649/engramux/internal/secret"
)

// ErrNoHandler is returned by [New] when the handler it was given cannot answer
// one of the five tools.
//
// It is an error rather than a nil check per call. [mcp.AddTool] registers a
// tool unconditionally, so a nil field would become a tool a model can see and
// a nil dereference when it calls one; refusing to build the server at all is
// the failure that is visible at the moment it is caused.
var ErrNoHandler = errors.New("mcpserver: the handler cannot answer all five tools")

// / New builds the MCP server spec 5.9 specifies, as the memory spec rev.2 widened
// it: five tools over h. The fifth is get_memory, and what the memory spec says
// about the count is that section 5.9 rows naming four are the ones that moved.
//
// # The project argument is required here and optional on the wire
//
// [ipc.SearchRequest.Project] means "every project" when empty, and it keeps
// that meaning - an existing CLI invocation must return what it always
// returned. The requirement belongs where it can be enforced structurally, and
// that is the tool schema: the SDK derives `required` from a field with no
// `omitempty`, validates arguments against it, and refuses a call that omits
// one before this package's code runs. A model has no working directory to
// mean, so "all projects" is not a default it can have meant.
//
// [mcp.AddTool] panics rather than returning an error when it cannot infer a
// schema from the argument type. That is a property of the types below and not
// of anything at run time: it either panics on every start or on none, and the
// tests in this package are what run it.
func New(h pipe.Handler) (*mcp.Server, error) {
	if h.Search == nil || h.GetEvent == nil || h.ListSessions == nil || h.Status == nil || h.GetMemory == nil {
		return nil, ErrNoHandler
	}

	// Version is the wire protocol version, which is the only version this
	// build states about itself (spec 5.3). A second version number minted
	// here would be one more thing to forget to raise.
	s := mcp.NewServer(&mcp.Implementation{
		Name:        "engramux",
		Title:       "Engramux",
		Description: "Search and read the Claude Code and Codex hook events captured on this machine.",
		Version:     ipc.Version,
	}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name: "search",
		Description: "Full-text search over the raw text of the hook events Engramux captured, " +
			"and over the memory Claude Code and Codex write for themselves. Best match first. " +
			"The reply carries two ranked lists: hits are events, and memory_hits are native memory " +
			"items - the two are ranked by separate indexes and their scores are not comparable. " +
			"Pass a hit's id to get_event, or a memory hit's id to get_memory, for the whole document.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, ipc.SearchReply, error) {
		reply, err := h.Search(ctx, ipc.SearchRequest{Query: in.Query, Limit: in.Limit, Project: in.Project})
		if err != nil {
			return nil, ipc.SearchReply{}, refused(err)
		}
		reply.Version, reply.Type = ipc.Version, ipc.Search
		return nil, reply, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_event",
		Description: "One captured event in full, with secrets masked. " +
			"The event is null when no event with that id exists in that project, and the payload " +
			"is null - with payload_bytes still set - when the masked event is too large to return.",
		// The output type is `any` where the other three are their reply
		// document, and that is a measurement rather than a preference.
		// The SDK derives an output schema from any Out that is not
		// `any`, and then validates the marshalled reply against it -
		// but [ipc.EventDocument.Payload] is a json.RawMessage, which
		// jsonschema-go infers from as the []byte it is. Typed, every
		// call with a JSON object payload is refused with
		// `validating /properties/event/properties/payload: type:
		// map[...] has type "object", want one of "null, array"`. That
		// is the reply spec 5.9 specifies failing its own schema, so the
		// schema is what goes. `any` produces no output schema, no
		// validation, and the same structured content and text block.
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getEventIn) (*mcp.CallToolResult, any, error) {
		reply, err := h.GetEvent(ctx, ipc.GetEventRequest{ID: in.ID, Project: in.Project})
		if err != nil {
			return nil, nil, refused(err)
		}
		reply.Version, reply.Type = ipc.Version, ipc.GetEvent
		return nil, reply, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_memory",
		Description: "One native memory item in full, with secrets masked. This is the equivalent of " +
			"get_event for the memory Claude Code and Codex write for themselves: pass the id of a " +
			"memory hit that a search returned. The item is null when no item with that id exists in " +
			"that project, and the body is null - with body_bytes still set - when it is too large.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getMemoryIn) (*mcp.CallToolResult, ipc.GetMemoryReply, error) {
		reply, err := h.GetMemory(ctx, ipc.GetMemoryRequest{ID: in.ID, Project: in.Project})
		if err != nil {
			return nil, ipc.GetMemoryReply{}, refused(err)
		}
		reply.Version, reply.Type = ipc.Version, ipc.GetMemory
		return nil, reply, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_sessions",
		Description: "The Claude Code and Codex sessions Engramux has captured for one project, newest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listSessionsIn) (*mcp.CallToolResult, ipc.ListSessionsReply, error) {
		reply, err := h.ListSessions(ctx, ipc.ListSessionsRequest{Project: in.Project, Limit: in.Limit})
		if err != nil {
			return nil, ipc.ListSessionsReply{}, refused(err)
		}
		reply.Version, reply.Type = ipc.Version, ipc.ListSessions
		return nil, reply, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "status",
		Description: "What the Engramux service currently holds: how many events, the breakdown by host " +
			"and event name, how many events are waiting in the spool, and how long the service has been up.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ statusIn) (*mcp.CallToolResult, ipc.StatusReply, error) {
		reply, err := h.Status(ctx)
		if err != nil {
			return nil, ipc.StatusReply{}, refused(err)
		}
		reply.Version, reply.Type = ipc.Version, ipc.Status
		return nil, reply, nil
	})

	return s, nil
}

// The tool arguments. A field with no `omitempty` is required in the schema the
// SDK infers, which is where spec 5.9 puts the project requirement; a field
// with one is optional and means what the wire says an unset value means.
//
// The descriptions are the jsonschema tags, and they are the whole of what a
// model has to go on: it cannot see this package, the spec, or the error it
// would get for guessing wrong.
type (
	searchIn struct {
		Query   string `json:"query" jsonschema:"the words to look for. Plain text, not FTS5 query syntax: every token is quoted, so an operator is matched literally"`
		Project string `json:"project" jsonschema:"the absolute path of the project worktree to search. It must be absolute, and a UNC path is refused"`
		Limit   int    `json:"limit,omitempty" jsonschema:"how many hits at most, 1 to 100. Defaults to 20"`
	}
	getEventIn struct {
		ID      string `json:"id" jsonschema:"the event id, as a search hit reports it"`
		Project string `json:"project" jsonschema:"the absolute path of the project worktree the event belongs to. An id from another project answers no event"`
	}
	getMemoryIn struct {
		ID string `json:"id" jsonschema:"the memory item id, as a search reply's memory_hits report it"`
		// Optional where get_event's is required, and the difference is
		// not an oversight: a memory item may belong to no project this
		// database has a row for, because Codex's memory is global and
		// Claude Code files its own under a directory key rather than a
		// path. Requiring one would make those items unreachable.
		Project string `json:"project,omitempty" jsonschema:"the absolute path of the project worktree the item belongs to. Optional: native memory may belong to no project, and omitting this searches all of them"`
	}
	listSessionsIn struct {
		Project string `json:"project" jsonschema:"the absolute path of the project worktree to list. It must be absolute, and a UNC path is refused"`
		Limit   int    `json:"limit,omitempty" jsonschema:"how many sessions at most, 1 to 100. Defaults to 20"`
	}
	// statusIn has no fields: the question has no parameters, and status is
	// the one tool that is not project-scoped - it reports what the service
	// holds, which is every project at once.
	statusIn struct{}
)

// refused is a handler's error on its way to a model.
//
// The text is masked for the reason this file's documentation gives, and the
// error identity is dropped with it: [errors.New] over the masked string, so
// nothing downstream can unwrap to a sentinel and act on a value the mask may
// have rewritten. A tool caller reads the text; it has no errors.Is.
func refused(err error) error {
	return errors.New(secret.MaskString(err.Error()))
}
