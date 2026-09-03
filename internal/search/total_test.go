package search_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/wotjr1649/engramux/internal/search"
)

// TestSearchReportsTheTotalBeyondTheLimit is backlog 33. A reply that carried
// hits and no total could not say how many events matched, so the product
// could not count its own corpus and "no results" meant four different things.
// The total is the count of everything the MATCH and the project filter
// admitted, before the limit cut it - so it is the limit's size that is under
// test here, and the filter's.
func TestSearchReportsTheTotalBeyondTheLimit(t *testing.T) {
	var docs []doc
	for i := range 3 {
		docs = append(docs, doc{
			name: fmt.Sprintf("t33-%d.json", i),
			payload: json.RawMessage(fmt.Sprintf(`{
  "session_id": "t33-total-%d",
  "hook_event_name": "UserPromptSubmit",
  "prompt_id": "t33-prompt-%d",
  "cwd": "C:\\Users\\fixture\\workspace\\fixture-project",
  "prompt": "the borogove was mimsy on day %d"
}`, i, i, i)),
		})
	}
	docs = append(docs, doc{
		name: "t33-other.json",
		payload: json.RawMessage(`{
  "session_id": "t33-other",
  "hook_event_name": "UserPromptSubmit",
  "prompt_id": "t33-prompt-other",
  "cwd": "C:\\Users\\fixture\\workspace\\fixture-project",
  "prompt": "nothing here matches"
}`),
	})
	db := ingestAll(t, docs)

	hits, total, err := search.Search(t.Context(), db, "borogove", "", 2, search.MatchAll)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Errorf("Search returned %d hits at limit 2, want 2", len(hits))
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 - the count has to be taken before the limit", total)
	}

	hits, total, err = search.Search(t.Context(), db, "borogove", "", 10, search.MatchAll)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 3 || total != 3 {
		t.Errorf("at limit 10: %d hits, total %d, want 3 and 3", len(hits), total)
	}

	// The filter is part of the count: a project that holds none of them
	// matches nothing, and a total that ignored the predicate would still
	// say 3 here.
	hits, total, err = search.Search(t.Context(), db, "borogove", "no-such-project", 10, search.MatchAll)
	if err != nil {
		t.Fatalf("Search (scoped): %v", err)
	}
	if len(hits) != 0 || total != 0 {
		t.Errorf("scoped to a project with no events: %d hits, total %d, want 0 and 0", len(hits), total)
	}
}
