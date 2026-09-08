package inject_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func pairKey(host string, payload []byte) string {
	field := "prompt_id"
	if host == "codex" {
		field = "turn_id"
	} else if host != "claude-code" {
		return ""
	}
	if len(payload) > 1048576 || !json.Valid(payload) {
		return ""
	}
	d := json.NewDecoder(bytes.NewReader(payload))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return ""
	}
	var key string
	seen := false
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return ""
		}
		var raw json.RawMessage
		if err = d.Decode(&raw); err != nil {
			return ""
		}
		if tok == field {
			if seen {
				return ""
			}
			seen = true
			if err = json.Unmarshal(raw, &key); err != nil {
				return ""
			}
		}
	}
	return key
}

func TestPairKeyUsesOnlyTheHostsField(t *testing.T) {
	for _, tc := range []struct{ host, payload, want string }{
		{"claude-code", `{"prompt_id":"p","turn_id":"other"}`, "p"},
		{"codex", `{"prompt_id":"other","turn_id":"t"}`, "t"},
		{"claude-code", `{"turn_id":"t"}`, ""},
		{"codex", `{"prompt_id":"p"}`, ""},
		{"unknown", `{"prompt_id":"p"}`, ""},
		{"claude-code", `{"prompt_id":null}`, ""},
		{"claude-code", `{"prompt_id":4}`, ""},
		{"claude-code", `{"prompt_id":"p","prompt_id":"p"}`, ""},
		{"claude-code", `{"prompt_id":"p"`, ""},
	} {
		if got := pairKey(tc.host, []byte(tc.payload)); got != tc.want {
			t.Errorf("key=%q want=%q", got, tc.want)
		}
	}
}

func TestMeasureExplicitPairResolver(t *testing.T) {
	if os.Getenv("ENGRAMUX_PAIR_AUDIT") != "1" {
		t.Skip("explicit-pair snapshot audit is opt-in")
	}
	uri, err := temporalSourceURI("../../.capture/session-resume-2026-09-08/engramux.db")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	rows, err := db.QueryContext(t.Context(), `SELECT id,host,session_id,project_id,event_name,received_at,
	CASE WHEN length(CAST(payload AS BLOB))<=1048576 THEN payload ELSE NULL END
	FROM events WHERE host IN ('claude-code','codex') AND event_name IN ('UserPromptSubmit','Stop') LIMIT 100001`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var records []pairRecord
	hosts := map[string]string{}
	for rows.Next() {
		var r pairRecord
		var payload []byte
		if err := rows.Scan(&r.id, &r.host, &r.session, &r.project, &r.kind, &r.stamp, &payload); err != nil {
			t.Fatal("scan pair metadata")
		}
		r.key = pairKey(r.host, payload)
		records = append(records, r)
		hosts[r.id] = r.host
		if len(records) > 100000 {
			t.Fatal("audit row bound")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	pairs := resolvePairs(records, 1<<62)
	counts := map[string]int{}
	for id := range pairs {
		counts[hosts[id]]++
	}
	if counts["claude-code"] != 604 || counts["codex"] != 39 {
		t.Fatalf("pair counts differ from independent audit: %v", counts)
	}
	t.Logf("explicit unique pairs: claude-code=%d codex=%d; source records=%d; no answer relevance claim", counts["claude-code"], counts["codex"], len(records))
}

type pairRecord struct {
	id, host, session, project, kind, key string
	stamp                                 int64
}

// This experiment resolves IDs only. Reading, masking, fencing and ranking
// answer bodies remain separate; a pair is not a relevance judgement.
func resolvePairs(records []pairRecord, cutoff int64) map[string]string {
	type scope struct{ host, session, project, key string }
	type group struct{ prompts, answers []string }
	groups := map[scope]*group{}
	for _, r := range records {
		if r.stamp <= 0 || r.stamp >= cutoff || r.id == "" || r.session == "" || r.project == "" || (r.host != "claude-code" && r.host != "codex") || strings.TrimSpace(r.key) == "" || len(r.key) > 256 || strings.ContainsRune(r.key, 0) {
			continue
		}
		k := scope{r.host, r.session, r.project, r.key}
		g := groups[k]
		if g == nil {
			g = &group{}
			groups[k] = g
		}
		switch r.kind {
		case "UserPromptSubmit":
			g.prompts = append(g.prompts, r.id)
		case "Stop":
			g.answers = append(g.answers, r.id)
		}
	}
	out := map[string]string{}
	for _, g := range groups {
		if len(g.prompts) == 1 && len(g.answers) == 1 {
			out[g.prompts[0]] = g.answers[0]
		}
	}
	return out
}

func TestPairResolverIntegrity(t *testing.T) {
	base := []pairRecord{
		{"p", "claude-code", "s", "project", "UserPromptSubmit", "turn", 20},
		{"a", "claude-code", "s", "project", "Stop", "turn", 10},
	}
	for _, tc := range []struct {
		name   string
		change func([]pairRecord) []pairRecord
		want   map[string]string
	}{
		{"reversed arrival", func(r []pairRecord) []pairRecord { return r }, map[string]string{"p": "a"}},
		{"duplicate answer", func(r []pairRecord) []pairRecord { x := r[1]; x.id = "a2"; return append(r, x) }, map[string]string{}},
		{"duplicate prompt", func(r []pairRecord) []pairRecord { x := r[0]; x.id = "p2"; return append(r, x) }, map[string]string{}},
		{"project mismatch", func(r []pairRecord) []pairRecord { r[1].project = "other"; return r }, map[string]string{}},
		{"session mismatch", func(r []pairRecord) []pairRecord { r[1].session = "other"; return r }, map[string]string{}},
		{"host mismatch", func(r []pairRecord) []pairRecord { r[1].host = "codex"; return r }, map[string]string{}},
		{"key mismatch", func(r []pairRecord) []pairRecord { r[1].key = "other"; return r }, map[string]string{}},
		{"tied cutoff", func(r []pairRecord) []pairRecord { r[1].stamp = 30; return r }, map[string]string{}},
		{"future prompt", func(r []pairRecord) []pairRecord { r[0].stamp = 31; return r }, map[string]string{}},
		{"invalid timestamp", func(r []pairRecord) []pairRecord { r[0].stamp = 0; return r }, map[string]string{}},
		{"missing key", func(r []pairRecord) []pairRecord { r[0].key = ""; r[1].key = ""; return r }, map[string]string{}},
		{"unknown host", func(r []pairRecord) []pairRecord { r[0].host = "unknown"; r[1].host = "unknown"; return r }, map[string]string{}},
		{"subagent excluded", func(r []pairRecord) []pairRecord { r[1].kind = "SubagentStop"; return r }, map[string]string{}},
		{"codex", func(r []pairRecord) []pairRecord { r[0].host = "codex"; r[1].host = "codex"; return r }, map[string]string{"p": "a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := tc.change(append([]pairRecord(nil), base...))
			if got := resolvePairs(records, 30); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("pairs=%v want=%v", got, tc.want)
			}
		})
	}
}
