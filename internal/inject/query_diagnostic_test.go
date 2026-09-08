package inject_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/secret"
)

func TestWriteSelectionSessionHoldout(t *testing.T) {
	if os.Getenv("ENGRAMUX_WRITE_SELECTION_HOLDOUT") != "1" {
		t.Skip("holdout creation is opt-in")
	}
	uri, err := temporalSourceURI(m7Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	excluded := map[string]bool{}
	for _, p := range m7LabelledPrompts(t, db, "../../.capture/m7/agent-2026-09-08", m7Agent) {
		excluded[p.session] = true
	}
	rows, err := db.QueryContext(t.Context(), `SELECT id,session_id,payload FROM events WHERE event_name='UserPromptSubmit' ORDER BY received_at,id`)
	if err != nil {
		t.Fatal("read holdout candidates")
	}
	defer func() { _ = rows.Close() }()
	var output []string
	sessions := map[string]bool{}
	for rows.Next() {
		var id, session string
		var payload []byte
		if err := rows.Scan(&id, &session, &payload); err != nil {
			t.Fatal("read holdout row")
		}
		if session == "" || excluded[session] {
			continue
		}
		var p struct {
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(secret.Mask(payload), &p) != nil || p.Prompt == "" {
			continue
		}
		output = append(output, strings.Join([]string{id, m7Script(p.Prompt), m7Todo, m7Line(p.Prompt)}, "\t"))
		sessions[session] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal("holdout cursor failed")
	}
	if err := rows.Close(); err != nil {
		t.Fatal("close holdout cursor")
	}
	if len(output) == 0 {
		t.Fatal("no independent session remains")
	}
	snapshot, err := os.Open(m7Snapshot)
	if err != nil {
		t.Fatal("open snapshot for digest")
	}
	h := sha256.New()
	_, hashErr := io.Copy(h, snapshot)
	closeErr := snapshot.Close()
	if hashErr != nil || closeErr != nil {
		t.Fatal("snapshot digest failed")
	}
	path := "../../.capture/selection-quality/holdout-2026-09-08/prompts.tsv"
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal("holdout already exists or cannot be created; refusing replacement")
	}
	header := fmt.Sprintf("# Session-disjoint selection holdout. No selector output was used.\n%s\n# snapshot SHA-256: %x\n# excluded development sessions: %d\n# columns: prompt_id, script, wanted_context, prompt\n", m7UnlabelledHeader, h.Sum(nil), len(excluded))
	_, writeErr := io.WriteString(f, header+strings.Join(output, "\n")+"\n")
	closeErr = f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("write holdout fixture")
	}
	t.Logf("wrote %d prompts from %d sessions disjoint from %d development sessions; no retrieval performed", len(output), len(sessions), len(excluded))
}

// This measures the existing selector without choosing replacements or exposing
// prompt text. The already-labelled sample is development evidence, not a holdout.
func TestDiagnoseSelectedQueryTerms(t *testing.T) {
	dir := os.Getenv("ENGRAMUX_QUERY_DIAGNOSTIC_DIR")
	if dir == "" {
		t.Skip("query diagnostic is opt-in")
	}
	uri, err := temporalSourceURI(m7Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	source, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	source.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})
	prompts := m7LabelledPrompts(t, source, dir, m7Agent)
	stamps := map[string]int64{}
	for _, p := range prompts {
		var stamp int64
		if err := source.QueryRowContext(t.Context(), "SELECT received_at FROM events WHERE id=?", p.id).Scan(&stamp); err != nil {
			t.Fatal("read timestamp")
		}
		stamps[p.id] = stamp
		if m7Drifted(p) {
			t.Fatal("project identity drift")
		}
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	prefix, err := newTemporalReplay(t, m7Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(prompts, func(i, j int) bool { return stamps[prompts[i].id] < stamps[prompts[j].id] })
	type counts struct{ prompts, terms, zeroTerms, broadTerms, anyAbsent, allAbsent, combinedEmpty, intersectionEmpty int }
	groups := map[string]*counts{}
	for _, p := range prompts {
		if err := prefix.advance(t, stamps[p.id]); err != nil {
			t.Fatal(err)
		}
		key := p.wanted + "/" + p.script
		c := groups[key]
		if c == nil {
			c = &counts{}
			groups[key] = c
		}
		c.prompts++
		terms := inject.QueryFor(p.prompt)
		if len(terms) == 0 {
			continue
		}
		absent := 0
		for _, term := range terms {
			_, total, err := search.Search(t.Context(), prefix.db, term, p.project, 1, search.MatchAll)
			if err != nil {
				t.Fatal("single-term diagnostic refused")
			}
			c.terms++
			if total == 0 {
				c.zeroTerms++
				absent++
			}
			if total > 200 {
				c.broadTerms++
			}
		}
		if absent > 0 {
			c.anyAbsent++
		}
		if absent == len(terms) {
			c.allAbsent++
		}
		_, total, err := search.Search(t.Context(), prefix.db, strings.Join(terms, " "), p.project, 1, search.MatchAll)
		if err != nil {
			t.Fatal("combined diagnostic refused")
		}
		if total == 0 {
			c.combinedEmpty++
			if absent == 0 {
				c.intersectionEmpty++
			}
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		c := groups[key]
		t.Logf("%s: prompts=%d terms=%d absent_terms=%d broad_terms=%d prompts_any_absent=%d prompts_all_absent=%d empty_AND=%d empty_intersection_with_all_terms_present=%d", key, c.prompts, c.terms, c.zeroTerms, c.broadTerms, c.anyAbsent, c.allAbsent, c.combinedEmpty, c.intersectionEmpty)
	}
}
