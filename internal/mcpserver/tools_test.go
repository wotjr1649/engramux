package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/pipe"
)

// TestTheToolSurfaceIsTheFourSpec59Names. The set is a decision, not an
// accident of what was easy to expose: `doctor` is deliberately not here,
// because its reply carries the real database path where every other reply
// masks it (spec 5.9), and ingest is not here because I-08 gives it the pipe.
func TestTheToolSurfaceIsTheFourSpec59Names(t *testing.T) {
	endpoint, token := serveForTest(t, stubHandler())
	cs := connect(t, endpoint, token)

	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list the tools: %v", err)
	}
	var got []string
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	slices.Sort(got)
	want := []string{"get_event", "list_sessions", "search", "status"}
	if !slices.Equal(got, want) {
		t.Fatalf("the tools are %v, want %v", got, want)
	}
}

// TestTheProjectArgumentIsRequiredInTheSchema is spec 5.9's one rule stated at
// two levels: on the wire the project is optional and empty means every
// project, and in the tool schema it is required, because that is where the SDK
// can enforce it structurally and because a model has no working directory to
// mean.
//
// Both halves are asserted. The schema has to say `required`, and a call that
// omits the argument has to be refused - by the SDK's own validation, before
// this package's handler runs, which is the whole reason the requirement was
// put in the schema rather than in a check of our own.
//
// status is not in the list. It is the one tool that is not project-scoped:
// it reports what the service holds, which is every project at once.
func TestTheProjectArgumentIsRequiredInTheSchema(t *testing.T) {
	endpoint, token := serveForTest(t, stubHandler())
	cs := connect(t, endpoint, token)

	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list the tools: %v", err)
	}
	scoped := map[string]map[string]any{
		"search":        {"query": "anything"},
		"get_event":     {"id": stubEventID},
		"list_sessions": {},
	}
	for _, tool := range res.Tools {
		args, ok := scoped[tool.Name]
		if !ok {
			continue
		}
		t.Run(tool.Name, func(t *testing.T) {
			if !slices.Contains(required(t, tool.InputSchema), "project") {
				t.Error(`the input schema does not require "project"`)
			}
			out, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool.Name, Arguments: args})
			if err != nil {
				t.Fatalf("call %s: %v", tool.Name, err)
			}
			if !out.IsError {
				t.Error("a call with no project was answered")
			}
		})
	}
}

// required is the `required` list of an input schema as the client received it,
// which is a map[string]any rather than a *jsonschema.Schema.
func required(t *testing.T, schema any) []string {
	t.Helper()

	b, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal the input schema: %v", err)
	}
	var doc struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode the input schema: %v", err)
	}
	return doc.Required
}

// TestEachToolAnswersItsOwnReplyDocument. The tools answer internal/ipc's reply
// documents rather than a set of MCP-only structs, and this holds the property
// that buys: each document's own Verify accepts what the tool returned, so the
// version and the type discriminator are stamped here exactly as internal/pipe
// stamps them.
//
// Verify is the assertion and not a field comparison, because Verify is what a
// caller of the pipe runs - a reply this passes is a reply that surface would
// accept too.
func TestEachToolAnswersItsOwnReplyDocument(t *testing.T) {
	endpoint, token := serveForTest(t, stubHandler())
	cs := connect(t, endpoint, token)
	const project = `D:\work`

	t.Run("search", func(t *testing.T) {
		var reply ipc.SearchReply
		call(t, cs, "search", map[string]any{"query": "anything", "project": project}, &reply)
		if err := reply.Verify(); err != nil {
			t.Fatalf("verify the reply: %v", err)
		}
		if len(reply.Hits) != 1 || reply.Hits[0].ID != stubEventID {
			t.Fatalf("the reply carries %d hits", len(reply.Hits))
		}
	})

	t.Run("get_event", func(t *testing.T) {
		var reply ipc.GetEventReply
		call(t, cs, "get_event", map[string]any{"id": stubEventID, "project": project}, &reply)
		if err := reply.Verify(); err != nil {
			t.Fatalf("verify the reply: %v", err)
		}
		if reply.Event == nil || reply.Event.ID != stubEventID {
			t.Fatal("the reply carries no event")
		}
		// The payload is spliced in as raw JSON rather than escaped into
		// a string (spec 5.9), and it survives the two encodings between
		// the handler and here.
		if string(reply.Event.Payload) != `{"hook_event_name":"PostToolUse"}` {
			t.Errorf("the payload arrived as %s", reply.Event.Payload)
		}
	})

	t.Run("list_sessions", func(t *testing.T) {
		var reply ipc.ListSessionsReply
		call(t, cs, "list_sessions", map[string]any{"project": project}, &reply)
		if err := reply.Verify(); err != nil {
			t.Fatalf("verify the reply: %v", err)
		}
		if len(reply.Sessions) != 1 {
			t.Fatalf("the reply carries %d sessions", len(reply.Sessions))
		}
	})

	t.Run("status", func(t *testing.T) {
		var reply ipc.StatusReply
		call(t, cs, "status", map[string]any{}, &reply)
		if err := reply.Verify(); err != nil {
			t.Fatalf("verify the reply: %v", err)
		}
		if reply.Events != 1 {
			t.Fatalf("the reply carries %d events", reply.Events)
		}
	})
}

