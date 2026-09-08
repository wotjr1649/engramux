package search_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

func TestGateM8NativeCoverageOfP5(t *testing.T) {
	m8P5Evaluate(t, "../../.capture/m8/pairs.tsv", "owner")
}

func TestEvaluateM8P5AgentEstimates(t *testing.T) {
	path := os.Getenv("ENGRAMUX_M8_AGENT_LABELS")
	if path == "" {
		t.Skip("agent P5 estimates require ENGRAMUX_M8_AGENT_LABELS; not an owner verdict")
	}
	m8P5Evaluate(t, path, "agent")
}

func m8P5Evaluate(t *testing.T, path, source string) {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // G304: local evaluation fixture, not a shipped input
	if err != nil {
		if source == "owner" && errors.Is(err, os.ErrNotExist) {
			t.Skip("M8 owner P5 NOT EVALUATED: no label fixture")
		}
		t.Fatal("M8 label fixture cannot be opened")
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	closeErr := f.Close()
	if closeErr != nil {
		t.Fatal("close M8 label fixture")
	}
	if readErr != nil || len(data) > 4<<20 {
		t.Fatal("M8 label fixture read failed or exceeded its bound")
	}
	labels, readErr := m8ParseLabels(strings.NewReader(string(data)), source)
	if readErr != nil {
		if source == "owner" && errors.Is(readErr, errM8Pending) {
			t.Skip("M8 owner P5 NOT EVALUATED: pending labels, not a passing evaluation")
		}
		t.Fatal(readErr)
	}
	if source == "agent" {
		if info, err := os.Stat(corpusDir); err != nil || !info.IsDir() {
			t.Fatal("explicit P5 agent evaluation requires its frozen capture corpus")
		}
	}
	docs := corpusDocs(t)
	digest, err := m8CorpusDigest(corpusDir, docs)
	if err != nil || m8CheckCorpusBinding(string(data), digest) != nil {
		t.Fatal("M8 labels do not match the frozen corpus")
	}
	pop, err := m8P5Plan(docs, labels)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("M8 P5 %s: %d positive failures, %d all-no within the candidate window, %d unknown", source, len(pop.cases), pop.allNo, pop.unknown)
	if len(pop.cases) == 0 {
		t.Fatal("P5 NOT EVALUATED: no completely judged positive failure")
	}
	// A captured cwd must pass the public query path's local-drive validation
	// before the ingest helper walks it. Do not print the rejected value.
	for _, d := range docs {
		var fields struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(d.payload, &fields); err != nil {
			continue
		}
		if fields.Cwd != "" {
			if _, err := project.FromArgument(fields.Cwd); err != nil {
				t.Fatal("corpus project path cannot be safely resolved")
			}
		}
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close P5 index: %v", err)
		}
	})
	for _, stmt := range []string{"PRAGMA temp_store=MEMORY", "PRAGMA foreign_keys=ON"} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	ingestInto(t, db, docs)
	pop, err = m8P5Plan(docs, labels) // the ingest helper assigned the event IDs
	if err != nil {
		t.Fatal(err)
	}
	bodies := m8NativeBodiesRequired(t, db, true)
	byID := make(map[string]doc, len(docs))
	for _, d := range docs {
		byID[d.id] = d
	}
	var literal, native, known, echo, reached int
	for _, c := range pop.cases {
		s := m8P5Retrieve(t, db, byID, bodies, c)
		if s.literal {
			literal++
		}
		if s.native {
			native++
		}
		if s.known {
			known++
		}
		if s.echo {
			echo++
		}
		if s.nativeHits > 0 {
			reached++
		}
	}
	t.Logf("P5 %s: literal coverage events %d/%d, native %d/%d; actual fix event %d/%d", source, literal, len(pop.cases), native, len(pop.cases), known, len(pop.cases))
	t.Logf("P5 diagnostics: original failure returned %d/%d; native index reached %d/%d over %d items", echo, len(pop.cases), reached, len(pop.cases), len(bodies))
	t.Log("P5 is retrospective fix retrieval, not as-of-trigger prediction; these figures alone do not prove task success or authorize publication")
}

