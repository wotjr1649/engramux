package search_test

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/secret"
)

// Keep the existing scaling gates unchanged. This probe separates their SQL
// read cost from masking and excerpting the records they actually return.
func TestDiagnoseSearchCostPhases(t *testing.T) {
	if os.Getenv("ENGRAMUX_SEARCH_COST_DIAGNOSTIC") != "1" {
		t.Skip("opt-in search cost diagnosis")
	}
	for _, memory := range []bool{false, true} {
		count, limit := 20000, 20
		build := payloadCorpus
		kind := "event"
		if memory {
			count, limit, build, kind = 5000, 1, memoryPayloadCorpus, "memory"
		}
		thin, fat := build(t, count, 64), build(t, count, 8192)
		var measured []string
		runCostBenchmark(t, func(b *testing.B) {
			measured = nil
			for iteration := 0; iteration < 4; iteration++ {
				for _, arm := range []struct {
					name string
					db   *sql.DB
				}{{"thin", thin}, {"fat", fat}} {
					sqlTime, postTime, goSQL, goPost, bytes := costPhases(b, arm.db, memory, limit, count)
					if iteration > 0 {
						measured = append(measured, fmt.Sprintf("%s %s run=%d benchmark_SQL=%s benchmark_post=%s Go_SQL=%s Go_post=%s returned_bytes=%d", kind, arm.name, iteration, sqlTime, postTime, goSQL, goPost, bytes))
					}
				}
			}
		})
		for _, line := range measured {
			t.Log(line)
		}
	}
}

func costPhases(t *testing.B, db *sql.DB, memory bool, limit, count int) (time.Duration, time.Duration, time.Duration, time.Duration, int) {
	t.Helper()
	term := payloadProbeTerm
	tokens, err := search.QueryTokens(term)
	if err != nil {
		t.Fatal(err)
	}
	query, args := search.CostEventQuery(tokens, "", limit, true, 0, nil, search.MatchAll)
	if memory {
		query, args = search.CostMemoryQuery(tokens, nil, limit, search.MatchAll)
	}
	type row struct {
		event   search.Hit
		memory  search.MemoryHit
		payload []byte
		body    string
	}
	goStart := time.Now()
	start := t.Elapsed()
	rows, err := db.QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var scanned []row
	var total int64
	var bytes int
	for rows.Next() {
		var r row
		if memory {
			var mod sql.NullInt64
			err = rows.Scan(&r.memory.ID, &r.memory.Host, &r.memory.Kind, &r.memory.SourcePath, &r.memory.Title, &r.body, &mod, &total)
			r.memory.HostModifiedMS = mod.Int64
			bytes += len(r.body)
		} else {
			err = rows.Scan(&r.event.ID, &r.event.Host, &r.event.EventName, &r.event.ReceivedAtMS, &r.payload, &total)
			bytes += len(r.payload)
		}
		if err != nil {
			t.Fatal(err)
		}
		scanned = append(scanned, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	sqlTime := t.Elapsed() - start
	goSQL := time.Since(goStart)
	if len(scanned) != limit || total != int64(count) {
		t.Fatal("phase probe does not reach the gate population")
	}
	goStart = time.Now()
	start = t.Elapsed()
	events := make([]search.Hit, 0, len(scanned))
	memories := make([]search.MemoryHit, 0, len(scanned))
	for _, r := range scanned {
		if memory {
			h := r.memory
			h.SourcePath = secret.MaskString(h.SourcePath)
			h.Title = secret.MaskString(h.Title)
			h.Excerpt = search.CostTextExcerpt(secret.MaskString(r.body), tokens)
			memories = append(memories, h)
		} else {
			h := r.event
			h.Excerpt = search.CostEventExcerpt(r.payload, tokens)
			events = append(events, h)
		}
	}
	postTime := t.Elapsed() - start
	goPost := time.Since(goStart)
	// Outside the timed phases, require exact equality with the shipped
	// public result. A copied assembly that drifts is a broken instrument.
	if memory {
		want, n, err := search.SearchMemory(t.Context(), db, term, nil, limit, search.MatchAll)
		if err != nil || n != total || !reflect.DeepEqual(want, memories) {
			t.Fatal("memory phase instrument differs from shipped output")
		}
	} else {
		want, n, err := search.Search(t.Context(), db, term, "", limit, search.MatchAll)
		if err != nil || n != total || !reflect.DeepEqual(want, events) {
			t.Fatal("event phase instrument differs from shipped output")
		}
	}
	return sqlTime, postTime, goSQL, goPost, bytes
}
