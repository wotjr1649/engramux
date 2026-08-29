package service

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/store"
)

// TestPhase5GateGetEventChecksTheProjectWithTheId is spec 8's Phase 5 isolation
// clause at the get-event surface.
//
// events.id is the relay-minted UUIDv7 and is the primary key of the whole
// table (I-05), so it is unique across every project: an id learned from one
// project's search is a valid key into another's rows. Checking the pair is what
// stops that, and the assertion is that the *same id* answers an event under its
// own project and nothing at all under the other one.
//
// It is a filtering-correctness clause, not an authorization one. Spec 2 puts
// the whole SID inside the trust boundary and the caller names its own project;
// nothing here is a permission check and nothing should be read as one.
func TestPhase5GateGetEventChecksTheProjectWithTheId(t *testing.T) {
	db, a, b := twoProjects(t)

	got, err := getEvent(t.Context(), db, ipc.GetEventRequest{ID: a.id, Project: a.root})
	if err != nil {
		t.Fatalf("get the event under its own project: %v", err)
	}
	if got.Event == nil {
		t.Fatal("the event is not readable under its own project")
	}
	if got.Event.ID != a.id {
		t.Fatalf("got event %q, want %q", got.Event.ID, a.id)
	}

	// The same id, the other project. Nothing, and the reply says nothing
	// about the id existing anywhere.
	across, err := getEvent(t.Context(), db, ipc.GetEventRequest{ID: a.id, Project: b.root})
	if err != nil {
		t.Fatalf("get the event under the other project: %v", err)
	}
	if across.Event != nil {
		t.Fatalf("project B answered project A's event %q", a.id)
	}

	// And the control: B's own event is readable under B, so the empty
	// answer above is the pair being checked and not a broken fixture.
	own, err := getEvent(t.Context(), db, ipc.GetEventRequest{ID: b.id, Project: b.root})
	if err != nil {
		t.Fatalf("get project B's own event: %v", err)
	}
	if own.Event == nil {
		t.Fatal("project B cannot read its own event, so the previous assertion proves nothing")
	}
}

// TestPhase5GateASearchScopedToOneProjectNeverReturnsAnother is spec 8's Phase
// 5 isolation clause at the search surface, and the half
// [TestPhase5GateGetEventChecksTheProjectWithTheId] does not cover.
//
// The two events carry the same searchable word, so the unscoped control finds
// both and each scoped search finds exactly its own. Without the shared word the
// scoping would be indistinguishable from the query simply not matching.
//
// It is a filtering-correctness clause. §2 puts the whole SID inside the trust
// boundary and the caller names its own project; nothing here is a permission
// check.
func TestPhase5GateASearchScopedToOneProjectNeverReturnsAnother(t *testing.T) {
	db, a, b := twoProjects(t)
	// The word both payloads carry - see twoProjects, where each prompt is
	// "the event of <project directory>".
	const shared = "event"

	all, err := searchEvents(t.Context(), db, ipc.SearchRequest{Query: shared})
	if err != nil {
		t.Fatalf("the unscoped search: %v", err)
	}
	if len(all.Hits) != 2 {
		t.Fatalf("the unscoped search returned %d hits, want both events - "+
			"the scoped assertions below would prove nothing", len(all.Hits))
	}

	for _, p := range []fixtureProject{a, b} {
		got, err := searchEvents(t.Context(), db, ipc.SearchRequest{Query: shared, Project: p.root})
		if err != nil {
			t.Fatalf("the search scoped to %s: %v", filepath.Base(p.root), err)
		}
		if len(got.Hits) != 1 {
			t.Fatalf("the search scoped to %s returned %d hits, want its own 1",
				filepath.Base(p.root), len(got.Hits))
		}
		if got.Hits[0].ID != p.id {
			t.Errorf("the search scoped to %s returned event %q, want its own %q",
				filepath.Base(p.root), got.Hits[0].ID, p.id)
		}
	}
}

// TestSearchRefusesAProjectItMustNotWalk. An empty project is every project and
// must not reach the walk at all; every other shape the walk refuses is refused
// here too.
func TestSearchRefusesAProjectItMustNotWalk(t *testing.T) {
	db, _, _ := twoProjects(t)

	if _, err := searchEvents(t.Context(), db, ipc.SearchRequest{Query: "event"}); err != nil {
		t.Fatalf("an empty project is every project, not a refusal: %v", err)
	}
	for _, project := range []string{filepath.Join("nested", "dir"), `\\host\share\dev`, "."} {
		if _, err := searchEvents(t.Context(), db, ipc.SearchRequest{Query: "event", Project: project}); err == nil {
			t.Errorf("the search was answered for project %q rather than refused", project)
		}
	}
}

