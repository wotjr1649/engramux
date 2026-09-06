package search_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wotjr1649/engramux/internal/memory"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/store"
)

// Gate M8 (memory spec §5): for P1 and P5, how many questions native memory
// alone could answer against how many verbatim retrieval can. **This pair of
// numbers is the honest form of "native-grade or better"**, which is what §1
// makes publication wait on.
//
// The rule this file implements was pre-registered in the spec's *What M8 will
// measure* before any figure existed, and this file must not be the place it
// quietly moves. One sentence, both indexes, both capabilities: **an index
// answers a question when its own top ten for that question carries text
// containing the question's answer literal.**
//
// # Only P1 is here, and the omission is deliberate
//
// P1's questions are gate M4's, derived mechanically, so P1 needs no label
// anywhere and runs today. P5's are the labelled failure-fix pairs of
// `.capture/m8/`, and on 2026-09-06 that file has no labels in it - the fixture
// this file writes is what the owner labels. A measurement that cannot be run
// cannot be watched failing either, so P5's half is not written yet rather than
// written blind: §8 forbids shipping the `[unverified]` claim that would be.
// What stops it drifting in the meantime is that its rule is already committed.
//
// # Containment is tested against what the index holds, not against a masked read
//
// Both sides are unmasked, and that is the symmetric answer rather than the
// convenient one. Masking is a presentation rule applied on the way out; what
// retrieval can actually reach is the indexed text - the payload's string leaves
// on one side and `memory_items.body` on the other. A literal that
// internal/secret rewrites would be unfindable under a masked comparison on both
// sides while the index finds it on both, which would understate the pair for a
// reason that has nothing to do with native memory. The derived query itself
// comes off the unmasked payload for the same reason, which is M4's choice and
// not a new one here.
//
// Containment folds case, because the tokenizer does: a document carrying
// `Error` is a document FTS5 returns for `error`, and a case-sensitive test here
// would call that a miss the retrieval did not make. [doc.hasLeaf] already holds
// that rule for the event side and this reuses it.
//
// # Nothing here logs a literal, a body or a path
//
// Every P1 query is a command line's token, a path's basename or a token cut
// from a failing line, which is exactly the shape AGENTS.md's row about
// TestPhase4Gate's corpus mode is about. This logs counts and figures only.
func TestGateM8NativeCoverageOfP1(t *testing.T) {
	docs := corpusDocs(t) // skips when the local corpus is absent
	db := ingestAll(t, docs)
	bodies := m8NativeBodies(t, db) // skips when this machine has no native memory

	byID := make(map[string]doc, len(docs))
	for _, d := range docs {
		byID[d.id] = d
	}

	var reached int
	for _, c := range m4Classes {
		candidates := m4CandidatesFor(t, docs, c)
		if len(candidates) == 0 {
			t.Errorf("%s: no candidate document in %d; the class measures nothing", c.name, len(docs))
			continue
		}
		sampled := m4Sample(candidates)

		var verbatim, native int
		for _, cand := range sampled {
			// The answer literal is the query itself: P1 is exact-span
			// recall, so the literal a query was cut from is the thing
			// an answer has to carry.
			if m8EventsAnswer(t, db, byID, cand.query, cand.query) {
				verbatim++
			}
			answered, hits := m8MemoryAnswers(t, db, bodies, cand.query, cand.query)
			if answered {
				native++
			}
			if hits > 0 {
				reached++
			}
		}

		n := float64(len(sampled))
		t.Logf("P1 %s: %d candidates of %d documents, %d sampled",
			c.name, len(candidates), len(docs), len(sampled))
		t.Logf("P1 %s: verbatim answers %d of %d (%.3f), native answers %d of %d (%.3f)",
			c.name, verbatim, len(sampled), float64(verbatim)/n, native, len(sampled), float64(native)/n)
	}

	// The one assertion, and it is not a bar on the result. M8 is reported
	// rather than gated, so a threshold here would be a number invented to
	// have one. What can be wrong is the *instrument*: a native side that
	// returns nothing for every query reports a coverage of zero that is
	// indistinguishable from a memory index nothing searched, which is
	// M12's lesson about a term that never arrived. So the gate asserts the
	// index was reachable and says nothing about what it held.
	if reached == 0 {
		t.Errorf("M8: not one P1 query returned a single native memory hit over %d items. "+
			"A native coverage read off this run would be an artefact of an index nothing "+
			"searched rather than a measurement of what native memory holds.", len(bodies))
	}
	t.Logf("P1: %d of the sampled queries reached at least one native item, over %d indexed items",
		reached, len(bodies))
	t.Logf("P1: the companion figure is M4's known-item recall@10 - 0.680, 0.520 and 0.760, " +
		"measured 2026-09-04 - and the spec says why the coverage row above is not it")
}