type m8P5Case struct {
	failure, query  string
	fixes, literals []string
}

type m8P5Population struct {
	cases          []m8P5Case
	allNo, unknown int
}

type m8P5Score struct {
	literal, native, known, echo bool
	nativeHits                   int
}

func TestM8P5RetrievalUsesOrAndSeparatesTheFixFromAnEcho(t *testing.T) {
	docs := []doc{
		{name: "failure", payload: []byte(`{"session_id":"s","tool_response":{"stdout":"error alpha beta repair_token"}}`)},
		{name: "fix", payload: []byte(`{"session_id":"s","tool_input":{"command":"repair_token alpha"}}`)},
	}
	for i := range docs {
		docs[i].leaves = leavesOf(docs[i].payload)
	}
	db := ingestAll(t, docs)
	body := "repair_token alpha"
	if _, err := db.ExecContext(t.Context(), `INSERT INTO memory_items (id,host,kind,source_path,entry_key,project_path,project_id,title,body,host_modified_at,privacy_class,redaction_version,indexed_at) VALUES ('m','codex','entry','synthetic','k','',NULL,'t',?,0,'',1,0)`, body); err != nil {
		t.Fatal(err)
	}
	byID := map[string]doc{docs[0].id: docs[0], docs[1].id: docs[1]}
	c := m8P5Case{failure: docs[0].id, query: "error alpha beta", fixes: []string{"missing", docs[1].id}, literals: []string{"missing", "repair_token"}}
	s := m8P5Retrieve(t, db, byID, map[string]string{"m": body}, c)
	if !s.literal || !s.native || !s.known || !s.echo || s.nativeHits != 1 {
		t.Fatalf("P5 retrieval=%+v", s)
	}
	c.fixes = []string{"not-retrieved"}
	s = m8P5Retrieve(t, db, byID, map[string]string{"m": body}, c)
	if !s.literal || s.known || !s.echo {
		t.Fatalf("literal/ID/echo mixed=%+v", s)
	}
}

func TestM8P5DoesNotCreditTheEleventhFix(t *testing.T) {
	docs := []doc{{name: "fix", payload: []byte(`{"session_id":"s","tool_input":{"command":"alpha uniquerepair"}}`)}}
	for i := range 10 {
		docs = append(docs, doc{name: fmt.Sprintf("decoy%d", i), payload: []byte(`{"session_id":"s","tool_input":{"command":"alpha beta error"}}`)})
	}
	for i := range docs {
		docs[i].leaves = leavesOf(docs[i].payload)
	}
	db := ingestAll(t, docs)
	hits, _, err := search.Search(t.Context(), db, "alpha beta error", "", 11, search.MatchAny)
	if err != nil || len(hits) != 11 || hits[10].ID != docs[0].id {
		t.Fatal("fixture did not place the fix at rank eleven")
	}
	byID := map[string]doc{}
	for _, d := range docs {
		byID[d.id] = d
	}
	c := m8P5Case{query: "alpha beta error", fixes: []string{docs[0].id}, literals: []string{"uniquerepair"}}
	s := m8P5Retrieve(t, db, byID, nil, c)
	if s.known || s.literal {
		t.Fatal("rank eleven counted as top ten")
	}
}

func m8P5Retrieve(t *testing.T, db *sql.DB, docs map[string]doc, bodies map[string]string, c m8P5Case) m8P5Score {
	t.Helper()
	hits, _, err := search.Search(t.Context(), db, c.query, "", m8K, search.MatchAny)
	if err != nil {
		t.Fatal("P5 event query refused")
	}
	mem, _, err := search.SearchMemory(t.Context(), db, c.query, nil, m8K, search.MatchAny)
	if err != nil {
		t.Fatal("P5 native query refused")
	}
	s := m8P5Score{nativeHits: len(mem)}
	for _, h := range hits {
		if h.ID == c.failure {
			s.echo = true
		}
		for _, id := range c.fixes {
			if h.ID == id {
				s.known = true
			}
		}
		for _, literal := range c.literals {
			if docs[h.ID].hasLeaf(literal) {
				s.literal = true
			}
		}
	}
	for _, h := range mem {
		for _, literal := range c.literals {
			if strings.Contains(strings.ToLower(bodies[h.ID]), strings.ToLower(literal)) {
				s.native = true
			}
		}
	}
	return s
}

