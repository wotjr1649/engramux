package inject_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// This diagnostic preserves the original M7 sample and reports no new relevance
// verdict. Native-memory versions at each trigger are unknown and stay absent.
func TestMeasureTemporalSelection(t *testing.T) {
	dir := os.Getenv("ENGRAMUX_TEMPORAL_AGENT_DIR")
	if dir == "" {
		t.Skip("temporal diagnostic is opt-in through ENGRAMUX_TEMPORAL_AGENT_DIR")
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
			t.Errorf("close temporal source: %v", err)
		}
	})
	prompts := m7LabelledPrompts(t, source, dir, m7Agent)
	stamps := make(map[string]int64, len(prompts))
	for _, p := range prompts {
		var stamp int64
		if err := source.QueryRowContext(t.Context(), "SELECT received_at FROM events WHERE id=?", p.id).Scan(&stamp); err != nil {
			t.Fatal("read trigger timestamp")
		}
		stamps[p.id] = stamp
		if m7Drifted(p) {
			t.Fatal("project identity changed: cannot replay the production injector faithfully")
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
	var emitted, blocks, bytes, unwanted, wanted, wantedEmitted, deadlines int
	var notifications, notificationInjections, notificationBytes int
	type triggerMetric struct {
		ID                    string
		Wanted                string
		CompletedNotification bool
		Events, Bytes         int
	}
	var metrics []triggerMetric
	type reviewBlock struct {
		PromptID, Prompt, EventID, EventName, Text string
		Bytes                                      int
	}
	var review []reviewBlock
	for _, p := range prompts {
		if err := prefix.advance(t, stamps[p.id]); err != nil {
			t.Fatal("advance temporal index")
		}
		var res inject.Result
		if os.Getenv("ENGRAMUX_TEMPORAL_PAIRED") == "1" {
			res = pairedBuild(t, prefix.db, p, stamps[p.id])
		} else if os.Getenv("ENGRAMUX_TEMPORAL_CONTINUATION") == "1" {
			res = continuationBuild(t, prefix.db, p, stamps[p.id])
		} else {
			res = m7Build(t, prefix.db, p)
		}
		notification := strings.HasPrefix(strings.TrimSpace(p.prompt), "<task-notification>") && strings.HasSuffix(strings.TrimSpace(p.prompt), "</task-notification>") && strings.Contains(p.prompt, "<status>completed</status>")
		if notification {
			notifications++
		}
		metric := triggerMetric{ID: p.id, Wanted: p.wanted, CompletedNotification: notification, Events: len(res.Events)}
		if p.wanted == m7Yes {
			wanted++
		}
		if res.Reason == inject.ReasonDeadline {
			deadlines++
		}
		if res.Text == "" {
			metrics = append(metrics, metric)
			continue
		}
		emitted++
		if notification {
			notificationInjections++
		}
		if p.wanted == m7Yes {
			wantedEmitted++
		}
		for _, b := range m7SplitBlocks(t, p.id, res) {
			if b.kind != "event" {
				t.Fatal("a current native-memory version entered historical replay")
			}
			var stamp int64
			if err := prefix.db.QueryRowContext(t.Context(), "SELECT received_at FROM events WHERE id=?", b.id).Scan(&stamp); err != nil {
				t.Fatal("read selected timestamp")
			}
			if stamp >= stamps[p.id] {
				t.Fatal("selected an event not strictly earlier than the trigger")
			}
			blocks++
			if os.Getenv("ENGRAMUX_WRITE_TEMPORAL_REVIEW") == "1" && p.wanted == m7Yes {
				var kind string
				if err := prefix.db.QueryRowContext(t.Context(), "SELECT event_name FROM events WHERE id=?", b.id).Scan(&kind); err != nil {
					t.Fatal("read review event kind")
				}
				review = append(review, reviewBlock{PromptID: p.id, Prompt: p.prompt, EventID: b.id, EventName: kind, Text: b.text, Bytes: b.bytes})
			}
			bytes += b.bytes
			metric.Bytes += b.bytes
			if notification {
				notificationBytes += b.bytes
			}
			if p.wanted == m7No {
				unwanted += b.bytes
			}
		}
		metrics = append(metrics, metric)
	}
	writeMetrics := os.Getenv("ENGRAMUX_WRITE_TEMPORAL_METRICS") == "1"
	writeReview := os.Getenv("ENGRAMUX_WRITE_TEMPORAL_REVIEW") == "1"
	if writeMetrics && writeReview {
		t.Fatal("select one export per run")
	}
	if writeMetrics || writeReview {
		var value any = metrics
		name := "temporal-trigger-metrics.json"
		if writeReview {
			value, name = review, "temporal-wanted-review.json"
			if os.Getenv("ENGRAMUX_TEMPORAL_PAIRED") == "1" {
				name = "temporal-paired-wanted-review.json"
			}
		}
		if os.Getenv("ENGRAMUX_TEMPORAL_CONTINUATION") == "1" {
			name = "continuation-v2-" + name
		}
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot("../../.capture/selection-quality")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := root.Close(); err != nil {
				t.Error(err)
			}
		}()
		f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal("metrics exist or cannot be created")
		}
		_, we := f.Write(b)
		ce := f.Close()
		if we != nil || ce != nil {
			t.Fatal("write trigger metrics failed")
		}
	}
	t.Logf("event-only temporal diagnostic: %d prompts, %d injected, %d blocks, %d bytes, %d deadline abstentions", len(prompts), emitted, blocks, bytes, deadlines)
	t.Logf("agent prompt estimates: %d/%d wanted prompts injected; unwanted bytes %d/%d (%.3f)", wantedEmitted, wanted, unwanted, bytes, m7Ratio(unwanted, bytes))
	t.Logf("completed notification triggers: %d prompts, %d injected, %d excerpt bytes", notifications, notificationInjections, notificationBytes)
	t.Log("NOT A QUALITY PASS: prefix outputs need fresh block judgements; no recall or task-utility claim")
}