// m8K is the k of the top ten, and it is M3's and M4's rather than a knob.
const m8K = 10

// m8EventsAnswer reports whether any of the event index's top [m8K] for query
// carries literal.
//
// A refusal is fatal for M4's reason: a derived query that trips the query
// bounds means the derivation is wrong, which is a broken gate rather than a
// coverage number to average in.
func m8EventsAnswer(t *testing.T, db *sql.DB, byID map[string]doc, query, literal string) bool {
	t.Helper()
	hits, _, err := search.Search(t.Context(), db, query, "", m8K, search.MatchAll)
	if err != nil {
		t.Fatalf("a derived query was refused by the event index: %v", err)
	}
	for _, h := range hits {
		if byID[h.ID].hasLeaf(literal) {
			return true
		}
	}
	return false
}

// m8MemoryAnswers reports whether any of the memory index's top [m8K] for query
// carries literal, and how many items it returned at all.
//
// The second return is what separates "native does not hold this" from "nothing
// was searched", and only the caller's non-vacuity assertion uses it.
//
// The scope is every project, which is what the event side already is: neither
// index may be given the narrower haystack.
func m8MemoryAnswers(t *testing.T, db *sql.DB, bodies map[string]string, query, literal string) (bool, int) {
	t.Helper()
	hits, _, err := search.SearchMemory(t.Context(), db, query, nil, m8K, search.MatchAll)
	if err != nil {
		t.Fatalf("a derived query was refused by the memory index: %v", err)
	}
	for _, h := range hits {
		if strings.Contains(strings.ToLower(bodies[h.ID]), strings.ToLower(literal)) {
			return true, len(hits)
		}
	}
	return false, len(hits)
}

// m8NativeBodies indexes this machine's own native memory into db and returns
// every item's body by id.
//
// It is gate M3's collector over gate M3's corpus, so what M8 measures the
// native side of is what the service would have written - and it goes into the
// same database the events are in, because the two indexes are separate tables
// and one connection is one less thing for a gate to get wrong.
//
// The bodies are read back rather than masked through GetMemoryItem: the doc
// comment above says why the comparison is against what the index holds.
func m8NativeBodies(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	c := &memory.Collector{ClaudeHome: memory.ClaudeHome(), CodexHome: memory.CodexHome()}
	rep, err := c.Collect(t.Context(), db, time.Now())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if rep.Written == 0 {
		t.Skip("no native memory on this machine to measure coverage over")
	}
	rows, err := db.QueryContext(t.Context(), `SELECT id, body FROM memory_items`)
	if err != nil {
		t.Fatalf("read the memory bodies: %v", err)
	}
	defer func() { _ = rows.Close() }()
	bodies := map[string]string{}
	for rows.Next() {
		var id, body string
		if err := rows.Scan(&id, &body); err != nil {
			t.Fatalf("scan a memory body: %v", err)
		}
		bodies[id] = body
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the memory bodies: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close the memory bodies: %v", err)
	}
	return bodies
}

// --- P5's fixture ------------------------------------------------------------

// m8Window is how many later events a failure is offered, and it is the owner's
// decision of 2026-09-06 rather than a tuning knob: one candidate misses a fix
// that came two steps later and undercounts the gate, five costs 466 lines to
// read for the same 94 decisions.
const m8Window = 3

