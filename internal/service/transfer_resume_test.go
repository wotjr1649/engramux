package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

func TestMeasureTransferNamedResume(t *testing.T) {
	if os.Getenv("ENGRAMUX_TRANSFER_NAMED_RESUME") != "1" {
		t.Skip("development named-session audit is opt-in")
	}
	const dir = "../../.capture/selection-quality/transfer-2026-09-08"
	data, err := os.ReadFile(dir + "/development-prompts.json")
	if err != nil {
		t.Fatal("development prompts unavailable")
	}
	var prompts []struct{ ID, Text string }
	if json.Unmarshal(data, &prompts) != nil || len(prompts) != 23 {
		t.Fatal("invalid development prompts")
	}
	// This fixed, previously inspected development case names a source Codex session.
	// The other named-session request specifies a handoff destination and is not this case.
	p := prompts[20]
	names := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`).FindAllString(p.Text, -1)
	if len(names) != 1 {
		t.Fatal("expected one named source session")
	}
	abs, err := filepath.Abs("../../.capture/session-resume-2026-09-08/engramux.db")
	if err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(abs), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	var host, session, payload string
	var cutoff int64
	if err := db.QueryRowContext(t.Context(), "SELECT host,session_id,received_at,json_object('cwd',json_extract(payload,'$.cwd')) FROM events WHERE id=? AND event_name='UserPromptSubmit'", p.ID).Scan(&host, &session, &cutoff, &payload); err != nil {
		t.Fatal("trigger missing")
	}
	var fields struct {
		CWD string `json:"cwd"`
	}
	if json.Unmarshal([]byte(payload), &fields) != nil || host != "codex" || cutoff <= 0 {
		t.Fatal("unexpected source scope")
	}
	var namedID string
	if err := db.QueryRowContext(t.Context(), "SELECT id FROM sessions WHERE host=? AND host_session_id=?", host, names[0]).Scan(&namedID); err != nil {
		t.Fatal("named session absent")
	}
	if namedID == session {
		t.Fatal("case is not cross-session")
	}
	// A connection-local view limits the unmodified product query to prior events.
	// Only a typed database timestamp enters this DDL; no captured text is SQL syntax.
	if _, err := db.ExecContext(t.Context(), fmt.Sprintf("CREATE TEMP VIEW events AS SELECT rowid AS rowid,* FROM main.events WHERE received_at>0 AND received_at<%d", cutoff)); err != nil {
		t.Fatal("prefix view failed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), readDeadline)
	got, err := sessionResume(ctx, db, ipc.SessionResumeRequest{Project: fields.CWD, Host: host, HostSessionID: names[0]})
	cancel()
	if err != nil {
		t.Fatal("named resume failed")
	}
	for _, msg := range []*ipc.ResumeMessage{got.LatestPrompt, got.LatestReply} {
		if msg != nil && (msg.ReceivedAtMS <= 0 || msg.ReceivedAtMS >= cutoff) {
			t.Fatal("future result")
		}
	}
	encoded, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(dir+"/named-source-resume.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("named export exists or unavailable")
	}
	_, we := f.Write(encoded)
	ce := f.Close()
	if we != nil || ce != nil {
		t.Fatal("named export failed")
	}
	if got.LatestReply == nil {
		t.Log("named source: no prior reply")
	} else {
		t.Logf("named source: reply_bytes=%d truncated=%t", len(got.LatestReply.Body), got.LatestReply.Truncated)
	}
}
