package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
		// A *stream* of values, which is the shape json.Decoder is
		// happiest with and json_valid rejects outright. Without the
		// json.Valid check in front of the walk the decoder reads
		// straight on into the second value and answers "x\ny" where
		// the backfill answers nothing.
		name:    "a stream of two objects is not one JSON value",
		payload: `{"a":"x"}{"b":"y"}`,
		want:    "",
	}, {
		name:    "a stream of two bare strings is not one JSON value",
		payload: `"a" "b"`,
		want:    "",
	}, {
		name:    "a value followed by another value is not one JSON value",
		payload: `{"a":"x"} 42`,
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

// TestLeavesCoercesWhatIsNotWellFormed pins two measured divergences between
// the two walks, so that each is a known boundary rather than a surprise.
//
// encoding/json coerces what is not well formed to U+FFFD; SQLite's json_tree
// hands the original through. Measured on modernc.org/sqlite v1.57.0, SQLite
// 3.53.3:
//
//   - Invalid UTF-8 inside a string. Go answers one U+FFFD per bad byte,
//     61 ef bf bd ef bf bd 62; json_tree answers the bytes, 61 ff fe 62.
//   - A lone surrogate escape. Go answers U+FFFD, ef bf bd; json_tree answers
//     the surrogate itself encoded as three bytes, ed a0 80.
//
// Neither is reachable from the corpus. All 902 captured payloads are valid
// UTF-8, and while 656 of them carry a \uXXXX escape, none carries a surrogate
// one - a surrogate *pair* is in agreementPayloads and the two walks agree on it
// exactly. TestTheTwoWalksAgree runs over those same bytes.
//
// Fixing either would mean hand-writing a JSON string unquoter that preserves
// ill-formed input. What makes pinning them acceptable rather than merely
// cheaper is TestTheTokenizerReadsBothIllFormedShapesTheSameWay below, which
// measures that the index cannot tell the two spellings apart.
func TestLeavesCoercesWhatIsNotWellFormed(t *testing.T) {
	for _, tc := range []struct{ name, payload, want string }{
		{"invalid UTF-8 bytes", "{\"k\":\"a\xff\xfeb\"}", "a\uFFFD\uFFFDb"},
		{"a lone high surrogate escape", `{"k":"a\uD800b"}`, "a\uFFFDb"},
		{"a lone low surrogate escape", `{"k":"a\uDC00b"}`, "a\uFFFDb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Leaves([]byte(tc.payload)); got != tc.want {
				t.Errorf("Leaves(%q) = % x, want % x", tc.payload, got, tc.want)
			}
		})
	}
}

