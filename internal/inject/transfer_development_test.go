package inject_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/wotjr1649/engramux/internal/secret"
)

func TestWriteTransferDevelopmentPrompts(t *testing.T) {
	if os.Getenv("ENGRAMUX_WRITE_TRANSFER_DEVELOPMENT") != "1" {
		t.Skip("development-only export is opt-in")
	}
	const dir = "../../.capture/selection-quality/transfer-2026-09-08"
	const source = "../../.capture/session-resume-2026-09-08/engramux.db"
	data, err := os.ReadFile(dir + "/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		ID      string `json:"event_id"`
		Session string `json:"session_id"`
		Project string `json:"project_id"`
		Stamp   int64  `json:"received_at"`
	}
	var manifest struct {
		Digests map[string]string
		Arms    map[string][]row
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if followupHash(t, source) != manifest.Digests["db"] || followupHash(t, source+"-wal") != manifest.Digests["wal"] {
		t.Fatal("snapshot binding mismatch")
	}
	selected := manifest.Arms["development"]
	if len(selected) != 23 {
		t.Fatal("unexpected development split")
	}
	uri, err := temporalSourceURI(source)
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
	type prompt struct{ ID, Text string }
	var output []prompt
	for _, r := range selected {
		var payload []byte
		if err := db.QueryRowContext(t.Context(), `SELECT CASE WHEN length(CAST(payload AS BLOB))<=1048576 THEN payload ELSE NULL END FROM events WHERE id=? AND session_id=? AND project_id=? AND received_at=? AND event_name='UserPromptSubmit'`, r.ID, r.Session, r.Project, r.Stamp).Scan(&payload); err != nil {
			t.Fatal("development source row mismatch")
		}
		var p struct {
			Prompt string `json:"prompt"`
		}
		if len(payload) == 0 || json.Unmarshal(secret.Mask(payload), &p) != nil {
			t.Fatal("invalid or oversized development payload")
		}
		output = append(output, prompt{r.ID, p.Prompt})
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	f, err := root.OpenFile("development-prompts.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("development export exists or cannot be created")
	}
	_, writeErr := f.Write(encoded)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("development export failed")
	}
	t.Logf("exported %d masked development prompts; holdout bodies were not read", len(output))
}
