package inject_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/secret"
)

// Freeze before opening development prompts. No selector is called here, and
// no private identity or text is printed. Earlier exact-session metadata
// exposure is disclosed in the manifest rather than mistaken for blindness.
func TestWriteFollowupSelectionSplit(t *testing.T) {
	if os.Getenv("ENGRAMUX_WRITE_FOLLOWUP_SPLIT") != "1" {
		t.Skip("followup split writer is opt-in")
	}
	const source = "../../.capture/session-resume-2026-09-08/engramux.db"
	const dest = "../../.capture/selection-quality/followup-2026-09-08"
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("split destination exists or cannot be inspected")
	}
	root := os.Getenv("ENGRAMUX_FOLLOWUP_PROJECT")
	p, err := project.FromArgument(root)
	if err != nil {
		t.Fatal("invalid evaluation project")
	}
	excluded := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("ENGRAMUX_FOLLOWUP_EXCLUDED_SESSIONS"), ";") {
		if s != "" {
			excluded[s] = true
		}
	}
	if len(excluded) != 2 {
		t.Fatal("provide the two explicitly exposed host/session identities")
	}
	open := func(path string) *sql.DB {
		t.Helper()
		u, err := temporalSourceURI(path)
		if err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", u)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
		return db
	}
	before, after := open(m7Snapshot), open(source)
	digests := map[string]string{"prior_db": followupHash(t, m7Snapshot), "db": followupHash(t, source), "wal": followupHash(t, source+"-wal")}
	old := map[string]bool{}
	oldRows, err := before.QueryContext(t.Context(), "SELECT id FROM sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = oldRows.Close() }()
	for oldRows.Next() {
		var s string
		if err := oldRows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		old[s] = true
	}
	if err := oldRows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := oldRows.Close(); err != nil {
		t.Fatal(err)
	}
	rows, err := after.QueryContext(t.Context(), "SELECT id,session_id,payload FROM events WHERE project_id=? AND event_name='UserPromptSubmit' ORDER BY received_at,rowid", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	type prompt struct{ ID, Session, Line string }
	var prompts []prompt
	sessions := map[string]bool{}
	var excludedRows, invalidRows int
	for rows.Next() {
		var id, s string
		var payload []byte
		if err := rows.Scan(&id, &s, &payload); err != nil {
			t.Fatal(err)
		}
		if old[s] {
			continue
		}
		if excluded[s] {
			excludedRows++
			continue
		}
		sessions[s] = true
		var doc struct {
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(secret.Mask(payload), &doc) != nil || strings.TrimSpace(doc.Prompt) == "" {
			invalidRows++
			continue
		}
		prompts = append(prompts, prompt{id, s, strings.Join([]string{id, m7Script(doc.Prompt), m7Todo, m7Line(doc.Prompt)}, "\t")})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	var order []string
	for s := range sessions {
		order = append(order, s)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := fmt.Sprintf("%x", sha256.Sum256([]byte(order[i]))), fmt.Sprintf("%x", sha256.Sum256([]byte(order[j])))
		if a == b {
			return order[i] < order[j]
		}
		return a < b
	})
	if len(order) < 2 {
		t.Fatal("not enough unreviewed sessions to split")
	}
	assignment := map[string]string{}
	for i, s := range order {
		arm := "holdout"
		if i < len(order)/2 {
			arm = "development"
		}
		assignment[s] = arm
	}
	outputs := map[string][]string{"development": {}, "holdout": {}}
	for _, p := range prompts {
		outputs[assignment[p.Session]] = append(outputs[assignment[p.Session]], p.Line)
	}
	if len(outputs["development"]) == 0 || len(outputs["holdout"]) == 0 {
		t.Fatal("empty split; do not reroll")
	}
	var schema int64
	if err := after.QueryRowContext(t.Context(), "SELECT max(version_id) FROM goose_db_version WHERE is_applied=1").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if followupHash(t, source) != digests["db"] || followupHash(t, source+"-wal") != digests["wal"] {
		t.Fatal("snapshot changed during freeze")
	}
	manifest := map[string]any{"digests": digests, "schema": schema, "go": runtime.Version(), "project_id": p.ID, "assignment": assignment, "explicit_exclusions": excluded, "excluded_prompt_rows": excludedRows, "invalid_prompt_rows": invalidRows, "split_rule": "stored session UTF-8 bytes, SHA-256 ascending, string tie-break, first floor(n/2) development", "prior_exposure": "Latest Stop IDs and body presence/length were aggregated for 32 project sessions. The explicitly reviewed session and current conversation are excluded. Not a new-user or fully independent corpus.", "status": "unlabelled; no selector run"}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dest, 0o750); err != nil {
		t.Fatal("cannot create fresh split")
	}
	outputRoot, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := outputRoot.Close(); err != nil {
			t.Error(err)
		}
	}()
	write := func(path string, b []byte) {
		t.Helper()
		f, err := outputRoot.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal("refusing fixture replacement")
		}
		_, we := f.Write(b)
		ce := f.Close()
		if we != nil || ce != nil {
			t.Fatal("fixture write failed")
		}
	}
	write("manifest.json", encoded)
	for _, arm := range []string{"development", "holdout"} {
		path := filepath.Join(dest, arm)
		if err := os.Mkdir(path, 0o750); err != nil {
			t.Fatal(err)
		}
		header := fmt.Sprintf("# Followup %s; see ../manifest.json\n%s\n# columns: prompt_id, script, wanted_context, prompt\n", arm, m7UnlabelledHeader)
		write(filepath.Join(arm, "prompts.tsv"), []byte(header+strings.Join(outputs[arm], "\n")+"\n"))
	}
	t.Logf("froze %d sessions, %d development and %d holdout prompts; excluded exposed rows=%d, invalid rows=%d; no retrieval", len(order), len(outputs["development"]), len(outputs["holdout"]), excludedRows, invalidRows)
}

func followupHash(t *testing.T, path string) string {
	t.Helper()
	capture, err := os.OpenRoot("../../.capture")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := capture.Close(); err != nil {
			t.Error(err)
		}
	}()
	rel, err := filepath.Rel("../../.capture", path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := capture.Open(rel)
	if err != nil {
		t.Fatal("cannot hash snapshot component")
	}
	h := sha256.New()
	_, readErr := io.Copy(h, f)
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		t.Fatal("snapshot hash failed")
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
