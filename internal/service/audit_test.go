package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/mcpserver"
	"github.com/wotjr1649/engramux/internal/pipe"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
	"github.com/wotjr1649/engramux/internal/store"
)

// TestPhase6RedactionAudit is spec 8's Phase 6 `[auto]` gate: the audit, over
// the scope spec 8's Phase 6 row names, with one detector.
//
// # What "one detector" buys, and why the existing tests were not enough
//
// Three tests already assert a redaction property, and each is narrower than
// its surface:
//
//   - TestPhase5GateNoReplyFieldCarriesAUserPath sweeps two of the four reply
//     documents, with one class in them.
//   - TestTheLogFileNeverCarriesASecret drives the log with one class, which is
//     the right shape for it - what that test holds is that the filter is
//     *installed* in a real service, and the per-class work is
//     internal/secret's.
//   - TestPhase6TheMaskedCorpusIsCleanUnderARescan sweeps the payload with every
//     real shape on this machine, and reaches no field around the payload.
//
// So nothing swept every surface with every class, and the gap is not
// hypothetical: get_event's and list_sessions' replies carry three fields the
// other two documents do not - the whole masked payload, a project root, and a
// session id - and no test put a secret through them.
//
// # The event is loaded so that every field is a separate carrier
//
// One event carries one generated sample of every shape in spec 6.1's table,
// each in its own string leaf, plus a user path in three places that are not
// the payload text: hook_event_name, which the store takes verbatim (I-04);
// session_id, which becomes host_session_id; and cwd, which becomes the project
// root that list_sessions reports. The database path handed to the handlers is
// the shape a real install has (spec 5.6).
//
// # Both a detector sweep and a literal search
//
// [secret.Detect] over the marshalled document is the assertion, and the
// literal search for each sample's own bytes is beside it rather than instead
// of it. They fail on different bugs: the detector catches a field nobody
// masked whatever it carries, and the literal search catches a mask that
// removed a class *tag* while leaving the bytes - which is what a placeholder
// widened to swallow its neighbour would do, and which [secret.Detect] would
// then report clean.
//
// # Nothing here prints a document
//
// A failure names the classes that survived, or the shape whose bytes did -
// never the bytes. Same reason as internal/service's Phase 5 clause: `origin`
// is public and a failure message is the first thing anyone pastes.
func TestPhase6RedactionAudit(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))

	samples := secrettest.All()
	payload := auditPayload(t, samples)

	// The premise. A payload the detector finds nothing in would satisfy
	// every assertion below by carrying nothing to leak, and a shape that
	// stopped generating a match would take its class out of the sweep
	// silently.
	if got, want := secret.Detect(payload), secret.Classes(); !slices.Equal(got, want) {
		t.Fatalf("the audit payload is tagged %v, want every class this ruleset reports, %v", got, want)
	}

	if ack, err := store.Ingest(t.Context(), db, ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: auditEventID,
		Payload:  payload,
	}, store.SourcePipe, time.Now()); err != nil || ack != ipc.Committed {
		t.Fatalf("ingest the audit event: status %q, err %v", ack, err)
	}

	h := handlers(db, auditDatabasePath, filepath.Join(dir, spoolDir), time.Now(), newReadGate())

	t.Run("reply documents", func(t *testing.T) {
		st, err := h.Status(t.Context())
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if len(st.Cells) != 1 {
			t.Fatalf("the breakdown has %d cells, want the 1 that was ingested", len(st.Cells))
		}
		auditClean(t, "the status reply", samples, st)

		sr, err := h.Search(t.Context(), ipc.SearchRequest{Query: auditTerm, Project: auditProject})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(sr.Hits) != 1 || sr.Hits[0].ID != auditEventID {
			t.Fatalf("the search returned %d hits, want the 1 that was ingested", len(sr.Hits))
		}
		// An empty excerpt would leave the one field built out of
		// payload text with nothing in it to have masked.
		if sr.Hits[0].Excerpt == "" {
			t.Fatal("the hit carries no excerpt, so the sweep does not reach payload text")
		}
		auditClean(t, "the search reply", samples, sr)

		ev, err := h.GetEvent(t.Context(), ipc.GetEventRequest{ID: auditEventID, Project: auditProject})
		if err != nil {
			t.Fatalf("get_event: %v", err)
		}
		if ev.Event == nil || len(ev.Event.Payload) == 0 {
			t.Fatal("get_event answered no payload, so the sweep has no masked payload to read")
		}
		auditClean(t, "the get_event reply", samples, ev)

		ls, err := h.ListSessions(t.Context(), ipc.ListSessionsRequest{Project: auditProject})
		if err != nil {
			t.Fatalf("list_sessions: %v", err)
		}
		if len(ls.Sessions) != 1 {
			t.Fatalf("list_sessions returned %d sessions, want the 1 that was ingested", len(ls.Sessions))
		}
		// The field this reply exists in the audit for. Empty, there is
		// nothing in it to have masked.
		if ls.ProjectRoot == "" {
			t.Fatal("list_sessions answered an empty project root, so the sweep does not reach one")
		}
		auditClean(t, "the list_sessions reply", samples, ls)
	})

	// The MCP result is not the reply document: the SDK marshals the reply
	// into structured content *and* into a text block beside it, so the
	// document leaves this machine twice per call. Marshalling the whole
	// [mcp.CallToolResult] sweeps both at once.
	cs := auditSession(t, h)

	t.Run("mcp tool results", func(t *testing.T) {
		for _, tc := range []struct {
			tool string
			args map[string]any
		}{
			{"status", map[string]any{}},
			{"search", map[string]any{"query": auditTerm, "project": auditProject}},
			{"get_event", map[string]any{"id": auditEventID, "project": auditProject}},
			{"list_sessions", map[string]any{"project": auditProject}},
		} {
			t.Run(tc.tool, func(t *testing.T) {
				res := auditCall(t, cs, tc.tool, tc.args)
				if res.IsError {
					t.Fatalf("%s was refused, so this sweeps a refusal and not a result", tc.tool)
				}
				auditClean(t, "the "+tc.tool+" result", samples, res)
			})
		}
	})

	// A tool error is as much an egress as a result (I-10), and the one
	// error that can carry a caller's own path is the project argument
	// coming back in a refusal. A UNC path is refused by
	// project.FromArgument, which puts up to 128 characters of it in the
	// message - so this is the refusal that would leak if [mcpserver] stopped
	// masking one.
	//
	// status is not here and cannot be: it takes no arguments, so there is
	// nothing a caller can put in it to come back.
	t.Run("mcp tool errors", func(t *testing.T) {
		for _, tc := range []struct {
			tool string
			args map[string]any
		}{
			{"search", map[string]any{"query": auditTerm, "project": auditUNCProject}},
			{"get_event", map[string]any{"id": auditEventID, "project": auditUNCProject}},
			{"list_sessions", map[string]any{"project": auditUNCProject}},
		} {
			t.Run(tc.tool, func(t *testing.T) {
				res := auditCall(t, cs, tc.tool, tc.args)
				if !res.IsError {
					t.Fatalf("%s accepted a UNC project, so this sweeps a result and not a refusal", tc.tool)
				}
				auditClean(t, "the "+tc.tool+" refusal", samples, res)
			})
		}
	})
}

