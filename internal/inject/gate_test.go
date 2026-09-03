package inject_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/memory"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// corpusDir is the raw capture corpus, relative to this package - which is
// where a test runs, so the two leading steps up reach the repository root.
// It is gitignored and holds raw prompts, file contents and user paths, so this
// gate skips without it rather than passing quietly.
var corpusDir = filepath.Join("..", "..", ".capture", "fixtures-raw")

// TestGateInjectionOverTheCorpus is memory spec §5's M5, M6, M9 and M10 in one pass
// over one corpus, which is the shape TestPhase1Gate and TestPhase4Gate already
// have: they measure different properties of the same run, and building the
// corpus four times would measure four corpora.
//
// # The prompts are the corpus's own
//
// "The whole corpus through the injector" is M5's wording and the prompts are
// the only part of it that is not already here: every UserPromptSubmit event in
// the captures carries one a person actually typed. Synthesising prompts would
// measure the synthesiser, which is the circularity gate M3 was reshaped for.
//
// # Nothing here prints a prompt, an excerpt or a path
//
// Every one of those is corpus text - the captures are the owner's own prompts,
// file contents and tool output, 900 of 902 of which carry their directory -
// and this test's output has to be safe to paste, which TestPhase4Gate's corpus
// mode is not (AGENTS.md's row on it). So what is logged is counts, byte sizes
// and durations, and a failure names a prompt by its index in the run.
func TestGateInjectionOverTheCorpus(t *testing.T) {
	docs := corpusPayloads(t)
	db := ingestCorpus(t, docs)
	collectNativeMemory(t, db)

	prompts := promptsIn(docs)
	if len(prompts) == 0 {
		t.Skip("the corpus carries no UserPromptSubmit event, so there is no prompt to inject against")
	}

	var (
		injected  int
		abstained int
		reasons   = map[string]int{}
		bytes     []int
		elapsed   []time.Duration
		fromEvent int
		fromMem   int
		fromBoth  int
	)
	for i, p := range prompts {
		start := time.Now()
		res, err := inject.Build(t.Context(), db, inject.Request{Prompt: p})
		took := time.Since(start)
		if err != nil {
			t.Fatalf("prompt %d: Build: %v", i, err)
		}
		elapsed = append(elapsed, took)

		// M10, asserted. The budget is a ceiling on the whole call and
		// not on the search alone, so this is the number the relay's own
		// clock would see.
		if took > inject.Budget {
			t.Errorf("M10: prompt %d took %s, over the %s budget", i, took, inject.Budget)
		}

		if res.Text == "" {
			abstained++
			reasons[res.Reason]++
			continue
		}
		injected++
		bytes = append(bytes, len(res.Text))

		// M5, asserted: zero replies over the cap.
		if len(res.Text) > inject.MaxBytes {
			t.Errorf("M5: prompt %d injected %d bytes, over the %d-byte cap",
				i, len(res.Text), inject.MaxBytes)
		}
		// M9, asserted: the payload is fenced and cannot close its own
		// fence. Zero occurrences over the corpus.
		m := fencePattern.FindStringSubmatch(res.Text)
		if m == nil {
			t.Errorf("M9: prompt %d injected %d bytes that are not a fenced document", i, len(res.Text))
		} else if strings.Contains(m[3], m[2]) {
			t.Errorf("M9: prompt %d injected a body carrying its own fence nonce", i)
		}

		switch {
		case len(res.Events) > 0 && len(res.Memory) > 0:
			fromBoth++
		case len(res.Memory) > 0:
			fromMem++
		default:
			fromEvent++
		}
	}

	// M10, reported. Nothing has measured what a cold read costs at this
	// database's size, so a rate is a number found rather than invented -
	// and every reading here is warm, which is what M-4 leaves unverified.
	sort.Slice(elapsed, func(i, j int) bool { return elapsed[i] < elapsed[j] })
	t.Logf("M10: %d prompts, worst %s, p95 %s, median %s, budget %s",
		len(elapsed), elapsed[len(elapsed)-1], percentile(elapsed, 95), percentile(elapsed, 50), inject.Budget)
	t.Logf("M10: %d of %d abstained on time (%.1f%%)",
		reasons[inject.ReasonDeadline], len(prompts),
		100*float64(reasons[inject.ReasonDeadline])/float64(len(prompts)))

	// M5, reported beside its assertion: a cap nothing approaches is a cap
	// that gates nothing, and the distribution is what says which it is.
	sort.Ints(bytes)
	if len(bytes) > 0 {
		t.Logf("M5: %d injections, largest %d B, p95 %d B, median %d B, cap %d B",
			len(bytes), bytes[len(bytes)-1], intPercentile(bytes, 95), intPercentile(bytes, 50), inject.MaxBytes)
	}

	t.Logf("M6: %d of %d prompts injected, %d abstained", injected, len(prompts), abstained)
	for _, r := range sortedKeys(reasons) {
		t.Logf("M6: %d abstained because %s", reasons[r], r)
	}

	// **This is not M8.** M8 is native memory's coverage of P1 and P5
	// against verbatim retrieval's, and it needs the labelled questions of
	// both classes - P5's fixture does not exist at all. What is reportable
	// without one is where an injection's content came from over the
	// corpus's own prompts, which is a coverage pair about this run and not
	// about those two classes. Reported under its own name so nobody reads
	// it as the gate.
	t.Logf("coverage (not M8): %d injections from events only, %d from native memory only, %d from both",
		fromEvent, fromMem, fromBoth)
}