// m8PairsEnv names the file [TestWriteM8Pairs] writes the unlabelled fixture to.
// It is an environment variable for [TestWriteM3Candidates]'s reason: writing a
// file is not something a suite run should do, and the operator naming the path
// is the whole interface.
const m8PairsEnv = "ENGRAMUX_WRITE_M8_PAIRS"

// m8MaxField is how much of a failure line or a candidate a fixture row carries.
// Long enough to tell one command from another, short enough that 281 of them
// are a file somebody reads rather than scrolls.
const m8MaxField = 200

// m8Pair is one failure and the candidates offered for it: the events that ran
// or edited something next in the same session.
type m8Pair struct {
	failure    int // index into the docs slice
	line       string
	candidates []int
}

// m8Pairs builds P5's candidate pairs by the spec's rule: a document whose tool
// output carries a failure-shaped line, and the next [m8Window] events in the
// same session that ran or edited something.
//
// # The order is the capture stamp and not the file name
//
// corpusDocs returns what os.ReadDir sorted, which is by file name - and a
// corpus file is named host__Event__nanos__pid.json, so a name sort groups a
// host's PostToolUse together and puts every PermissionRequest before them. That
// is not time order, and a "later event in the same session" read off it would
// pair a failure with something that happened before it. The nanosecond stamp is
// the third field and is what this sorts on.
//
// events.received_at cannot do it: that is when the row was ingested, which for
// a corpus replayed into a temporary database is the moment the test ran.
//
// A failure with no qualifying later event yields no pair at all rather than an
// empty one, so the fixture never asks a question with nothing to answer it.
func m8Pairs(docs []doc) []m8Pair {
	order := make([]int, len(docs))
	for i := range docs {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return m8Stamp(docs[order[a]].name) < m8Stamp(docs[order[b]].name)
	})

	bySession := map[string][]int{}
	for _, i := range order {
		sid := m8Session(docs[i].payload)
		bySession[sid] = append(bySession[sid], i)
	}

	var out []m8Pair
	for _, i := range order {
		line := m8FailureLine(store.Derive(docs[i].payload).Output)
		if line == "" {
			continue
		}
		seq := bySession[m8Session(docs[i].payload)]
		at := -1
		for k, j := range seq {
			if j == i {
				at = k
				break
			}
		}
		var candidates []int
		for _, j := range seq[at+1:] {
			if m8Fix(docs[j]) == "" {
				continue
			}
			candidates = append(candidates, j)
			if len(candidates) == m8Window {
				break
			}
		}
		if len(candidates) == 0 {
			continue
		}
		out = append(out, m8Pair{failure: i, line: line, candidates: candidates})
	}
	return out
}

