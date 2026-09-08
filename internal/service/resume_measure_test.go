package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/search"
)

// This is a known-item continuation measurement over a frozen real database,
// not agent task success, relevance judging or permission to enable injection.
func TestMeasureSessionResumeOverFrozenHistory(t *testing.T) {
	path := os.Getenv("ENGRAMUX_RESUME_SNAPSHOT")
	root := os.Getenv("ENGRAMUX_RESUME_PROJECT")
	if path == "" {
		t.Skip("set ENGRAMUX_RESUME_SNAPSHOT and ENGRAMUX_RESUME_PROJECT")
	}
	p, err := project.FromArgument(root)
	if err != nil {
		t.Fatal("invalid evaluation project")
	}
	abs, err := filepath.Abs(path)
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
	// Enumerate the independent reference population without the candidate
	// query's window or LIMIT: pick the latest tuple in Go for every session.
	type key struct{ host, session string }
	type ref struct {
		id         string
		stamp, row int64
	}
	want := map[key]ref{}
	rows, err := db.QueryContext(t.Context(), `SELECT e.id,e.received_at,e.rowid,e.host,s.host_session_id
		FROM events e JOIN sessions s ON s.id=e.session_id
		WHERE e.project_id=? AND e.event_name='Stop' AND e.host IN ('claude-code','codex') AND e.host=s.host`, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k key
		var r ref
		if err := rows.Scan(&r.id, &r.stamp, &r.row, &k.host, &k.session); err != nil {
			t.Fatal(err)
		}
		old, ok := want[k]
		if !ok || r.stamp > old.stamp || (r.stamp == old.stamp && r.row > old.row) {
			want[k] = r
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("no eligible captured replies")
	}
	var ftsFound, resumeFound, bodyFound, shortened int
	var worst time.Duration
	for k, expected := range want {
		ctx, cancel := context.WithTimeout(t.Context(), readDeadline)
		hits, _, err := search.Search(ctx, db, k.session, p.ID, 100, search.MatchAny)
		cancel()
		if err != nil {
			t.Fatal("baseline search failed")
		}
		found := false
		for _, h := range hits {
			if h.ID == expected.id {
				found = true
			}
		}
		if found {
			ftsFound++
		}
		ctx, cancel = context.WithTimeout(t.Context(), readDeadline)
		start := time.Now()
		got, err := sessionResume(ctx, db, ipc.SessionResumeRequest{Project: root, Host: k.host, HostSessionID: k.session})
		took := time.Since(start)
		cancel()
		if took > worst {
			worst = took
		}
		if err != nil {
			t.Fatal("resume read failed")
		}
		if got.LatestReply != nil && got.LatestReply.EventID == expected.id {
			resumeFound++
		} else {
			t.Fatal("resume missed the latest captured reply")
		}
		if got.LatestReply.Body != "" {
			bodyFound++
		}
		if got.LatestReply.Truncated {
			shortened++
		}
		if k.session == os.Getenv("ENGRAMUX_RESUME_REVIEW_SESSION") {
			t.Logf("user-named session: final captured reply in FTS top100=%v; resume=true; body bytes=%d; truncated=%v", found, len(got.LatestReply.Body), got.LatestReply.Truncated)
			if os.Getenv("ENGRAMUX_WRITE_RESUME_REVIEW") == "1" {
				b, err := json.MarshalIndent(got, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				f, err := os.OpenFile(filepath.Join(filepath.Dir(abs), "named-session.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
				if err != nil {
					t.Fatal("review output already exists or cannot be created")
				}
				_, writeErr := f.Write(b)
				closeErr := f.Close()
				if writeErr != nil || closeErr != nil {
					t.Fatal("write review artifact failed")
				}
			}
		}
	}
	t.Logf("latest captured reply: FTS top100=%d/%d; exact resume=%d/%d; nonempty bodies=%d; truncated=%d; slowest resume=%s", ftsFound, len(want), resumeFound, len(want), bodyFound, shortened, worst)
}