// M6, and it is the half that can be asserted: a prompt whose query matches
// nothing in the corpus emits exactly zero bytes, at 100%.
//
// The prompts are the corpus's own, and the ones counted are those whose
// derived query the corpus does not answer - which is "no relevant history"
// measured against this corpus rather than assumed. The synthetic arm below it
// is the other direction: text nothing could have captured.
func TestGateM6ZeroByteAbstention(t *testing.T) {
	docs := corpusPayloads(t)
	db := ingestCorpus(t, docs)
	collectNativeMemory(t, db)

	tested := 0
	for i, p := range promptsIn(docs) {
		terms := inject.QueryFor(p)
		if len(terms) == 0 {
			continue
		}
		query := strings.Join(terms, " ")
		hits, _, err := search.Search(t.Context(), db, query, "", 1)
		if err != nil {
			continue // a query internal/search refuses is not this arm's subject
		}
		mem, _, err := search.SearchMemory(t.Context(), db, query, nil, 1)
		if err != nil {
			continue
		}
		if len(hits) != 0 || len(mem) != 0 {
			continue
		}
		tested++
		res, err := inject.Build(t.Context(), db, inject.Request{Prompt: p})
		if err != nil {
			t.Fatalf("prompt %d: Build: %v", i, err)
		}
		if res.Text != "" {
			t.Errorf("M6: prompt %d matched nothing and injected %d bytes", i, len(res.Text))
		}
	}
	t.Logf("M6: %d prompts whose query the corpus does not answer, all zero bytes", tested)

	// The synthetic arm: text no capture could carry, so "no relevant
	// history" is a property of the input rather than of this corpus.
	for i := range 25 {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		res, err := inject.Build(t.Context(), db,
			inject.Request{Prompt: "zoraphex " + id.String() + " quilvorn thratmede"})
		if err != nil {
			t.Fatalf("synthetic %d: Build: %v", i, err)
		}
		if res.Text != "" {
			t.Errorf("M6: synthetic prompt %d injected %d bytes", i, len(res.Text))
		}
	}
}

// M10's other half: the deadline is enforced rather than merely present.
//
// A deadline that is never approached is not evidence that it holds, so this
// drives the budget below the cost of a real search over the real corpus -
// which is the same inequality as a search made slower than the budget, from
// the side a test can reach.
func TestGateM10TheDeadlineIsEnforced(t *testing.T) {
	docs := corpusPayloads(t)
	db := ingestCorpus(t, docs)
	collectNativeMemory(t, db)

	prompts := promptsIn(docs)
	if len(prompts) == 0 {
		t.Skip("the corpus carries no UserPromptSubmit event")
	}

	// The control: at the real budget this prompt injects something, so the
	// arm below is measuring the deadline and not an empty result.
	warm, err := inject.Build(t.Context(), db, inject.Request{Prompt: prompts[0]})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Logf("M10: the control prompt injected %d bytes at the full budget", len(warm.Text))

	for _, budget := range []time.Duration{time.Microsecond, 100 * time.Microsecond} {
		for i, p := range prompts {
			res, err := inject.BuildWithBudget(t.Context(), db, inject.Request{Prompt: p}, budget)
			if err != nil {
				t.Fatalf("prompt %d at %s: Build: %v", i, budget, err)
			}
			if res.Text != "" {
				t.Errorf("M10: prompt %d injected %d bytes under a %s budget",
					i, len(res.Text), budget)
			}
		}
	}
	t.Logf("M10: %d prompts under budgets of 1µs and 100µs, all zero bytes", len(prompts))
}