// TestGetEventMasksTheWholeReply is the egress clause at this surface. It has
// [TestPhase5GateNoReplyFieldCarriesAUserPath]'s shape and reuses its sweep:
// whatever a future field carries, it is in the document this walks.
func TestGetEventMasksTheWholeReply(t *testing.T) {
	db, a, _ := twoProjects(t)

	got, err := getEvent(t.Context(), db, ipc.GetEventRequest{ID: a.id, Project: a.root})
	if err != nil {
		t.Fatalf("get the event: %v", err)
	}
	requireNoSecretSurvives(t, "the get-event reply", got)

	// The stored row still holds what was captured (I-10 tags rather than
	// destroys), so the masking above is an egress and not a claim about
	// the database.
	var stored string
	if err := db.QueryRowContext(t.Context(),
		`SELECT payload FROM events WHERE id = ?`, a.id).Scan(&stored); err != nil {
		t.Fatalf("read events.payload: %v", err)
	}
	// Compared against the JSON spelling of the name, not the Go one: the
	// column holds the bytes the host wrote, where every backslash of a
	// Windows path is doubled.
	inJSON, err := json.Marshal(egressEventName)
	if err != nil {
		t.Fatalf("marshal the event name: %v", err)
	}
	if !strings.Contains(stored, strings.Trim(string(inJSON), `"`)) {
		t.Error("the row no longer holds the user path, so it was masked on the way in " +
			"and the reply proves nothing")
	}
}

// TestGetEventLeavesOutAPayloadOverTheBound holds spec 5.9's bound.
//
// The payload is left out rather than cut, and PayloadBytes is set either way.
// A cut JSON document does not parse, and one that parses and is short is worse
// - it looks whole. The distinction that matters to a caller is that "too large"
// still returns an event, while "no such event" returns none.
func TestGetEventLeavesOutAPayloadOverTheBound(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))
	root := repoAt(t, filepath.Join(dir, "big"))

	// One string leaf comfortably over the bound, made of words rather than
	// one long run. That is not decoration: spec 6.1's opaque class matches
	// any alphanumeric run of 40 or more, so a megabyte of one repeated
	// letter is a *secret* by that rule and masks down to a placeholder -
	// which is how this assertion first passed for the wrong reason. The
	// spaces are what keep every run short enough that nothing matches, so
	// the masked size is the stored size.
	big, err := json.Marshal(map[string]string{
		"session_id":      "over-the-bound",
		"hook_event_name": "UserPromptSubmit",
		"cwd":             root,
		"prompt":          strings.Repeat("the quick brown fox ", ipc.MaxEventPayloadBytes/20+1),
	})
	if err != nil {
		t.Fatalf("marshal the payload: %v", err)
	}
	const id = "8f1c2a10-0000-7000-8000-0000000000b1"
	ingestOne(t, db, id, big)

	got, err := getEvent(t.Context(), db, ipc.GetEventRequest{ID: id, Project: root})
	if err != nil {
		t.Fatalf("get the event: %v", err)
	}
	if got.Event == nil {
		t.Fatal("an over-large event is not a missing event")
	}
	if got.Event.Payload != nil {
		t.Errorf("the payload is on the wire at %d bytes, over the %d-byte bound",
			got.Event.PayloadBytes, ipc.MaxEventPayloadBytes)
	}
	if got.Event.PayloadBytes <= ipc.MaxEventPayloadBytes {
		t.Errorf("payload_bytes = %d, want more than the bound %d - "+
			"the field is what tells a caller why the payload is absent",
			got.Event.PayloadBytes, ipc.MaxEventPayloadBytes)
	}

	// The control: an ordinary payload is carried, so the two assertions
	// above are the bound firing rather than the field never being set.
	ordinary, a, _ := twoProjects(t)
	small, err := getEvent(t.Context(), ordinary, ipc.GetEventRequest{ID: a.id, Project: a.root})
	if err != nil {
		t.Fatalf("get an ordinary event: %v", err)
	}
	if small.Event == nil || small.Event.Payload == nil {
		t.Fatal("an ordinary event's payload is not on the wire, so the bound above proves nothing")
	}
}

