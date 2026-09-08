package search_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/search"
)

type m8TurnScope struct{ host, session, cwd, key string }

func m8TurnOf(d doc) m8TurnScope {
	var fields map[string]any
	if json.Unmarshal(d.payload, &fields) != nil {
		return m8TurnScope{}
	}
	h := host.Detect(fields)
	field := "prompt_id"
	if h == "codex" {
		field = "turn_id"
	} else if h != "claude-code" {
		return m8TurnScope{}
	}
	key, _ := fields[field].(string)
	session, _ := fields["session_id"].(string)
	cwd, _ := fields["cwd"].(string)
	if key == "" || len(key) > 256 || session == "" || cwd == "" {
		return m8TurnScope{}
	}
	return m8TurnScope{h, session, cwd, key}
}

// This is bounded extra context, not a same-k ranking improvement. Anchor
// selection never reads the labelled failure ID or the expected fixes.
func m8ExpandTurns(docs map[string]doc, hits []search.Hit, limit int) []string {
	return m8ExpandContext(docs, hits, limit, true)
}

func m8ExpandContext(docs map[string]doc, hits []search.Hit, limit int, sameTurn bool) []string {
	var out []string
	scopes := map[string]m8TurnScope{}
	for id, d := range docs {
		scope := m8TurnOf(d)
		if !sameTurn && scope.key != "" {
			scope.key = "any-turn"
		}
		scopes[id] = scope
	}
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.ID] = true
	}
	for _, h := range hits {
		anchor := docs[h.ID]
		scope := scopes[h.ID]
		if scope.key == "" {
			continue
		}
		var candidates []doc
		for _, d := range docs {
			if !seen[d.id] && m8Stamp(d.name) > m8Stamp(anchor.name) && scopes[d.id] == scope {
				candidates = append(candidates, d)
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			if m8Stamp(candidates[i].name) == m8Stamp(candidates[j].name) {
				return candidates[i].name < candidates[j].name
			}
			return m8Stamp(candidates[i].name) < m8Stamp(candidates[j].name)
		})
		for _, d := range candidates {
			if len(out) >= limit {
				return out
			}
			seen[d.id] = true
			out = append(out, d.id)
		}
	}
	return out
}

func TestM8TurnExpansionIsBoundedAndScoped(t *testing.T) {
	makeDoc := func(id, name, session, key string) doc {
		b, err := json.Marshal(map[string]string{"session_id": session, "cwd": "/synthetic", "prompt_id": key})
		if err != nil {
			t.Fatal(err)
		}
		return doc{id: id, name: name, payload: b}
	}
	docs := map[string]doc{
		"seed":  makeDoc("seed", "h__A__100__1.json", "s", "k"),
		"fix":   makeDoc("fix", "h__A__200__1.json", "s", "k"),
		"later": makeDoc("later", "h__A__300__1.json", "s", "k"),
		"other": makeDoc("other", "h__A__150__1.json", "other", "k"),
	}
	got := m8ExpandTurns(docs, []search.Hit{{ID: "seed"}}, 1)
	if len(got) != 1 || got[0] != "fix" {
		t.Fatalf("expanded=%v", got)
	}
}
