package inject_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// TestInvokesEngramux is backlog 41's decision, and the pair of rows that
// matter is the last two: the corpus of the repository that builds this product
// is largely prose about it, so a string match would exclude the owner's own
// work along with the product's own noise.
func TestInvokesEngramux(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want bool
	}{
		{`engramux search the thing`, true},
		{`engramux.exe status`, true},
		{`ENGRAMUX.EXE status`, true},
		{`./dist/engramux.exe search foo`, true},
		{`"C:\Users\x\AppData\Local\engramux\bin\engramux.exe" doctor`, true},
		{`cd d:/AI_DEV/engramux && ./dist/engramux.exe search foo`, true},
		{`go build ./... ; engramux status`, true},
		{`engramux search a | head -3`, true},
		{`(engramux status)`, true},
		{"go test ./...\nengramux search x", true},

		{``, false},
		{`go build -o dist/engramux.exe ./cmd/engramux`, false},
		{`grep -rn "engramux" .`, false},
		{`cat docs/superpowers/specs/engramux.md`, false},
		{`ls d:/AI_DEV/engramux`, false},
		{`git commit -m "engramux search is slow"`, false},
	} {
		if got := inject.InvokesEngramux(tc.cmd); got != tc.want {
			t.Errorf("InvokesEngramux(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

// A prompt with nothing to search on emits zero bytes. This is gate M6's shape
// at the unit level, and the reason splits the two cases a reader of the log
// has to tell apart: a prompt that reduced to no term at all, and a prompt
// whose terms were in too much of the corpus to be recall.
func TestBuildAbstainsOnAPromptWithNothingToSearchOn(t *testing.T) {
	db := corpus(t, event("PostToolUse", "engramux relay budget", "go test ./..."))
	for _, tc := range []struct{ prompt, reason string }{
		{"", inject.ReasonNoTerms},
		{"   ", inject.ReasonNoTerms},
		{"a b c d", inject.ReasonNoTerms},
		// Reduces to one common word; over this corpus nothing carries it,
		// and over a real one the selectivity ceiling is what refuses it.
		{"how do I fix this", inject.ReasonNoHits},
		// Korean function words are six bytes each, so no length rule
		// separates them from a Korean noun. Nothing matches them either.
		{"그럼 그거 해줘", inject.ReasonNoHits},
	} {
		res, err := inject.Build(t.Context(), db, inject.Request{Prompt: tc.prompt})
		if err != nil {
			t.Fatalf("Build(%q): %v", tc.prompt, err)
		}
		if res.Text != "" {
			t.Errorf("Build(%q) injected %d bytes, want zero", tc.prompt, len(res.Text))
		}
		if res.Reason != tc.reason {
			t.Errorf("Build(%q) abstained for %q, want %q", tc.prompt, res.Reason, tc.reason)
		}
	}
}

// A query that matched a large share of the corpus is refused rather than
// ranked: what it would inject is whatever bm25 put on top of an
// undifferentiated set, which is the distractor §6 cites Context Rot for.
func TestBuildAbstainsWhenTheQueryMatchesTooMuch(t *testing.T) {
	var docs []seeded
	for i := range 250 {
		docs = append(docs, event("PostToolUse", fmt.Sprintf("checkpoint threshold question %d", i), ""))
	}
	db := corpus(t, docs...)

	res, err := inject.Build(t.Context(), db, inject.Request{Prompt: "checkpoint threshold question"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Text != "" {
		t.Errorf("Build injected %d bytes for a query matching 250 of 250 documents", len(res.Text))
	}
	if res.Reason != inject.ReasonTooBroad {
		t.Errorf("Build abstained for %q, want %q", res.Reason, inject.ReasonTooBroad)
	}
}

// The identifier rule, which is what keeps `M3` and `00005` in a query that a
// length ranking would have spent on the prose around them. Asserted on the
// reduction itself rather than through a search, because what is being measured
// is which three terms were chosen and not what they then matched.
func TestTheQueryPrefersAnIdentifierToALongerWord(t *testing.T) {
	for _, tc := range []struct {
		prompt string
		want   []string
	}{
		// Every word here is longer than the identifier, and the
		// identifier still goes first.
		{"어떤 마이그레이션이 00005 였는지 설명해줘", []string{"마이그레이션이", "00005", "설명해줘"}},
		{"why did the WAL checkpoint threshold move", []string{"WAL", "checkpoint", "threshold"}},
		{"what does internal/search do about bm25", []string{"internal/search", "about", "bm25"}},
		// No identifier: length decides, and the sub-minimum words are
		// gone before it does.
		{"what did the checkpoint threshold decide", []string{"checkpoint", "threshold", "decide"}},
		// Nothing survives the minimum at all.
		{"a b c d", nil},
		{"", nil},
	} {
		got := inject.QueryFor(tc.prompt)
		if len(got) != len(tc.want) {
			t.Errorf("QueryFor(%q) = %v, want %v", tc.prompt, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("QueryFor(%q) = %v, want %v", tc.prompt, got, tc.want)
				break
			}
		}
	}
}

// A prompt that is distinctive and matches nothing emits zero bytes, and says
// which of the two it was. The pair is what makes the reason usable: an
// abstention for the prompt and an abstention for the corpus are different
// findings.
func TestBuildAbstainsWhenNothingMatches(t *testing.T) {
	db := corpus(t, event("PostToolUse", "the relay spooled an event", ""))
	res, err := inject.Build(t.Context(), db, inject.Request{Prompt: "quaternion holography interferometer"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Text != "" {
		t.Errorf("Build injected %d bytes over a corpus with no match", len(res.Text))
	}
	if res.Reason != inject.ReasonNoHits {
		t.Errorf("Build abstained for %q, want %q", res.Reason, inject.ReasonNoHits)
	}
}

// The prompt's own event is already in the database by the time the injector
// runs - the relay delivers before it injects - and its text is the query, so
// it would be its own top hit every time.
func TestBuildExcludesThePromptsOwnEvent(t *testing.T) {
	own := event("UserPromptSubmit", "what did the checkpoint threshold decide", "")
	other := event("PostToolUse", "what the checkpoint threshold decide was raised to", "")
	db := corpus(t, own, other)

	res, err := inject.Build(t.Context(), db, inject.Request{
		Prompt:    "what did the checkpoint threshold decide",
		ExcludeID: own.id,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(res.Events) == 0 {
		t.Fatalf("Build injected nothing; the other event should have matched")
	}
	for _, id := range res.Events {
		if id == own.id {
			t.Errorf("Build injected the prompt's own event %q", id)
		}
	}
	if !contains(res.Events, other.id) {
		t.Errorf("Build injected %v, want the other event %q", res.Events, other.id)
	}
}

// Backlog 41: a search's own capture must not become a candidate for the next
// prompt. The kept row is the control - it carries the same words and is not an
// invocation.
func TestBuildExcludesThisProductsOwnCommands(t *testing.T) {
	noise := event("PreToolUse", "running a shell command", "engramux search checkpoint threshold")
	prose := event("PostToolUse", "the checkpoint threshold decision is spec 5.4's", "grep -rn checkpoint threshold .")
	db := corpus(t, noise, prose)

	res, err := inject.Build(t.Context(), db, inject.Request{Prompt: "checkpoint threshold decision"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if contains(res.Events, noise.id) {
		t.Errorf("Build injected this product's own search, %q", noise.id)
	}
	if !contains(res.Events, prose.id) {
		t.Errorf("Build injected %v, want the grep %q kept - a mention is not an invocation",
			res.Events, prose.id)
	}
}

// Gate M5 at the unit level: the finished payload, fence included, never
// exceeds the cap. The corpus here is deliberately far larger than the cap, so
// the assertion is about the assembly and not about there being little to say.
func TestBuildStaysUnderTheByteCap(t *testing.T) {
	var docs []seeded
	for i := range 40 {
		docs = append(docs, event("PostToolUse",
			fmt.Sprintf("checkpoint threshold %d %s", i, strings.Repeat("padding ", 200)), ""))
	}
	db := corpus(t, docs...)

	res, err := inject.Build(t.Context(), db, inject.Request{Prompt: "checkpoint threshold padding"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Text == "" {
		t.Fatalf("Build abstained (%s) over a corpus of matches", res.Reason)
	}
	if len(res.Text) > inject.MaxBytes {
		t.Errorf("Build injected %d bytes, over the %d-byte cap", len(res.Text), inject.MaxBytes)
	}
	// And it used the room it had, rather than passing the cap by injecting
	// almost nothing.
	if len(res.Text) < inject.MaxBytes/2 {
		t.Errorf("Build injected %d bytes of a %d-byte budget over a corpus with 40 matches",
			len(res.Text), inject.MaxBytes)
	}
}

// Gate M9 at the unit level: what Build returns is fenced, and the body cannot
// close its own fence.
func TestBuildFencesWhatItInjects(t *testing.T) {
	db := corpus(t, event("PostToolUse", "the checkpoint threshold was raised", ""))
	res, err := inject.Build(t.Context(), db, inject.Request{Prompt: "checkpoint threshold raised"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	m := fencePattern.FindStringSubmatch(res.Text)
	if m == nil {
		t.Fatalf("Build returned text that is not fenced: %q", res.Text)
	}
	if strings.Contains(m[3], m[2]) {
		t.Errorf("the fenced body carries the nonce")
	}
}

// Every excerpt carries where it came from, which is §6's fourth mitigation: a
// reader can tell recall from instruction and an incident can be traced.
func TestEveryExcerptCarriesItsProvenance(t *testing.T) {
	e := event("PostToolUse", "the checkpoint threshold was raised", "")
	db := corpus(t, e)
	res, err := inject.Build(t.Context(), db, inject.Request{Prompt: "checkpoint threshold raised"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(res.Text, "[event "+e.id+" ") {
		t.Errorf("the payload carries no provenance line for %q:\n%s", e.id, res.Text)
	}
	if !strings.Contains(res.Text, time.Now().UTC().Format("2006-01-02T")) {
		t.Errorf("the provenance line carries no timestamp:\n%s", res.Text)
	}
}

// Gate M10 at the unit level: a budget the search cannot finish inside emits
// zero bytes down M6's own path rather than a second failure mode beside it.
func TestBuildAbstainsWhenTheBudgetIsBlown(t *testing.T) {
	db := corpus(t, event("PostToolUse", "the checkpoint threshold was raised", ""))
	// A budget already in the past rather than a very small one: a positive
	// nanosecond needs the context's timer goroutine to be scheduled before
	// database/sql looks, which is a race the test lost about one run in
	// five. A deadline behind the clock is cancelled at construction.
	res, err := inject.BuildWithBudget(t.Context(), db,
		inject.Request{Prompt: "checkpoint threshold raised"}, -time.Second)
	if err != nil {
		t.Fatalf("BuildWithBudget: %v", err)
	}
	if res.Text != "" {
		t.Errorf("BuildWithBudget injected %d bytes past its deadline", len(res.Text))
	}
	if res.Reason != inject.ReasonDeadline {
		t.Errorf("BuildWithBudget abstained for %q, want %q", res.Reason, inject.ReasonDeadline)
	}
}

// A cancelled caller is not a deadline miss and not an error either: it is the
// same zero bytes, because there is nobody left to inject into.
func TestBuildAbstainsOnACancelledCaller(t *testing.T) {
	db := corpus(t, event("PostToolUse", "the checkpoint threshold was raised", ""))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	res, err := inject.Build(ctx, db, inject.Request{Prompt: "checkpoint threshold raised"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Text != "" {
		t.Errorf("Build injected %d bytes for a cancelled caller", len(res.Text))
	}
}

// seeded is one event this package's tests put in a database, and the id
// store.Ingest committed it under.
type seeded struct {
	id      string
	payload []byte
}

// event builds one capture-shaped payload. cmd goes to tool_input.command,
// which is the column migration 00005 derives and the one the self-exclusion
// reads.
func event(name, text, cmd string) seeded {
	doc := map[string]any{
		"hook_event_name": name,
		"session_id":      "s",
		"cwd":             "/w",
		"tool_response":   map[string]any{"stdout": text},
	}
	if cmd != "" {
		doc["tool_input"] = map[string]any{"command": cmd}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return seeded{id: id.String(), payload: b}
}

// corpus builds one database from an empty directory through the production
// path, the same shape internal/search's ingestAll has.
func corpus(t *testing.T, docs ...seeded) *sql.DB {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "engramux.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	// An open handle makes t.TempDir()'s cleanup fail on Windows, and the
	// WAL sidecar counts.
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	now := time.Now()
	for _, d := range docs {
		status, err := store.Ingest(t.Context(), db, ipc.Envelope{
			Version:  ipc.Version,
			Type:     ipc.IngestEvent,
			IngestID: d.id,
			Payload:  d.payload,
		}, store.SourcePipe, now)
		if err != nil {
			t.Fatalf("Ingest: %v", err)
		}
		if status != ipc.Committed {
			t.Fatalf("Ingest answered %q, want %q", status, ipc.Committed)
		}
	}
	return db
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// The two lists alternate rather than running one after the other. They are
// ranked by separate indexes over separate populations and their scores are not
// comparable, so a concatenation would spend the whole budget on whichever list
// came first - and P4, one query reaching what only the other host knows, is
// the half that would never fit.
func TestAssembleAlternatesBetweenTheTwoLists(t *testing.T) {
	var hits []search.Hit
	var mem []search.MemoryHit
	for i := range 5 {
		hits = append(hits, search.Hit{ID: fmt.Sprintf("e%d", i), Host: "codex", Excerpt: "event body"})
		mem = append(mem, search.MemoryHit{ID: fmt.Sprintf("m%d", i), Host: "codex", Excerpt: "memory body"})
	}
	// Room for four blocks and not ten, so the question "which four" has an
	// answer that a concatenation would get wrong.
	body, events, memories := inject.Assemble(hits, mem, 200)
	if len(events) == 0 || len(memories) == 0 {
		t.Fatalf("Assemble took %d events and %d memory items; want both lists represented", len(events), len(memories))
	}
	if d := len(events) - len(memories); d > 1 || d < -1 {
		t.Errorf("Assemble took %d events against %d memory items, which is not an alternation",
			len(events), len(memories))
	}
	// Rank order is kept inside each list.
	if events[0] != "e0" || memories[0] != "m0" {
		t.Errorf("Assemble started with %q and %q, want the best-ranked of each list", events[0], memories[0])
	}
	if len(body) > 200 {
		t.Errorf("Assemble wrote %d bytes against a budget of 200", len(body))
	}
}