// InvokesEngramux over the corpus, which is the measurement backlog 41's
// decision rests on: this repository's own captures are largely prose about
// `engramux`, so the exclusion has to separate the invocations from the rest
// and the numbers are what say it does.
func TestTheSelfExclusionOverTheCorpus(t *testing.T) {
	docs := corpusPayloads(t)
	var mentions, invocations int
	for _, payload := range docs {
		d := store.Derive(payload)
		if d.Cmd == "" {
			continue
		}
		if strings.Contains(strings.ToLower(d.Cmd), "engramux") {
			mentions++
			if inject.InvokesEngramux(d.Cmd) {
				invocations++
			}
		}
	}
	if mentions == 0 {
		t.Skip("no captured command line mentions this product")
	}
	// The one direction this corpus can assert. A string match would have
	// answered `mentions` here, and every one of those 216 command lines is
	// the owner's own work on this product rather than the product running.
	if invocations == mentions {
		t.Errorf("every one of the %d command lines mentioning this product was read as an "+
			"invocation, which is what a string match would have done", mentions)
	}
	// Zero is the expected answer over `fixtures-raw` and is reported rather
	// than asserted either way: those captures were taken before this
	// product had a binary to run, so every mention in them is a path in a
	// `cd`, an `ls` or a `go build -o`. The positive direction is
	// TestInvokesEngramux's ten invocation rows, and the corpus that would
	// carry real ones is the installed database, which no test may open
	// (I-07).
	t.Logf("self-exclusion: %d of %d command lines mentioning this product invoke it", invocations, mentions)
}

// corpusPayloads reads the raw captures, or skips. The same reader
// internal/search's corpusDocs is, minus the parts only that gate's classes
// need.
func corpusPayloads(t *testing.T) [][]byte {
	t.Helper()
	entries, err := os.ReadDir(corpusDir)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no raw corpus at %s; this gate measures the owner's own captures", corpusDir)
	}
	if err != nil {
		t.Fatalf("read the corpus: %v", err)
	}
	var out [][]byte
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		//nolint:gosec // G304: reading this package's own corpus directory by construction
		raw, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			t.Fatalf("read a capture: %v", err)
		}
		var capture struct {
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &capture); err != nil {
			t.Fatalf("parse a capture: %v", err)
		}
		if len(capture.Payload) == 0 || string(capture.Payload) == "null" {
			t.Fatalf("a capture carries no payload to ingest")
		}
		var head struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(capture.Payload, &head)
		if head.SessionID == "selftest" { // 1.0 spec §7.5
			continue
		}
		out = append(out, capture.Payload)
	}
	if len(out) == 0 {
		t.Skipf("%s holds no captures", corpusDir)
	}
	return out
}

// ingestCorpus builds one database through the production path.
func ingestCorpus(t *testing.T, payloads [][]byte) *sql.DB {
	t.Helper()
	docs := make([]seeded, 0, len(payloads))
	for _, p := range payloads {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatalf("mint an ingest id: %v", err)
		}
		docs = append(docs, seeded{id: id.String(), payload: p})
	}
	return corpus(t, docs...)
}

// collectNativeMemory indexes this machine's own native memory into the same
// database, so the injector selects over the corpus M-2 built and not over half
// of it. A machine with none is not a failure: the event half still measures.
func collectNativeMemory(t *testing.T, db *sql.DB) {
	t.Helper()
	c := &memory.Collector{ClaudeHome: memory.ClaudeHome(), CodexHome: memory.CodexHome()}
	rep, err := c.Collect(t.Context(), db, time.Now())
	if err != nil {
		t.Fatalf("collect native memory: %v", err)
	}
	t.Logf("native memory: %d items indexed", rep.Written)
}

// promptsIn returns every prompt the corpus carries, deduplicated. They are the
// only prompts this gate has that a person actually typed.
func promptsIn(payloads [][]byte) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range payloads {
		var ev struct {
			Name   string `json:"hook_event_name"`
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(p, &ev); err != nil {
			continue
		}
		if ev.Name != "UserPromptSubmit" || ev.Prompt == "" || seen[ev.Prompt] {
			continue
		}
		seen[ev.Prompt] = true
		out = append(out, ev.Prompt)
	}
	return out
}

func percentile(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	i := len(d) * p / 100
	if i >= len(d) {
		i = len(d) - 1
	}
	return d[i]
}

func intPercentile(v []int, p int) int {
	if len(v) == 0 {
		return 0
	}
	i := len(v) * p / 100
	if i >= len(v) {
		i = len(v) - 1
	}
	return v[i]
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
