package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/fixtures"
	"github.com/wotjr1649/engramux/internal/secret"
)

// TestLeaves pins what the indexed text of a payload is, one shape at a time.
// Every case asserts the exact string, because the properties that matter here
// - document order, keys absent, an empty leaf still occupying a line - are all
// invisible to a length or a substring check.
//
// The migration's SQL walk has to produce the same text for the same bytes;
// TestTheTwoWalksAgree is what holds them together. These cases are the ones
// that say what the text is in the first place.
func TestLeaves(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		want    string
	}{{
		name:    "an object's values, not its keys",
		payload: `{"hook_event_name":"PostToolUse","cwd":"D:\\work"}`,
		want:    "PostToolUse\nD:\\work",
	}, {
		// Document order, which is what rules out a map walk: `b` is
		// written first and comes back first, where a sorted walk would
		// answer alpha, beta.
		name:    "document order, not key order",
		payload: `{"b":"beta","a":"alpha"}`,
		want:    "beta\nalpha",
	}, {
		name:    "nested objects and arrays, depth-first in document order",
		payload: `{"b":"beta","a":["a1",{"deep":"d1"},["a2"]],"c":"gamma"}`,
		want:    "beta\na1\nd1\na2\ngamma",
	}, {
		// A key whose value is an object is still a key, and the walk has
		// to come back to key position after the nested container closes.
		// Getting that wrong swallows `c` or emits `a`.
		name:    "key position is restored after a nested container",
		payload: `{"a":{"x":"one"},"c":"two"}`,
		want:    "one\ntwo",
	}, {
		name:    "a key that looks like content is still a key",
		payload: `{"cwd":1,"session":2}`,
		want:    "",
	}, {
		// An empty leaf keeps its line. It is a payload with a leaf, not a
		// payload without one, and json_tree counts it the same way.
		name:    "an empty string is a leaf",
		payload: `{"empty":"","x":"after"}`,
		want:    "\nafter",
	}, {
		name:    "a bare string is its own leaf",
		payload: `"bare string"`,
		want:    "bare string",
	}, {
		name:    "a bare number has no leaves",
		payload: `42`,
		want:    "",
	}, {
		name:    "an object of non-strings has no leaves",
		payload: `{"n":1,"b":true,"z":null}`,
		want:    "",
	}, {
		name:    "an empty object has no leaves",
		payload: `{}`,
		want:    "",
	}, {
		name:    "a top-level array is walked",
		payload: `["one","two"]`,
		want:    "one\ntwo",
	}, {
		// A host may write any character as \uXXXX (spec 6.1), so the
		// escape has to be decoded or the text is indexed as backslash-u.
		// The surrogate pair is the case a naive decoder loses.
		name:    "\\u escapes are decoded, surrogate pairs included",
		payload: `{"k":"\uD55C\uAE00 \u0041 \uD83D\uDE00"}`,
		want:    "한글 A 😀",
	}, {
		name:    "an embedded newline does not become two leaves",
		payload: `{"k":"one\ntwo"}`,
		want:    "one\ntwo",
	}, {
		// Not JSON at all. The SQL backfill's json_valid guard answers ''
		// for the same bytes, and a partial walk up to the syntax error
		// would not match it.
		name:    "a payload that is not JSON has no leaves",
		payload: `{"a":"kept", garbage`,
		want:    "",
	}, {
		name:    "trailing bytes after a complete value are not JSON either",
		payload: `{"a":"kept"} and then some`,
		want:    "",
	}, {
		name:    "empty bytes have no leaves",
		payload: ``,
		want:    "",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Leaves([]byte(tc.payload)); got != tc.want {
				t.Errorf("Leaves(%q) = %q, want %q", tc.payload, got, tc.want)
			}
		})
	}
}

