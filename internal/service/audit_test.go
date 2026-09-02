package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
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
// literal search is beside it rather than instead of it. They fail on different
// bugs: the detector catches a field nobody masked whatever it carries, and the
// literal search catches a mask that removed the *shape* a rule matches on and
// left the credential body behind - which the detector then reports clean,
// because the part it matches on is the part that was removed.
//
// What the literal search looks for is [secrettest.Sample.Needle] and not
// Sample.Secret, and that distinction is the whole of what makes the second
// assertion real rather than decorative. Its doc comment has the two reasons.
//
// # Nothing here prints a document
//
// A failure names the classes that survived, or the shape whose bytes did -
// never the bytes. Same reason as internal/service's Phase 5 clause: `origin`
// is public and a failure message is the first thing anyone pastes.
func TestPhase6RedactionAudit(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))

	payload, samples := auditPayload(t)

	// The premise. A payload the detector finds nothing in would satisfy
	// every assertion below by carrying nothing to leak, and a shape that
	// stopped generating a match would take its class out of the sweep
	// silently.
	if got, want := secret.Detect(payload), secret.Classes(); !slices.Equal(got, want) {
		t.Fatalf("the audit payload is tagged %v, want every class this ruleset reports, %v", got, want)
	}
	// The other half of the premise, and the one that was missing. A needle
	// that is not in the document to begin with makes auditClean's literal
	// half hold for a reason that has nothing to do with masking - which is
	// what a raw Sample.Secret did for three shapes, silently, until review
	// found it. Requiring every needle here is what keeps that from
	// happening again to a shape nobody is looking at.
	for _, s := range samples {
		if !bytes.Contains(payload, []byte(s.Needle())) {
			t.Fatalf("the %s sample's bytes are not in the audit payload, so the literal "+
				"half of every sweep below is inert for it", s.Shape)
		}
	}

	if ack, err := store.Ingest(t.Context(), db, ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: auditEventID,
		Payload:  payload,
	}, store.SourcePipe, time.Now()); err != nil || ack != ipc.Committed {
		t.Fatalf("ingest the audit event: status %q, err %v", ack, err)
	}

	h := handlers(db, auditDatabasePath, filepath.Join(dir, spoolDir), time.Now(), newReadGate(), newHealth())

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
				auditAnswered(t, tc.tool, res)
				if res.StructuredContent == nil {
					t.Fatalf("the %s result carries no structured content, so half of what a "+
						"model reads is not in the sweep below", tc.tool)
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
				auditAnswered(t, tc.tool, res)
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
	//
	// Its user directory is auditUser, the same as auditProject's, so the
	// cwd sample's own bytes are what a sweep of the status reply searches
	// for - a fifth sample for this one would carry an identical needle.
	auditDatabasePath = `C:\Users\auditor\AppData\Local\engramux\engramux.db`

	// auditUser is the invented user directory name inside the three paths
	// above, and it is what ClassUserPath captures out of each of them - so
	// it is the substring that must not survive any of them.
	auditUser = "auditor"
)