// TestM8PairsFollowsTheWindowRule holds the four things [m8Pairs] can get wrong
// silently, and each case is one of them.
//
// The ordering case is the one that would otherwise pass a review: corpusDocs
// hands back a name sort, a name sort is not a time sort, and a fixture built
// off one asks the owner whether a command that ran *before* a failure resolved
// it. The case is built so that the two orders disagree - the failure sorts
// last by name and first by stamp - and a name-ordered implementation answers
// one pair where the rule answers none.
func TestM8PairsFollowsTheWindowRule(t *testing.T) {
	fail := func(name string) doc {
		return doc{name: name, payload: json.RawMessage(
			`{"session_id":"s1","tool_response":{"stdout":"go: error loading widget"}}`)}
	}
	fix := func(name, sid string) doc {
		return doc{name: name, payload: json.RawMessage(
			`{"session_id":"` + sid + `","tool_input":{"command":"go build ./cmd/x"}}`)}
	}
	quiet := func(name string) doc {
		return doc{name: name, payload: json.RawMessage(
			`{"session_id":"s1","tool_response":{"stdout":"ok"}}`)}
	}
	const failLine = "go: error loading widget"

	for _, tc := range []struct {
		name       string
		docs       []doc
		pairs      int
		candidates int
		line       string
	}{{
		// PermissionRequest sorts before PostToolUse by name and after
		// it by stamp. The fix happened first, so there is no pair.
		name:  "a candidate earlier in time is not later in the session",
		docs:  []doc{fail("h__PermissionRequest__200__1.json"), fix("h__PostToolUse__100__1.json", "s1")},
		pairs: 0,
	}, {
		name: "the window stops at three",
		docs: []doc{
			fail("h__A__100__1.json"),
			fix("h__A__101__1.json", "s1"), fix("h__A__102__1.json", "s1"),
			fix("h__A__103__1.json", "s1"), fix("h__A__104__1.json", "s1"),
		},
		pairs: 1, candidates: 3, line: failLine,
	}, {
		name: "an event that ran nothing does not consume a place",
		docs: []doc{
			fail("h__A__100__1.json"), quiet("h__A__101__1.json"),
			fix("h__A__102__1.json", "s1"),
		},
		pairs: 1, candidates: 1, line: failLine,
	}, {
		name:  "another session's later event is not a candidate",
		docs:  []doc{fail("h__A__100__1.json"), fix("h__A__101__1.json", "s2")},
		pairs: 0,
	}, {
		// P5 queries with this line, so which of a multi-line output
		// becomes it is the rule and not a detail. The first is the
		// failure; a later one is usually the same failure restated or
		// the tool's own summary of it.
		name: "the first failure-shaped line wins, not the last",
		docs: []doc{
			{name: "h__A__100__1.json", payload: json.RawMessage(
				`{"session_id":"s1","tool_response":{"stdout":"go: error loading widget\nhint: cannot continue"}}`)},
			fix("h__A__101__1.json", "s1"),
		},
		pairs: 1, candidates: 1, line: failLine,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got := m8Pairs(tc.docs)
			if len(got) != tc.pairs {
				t.Fatalf("m8Pairs returned %d pairs, want %d", len(got), tc.pairs)
			}
			if tc.pairs == 0 {
				return
			}
			if n := len(got[0].candidates); n != tc.candidates {
				t.Errorf("the pair carries %d candidates, want %d", n, tc.candidates)
			}
			if got[0].failure != 0 {
				t.Errorf("the pair is anchored on document %d, want 0", got[0].failure)
			}
			// The line is what the owner reads and what P5 queries
			// with, so it is asserted by value rather than by being
			// non-empty.
			if got[0].line != tc.line {
				t.Errorf("the failure line is %q, want %q", got[0].line, tc.line)
			}
		})
	}
}

// m8Stamp is the nanosecond field of a corpus file name, or 0 when the name is
// not one. A fixture file keeps its position rather than jumping to the front.
func m8Stamp(name string) int64 {
	parts := strings.Split(name, "__")
	if len(parts) < 3 {
		return 0
	}
	n, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// m8Session is the payload's session id, and "" for a payload that is not an
// object or carries none. Everything without one shares a bucket, which is the
// honest answer: nothing says those events are in one session, so nothing should
// pair them across a boundary that may not exist.
func m8Session(payload []byte) string {
	var head struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(payload, &head); err != nil {
		return ""
	}
	return head.SessionID
}

// m8FailureLine is the first line of a tool's output that reads like a failure
// and carries a token, which is [m4DeriveFromOutput]'s own rule stopped one step
// earlier: that one takes the longest token of this line, and P5 queries with
// the line.
func m8FailureLine(out string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if m4ErrorLine.MatchString(line) && m4LongestToken(line) != "" {
			return line
		}
	}
	return ""
}

// m8Fix is what a candidate event did - the command it ran, or failing that the
// file it touched - and "" for an event that did neither. It is [store.Derive]'s
// own two columns and no new rule.
func m8Fix(d doc) string {
	got := store.Derive(d.payload)
	if got.Cmd != "" {
		return got.Cmd
	}
	return got.Paths
}