// TestLeavesCoercesInvalidUTF8 is a measured divergence between the two walks,
// pinned here so that it is a known boundary rather than a surprise.
//
// encoding/json coerces a byte sequence that is not valid UTF-8 to U+FFFD, one
// per bad byte; SQLite's json_tree hands the raw bytes back unchanged. So for a
// payload carrying invalid UTF-8 inside a string, Ingest indexes replacement
// characters where the migration's backfill would index the original bytes.
//
// It is not reachable from the corpus: all 902 captured payloads are valid
// UTF-8, so no live row can hit it, and TestTheTwoWalksAgree runs over exactly
// those bytes. It is also immaterial to search - unicode61 treats both the bad
// bytes and U+FFFD as separators, so the tokens on either side are the same
// either way. Both halves are why this is pinned rather than fixed: making the
// Go walk preserve raw bytes means hand-writing a JSON string unquoter.
func TestLeavesCoercesInvalidUTF8(t *testing.T) {
	payload := []byte("{\"k\":\"a\xff\xfeb\"}")
	const want = "a\uFFFD\uFFFDb"
	if got := Leaves(payload); got != want {
		t.Errorf("Leaves(%q) = % x, want % x", payload, got, want)
	}
}

// TestLeavesSeparatesLeavesThatWereNotAdjacent. Two leaves joined by a space
// would let a phrase query match across a boundary the document never had. The
// newline is what stops that, and unicode61 splits on it.
func TestLeavesSeparatesLeavesThatWereNotAdjacent(t *testing.T) {
	got := Leaves([]byte(`{"a":"alpha","b":"beta"}`))
	if strings.Contains(got, "alpha beta") {
		t.Errorf("Leaves joined two leaves into one phrase: %q", got)
	}
	if got != "alpha\nbeta" {
		t.Errorf("Leaves = %q, want %q", got, "alpha\nbeta")
	}
}

// corpusDir is the local, gitignored raw capture corpus, relative to this
// package. Same path and skip pattern as internal/fixtures, internal/host and
// internal/search, each of which loads it for its own shape; this one wants the
// payload bytes and the file name and nothing else. Four short readers rather
// than one shared package, because what they have in common is ten lines of
// os.ReadDir and no behaviour.
var corpusDir = filepath.Join("..", "..", ".capture", "fixtures-raw")

// namedPayload is one payload to walk, and the only thing about it that may
// ever be printed: a corpus file name is host__Event__nanos__pid.json and
// carries no user text, where the payload carries prompts, file contents and
// absolute paths (I-10, spec 7.5).
type namedPayload struct {
	name    string
	payload []byte
}

// corpusPayloads returns every capture's payload, skipping the whole test when
// the corpus is absent. Spec 7.5's synthetic self-test is filtered, so "the
// corpus" means the same 901 documents here as it does in internal/search.
func corpusPayloads(t *testing.T) []namedPayload {
	t.Helper()
	entries, err := os.ReadDir(corpusDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read %s: %v", corpusDir, err)
	}
	var out []namedPayload
	for _, e := range entries { // os.ReadDir sorts by file name
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		//nolint:gosec // G304: reading the local corpus directory by construction
		raw, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var capture struct {
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &capture); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		if len(capture.Payload) == 0 || string(capture.Payload) == "null" {
			t.Fatalf("%s carries no payload", e.Name())
		}
		var head struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(capture.Payload, &head)
		if head.SessionID == "selftest" { // spec 7.5
			continue
		}
		out = append(out, namedPayload{name: e.Name(), payload: capture.Payload})
	}
	return out
}

// agreementPayloads are the shapes the corpus does not have, so that the two
// walks are compared on them too. Each one is a boundary in the SQL half:
// group_concat over no rows is NULL rather than the empty string, and json_tree
// raises `malformed JSON` rather than returning nothing.
var agreementPayloads = []namedPayload{
	{"a bare number", []byte(`42`)},
	{"a bare string", []byte(`"only a string"`)},
	{"an object with no string leaves", []byte(`{"n":1,"b":true,"z":null}`)},
	{"an empty object", []byte(`{}`)},
	{"an empty array", []byte(`[]`)},
	{"an empty leaf between two others", []byte(`{"a":"one","e":"","b":"two"}`)},
	{"a nested mixture", []byte(`{"b":"beta","a":["a1",{"deep":"d1"},["a2"]],"c":"gamma"}`)},
	{"not JSON at all", []byte(`{"a":"kept", garbage`)},
}