// auditPayload is one hook event carrying every generated sample, each in its
// own leaf, plus a user path in the fields around the payload text that reach
// an egress. It returns the payload and every sample whose bytes must not
// survive a sweep.
//
// # The three field samples are returned, not just placed
//
// hook_event_name, session_id and cwd each carry a user path, and each is a
// field the reply documents were leaking before anything masked it. Placing a
// value there is only half of covering it: [auditClean]'s literal half searches
// for the samples it is given, so a value placed but not returned is covered by
// the detector alone. That is the weaker of the two assertions - it reports
// clean on a mask that writes a placeholder and leaves the bytes beside it,
// because isPlaceholder skips the span it wrote.
//
// secrettest.Of generates a fresh value per call, so these have to be captured
// once and reused rather than called twice.
//
// It is built with json.Marshal rather than written out as a literal, so the
// backslashes and the newlines inside a private key are escaped by the encoder
// and the stored bytes are the ones a host would have sent.
func auditPayload(t *testing.T) ([]byte, []secrettest.Sample) {
	t.Helper()

	samples := secrettest.All()
	leaves := make(map[string]any, len(samples)+4)
	for i, s := range samples {
		// A neutral key. A key naming a credential makes the whole leaf
		// a credential by a different rule (spec 6.1), and that rule is
		// internal/secret's to test - here it would only make two
		// samples indistinguishable in a failure.
		leaves[fmt.Sprintf("sample_%02d", i)] = s.Value
	}

	eventName, sessionID := secrettest.Of(secret.ClassUserPath), secrettest.Of(secret.ClassUserPath)
	eventName.Shape, sessionID.Shape = "hook_event_name", "session_id"
	leaves["hook_event_name"] = eventName.Value
	leaves["session_id"] = sessionID.Value
	leaves["cwd"] = auditProject
	leaves["prompt"] = "the event says " + auditTerm + " and nothing else"
	samples = append(samples, eventName, sessionID, secrettest.Sample{
		Class: secret.ClassUserPath, Shape: "cwd", Value: auditProject, Secret: auditUser,
	})

	b, err := json.Marshal(leaves)
	if err != nil {
		t.Fatalf("marshal the audit payload: %v", err)
	}
	return b, samples
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

// TestPhase6AnEventIdThatCarriesAUserPathIsMasked is backlog 29, closed.
//
// events.id is TEXT NOT NULL PRIMARY KEY with no CHECK, internal/store inserts
// the envelope's IngestID verbatim, and internal/pipe's validate requires only
// that it is non-empty - so the column holds whatever reached it. Every other
// untrusted string on a reply document is masked; this one reached an MCP
// reader as it was stored, where the session id beside it is masked and the
// event name is masked and then bounded.
//
// # Masking an id sounds like it should break the flow it exists for, and does
// not
//
// A model gets an id from a search hit and hands it back to get_event. If the
// id were rewritten on the way out, that would stop working. It is not: a
// UUIDv7 is hex and hyphens, its longest unbroken run is 12 characters, and
// spec 6.1's shortest matching rule is a 40-character opaque run - so
// [secret.MaskString] returns a real id unchanged, byte for byte, and only a
// secret-shaped one is rewritten. The test below holds both halves, because
// only holding the second would pass on an implementation that masked
// everything.
//
// The consequence for a secret-shaped id is that get_event cannot be asked for
// it by the id a hit reported. That is accepted and it is not new: internal/ipc
// says a shortened id is not an id, which is why list_sessions masks
// Session.ID rather than truncating it, and this is the same trade on the same
// reasoning.
func TestPhase6AnEventIdThatCarriesAUserPathIsMasked(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))

	// The shape a corrupted or hand-edited spool record produces. Nothing
	// on the ingest path refuses it.
	hostile := secrettest.Of(secret.ClassUserPath)
	hostile.Shape = "an event id"

	for _, id := range []string{auditEventID, hostile.Value} {
		if ack, err := store.Ingest(t.Context(), db, ipc.Envelope{
			Version:  ipc.Version,
			Type:     ipc.IngestEvent,
			IngestID: id,
			Payload:  auditIDPayload(t, id),
		}, store.SourcePipe, time.Now()); err != nil || ack != ipc.Committed {
			t.Fatalf("ingest: status %q, err %v", ack, err)
		}
	}

	h := handlers(db, auditDatabasePath, filepath.Join(dir, spoolDir), time.Now(), newReadGate(), newHealth())
	samples := []secrettest.Sample{hostile}

	t.Run("a real id survives masking unchanged", func(t *testing.T) {
		ev, err := h.GetEvent(t.Context(), ipc.GetEventRequest{ID: auditEventID, Project: auditProject})
		if err != nil {
			t.Fatalf("get_event: %v", err)
		}
		if ev.Event == nil {
			t.Fatal("get_event answered no event for an id it was just given")
		}
		// The whole search-then-read flow depends on this. An
		// implementation that masked every id would pass the sweeps
		// below and break the product.
		if ev.Event.ID != auditEventID {
			t.Fatalf("a UUIDv7 id came back rewritten, so a hit's id no longer round-trips to get_event")
		}
	})

	t.Run("search", func(t *testing.T) {
		reply, err := h.Search(t.Context(), ipc.SearchRequest{Query: auditTerm, Project: auditProject})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(reply.Hits) != 2 {
			t.Fatalf("the search returned %d hits, want the 2 that were ingested", len(reply.Hits))
		}
		auditClean(t, "the search reply", samples, reply)
	})

	t.Run("get_event", func(t *testing.T) {
		// Looked up by the stored id, which is what a caller with a
		// corrupted spool record would have. What comes back is what
		// this checks.
		reply, err := h.GetEvent(t.Context(), ipc.GetEventRequest{ID: hostile.Value, Project: auditProject})
		if err != nil {
			t.Fatalf("get_event: %v", err)
		}
		if reply.Event == nil {
			t.Fatal("get_event answered no event, so the sweep below reads an empty reply")
		}
		auditClean(t, "the get_event reply", samples, reply)
	})
}

// auditIDPayload is a minimal event that the audit term finds, under the
// audit's project. The id is not in it - the point of the test above is the id
// column, not the payload.
func auditIDPayload(t *testing.T, id string) []byte {
	t.Helper()

	b, err := json.Marshal(map[string]string{
		"session_id":      "phase6-id-" + strconv.Itoa(len(id)),
		"hook_event_name": "PreToolUse",
		"cwd":             auditProject,
		"prompt":          "the event says " + auditTerm + " and nothing else",
	})
	if err != nil {
		t.Fatalf("marshal the payload: %v", err)
	}
	return b
}

// auditAnswered is the vacuity guard the MCP sweeps need and the reply
// documents get from their own field checks.
//
// [mcp.CallToolResult.Content] has no omitempty, so a result with no content
// block marshals to `{"content":null}` - a document [auditClean] reports clean
// because there is nothing in it to find. Every call this test makes has an
// answer, a refusal included, so an empty one is a broken sweep and not a pass.
func auditAnswered(t *testing.T, tool string, res *mcp.CallToolResult) {
	t.Helper()

	if len(res.Content) == 0 {
		t.Fatalf("the %s result carries no content block, so the sweep below reads an empty document", tool)
	}
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
		if bytes.Contains(b, []byte(s.Needle())) {
			// The shape, never the value.
			t.Errorf("%s still carries the %s bytes of a %s sample", what, s.Shape, s.Class)
		}
	}
}