// TestTheTokenizerReadsBothIllFormedShapesTheSameWay is the measurement behind
// the "immaterial to search" half of the comment above, which was an assertion
// until this existed.
//
// Three rows are indexed whose text differs only in how the ill-formed run
// between two words is spelled - one per side of each divergence - and
// fts5vocab in instance mode is asked what tokens each row actually
// contributed. Equal term lists mean the index cannot tell the divergences
// apart, which is the claim.
//
// The count is asserted too, and separately: two equal one-token lists would
// also satisfy equality, and that is the failure where the ill-formed run
// joined the words instead of separating them. The terms themselves are not
// asserted - the tokenizer decides those, and pinning them here would be a test
// of the tokenizer rather than of the separator.
func TestTheTokenizerReadsBothIllFormedShapesTheSameWay(t *testing.T) {
	ctx := t.Context()
	db := migrated(t)
	seed(t, db)

	// One spelling per side of each divergence above: the invalid bytes and
	// the lone surrogate as json_tree hands them over, and the U+FFFD run
	// Leaves hands over in place of either.
	const first, second = "alphaTokenOne", "betaTokenTwo"
	spellings := []struct{ name, leaves string }{
		{"the invalid bytes json_tree would index", first + "\xff\xfe" + second},
		{"the lone surrogate json_tree would index", first + "\xed\xa0\x80" + second},
		{"the U+FFFD run Leaves indexes", first + "\uFFFD" + second},
	}

	rowids := make([]int64, len(spellings))
	for i, sp := range spellings {
		id := fmt.Sprintf("tok-%d", i)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO events (id, project_id, session_id, host, source, event_name,
			                    payload, leaves, privacy_class, redaction_version, received_at)
			VALUES (?, ?, ?, 'codex', 'pipe', 'PostToolUse', '{}', ?, '', ?, ?)`,
			id, seedProject, seedSession, sp.leaves, int64(secret.Version), int64(3000+i)); err != nil {
			t.Fatalf("INSERT %s: %v", sp.name, err)
		}
		rowids[i] = rowidOf(t, db, id)
	}

	// fts5vocab reads the index, so this reports the tokens FTS5 actually
	// stored rather than the tokens this test believes it should have.
	if _, err := db.ExecContext(ctx,
		`CREATE VIRTUAL TABLE tokens USING fts5vocab('events_fts', 'instance')`); err != nil {
		t.Fatalf("CREATE VIRTUAL TABLE ... fts5vocab: %v", err)
	}
	terms := func(rowid int64) []string {
		return queryStrings(t, db, fmt.Sprintf(
			`SELECT term FROM tokens WHERE doc = %d ORDER BY offset`, rowid))
	}

	want := terms(rowids[0])
	if len(want) != 2 {
		t.Fatalf("%s produced %d tokens %q, want 2 - the ill-formed run did not separate the words",
			spellings[0].name, len(want), want)
	}
	for i, sp := range spellings[1:] {
		if got := terms(rowids[i+1]); !slices.Equal(got, want) {
			t.Errorf("%s indexed %q where %s indexed %q; the tokenizer can tell the two apart",
				sp.name, got, spellings[0].name, want)
		}
	}
}

// TestLeavesSeparatesLeavesThatWereNotAdjacent. Something has to go between two
// leaves or the last token of one and the first of the next fuse into a single
// token; that it is a newline rather than a space is the migration's char(10)
// and nothing else. What the newline does *not* buy is a phrase boundary -
// unicode61 drops it exactly as it drops a space, so a phrase query spans two
// leaves. [TestANewlineIsNotAPhraseBoundary] measures both halves of that
// against the index.
func TestLeavesSeparatesLeavesThatWereNotAdjacent(t *testing.T) {
	if got := Leaves([]byte(`{"a":"alpha","b":"beta"}`)); got != "alpha\nbeta" {
		t.Errorf("Leaves = %q, want %q", got, "alpha\nbeta")
	}
}

// nestedJSON wraps the string leaf "deep" in depth containers. shape is the
// sequence of container characters to cycle through: "{" nests objects all the
// way down, "[" arrays, "{[" alternates the two. Depth 0 is the bare string.
//
// Built here rather than written as a literal because at these depths a literal
// is two kilobytes of punctuation, and because every caller's boundary has to
// move with [sqliteJSONDepthLimit] rather than being retyped beside it.
func nestedJSON(depth int, shape string) []byte {
	var b strings.Builder
	for i := range depth {
		if shape[i%len(shape)] == '{' {
			b.WriteString(`{"k":`)
		} else {
			b.WriteByte('[')
		}
	}
	b.WriteString(`"deep"`)
	for i := depth - 1; i >= 0; i-- {
		if shape[i%len(shape)] == '{' {
			b.WriteByte('}')
		} else {
			b.WriteByte(']')
		}
	}
	return []byte(b.String())
}

// goJSONDepthLimit is encoding/json's own nesting limit, which is nowhere in
// the production code: it is here only so that the gap between it and
// [sqliteJSONDepthLimit] - the whole reason the guard exists - is a measured
// number rather than a remark. Go accepts ten times what SQLite does.
const goJSONDepthLimit = 10000

// jsonValid asks SQLite, so that the number in [sqliteJSONDepthLimit] is the
// linked-in driver's answer and not this file's opinion of it.
func jsonValid(t *testing.T, db *sql.DB, payload []byte) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRowContext(t.Context(), `SELECT json_valid(?)`, string(payload)).Scan(&v); err != nil {
		t.Fatalf("json_valid: %v", err)
	}
	return v
}

// TestSQLiteRefusesJSONNestedPastTheDepthGuard re-derives [sqliteJSONDepthLimit]
// against the driver actually linked in, which is what that constant's doc
// comment promises happens when the driver moves. It is asserted from the
// constant rather than from a literal, so moving the constant moves the payloads
// and this fails rather than quietly measuring somewhere else.
//
// Three things per shape, and the third is why the guard is in [Leaves] and not
// only in the migration: json_tree over a payload json_valid refuses does not
// return zero rows, it raises. The backfill never reaches it because its CASE
// tests json_valid first; a Go walk with no guard reaches its own limit ten
// times further down and indexes text no upgrade could reproduce.
func TestSQLiteRefusesJSONNestedPastTheDepthGuard(t *testing.T) {
	db := migrated(t)
	for _, shape := range []struct{ name, chars string }{
		{"objects", "{"},
		{"arrays", "["},
		{"objects and arrays alternating", "{["},
	} {
		t.Run(shape.name, func(t *testing.T) {
			at := nestedJSON(sqliteJSONDepthLimit, shape.chars)
			past := nestedJSON(sqliteJSONDepthLimit+1, shape.chars)

			if got := jsonValid(t, db, at); got != 1 {
				t.Errorf("json_valid at depth %d = %d, want 1", sqliteJSONDepthLimit, got)
			}
			if got := jsonValid(t, db, past); got != 0 {
				t.Errorf("json_valid at depth %d = %d, want 0", sqliteJSONDepthLimit+1, got)
			}
			// Go's own limit is ten times SQLite's, so encoding/json
			// accepts both of these. That gap is the divergence the
			// guard closes and there is no Go-side error to lean on.
			// Its own boundary is measured here too, because the
			// constant's doc comment states it.
			if !json.Valid(at) || !json.Valid(past) {
				t.Errorf("encoding/json refused depth %d/%d itself; the guard has nothing to close",
					sqliteJSONDepthLimit, sqliteJSONDepthLimit+1)
			}
			if !json.Valid(nestedJSON(goJSONDepthLimit, shape.chars)) {
				t.Errorf("encoding/json refused depth %d, want accepted", goJSONDepthLimit)
			}
			if json.Valid(nestedJSON(goJSONDepthLimit+1, shape.chars)) {
				t.Errorf("encoding/json accepted depth %d, want refused", goJSONDepthLimit+1)
			}

			var n int64
			err := db.QueryRowContext(t.Context(),
				`SELECT count(*) FROM json_tree(?)`, string(past)).Scan(&n)
			if err == nil {
				t.Fatalf("json_tree at depth %d returned %d rows, want an error", sqliteJSONDepthLimit+1, n)
			}
			if !strings.Contains(err.Error(), "malformed JSON") {
				t.Errorf("json_tree at depth %d failed with %v, want malformed JSON",
					sqliteJSONDepthLimit+1, err)
			}

			if got := Leaves(at); got != "deep" {
				t.Errorf("Leaves at depth %d = %q, want %q", sqliteJSONDepthLimit, got, "deep")
			}
			if got := Leaves(past); got != "" {
				t.Errorf("Leaves at depth %d = %q, want %q", sqliteJSONDepthLimit+1, got, "")
			}
		})
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

// corpusPayloads returns every capture's payload, and nil when the corpus is
// absent - the caller keeps running over the fixtures and the hand-written
// shapes, which is why nothing here skips. The payload count the agreement test
// logs is therefore 16 on a machine without .capture/ and 917 with it.
//
// Spec 7.5's synthetic self-test is filtered, so "the corpus" means the same 901
// documents here as it does in internal/search.
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
// walks are compared on them too. Each one is a boundary in one half or the
// other: group_concat over no rows is NULL rather than the empty string,
// json_tree raises `malformed JSON` rather than returning nothing, and
// json.Decoder streams where json_valid demands exactly one value.
//
// The three stream shapes are the ones that measured as a real divergence
// before [Leaves] checked json.Valid first: the decoder read on into the second
// value and answered text where the backfill answered nothing.
//
// The surrogate-pair escape is here because 656 of 902 captures carry a
// backslash-u escape and *none* carries a surrogate one, so the corpus exercises
// the easy half of that path and not the hard half. The two walks agree on it -
// measured, both sides answer the same four bytes for U+1F600. They do not agree
// on a *lone* surrogate; that is [TestLeavesCoercesWhatIsNotWellFormed].
//
// The overflowing number is a value both json_valid and json.Valid accept and
// json.Decoder.Token does not, which took the whole Go walk to "" until it read
// numbers as [json.Number].
//
// The nested payloads sit one on each side of [sqliteJSONDepthLimit], in all
// three shapes, and both sides of each pair are needed: the shallower one alone
// passes with a guard that is one too tight, the deeper one alone with a guard
// that is one too loose.
var agreementPayloads = []namedPayload{
	{"a bare number", []byte(`42`)},
	{"a bare string", []byte(`"only a string"`)},
	{"an object with no string leaves", []byte(`{"n":1,"b":true,"z":null}`)},
	{"an empty object", []byte(`{}`)},
	{"an empty array", []byte(`[]`)},
	{"an empty leaf between two others", []byte(`{"a":"one","e":"","b":"two"}`)},
	{"a nested mixture", []byte(`{"b":"beta","a":["a1",{"deep":"d1"},["a2"]],"c":"gamma"}`)},
	{"not JSON at all", []byte(`{"a":"kept", garbage`)},
	{"a stream of two objects", []byte(`{"a":"x"}{"b":"y"}`)},
	{"two bare values in a row", []byte(`"a" "b"`)},
	{"a value followed by a number", []byte(`{"a":"x"} 42`)},
	{"a number too large for float64, between two leaves", []byte(`{"a":"alpha","n":1e400,"b":"beta"}`)},
	{"objects nested to SQLite's limit", nestedJSON(sqliteJSONDepthLimit, "{")},
	{"objects nested one past it", nestedJSON(sqliteJSONDepthLimit+1, "{")},
	{"arrays nested to SQLite's limit", nestedJSON(sqliteJSONDepthLimit, "[")},
	{"arrays nested one past it", nestedJSON(sqliteJSONDepthLimit+1, "[")},
	{"objects and arrays alternating to SQLite's limit", nestedJSON(sqliteJSONDepthLimit, "{[")},
	{"objects and arrays alternating one past it", nestedJSON(sqliteJSONDepthLimit+1, "{[")},
	{"a surrogate-pair escape", []byte(`{"k":"\uD55C\uAE00 \u0041 \uD83D\uDE00"}`)},
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