func TestTemporalReplayIndexesOnlyStrictlyEarlierEvents(t *testing.T) {
	past := event("PostToolUse", "quartz repair answer", "")
	tied := event("PostToolUse", "quartz later answer", "")
	future := event("PostToolUse", "quartz future answer", "")
	source := corpus(t, past, tied, future)
	for i, id := range []string{past.id, tied.id, future.id} {
		if _, err := source.ExecContext(t.Context(), "UPDATE events SET received_at=? WHERE id=?", 100+i*100, id); err != nil {
			t.Fatal(err)
		}
	}
	var seq int
	var name, path string
	if err := source.QueryRowContext(t.Context(), "PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	prefix, err := newTemporalReplay(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := prefix.advance(t, 200); err != nil {
		t.Fatal(err)
	}
	assertHits := func(want int64) {
		t.Helper()
		hits, total, err := search.Search(t.Context(), prefix.db, "quartz", "", 10, search.MatchAll)
		if err != nil || total != want || int64(len(hits)) != want {
			t.Fatalf("prefix hits=%d total=%d error=%v, want %d", len(hits), total, err, want)
		}
		if want == 1 && hits[0].ID != past.id {
			t.Fatal("the only result must be the earlier event")
		}
	}
	assertHits(1)
	_, total, err := search.Search(t.Context(), prefix.db, "future", "", 10, search.MatchAll)
	if err != nil || total != 0 {
		t.Fatalf("future-only token total=%d error=%v", total, err)
	}
	if err := prefix.advance(t, 200); err != nil {
		t.Fatal(err)
	}
	assertHits(1)
	if err := prefix.advance(t, 301); err != nil {
		t.Fatal(err)
	}
	assertHits(3)
	if err := prefix.advance(t, 200); !errors.Is(err, errTemporalOrder) {
		t.Fatalf("backward replay error=%v", err)
	}
	if _, err := prefix.db.ExecContext(t.Context(), "DELETE FROM history.events"); err == nil {
		t.Fatal("the attached snapshot accepted a write")
	}
	var count int
	if err := prefix.db.QueryRowContext(t.Context(), "SELECT count(*) FROM history.events").Scan(&count); err != nil || count != 3 {
		t.Fatalf("source count=%d error=%v", count, err)
	}
	if filepath.Ext(path) != ".db" {
		t.Fatal("fixture must exercise a disk snapshot")
	}
}

var errTemporalOrder = errors.New("temporal replay cannot move backward")

type temporalReplay struct {
	db     *sql.DB
	before int64
}

// The replay uses the real schema and FTS triggers, but no disk-backed copy of
// captured text. Store.Open's WAL requirement belongs to the service, not this
// disposable in-memory measurement. The attached source is always read-only.
func newTemporalReplay(t *testing.T, path string) (*temporalReplay, error) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close temporal replay: %v", err)
		}
	})
	for _, stmt := range []string{"PRAGMA temp_store=MEMORY", "PRAGMA foreign_keys=ON"} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			return nil, err
		}
	}
	if err := store.Migrate(t.Context(), db); err != nil {
		return nil, err
	}
	uri, err := temporalSourceURI(path)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(t.Context(), "ATTACH DATABASE ? AS history", uri); err != nil {
		return nil, err
	}
	// These rows supply foreign keys only: neither table participates in FTS
	// ranking. The project identity is the one captured, never today's cwd.
	for _, stmt := range []string{
		"INSERT INTO main.projects SELECT * FROM history.projects",
		"INSERT INTO main.sessions SELECT * FROM history.sessions",
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			return nil, err
		}
	}
	return &temporalReplay{db: db}, nil
}

func temporalSourceURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(abs), RawQuery: "mode=ro"}
	return u.String(), nil
}

func (r *temporalReplay) advance(t *testing.T, before int64) error {
	t.Helper()
	if before <= 0 || before < r.before {
		return errTemporalOrder
	}
	// Equal timestamps cannot establish which commit was visible first. Exclude
	// the whole tied group instead of inventing an order from UUIDs or rowids.
	_, err := r.db.ExecContext(t.Context(), `INSERT INTO main.events
		(rowid, id, project_id, session_id, host, source, event_name, tool_name,
		 tool_use_id, payload, privacy_class, redaction_version, received_at,
		 leaves, derived_cmd, derived_paths, derived_output)
		SELECT rowid, id, project_id, session_id, host, source, event_name, tool_name,
		 tool_use_id, payload, privacy_class, redaction_version, received_at,
		 leaves, derived_cmd, derived_paths, derived_output
		FROM history.events WHERE received_at > 0 AND received_at >= ? AND received_at < ?
		ORDER BY received_at, rowid`, r.before, before)
	if err == nil {
		r.before = before
	}
	return err
}