// TestWriteM8Pairs writes gate M8's unlabelled P5 fixture, and it is not part of
// a suite run: it writes only when [m8PairsEnv] names a file.
//
// # The labeller is not shown what either index returns
//
// That is M7's discipline and the memory spec inherits it here before the
// fixture has a row: a label written with the answer visible measures the
// answer. Every candidate below comes from the mechanical rule in [m8Pairs] and
// never from a search result, so the question being asked is *did this resolve
// that* and not *was the ranking right*. Nothing in this file searches anything.
//
// # It logs counts, and the file it writes is never committed
//
// `.capture/` is gitignored and the corpus is the owner's own captures, so a
// failure line or a command line printed here would put one in a terminal and
// possibly in a commit message. Measured: 0 of this test's log lines carry one.
// The rows themselves are masked with internal/secret on the way out, which is
// what M7's own prompts.tsv already does.
func TestWriteM8Pairs(t *testing.T) {
	out := os.Getenv(m8PairsEnv)
	if out == "" {
		t.Skipf("set %s to a path to write gate M8's unlabelled P5 fixture there", m8PairsEnv)
	}
	docs := corpusDocs(t)
	pairs := m8Pairs(docs)
	if len(pairs) == 0 {
		t.Fatalf("no failure in %d documents has a later event that ran or edited anything; "+
			"the fixture would ask nothing", len(docs))
	}

	//nolint:gosec // G301: the operator names the directory, which is this harness's whole interface
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		t.Fatalf("make the output directory: %v", err)
	}
	f, err := os.Create(out) //nolint:gosec // G304: a path the operator passed in
	if err != nil {
		t.Fatalf("create %s: %v", out, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("close the output: %v", err)
		}
	}()

	var rows []string
	for n, p := range pairs {
		rows = append(rows, fmt.Sprintf("# failure %d of %d | %s", n+1, len(pairs), m8Field(p.line)))
		for _, j := range p.candidates {
			rows = append(rows, fmt.Sprintf("%s\t%s\tTODO\t%s",
				docs[p.failure].name, docs[j].name, m8Field(m8Fix(docs[j]))))
		}
		rows = append(rows, "")
	}
	labels := len(rows) - 2*len(pairs) // one comment and one blank per failure

	w := bufio.NewWriter(f)
	for _, l := range append(m8FixtureHeader(len(pairs), labels), rows...) {
		if _, err := fmt.Fprintln(w, l); err != nil {
			t.Fatalf("write %s: %v", out, err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush %s: %v", out, err)
	}
	t.Logf("M8: %d failures anchored of %d documents, %d candidate rows to label", len(pairs), len(docs), labels)
}

// m8FixtureHeader is what the labeller reads first. It asks the question the
// memory spec registered and no other one.
func m8FixtureHeader(pairs, labels int) []string {
	return []string{
		"# Gate M8, capability P5. Replace TODO with yes or no.",
		"#",
		"# The question is: DID THIS EVENT RESOLVE THAT FAILURE?",
		"#",
		"# yes: this command or edit is what fixed the failure in the # line above.",
		"# no:  it is not - a retry, an unrelated step, or a look at something else.",
		"#",
		"# Every candidate of one failure answered no is a legitimate outcome and means",
		"# the corpus holds no fix for it. Those failures leave P5's population and their",
		"# count is reported; a recall figure over questions with no answer in the corpus",
		"# measures the corpus rather than the retrieval.",
		"#",
		"# Answer from the failure and the candidates alone. Nothing here is a search",
		"# result and nothing here is what native memory or the index returned - a",
		"# label written with the answer visible measures the answer (memory spec,",
		"# What M8 will measure).",
		"#",
		"# columns: failure, candidate, resolved, what the candidate ran or touched",
		"# A candidate is the next event in the same session that ran or edited",
		"# something; up to three per failure, nearest first.",
		fmt.Sprintf("# %d failures, %d candidates. This file is under .capture/ and is never committed.", pairs, labels),
		"",
	}
}

// TestM8FieldIsSomethingAPersonCanLabel holds the three ways a fixture cell goes
// wrong: a byte that makes the file unprintable, a tab that invents a column,
// and a trim that either empties the cell or cuts a rune in half.
func TestM8FieldIsSomethingAPersonCanLabel(t *testing.T) {
	t.Run("nothing unprintable survives", func(t *testing.T) {
		// The residue is the point of the assertion rather than a
		// tolerated wart: the rule removes the ESC *byte*, so an ANSI
		// sequence loses what made it a sequence and leaves `[31m`
		// standing as ordinary text. That is printable, carries no tab
		// and cannot make grep call the file binary, which is all this
		// cell has to be - stripping the sequence would be a parser for
		// a cosmetic gain in a file 94 rows long.
		got := m8Field("go build\x00 ./cmd\x1b[31m/x\x07")
		if want := "go build ./cmd [31m/x"; got != want {
			t.Errorf("m8Field = %q, want %q", got, want)
		}
		for _, r := range got {
			if !unicode.IsPrint(r) {
				t.Errorf("m8Field kept the unprintable %U", r)
			}
		}
	})
	t.Run("a tab or a newline cannot invent a column", func(t *testing.T) {
		got := m8Field("go\ttest\n-p 1\r\n./...")
		if want := "go test -p 1 ./..."; got != want {
			t.Errorf("m8Field = %q, want %q", got, want)
		}
		if strings.ContainsAny(got, "\t\n\r") {
			t.Errorf("m8Field kept a TSV separator in %q", got)
		}
	})
	t.Run("a run with no space is cut at a rune boundary, not to nothing", func(t *testing.T) {
		// Hangul is three bytes a syllable, so a byte cut lands
		// mid-rune two times in three.
		got := m8Field(strings.Repeat("코퍼스", 100))
		if got == "" {
			t.Fatal("m8Field emptied a cell the labeller has to read")
		}
		if len(got) > m8MaxField {
			t.Errorf("m8Field returned %d bytes, cap %d", len(got), m8MaxField)
		}
		if !utf8.ValidString(got) {
			t.Errorf("m8Field cut a rune in half")
		}
	})
}

// m8Field makes one text safe to put in a TSV cell: masked, stripped of what is
// not printable, flattened onto one line, and trimmed.
//
// # strings.Fields is not enough, and the corpus is what says so
//
// A tool's output is arbitrary bytes and this corpus holds some: measured
// 2026-09-06, the first fixture written without this carried **63 control
// characters over two of its 281 rows, 36 of them NUL**. Fields splits on
// Unicode whitespace, and NUL is not whitespace - so it survived, the file was
// valid UTF-8 with embedded NULs, and grep called the whole thing a binary file
// and answered `Binary file ... matches` instead of the rows. `grep -c` kept
// counting correctly, so the two disagreed and the fixture read as malformed
// when it was the tool refusing to print it. A file a person labels by hand and
// a parser reads by tab may not contain either.
//
// So anything [unicode.IsPrint] rejects becomes a space *before* Fields, which
// then collapses the run - one rule covering NUL, a stray carriage return and
// the ESC of an ANSI sequence alike, rather than a list of the ones this corpus
// happened to hold. It removes the ESC and not the sequence: `[31m` is left
// standing as printable text, which is not pretty and is all this cell has to
// be. TestM8FieldIsSomethingAPersonCanLabel asserts that residue by value so
// that a later reader knows it was decided rather than missed.
//
// # The trim
//
// [m3TrimToWord] is the trim gate M3's candidates use, so a line with a word
// boundary to cut at is cut the same way in both fixtures - but it answers "" for
// a run with no space in it, and half of what this fixture carries is a path or a
// single long command. So a text that survives masking and loses everything to
// the trim falls back to a rune-safe cut rather than to an empty cell the
// labeller cannot judge.
func m8Field(s string) string {
	printable := strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return ' '
	}, secret.MaskString(s))
	flat := strings.Join(strings.Fields(printable), " ")
	if trimmed := m3TrimToWord(flat, m8MaxField); trimmed != "" || flat == "" {
		return trimmed
	}
	for i := m8MaxField; i > 0; i-- {
		if utf8.RuneStart(flat[i]) {
			return flat[:i]
		}
	}
	return flat
}
