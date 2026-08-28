package search_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/store"
)

// corpusDir is the local, gitignored raw capture corpus, relative to this
// package. Same path and skip pattern as internal/fixtures and internal/host.
var corpusDir = filepath.Join("..", "..", ".capture", "fixtures-raw")

// doc is one document the gate searches for: one event, its bytes, the string
// leaves a query is derived from, and the events.id it was ingested under.
//
// name is a fixture file name or a corpus file name. It is the only thing about
// a corpus document that a failure message may print besides the derived query
// (I-10): the names are host__Event__nanos__pid.json and carry no user text.
type doc struct {
	name    string
	payload json.RawMessage
	leaves  []string
	id      string
}

// hasLeaf reports whether any of d's leaves contains want, case-insensitively;
// want is lower-cased already, since only the leaf is folded. It is the
// precision assertion's denominator, one document at a time.
func (d doc) hasLeaf(want string) bool {
	for _, l := range d.leaves {
		if strings.Contains(strings.ToLower(l), want) {
			return true
		}
	}
	return false
}

// leavesOf returns every string leaf of a JSON payload, in a deterministic
// order: object keys sorted, array elements in index order.
//
// Object keys are structure, not content, and are not leaves - the same
// distinction internal/secret's package doc draws and for the same reason. A
// key that is a token in every document is exactly what the precision assertion
// exists to catch, so a walker that collected keys would hide it.
//
// A payload that is not a JSON object, or not JSON at all, yields no leaves.
// That is a document with no candidate for any class, which is a document the
// classes skip rather than a failure: I-04 stores it either way.
func leavesOf(raw json.RawMessage) []string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			out = append(out, x)
		case map[string]any:
			for _, k := range slices.Sorted(maps.Keys(x)) {
				walk(x[k])
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return out
}

// fixtureDocs is the mode that always runs: the four Phase 1 fixtures, reached
// through All() so a fixture dropped from that list fails here rather than
// going quietly unsearched.
func fixtureDocs(t testing.TB) []doc {
	t.Helper()
	all := fixtures.All()
	if len(all) == 0 {
		t.Fatalf("fixtures.All() is empty; there is nothing to search")
	}
	docs := make([]doc, 0, len(all))
	for _, f := range all {
		b, err := f.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", f.File, err)
		}
		docs = append(docs, doc{name: f.File, payload: b, leaves: leavesOf(b)})
	}
	return docs
}

// corpusDocs is the mode that runs when the raw corpus is present: every
// capture's payload, in sorted file-name order so a run is reproducible.
//
// The payload is taken as json.RawMessage and handed on unmodified. Decoding
// and re-encoding it would change the bytes the index is built over - key
// order, number formatting, escaping - and the gate would then be measuring a
// document no host ever sent.
//
// Spec 7.5's synthetic self-test is filtered. The capture probe internal/host
// filters as well is not: it carries no leaf any class can use and no cwd, so
// it contributes nothing but one more document to the precision denominator,
// where an extra document only makes the precondition easier to hold.
func corpusDocs(t testing.TB) []doc {
	t.Helper()
	entries, err := os.ReadDir(corpusDir)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no raw corpus at %s; the gate ran over the fixtures only", corpusDir)
	}
	if err != nil {
		t.Fatalf("read %s: %v", corpusDir, err)
	}

	var docs []doc
	for _, e := range entries { // os.ReadDir sorts by file name
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(corpusDir, e.Name())
		//nolint:gosec // G304: reading the local corpus directory by construction
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var capture struct {
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &capture); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		if len(capture.Payload) == 0 {
			continue
		}
		var head struct {
			SessionID string `json:"session_id"`
		}
		// A payload that is not an object has no session id and is kept.
		_ = json.Unmarshal(capture.Payload, &head)
		if head.SessionID == "selftest" { // spec 7.5
			continue
		}
		docs = append(docs, doc{name: e.Name(), payload: capture.Payload, leaves: leavesOf(capture.Payload)})
	}
	if len(docs) == 0 {
		t.Fatalf("%s holds no captures; the corpus mode would gate nothing", corpusDir)
	}
	return docs
}

// ingestAll builds one database from an empty directory through the production
// path - store.Open, store.Migrate, store.Ingest per document - and fills in
// each document's events.id. Nothing else writes to it.
func ingestAll(t testing.TB, docs []doc) *sql.DB {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "engramux.db"))
	if err != nil {
		t.Fatalf("store.Open: %v\nA \"database is locked\" here is a development service holding its own "+
			"database, not this one - but check nothing else is writing under the temp directory.", err)
	}
	// On Windows an open handle makes t.TempDir()'s cleanup fail, and the
	// WAL sidecar counts.
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	ingestInto(t, db, docs)
	return db
}

// ingestInto stores every document in db through store.Ingest and fills in each
// one's events.id.
//
// The id is a fresh UUIDv7 because that is what the relay mints and what I-05
// makes the idempotency key; ingesting the same corpus twice in one run under
// reused ids would leave one row per pair and half the documents unfindable.
// That is also what lets [BenchmarkPrefixIndex] reach a scale this corpus does
// not have, by handing the same payloads back a second time under new ids.
func ingestInto(t testing.TB, db *sql.DB, docs []doc) {
	t.Helper()
	now := time.Now()
	for i := range docs {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatalf("mint an ingest id: %v", err)
		}
		docs[i].id = id.String()
		status, err := store.Ingest(t.Context(), db, ipc.Envelope{
			Version:  ipc.Version,
			Type:     ipc.IngestEvent,
			IngestID: docs[i].id,
			Payload:  docs[i].payload,
		}, store.SourcePipe, now)
		if err != nil {
			t.Fatalf("%s: Ingest: %v", docs[i].name, err)
		}
		if status != ipc.Committed {
			t.Fatalf("%s: Ingest answered %q, want %q", docs[i].name, status, ipc.Committed)
		}
	}
}