const (
	// auditEventID is the relay-minted UUIDv7 the event is ingested under
	// and the id get_event is asked for.
	auditEventID = "0192f0c0-0000-7000-8000-000000006a17"

	// auditTerm is the invented word the search finds the event by. It is in
	// no fixture and no other payload, and it is not one of the things being
	// masked - the event has to be findable by something safe to type.
	auditTerm = "quernstoneAudited"

	// auditProject is the event's cwd, and therefore the project root
	// list_sessions reports. It is under a Windows user directory because
	// that is the whole point of the field, and the user name is invented so
	// that the assertion says the same thing on a machine whose temporary
	// directory is not under a user profile - or that has no user profile.
	auditProject = `C:\Users\auditor\work\audited-project`

	// auditUNCProject is refused by project.FromArgument, which quotes it
	// back into the error, and carries a user path for the refusal to leak.
	auditUNCProject = `\\fileserver\Users\auditor\work\audited-project`

	// auditDatabasePath is the shape spec 5.6's layout has under a real user
	// profile. status masks it; doctor deliberately does not, and doctor is
	// out of this audit's scope for that reason (spec 8, Phase 6).
	auditDatabasePath = `C:\Users\auditor\AppData\Local\engramux\engramux.db`
)

// auditPayload is one hook event carrying every generated sample, each in its
// own leaf, plus a user path in the three fields around the payload text that
// reach an egress.
//
// It is built with json.Marshal rather than written out as a literal, so the
// backslashes and the newlines inside a private key are escaped by the encoder
// and the stored bytes are the ones a host would have sent.
func auditPayload(t *testing.T, samples []secrettest.Sample) []byte {
	t.Helper()

	leaves := make(map[string]any, len(samples)+4)
	for i, s := range samples {
		// A neutral key. A key naming a credential makes the whole leaf
		// a credential by a different rule (spec 6.1), and that rule is
		// internal/secret's to test - here it would only make two
		// samples indistinguishable in a failure.
		leaves[fmt.Sprintf("sample_%02d", i)] = s.Value
	}
	leaves["hook_event_name"] = secrettest.Of(secret.ClassUserPath).Value
	leaves["session_id"] = secrettest.Of(secret.ClassUserPath).Value
	leaves["cwd"] = auditProject
	leaves["prompt"] = "the event says " + auditTerm + " and nothing else"

	b, err := json.Marshal(leaves)
	if err != nil {
		t.Fatalf("marshal the audit payload: %v", err)
	}
	return b
}

// auditSession opens one MCP session against a server built over h.
//
// The transport is in memory rather than the Streamable HTTP one the service
// publishes. What is being swept is the document a tool answers with, which is
// the same on either transport; that the HTTP transport is bound, authorised
// and cross-origin protected is spec 8's Phase 5 clauses, in
// internal/mcpserver, against Listen and Serve.
func auditSession(t *testing.T, h pipe.Handler) *mcp.ClientSession {
	t.Helper()

	srv, err := mcpserver.New(h)
	if err != nil {
		t.Fatalf("build the MCP server: %v", err)
	}
	client, server := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go func() { _ = srv.Run(ctx, server) }()

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "engramux-audit", Version: "1"}, nil).
		Connect(ctx, client, nil)
	if err != nil {
		t.Fatalf("connect to the MCP server: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// auditCall makes one tool call and returns its result, refusal included.
func auditCall(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		// A transport or schema failure, not a tool refusal - those come
		// back as a result with IsError set.
		t.Fatalf("call %s: %v", tool, err)
	}
	return res
}

// auditClean marshals what leaves the machine and requires both that the
// detector reports nothing in it and that no sample's own bytes are in it.
func auditClean(t *testing.T, what string, samples []secrettest.Sample, doc any) {
	t.Helper()

	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal %s: %v", what, err)
	}
	if classes := secret.Detect(b); len(classes) != 0 {
		// The classes, never the document.
		t.Errorf("%s carries %v", what, classes)
	}
	for _, s := range samples {
		if bytes.Contains(b, []byte(s.Secret)) {
			// The shape, never the value.
			t.Errorf("%s still carries the %s bytes of a %s sample", what, s.Shape, s.Class)
		}
	}
}