// TestGetEventRefusesAProjectItMustNotWalk is the trust boundary at this
// surface. internal/project holds the rule and its own tests; this holds that
// this handler asks it before it touches the database.
func TestGetEventRefusesAProjectItMustNotWalk(t *testing.T) {
	db, a, _ := twoProjects(t)

	for _, tc := range []struct {
		name string
		req  ipc.GetEventRequest
	}{
		{"no id", ipc.GetEventRequest{Project: a.root}},
		{"no project", ipc.GetEventRequest{ID: a.id}},
		{"a relative project", ipc.GetEventRequest{ID: a.id, Project: filepath.Join("nested", "dir")}},
		{"a UNC project", ipc.GetEventRequest{ID: a.id, Project: `\\host\share\dev`}},
		{"an over-long id", ipc.GetEventRequest{ID: strings.Repeat("a", ipc.MaxEventIDBytes+1), Project: a.root}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := getEvent(t.Context(), db, tc.req)
			if err == nil {
				t.Fatalf("the request was answered rather than refused: event %v", got.Event != nil)
			}
		})
	}
}

// TestListSessionsMasksTheProjectRootAndScopesToIt is the list-sessions half of
// the same two clauses: the reply is masked, and it holds one project's sessions
// and no other's.
func TestListSessionsMasksTheProjectRootAndScopesToIt(t *testing.T) {
	db, a, b := twoProjects(t)

	got, err := listSessions(t.Context(), db, ipc.ListSessionsRequest{Project: a.root})
	if err != nil {
		t.Fatalf("list project A's sessions: %v", err)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("project A has %d sessions, want the 1 that was ingested", len(got.Sessions))
	}
	if got.Sessions[0].HostSessionID != a.session {
		t.Errorf("project A's session is %q, want %q", got.Sessions[0].HostSessionID, a.session)
	}
	requireNoSecretSurvives(t, "the list-sessions reply", got)

	// The root is the one the caller asked about, resolved and masked -
	// not the raw path, and not the other project's.
	if want := project.Identify(a.root).Root; got.ProjectRoot == want {
		t.Errorf("project_root is the unmasked root")
	}
	if strings.Contains(got.ProjectRoot, filepath.Base(b.root)) {
		t.Errorf("project_root names the other project")
	}

	other, err := listSessions(t.Context(), db, ipc.ListSessionsRequest{Project: b.root})
	if err != nil {
		t.Fatalf("list project B's sessions: %v", err)
	}
	if len(other.Sessions) != 1 || other.Sessions[0].HostSessionID != b.session {
		t.Fatalf("project B's listing is %+v, want only its own session %q", other.Sessions, b.session)
	}
}

// fixtureProject is one project a test ingested into: where it is, the session
// the host called it, and the one event id under it.
type fixtureProject struct {
	root    string
	session string
	id      string
}

// twoProjects builds one database holding two projects with one event each.
//
// Each project gets a .git of its own, so the walk up stops inside the temporary
// directory rather than at whatever is above it. Without that, a machine whose
// temporary directory happens to sit inside a repository would collapse both
// into one project and the isolation assertions would pass by accident.
//
// One event carries the user-path event name the egress sweep needs; both carry
// the cwd that decides the project.
func twoProjects(t *testing.T) (*sql.DB, fixtureProject, fixtureProject) {
	t.Helper()
	dir := t.TempDir()
	db := openMigrated(t, filepath.Join(dir, dbName))

	a := fixtureProject{
		root:    repoAt(t, filepath.Join(dir, "project-a")),
		session: "session-a",
		id:      "8f1c2a10-0000-7000-8000-0000000000a1",
	}
	b := fixtureProject{
		root:    repoAt(t, filepath.Join(dir, "project-b")),
		session: "session-b",
		id:      "8f1c2a10-0000-7000-8000-0000000000b2",
	}
	for _, p := range []fixtureProject{a, b} {
		payload, err := json.Marshal(map[string]string{
			"session_id":      p.session,
			"hook_event_name": egressEventName,
			"cwd":             p.root,
			"prompt":          "the event of " + filepath.Base(p.root),
		})
		if err != nil {
			t.Fatalf("marshal the payload: %v", err)
		}
		ingestOne(t, db, p.id, payload)
	}
	return db, a, b
}

// repoAt creates dir with a .git directory in it, which is what makes it a
// worktree root to internal/project's walk.
func repoAt(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Join(dir, ".git"), err)
	}
	return dir
}

// ingestOne stores one event through the production path and requires it to
// commit.
func ingestOne(t *testing.T, db *sql.DB, id string, payload []byte) {
	t.Helper()
	ack, err := store.Ingest(t.Context(), db, ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: id,
		Payload:  payload,
	}, store.SourcePipe, time.Now())
	if err != nil || ack != ipc.Committed {
		t.Fatalf("ingest %s: status %q, err %v", id, ack, err)
	}
}