// TestARefusedCallCarriesAMaskedReason is backlog 27 at the surface that raised
// it, and the mask beside it.
//
// A refused pipe request answers a bare rejected [ipc.Ack], which has no field
// for a reason, so a caller learns only that it was refused - a person guesses,
// a model cannot. These tools do not cross the wire, so the handler's own error
// is in hand and is what the call returns.
//
// It is masked on the way out, because a tool error is as much an egress as a
// reply (I-10, spec 8's Phase 5 egress clause). The error below carries a user
// path that the caller did not send, which is the case the mask exists for.
func TestARefusedCallCarriesAMaskedReason(t *testing.T) {
	h := stubHandler()
	h.Search = func(context.Context, ipc.SearchRequest) (ipc.SearchReply, error) {
		return ipc.SearchReply{}, errors.New(`service: open C:\Users\someone\AppData\Local\engramux\engramux.db: refused`)
	}
	endpoint, token := serveForTest(t, h)
	cs := connect(t, endpoint, token)

	out, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "anything", "project": `D:\work`},
	})
	if err != nil {
		t.Fatalf("call search: %v", err)
	}
	if !out.IsError {
		t.Fatal("the refusal was answered as a result")
	}

	text := contentText(t, out)
	if !strings.Contains(text, "refused") {
		t.Error("the refusal carries no reason a caller could act on")
	}
	if strings.Contains(text, "someone") {
		// The text is not printed: a failure here means it carries the
		// user name.
		t.Error("the refusal carries the user name out of the path")
	}
}

// contentText is the text of a result's content blocks, joined.
func contentText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	var b strings.Builder
	for _, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			t.Fatalf("the result carries a %T, want text", c)
		}
		b.WriteString(tc.Text)
	}
	return b.String()
}

// call runs one tool and decodes its structured content into reply.
//
// The structured content is re-marshalled rather than read as bytes: the client
// decodes it into an `any`, because [mcp.CallToolResult.StructuredContent] is
// typed that way on both ends.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, reply any) {
	t.Helper()

	out, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if out.IsError {
		t.Fatalf("%s was refused: %s", name, contentText(t, out))
	}
	b, err := json.Marshal(out.StructuredContent)
	if err != nil {
		t.Fatalf("marshal %s's structured content: %v", name, err)
	}
	if err := json.Unmarshal(b, reply); err != nil {
		t.Fatalf("decode %s's structured content: %v", name, err)
	}
}

// connect opens one MCP session against endpoint with token.
func connect(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{Name: "engramux-test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearer{token: token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// bearer adds the Authorization header every request needs.
//
// A RoundTripper rather than the SDK's OAuthHandler, because there is no OAuth
// here: spec 5.9's transport is a static bearer, which MCP revision 2026-07-28
// permits precisely because it does not claim the OAuth framework's
// conformance. It is also the shape both hosts write - a static header in a
// configuration file - so this is the client the installer is aiming at.
type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	// Cloned: RoundTrip must not modify the request it is given.
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// TestNewRefusesAHandlerThatCannotAnswerAllFour. A nil field on a
// [pipe.Handler] means "refuse that request type" to internal/pipe, and it
// cannot mean that here: [mcp.AddTool] registers a tool unconditionally, so a
// nil field would be a tool a model can see and a nil dereference when it calls
// one.
func TestNewRefusesAHandlerThatCannotAnswerAllFour(t *testing.T) {
	full := stubHandler()
	for _, tc := range []struct {
		name string
		drop func(*pipe.Handler)
	}{
		{"no Search", func(h *pipe.Handler) { h.Search = nil }},
		{"no GetEvent", func(h *pipe.Handler) { h.GetEvent = nil }},
		{"no ListSessions", func(h *pipe.Handler) { h.ListSessions = nil }},
		{"no Status", func(h *pipe.Handler) { h.Status = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := full
			tc.drop(&h)
			if _, err := New(h); !errors.Is(err, ErrNoHandler) {
				t.Fatalf("New: %v, want %v", err, ErrNoHandler)
			}
		})
	}
	if _, err := New(full); err != nil {
		t.Fatalf("New with every handler: %v", err)
	}
}
