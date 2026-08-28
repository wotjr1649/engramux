package search_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
)

// t5Payload is one event carrying every shape T5's gate names, so that one
// ingest answers all of them: a hyphenated identifier, an absolute Windows
// path, the word `and` as content, and a Latin stem with a Korean particle
// attached.
//
// The backslashes are real. A JSON string escapes each one, so this decodes to
// C:\Users\fixture\workspace\fixture-project\internal\search\main_test.go - which is the shape
// spec 5.7 measured `no such column: C` on, and the shape a shell heredoc
// silently halves (AGENTS.md).
//
// `and` is in the prose deliberately: it is what makes the bare-AND row an
// assertion about content rather than about an absent word. If AND were read as
// an operator this query would be a syntax error, and if it were dropped the
// row would not come back.
const t5Payload = `{
  "session_id": "t5-query-construction",
  "hook_event_name": "UserPromptSubmit",
  "cwd": "C:\\Users\\fixture\\workspace\\fixture-project",
  "prompt": "the run-time budget and the path C:\\Users\\fixture\\workspace\\fixture-project\\internal\\search\\main_test.go — Codex는 파일을 읽는다"
}`

// TestSearchSurvivesRawInput is T5's gate: the text a person typed reaches
// MATCH through the builder, so every shape below answers with rows or with one
// of this package's own errors, and an FTS5 syntax error never reaches the
// caller.
//
// Every row went through [search.Search] before the builder existed as well.
// Four of them - the hyphenated identifier, the Windows path, the bare AND and
// the lone quote - came back as `SQL logic error: no such column: …` or
// `fts5: syntax error near …`, which is what this test is here to keep out.
func TestSearchSurvivesRawInput(t *testing.T) {
	docs := []doc{{name: "t5.json", payload: json.RawMessage(t5Payload)}}
	db := ingestAll(t, docs)
	want := docs[0].id

	for _, tc := range []struct {
		name    string
		text    string
		found   bool
		wantErr error
	}{
		// Bare, `run-time*` answers `no such column: time` (spec 5.7).
		{name: "a hyphenated identifier", text: "run-time", found: true},
		// Bare, this answers `no such column: C`.
		{
			name:  "an absolute Windows path",
			text:  `C:\Users\fixture\workspace\fixture-project\internal\search\main_test.go`,
			found: true,
		},
		// The basename alone, which is the gate's path-basename class:
		// unicode61 splits the dots, so quoted it is a phrase.
		{name: "a path basename", text: "main_test.go", found: true},
		// AND is content. Bare it is an FTS5 syntax error; quoted it
		// matches the word in the prose above.
		{name: "a bare AND", text: "AND", found: true},
		// A token that tokenizes to nothing: legal, and zero rows with no
		// error rather than a refusal. Bare, it is an unterminated string.
		{name: "a lone double quote", text: `"`, found: false},
		// unicode61 reads Codex는 as one token, so only the trailing star
		// reaches it (spec 5.7). This is the row that makes the expansion
		// load-bearing rather than a convenience.
		{name: "a Latin stem carrying a Korean particle", text: "Codex", found: true},
		// Two characters of a longer Korean token, which matches nothing
		// without the star and is not reachable by a prefix index either.
		{name: "two characters of a Korean word", text: "파일", found: true},
		// A token that tokenizes to nothing is dropped from the AND rather
		// than zeroing it, so the real token still decides the result.
		{name: "an empty token beside a real one", text: "--- 파일", found: true},
		// The join between tokens is an AND and not an OR: one term that
		// matches nothing takes the whole query to nothing.
		{name: "a real token and an absent one", text: "run-time zzznotinthisevent", found: false},
		// The sentinel reaches the caller through Search, unwrapped enough
		// for errors.Is. The other two bounds are pinned on the builder in
		// TestQueryBounds.
		{name: "no query at all", text: "   ", wantErr: search.ErrEmptyQuery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := search.Search(t.Context(), db, tc.text, 10)
			if err != nil {
				assertCleanError(t, tc.text, err, tc.wantErr)
				return
			}
			if tc.wantErr != nil {
				t.Fatalf("Search(%q) returned %d hits, want %v", tc.text, len(hits), tc.wantErr)
			}
			if !tc.found {
				if len(hits) != 0 {
					t.Fatalf("Search(%q) returned %d hits, want none", tc.text, len(hits))
				}
				return
			}
			if len(hits) != 1 || hits[0].ID != want {
				t.Fatalf("Search(%q) = %v, want the one event %s", tc.text, hits, want)
			}
		})
	}
}

// assertCleanError requires an error from [search.Search] to be one this
// package raised itself and never one SQLite's query parser raised.
//
// Both halves are needed. errors.Is alone would pass an unwrapped sqlite error
// that some future code decided to wrap in a sentinel, and the string check
// alone would pass any error at all as long as it was worded differently. The
// two strings are the exact wordings spec 5.7 measured on this corpus's shapes.
func assertCleanError(t *testing.T, text string, err, want error) {
	t.Helper()
	for _, bad := range []string{"syntax error", "no such column"} {
		if strings.Contains(err.Error(), bad) {
			t.Fatalf("Search(%q): a SQL %s reached the caller: %v", text, bad, err)
		}
	}
	if want == nil {
		t.Fatalf("Search(%q): unexpected error: %v", text, err)
	}
	if !errors.Is(err, want) {
		t.Fatalf("Search(%q): err = %v, want %v", text, err, want)
	}
}