// TestTheTwoWalksAgree is the whole reason the walk is allowed to exist in two
// places at once. [Leaves] fills events.leaves on the way in; the 00002
// migration's json_tree backfill fills it for every row that was already there.
// If they disagree, the live installation's 1,800 captured events are indexed
// differently from every event after them, and nothing else in the suite would
// say so - the index would be internally consistent and integrity-check would
// pass.
//
// So it is run as the upgrade path rather than as a comparison of two
// expressions: the database is built at version 1, filled with the fixtures,
// every corpus payload and the shapes above, and only then migrated. What is
// read back is what the migration's own expression wrote.
//
// Equality, not length: the failure this is guarding against is a separator or
// an order, and both keep the length.
func TestTheTwoWalksAgree(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, dbPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)

	p, err := provider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatalf("UpTo(1): %v", err)
	}
	seed(t, db)

	want := make(map[string]namedPayload)
	for _, f := range fixtures.All() {
		b, err := f.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", f.File, err)
		}
		want[insertAtVersionOne(t, db, len(want), b)] = namedPayload{f.File, b}
	}
	for _, np := range agreementPayloads {
		want[insertAtVersionOne(t, db, len(want), np.payload)] = np
	}
	corpus := corpusPayloads(t)
	for _, np := range corpus {
		want[insertAtVersionOne(t, db, len(want), np.payload)] = np
	}
	t.Logf("comparing %d payloads: %d fixtures, %d hand-written shapes, %d corpus captures",
		len(want), len(fixtures.All()), len(agreementPayloads), len(corpus))

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate the rest of the way up: %v", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT id, payload, leaves FROM events`)
	if err != nil {
		t.Fatalf("read back the backfilled column: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close: %v", err)
		}
	}()
	var compared int
	for rows.Next() {
		var id, payload string
		var leaves sql.NullString
		if err := rows.Scan(&id, &payload, &leaves); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		np, ok := want[id]
		if !ok {
			np = namedPayload{name: "the seed row", payload: []byte(payload)}
		}
		// NULL means the backfill never touched the row, which is a
		// different failure from having written the wrong text.
		if !leaves.Valid {
			t.Fatalf("%s: leaves is NULL after the migration; the backfill missed the row", np.name)
		}
		got, sqlText := Leaves(np.payload), leaves.String
		if got != sqlText {
			t.Errorf("%s: the Go walk produced %d bytes and the SQL backfill %d, first differing byte at %d",
				np.name, len(got), len(sqlText), firstDiff([]byte(got), []byte(sqlText)))
		}
		compared++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if compared != len(want)+1 { // +1 for seed's own events row
		t.Fatalf("compared %d rows, want %d", compared, len(want)+1)
	}
}

// insertAtVersionOne writes one events row directly, because at version 1 there
// is no leaves column for Ingest to fill and filling it is what is under test.
// It returns the id it used.
func insertAtVersionOne(t *testing.T, db *sql.DB, n int, payload []byte) string {
	t.Helper()
	id := fmt.Sprintf("walk-%06d", n)
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO events (id, project_id, session_id, host, source, event_name,
		                    payload, privacy_class, redaction_version, received_at)
		VALUES (?, ?, ?, 'codex', 'pipe', 'PostToolUse', ?, '', ?, ?)`,
		id, seedProject, seedSession, string(payload), int64(secret.Version), int64(2000+n)); err != nil {
		t.Fatalf("INSERT %s: %v", id, err)
	}
	return id
}