func TestM8P5PopulationCountsFailuresNotFixes(t *testing.T) {
	docs := []doc{
		{name: "h__A__100__1.json", id: "failure", payload: []byte(`{"session_id":"s","tool_response":{"stdout":"error alpha beta"}}`)},
		{name: "h__A__200__1.json", id: "fix1", payload: []byte(`{"session_id":"s","tool_input":{"command":"repair_alpha"}}`)},
		{name: "h__A__300__1.json", id: "fix2", payload: []byte(`{"session_id":"s","tool_input":{"command":"repair_beta"}}`)},
	}
	for _, tc := range []struct {
		first, second         string
		positive, no, unknown int
	}{
		{"yes", "yes", 1, 0, 0}, {"no", "no", 0, 1, 0}, {"yes", "unknown", 0, 0, 1},
	} {
		labels := map[m8LabelKey]string{{docs[0].name, docs[1].name}: tc.first, {docs[0].name, docs[2].name}: tc.second}
		got, err := m8P5Plan(docs, labels)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.cases) != tc.positive || got.allNo != tc.no || got.unknown != tc.unknown {
			t.Fatalf("population counts=%d/%d/%d", len(got.cases), got.allNo, got.unknown)
		}
		if tc.positive > 0 && (len(got.cases[0].fixes) != 2 || len(got.cases[0].literals) != 2 || got.cases[0].query != "error alpha beta") {
			t.Fatal("multiple fixes or registered query lost")
		}
	}
	if _, err := m8P5Plan(docs, map[m8LabelKey]string{}); !errors.Is(err, errM8Rows) {
		t.Fatal("missing pairs accepted")
	}
}

func TestM8P5QueryUsesOnlyTheFirst32Tokens(t *testing.T) {
	line := "error " + strings.Repeat("alpha ", 31) + "excluded"
	if got := m8P5Query(line); got != "error "+strings.TrimSpace(strings.Repeat("alpha ", 31)) {
		t.Fatal("query does not preserve the registered token window")
	}
}

func m8P5Plan(docs []doc, labels map[m8LabelKey]string) (m8P5Population, error) {
	var out m8P5Population
	consumed := 0
	for _, pair := range m8Pairs(docs) {
		failure := docs[pair.failure]
		if m8Session(failure.payload) == "" || m8Stamp(failure.name) <= 0 {
			return out, errM8Rows
		}
		c := m8P5Case{failure: failure.id, query: m8P5Query(pair.line)}
		unknown := false
		for _, i := range pair.candidates {
			fix := docs[i]
			label, ok := labels[m8LabelKey{failure.name, fix.name}]
			if !ok {
				return out, errM8Rows
			}
			consumed++
			switch label {
			case "unknown":
				unknown = true
			case "no":
			case "yes":
				if m8Stamp(fix.name) <= m8Stamp(failure.name) || strings.Split(fix.name, "__")[0] != strings.Split(failure.name, "__")[0] {
					return out, errM8Rows
				}
				derived := store.Derive(fix.payload)
				literal := m4DeriveFromCommand(derived.Cmd)
				if derived.Cmd == "" {
					literal = m4DeriveFromPath(derived.Paths)
				}
				if literal == "" {
					return out, errM8Rows
				}
				c.fixes = append(c.fixes, fix.id)
				c.literals = append(c.literals, literal)
			default:
				return out, errM8Rows
			}
		}
		switch {
		case unknown:
			out.unknown++
		case len(c.fixes) == 0:
			out.allNo++
		default:
			out.cases = append(out.cases, c)
		}
	}
	if consumed != len(labels) {
		return out, errM8Rows
	}
	return out, nil
}
func m8P5Query(line string) string {
	fields := strings.Fields(line)
	return strings.Join(fields[:min(len(fields), 32)], " ")
}
