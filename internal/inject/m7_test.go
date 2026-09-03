package inject_test

import (
	"bufio"
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/inject"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/store"
)

// Gate M7, in three passes over one frozen corpus. The spec section *What M7
// will measure* owns every value below; this file restates none of the
// reasoning and holds only the mechanism.
//
// # Why a snapshot and not the running service
//
// The machine that runs this is the machine that generates the corpus - it went
// from 20,075 to 20,993 events inside the session that wrote this file - so a
// number pinned against the live database is not re-derivable by anyone,
// including the person who pinned it. I-07 forbids a second process opening the
// service's own database; a copy at another path is a different file and the
// service does not hold it. AGENTS.md's first carve-out is what permits the
// stop, and it records that the snapshot is the pair: the .db and its .db-wal,
// because a hard-killed service never checkpoints.
//
// Measuring in-process also returns what the wire does not. ipc.InjectReply
// carries the fenced text and nothing else, where [inject.Build] returns the ids
// it selected and the elapsed time it measured on its own clock - so M9 and M10
// become claimable here rather than disclaimed.
//
// # Why three passes
//
// Because `should_inject` has to be judged before the injector's output is
// visible. A labeller who has already seen what came back cannot un-see it, and
// an abstention scored after the fact is either dropped from the average - which
// leaves a gate passable by abstaining harder - or scored 0/0, which is not a
// number.
//
// # Nothing here prints corpus text
//
// Every prompt and every excerpt goes to a file under .capture/, which is
// gitignored. The tests log counts, byte figures and ratios. The prompt written
// to that file is masked with [secret.Mask] first: it is read straight out of
// events.payload, which is where a secret is tagged rather than destroyed
// (I-10), and every other egress in this product filters on that tag.
const (
	m7Dir      = "../../.capture/m7"
	m7Snapshot = m7Dir + "/snapshot/engramux.db"
)

// m7FixtureDir is where the two labelled files live. It is a variable and reads
// an environment override for one reason: a labelled fixture costs hours of
// somebody's attention, and handing that over without having run the passes
// that consume it end to end is how those hours get spent twice. The override
// points the passes at a scratch directory beside the same snapshot, so the
// machinery can be exercised against throwaway labels without the real files
// being anywhere near it. Nothing shipped reads this.
var m7FixtureDir = cmp.Or(os.Getenv("ENGRAMUX_M7_DIR"), m7Dir)

func m7PromptsPath() string { return filepath.Join(m7FixtureDir, "prompts.tsv") }
func m7BlocksPath() string  { return filepath.Join(m7FixtureDir, "blocks.tsv") }

// m7Want is how many prompts the fixture holds, and m7Bar is the pre-registered
// gate: the relevant-byte share, strictly above it. Both are the spec's.
const (
	m7Want = 150
	m7Bar  = 0.50
)

// m7Todo is the marker a human replaces. A file still carrying one is not a
// labelled file, and every pass that reads one refuses rather than guessing.
const m7Todo = "TODO"

// m7Yes and m7No are the two answers. Anything else in a label column is an
// error, not a default - a typo that read as "no" would move the gate silently.
const (
	m7Yes = "yes"
	m7No  = "no"
)

// m7Prompt is one sampled prompt: the event it was captured as, the text the
// relay would have sent, and the worktree it was typed in.
type m7Prompt struct {
	id      string
	session string
	prompt  string
	cwd     string
	should  string // the human's should_inject label, "" before pass 1 is answered
}

// m7Block is one excerpt block inside an injection, with the bytes it spent.
type m7Block struct {
	promptID string
	id       string
	kind     string
	bytes    int
	text     string
	relevant string // the human's label, "" before pass 2 is answered
}

// TestWriteM7Prompts is pass 1: it writes the prompts to be labelled and does
// not run the injector at all.
//
// It refuses to overwrite an existing file. The labels in it are the only part
// of this gate a machine cannot reproduce, and a rerun that clobbered them would
// destroy the fixture in the one way nothing else can repair.
func TestWriteM7Prompts(t *testing.T) {
	db := m7Open(t)
	prompts := m7Sample(t, db)

	sessions := map[string]bool{}
	for _, p := range prompts {
		sessions[p.session] = true
	}
	t.Logf("sampled %d prompts across %d distinct sessions", len(prompts), len(sessions))

	if _, err := os.Stat(m7PromptsPath()); err == nil {
		t.Skipf("%s exists; not overwriting a file that may carry labels", m7PromptsPath())
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat the prompt file: %v", err)
	}

	header := []string{
		"# Gate M7, pass 1. Replace TODO with yes or no, then run pass 2.",
		"#",
		"# yes: given only this prompt, earlier sessions on this machine plausibly hold",
		"#      something worth putting in front of the model.",
		"# no:  they do not, and the right answer is zero bytes.",
		"#",
		"# Answer from the prompt alone. Pass 2 is what shows you the injector's output,",
		"# and it is deliberately after this - a label written with the answer visible",
		"# measures the answer (memory spec, What M7 will measure).",
		"#",
		"# columns: prompt_id, should_inject, prompt",
		"# This file is under .capture/ and is never committed.",
		"",
	}
	rows := make([]string, 0, len(prompts))
	for _, p := range prompts {
		rows = append(rows, strings.Join([]string{p.id, m7Todo, m7Line(secret.MaskString(p.prompt))}, "\t"))
	}
	m7Write(t, m7PromptsPath(), header, rows)
	t.Logf("wrote %d rows to the prompt file", len(rows))
}

// TestWriteM7Blocks is pass 2: it runs the injector over the labelled prompts
// and writes every emitted block for a second labelling pass.
//
// It also reports, over this corpus and for the first time, everything the spec
// lists as reported rather than gated.
func TestWriteM7Blocks(t *testing.T) {
	db := m7Open(t)
	prompts := m7LabelledPrompts(t, db)

	var (
		rows      []string
		emitted   int
		abstained int
		byReason  = map[string]int{}
		bytes     []int
		elapsed   []time.Duration
		fenced    int
		carrying  int
	)
	for _, p := range prompts {
		res := m7Build(t, db, p)
		elapsed = append(elapsed, res.Elapsed)
		if res.Text == "" {
			abstained++
			byReason[res.Reason]++
			continue
		}
		emitted++
		bytes = append(bytes, len(res.Text))
		if ok, carries := m7Fenced(res.Text); ok {
			fenced++
			if carries {
				carrying++
			}
		}
		for _, b := range m7SplitBlocks(t, p.id, res) {
			rows = append(rows, strings.Join([]string{
				b.promptID, b.id, b.kind, strconv.Itoa(b.bytes), m7Todo, m7Line(b.text),
			}, "\t"))
		}
	}

	m7Report(t, prompts, emitted, abstained, byReason, bytes, elapsed, fenced, carrying, len(rows))

	if _, err := os.Stat(m7BlocksPath()); err == nil {
		t.Skipf("%s exists; not overwriting a file that may carry labels", m7BlocksPath())
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat the block file: %v", err)
	}
	header := []string{
		"# Gate M7, pass 2. Replace TODO with yes or no for every row, then run the gate.",
		"#",
		"# yes: this excerpt was worth the bytes it spent on this prompt.",
		"# no:  it was not - it is a distractor, or it is unrelated.",
		"#",
		"# The gate is the relevant-byte share and not a count, so a long block marked",
		"# no costs more than a short one. Judge the block, not the document it came from.",
		"#",
		"# columns: prompt_id, block_id, kind, bytes, relevant, excerpt",
		"# This file is under .capture/ and is never committed.",
		"",
	}
	m7Write(t, m7BlocksPath(), header, rows)
	t.Logf("wrote %d block rows over %d injecting prompts", len(rows), emitted)
}

// TestGateM7PrecisionAtBudget is the gate. It re-runs the injector against the
// frozen corpus and scores what comes back *now* against the labels, rather than
// re-reading a number out of the labelled file.
//
// That distinction is the whole difference between a gate and a record. A pass
// that scored the file would stay green through any change to the selector, the
// ranking or the budget, because nothing in it would have re-run.
func TestGateM7PrecisionAtBudget(t *testing.T) {
	db := m7Open(t)
	prompts := m7LabelledPrompts(t, db)
	labels := m7LabelledBlocks(t)

	type scored struct {
		prompt     m7Prompt
		blocks     []m7Block
		relBytes   int
		totalBytes int
	}
	var (
		runs       []scored
		unlabelled []string
		emitted    int
		coverage   int
		wantInject int
		falsePos   int
		falseNeg   int
	)
	for _, p := range prompts {
		res := m7Build(t, db, p)
		if p.should == m7Yes {
			wantInject++
		}
		if res.Text == "" {
			if p.should == m7Yes {
				falseNeg++
			}
			continue
		}
		emitted++
		if p.should == m7No {
			falsePos++
		}
		s := scored{prompt: p, blocks: m7SplitBlocks(t, p.id, res)}
		for _, b := range s.blocks {
			s.totalBytes += b.bytes
			switch labels[b.promptID+"\x00"+b.id] {
			case m7Yes:
				s.relBytes += b.bytes
			case m7No:
			default:
				unlabelled = append(unlabelled, b.promptID+" "+b.id)
			}
		}
		if s.relBytes > 0 && p.should == m7Yes {
			coverage++
		}
		runs = append(runs, s)
	}

	// An emitted block with no label means the corpus or the code moved under
	// the fixture. Guessing would report a number about a fixture that no
	// longer exists, so this is loud.
	if len(unlabelled) > 0 {
		t.Fatalf("%d emitted blocks carry no label - the frozen corpus or the injector has moved "+
			"since %s was written. Re-run pass 2 into a fresh file and re-label, or restore the "+
			"snapshot this fixture was built against", len(unlabelled), m7BlocksPath())
	}

	share := m7Share(runs, func(s scored) (int, int) { return s.relBytes, s.totalBytes })

	// Reported, not gated.
	var blocksTotal, blocksRelevant int
	for _, s := range runs {
		for _, b := range s.blocks {
			blocksTotal++
			if labels[b.promptID+"\x00"+b.id] == m7Yes {
				blocksRelevant++
			}
		}
	}
	t.Logf("M7 gate: relevant-byte share %.3f, bar strictly above %.2f", share, m7Bar)
	t.Logf("reported: pooled block precision %.3f over %d blocks", m7Ratio(blocksRelevant, blocksTotal), blocksTotal)
	t.Logf("reported: %d of %d prompts injected, %d abstained", emitted, len(prompts), len(prompts)-emitted)
	t.Logf("reported: coverage %d of %d should_inject prompts got a relevant block", coverage, wantInject)
	t.Logf("reported: %d false positives, %d false negatives", falsePos, falseNeg)

	if !(share > m7Bar) {
		t.Errorf("relevant-byte share %.3f, want strictly above %.2f. This is the pre-registered "+
			"bar in the memory spec's *What M7 will measure*; moving it is a spec revision, not a "+
			"constant edit", share, m7Bar)
	}

	m7NonVacuity(t, db, prompts, labels, share)
}

// m7NonVacuity runs the arms the spec registers, and says which of them proved
// anything.
//
// The shape below is a correction made while implementing it and before any
// label existed. Two of the three arms first registered are vacuous by
// construction on a fixture this size, and an arm that passes by construction is
// worse than no arm: it reads like evidence.
//
// A shuffled prompt-to-block assignment always scores zero, because a label is
// keyed by prompt *and* block and a block belongs to one prompt - so the lookup
// misses whatever the labels say, including when every one of them says yes. It
// is dropped rather than kept as decoration.
//
// The reversed and broadened arms have the mirror defect. Only what the shipped
// injector emitted was ever put in front of a labeller, so an arm that retrieves
// something else scores its blocks as unjudged, and unjudged is irrelevant. When
// the overlap is thin the arm is *inconclusive*, not passed, and it says so.
// What would make it conclusive is labelling the whole candidate pool rather
// than the handful that fit the byte cap, which is a bigger fixture than this
// one.
//
// What replaces the shuffle is an arm with no such hole: on the prompts the
// owner said hold nothing worth recalling, every byte spent is a byte spent
// wrongly. It uses pass 1's labels, which are written before any output is
// visible, so nothing about it can be answered by the injector's own choices.
func m7NonVacuity(t *testing.T, db *sql.DB, prompts []m7Prompt, labels map[string]string, real float64) {
	t.Helper()

	// Arm one: the injector must not spend bytes on a prompt the owner said
	// has nothing behind it.
	var falseBytes, allBytes int
	for _, p := range prompts {
		res := m7Build(t, db, p)
		if res.Text == "" {
			continue
		}
		for _, b := range m7SplitBlocks(t, p.id, res) {
			allBytes += b.bytes
			if p.should == m7No {
				falseBytes += b.bytes
			}
		}
	}
	waste := m7Ratio(falseBytes, allBytes)
	t.Logf("arm  false-positive bytes      %.3f of all emitted bytes went to should_inject=no prompts", waste)
	if waste > m7Bar {
		t.Errorf("%.3f of every byte this injector emitted went to a prompt the owner said holds "+
			"nothing worth recalling, above the bar of %.2f. The precision figure above cannot "+
			"redeem that: it is scored only over prompts that were injected into", waste, m7Bar)
	}

	m7Arm(t, "reversed ranking", m7Retrieved(t, db, prompts, labels, search.MatchAll, true), real)
	m7Arm(t, "MatchAny selector", m7Retrieved(t, db, prompts, labels, search.MatchAny, false), real)
}

// m7MinOverlap is how much of an arm's emitted bytes must carry a label before
// that arm is evidence rather than an artefact of the pool.
const m7MinOverlap = 0.20

// m7Arm reports one retrieval arm and fails when it scores above the bar. An arm
// whose blocks were mostly never judged is reported as inconclusive: it did not
// pass, nothing looked.
func m7Arm(t *testing.T, name string, got m7ArmResult, real float64) {
	t.Helper()
	t.Logf("arm  %-24s share %.3f against the real %.3f, %.0f%% of its bytes judged",
		name, got.share, real, got.overlap*100)
	if got.overlap < m7MinOverlap {
		t.Logf("arm  %-24s INCONCLUSIVE - only %.0f%% of what it emitted was ever labelled, and "+
			"unjudged scores as irrelevant, so a low share here is the pool and not the ranking",
			name, got.overlap*100)
		return
	}
	if got.share > m7Bar {
		t.Errorf("the %q arm scored %.3f, above the bar of %.2f. An arm that passes means the gate "+
			"is not measuring what it claims - the same shape rev.12 records a rank ceiling being "+
			"deleted for", name, got.share, m7Bar)
	}
}

// m7ArmResult is one arm's score and how much of it anybody judged.
type m7ArmResult struct {
	share   float64
	overlap float64
}

// m7Retrieved reconstructs the injector's tail from its exported pieces and
// returns the relevant-byte share of what that arm would have emitted.
func m7Retrieved(t *testing.T, db *sql.DB, prompts []m7Prompt, labels map[string]string, mode search.Match, reverse bool) m7ArmResult {
	t.Helper()
	probe, err := inject.Fence("")
	if err != nil {
		t.Fatalf("fence probe: %v", err)
	}
	budget := inject.MaxBytes - len(probe)

	var relBytes, judgedBytes, totalBytes int
	for _, p := range prompts {
		terms := inject.QueryFor(p.prompt)
		if len(terms) == 0 {
			continue
		}
		query := strings.Join(terms, " ")
		hits, _, err := search.Search(t.Context(), db, query, "", 30, mode)
		if err != nil {
			continue // a query this arm cannot express is not evidence either way
		}
		mem, _, err := search.SearchMemory(t.Context(), db, query, nil, 30, mode)
		if err != nil {
			continue
		}
		if reverse {
			for i, j := 0, len(hits)-1; i < j; i, j = i+1, j-1 {
				hits[i], hits[j] = hits[j], hits[i]
			}
			for i, j := 0, len(mem)-1; i < j; i, j = i+1, j-1 {
				mem[i], mem[j] = mem[j], mem[i]
			}
		}
		body, events, memories := inject.Assemble(hits, mem, budget)
		if body == "" {
			continue
		}
		for _, b := range m7SplitBody(t, p.id, body, events, memories, false) {
			totalBytes += b.bytes
			switch labels[b.promptID+"\x00"+b.id] {
			case m7Yes:
				relBytes += b.bytes
				judgedBytes += b.bytes
			case m7No:
				judgedBytes += b.bytes
			}
		}
	}
	return m7ArmResult{share: m7Ratio(relBytes, totalBytes), overlap: m7Ratio(judgedBytes, totalBytes)}
}

// m7SplitBlocks splits one [inject.Result]'s fenced text into its blocks.
func m7SplitBlocks(t *testing.T, promptID string, res inject.Result) []m7Block {
	t.Helper()
	return m7SplitBody(t, promptID, res.Text, res.Events, res.Memory, true)
}

// m7SplitBody locates each block by the exact marker its own id produces and
// asserts that marker occurs exactly once.
//
// This is not a scan for a delimiter. §6 says the corpus is attacker-reachable
// and an excerpt is a raw window of captured text, so a line beginning "[event "
// is a string somebody could have written into a page the agent fetched. What
// makes this safe is that the ids are handed over by the injector rather than
// read out of the body: a forged marker only collides when it carries one of
// *these* ids, and then the count is two and this fails loudly instead of
// splitting in the wrong place.
func m7SplitBody(t *testing.T, promptID, body string, events, memories []string, fenced bool) []m7Block {
	t.Helper()
	type mark struct {
		at   int
		id   string
		kind string
	}
	var marks []mark
	add := func(kind string, ids []string) {
		for _, id := range ids {
			needle := "[" + kind + " " + id + " "
			if n := strings.Count(body, needle); n != 1 {
				t.Fatalf("the marker for %s %s occurs %d times in one injection, want 1 - "+
					"a captured excerpt may be carrying it", kind, id, n)
			}
			marks = append(marks, mark{at: strings.Index(body, needle), id: id, kind: kind})
		}
	}
	add("event", events)
	add("memory", memories)
	sort.Slice(marks, func(i, j int) bool { return marks[i].at < marks[j].at })

	// A fenced text ends at its close marker; a raw body from
	// [inject.Assemble] - which is what the retrieval arms build - ends where
	// it ends. The caller says which, because inferring it would let a
	// truncated fence read as a raw body and lose gate M9's own invariant.
	end := len(body)
	if fenced {
		end = strings.LastIndex(body, "\n</engramux-data ")
		if end < 0 {
			t.Fatalf("no fence close in an injection - gate M9's own invariant")
		}
		if len(marks) > 0 && marks[len(marks)-1].at > end {
			t.Fatalf("a block marker sits after the fence close")
		}
	}

	out := make([]m7Block, 0, len(marks))
	for i, m := range marks {
		stop := end
		if i+1 < len(marks) {
			stop = marks[i+1].at
		}
		out = append(out, m7Block{
			promptID: promptID,
			id:       m.id,
			kind:     m.kind,
			bytes:    stop - m.at,
			text:     body[m.at:stop],
		})
	}
	return out
}

// m7Fenced reports whether an injection is fenced and whether its body carries
// its own nonce, which is gate M9 over this corpus.
func m7Fenced(text string) (fenced, carries bool) {
	open := strings.Index(text, "<engramux-data ")
	if open < 0 {
		return false, false
	}
	rest := text[open+len("<engramux-data "):]
	gt := strings.Index(rest, ">")
	if gt < 0 {
		return false, false
	}
	nonce := rest[:gt]
	close := "\n</engramux-data " + nonce + ">"
	if !strings.HasSuffix(text, close) {
		return false, false
	}
	body := rest[gt+2 : len(rest)-len(close)+1]
	return true, strings.Contains(body, nonce)
}

// m7Build runs one injection with the request the relay would have sent.
func m7Build(t *testing.T, db *sql.DB, p m7Prompt) inject.Result {
	t.Helper()
	res, err := inject.Build(t.Context(), db, inject.Request{
		Prompt: p.prompt, Project: p.cwd, ExcludeID: p.id,
	})
	if err != nil {
		t.Fatalf("inject over the snapshot: %v", err)
	}
	return res
}

// m7Report logs everything the spec lists as reported rather than gated.
func m7Report(t *testing.T, prompts []m7Prompt, emitted, abstained int, byReason map[string]int,
	bytes []int, elapsed []time.Duration, fenced, carrying, blocks int,
) {
	t.Helper()
	sort.Ints(bytes)
	sort.Slice(elapsed, func(i, j int) bool { return elapsed[i] < elapsed[j] })

	t.Logf("M5 over this corpus: %d injections, largest %d B, median %d B, cap %d B",
		len(bytes), m7Last(bytes), m7Median(bytes), inject.MaxBytes)
	t.Logf("M6 over this corpus: %d of %d prompts injected, %d abstained",
		emitted, len(prompts), abstained)
	for _, r := range m7SortedReasons(byReason) {
		t.Logf("M6 abstention reason %-2d x %q", byReason[r], r)
	}
	t.Logf("M9 over this corpus: %d of %d fenced, %d bodies carrying their own nonce",
		fenced, emitted, carrying)
	if len(elapsed) > 0 {
		t.Logf("M10 over this corpus: worst %v, median %v, budget %v",
			elapsed[len(elapsed)-1].Round(time.Microsecond),
			elapsed[len(elapsed)/2].Round(time.Microsecond), inject.Budget)
	}
	t.Logf("%d blocks over %d injecting prompts", blocks, emitted)
}

func m7SortedReasons(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func m7Last(v []int) int {
	if len(v) == 0 {
		return 0
	}
	return v[len(v)-1]
}

func m7Median(v []int) int {
	if len(v) == 0 {
		return 0
	}
	return v[len(v)/2]
}

func m7Ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func m7Share[T any](runs []T, of func(T) (int, int)) float64 {
	var a, b int
	for _, r := range runs {
		x, y := of(r)
		a += x
		b += y
	}
	return m7Ratio(a, b)
}

// m7Open opens the frozen snapshot, skipping when it is not there.
func m7Open(t *testing.T) *sql.DB {
	t.Helper()
	if _, err := os.Stat(m7Snapshot); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no snapshot at %s - stop the service, copy the .db and .db-wal pair there, "+
			"and start it again (AGENTS.md's first carve-out)", m7Snapshot)
	} else if err != nil {
		t.Fatalf("stat the snapshot: %v", err)
	}
	db, err := store.Open(t.Context(), m7Snapshot)
	if err != nil {
		t.Fatalf("open the snapshot: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the snapshot: %v", err)
		}
	})
	return db
}

// m7Sample is the pre-registered sample: every UserPromptSubmit event ordered by
// the instant it was received, then every floor(N/m7Want)-th.
//
// No seed, so anyone holding the snapshot re-derives the same thirty without
// being handed one. Systematic over time rather than round-robin across
// sessions: with more sessions than prompts wanted, a round-robin can only ever
// take each session's first prompt, and an opening prompt is not a typical one.
func m7Sample(t *testing.T, db *sql.DB) []m7Prompt {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT id, session_id, payload FROM events WHERE event_name = 'UserPromptSubmit' ORDER BY received_at`)
	if err != nil {
		t.Fatalf("read the prompts: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var all []m7Prompt
	for rows.Next() {
		var id, session, payload string
		if err := rows.Scan(&id, &session, &payload); err != nil {
			t.Fatalf("scan a prompt: %v", err)
		}
		var p struct {
			Prompt string `json:"prompt"`
			Cwd    string `json:"cwd"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil || p.Prompt == "" {
			continue
		}
		all = append(all, m7Prompt{id: id, session: session, prompt: p.Prompt, cwd: p.Cwd})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the prompts: %v", err)
	}
	if len(all) < m7Want {
		t.Skipf("the snapshot holds %d usable prompts, fewer than the %d the fixture wants", len(all), m7Want)
	}

	// k*N/want rather than a fixed stride. An integer stride leaves the tail
	// of the window unsampled whenever want does not divide N - at 150 of 376
	// it would stop at index 298 and never look at the last fifth - and the
	// tail is the most recent work, which is the part a prompt is most likely
	// to have history for.
	out := make([]m7Prompt, 0, m7Want)
	for k := range m7Want {
		out = append(out, all[k*len(all)/m7Want])
	}
	t.Logf("%d prompts in the snapshot, taking %d spread evenly across the window", len(all), len(out))
	return out
}

// m7LabelledPrompts is [m7Sample] with pass 1's answers attached, refusing a
// file that is missing, unlabelled or out of step with the sample.
func m7LabelledPrompts(t *testing.T, db *sql.DB) []m7Prompt {
	t.Helper()
	labels := map[string]string{}
	for _, f := range m7Read(t, m7PromptsPath(), 3) {
		labels[f[0]] = m7Answer(t, m7PromptsPath(), f[1])
	}
	out := m7Sample(t, db)
	for i := range out {
		v, ok := labels[out[i].id]
		if !ok {
			t.Fatalf("%s has no row for a sampled prompt - it was written against a different "+
				"snapshot. Re-run pass 1 into a fresh file", m7PromptsPath())
		}
		out[i].should = v
	}
	return out
}

// m7LabelledBlocks reads pass 2's answers, keyed by prompt and block.
func m7LabelledBlocks(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range m7Read(t, m7BlocksPath(), 6) {
		out[f[0]+"\x00"+f[1]] = m7Answer(t, m7BlocksPath(), f[4])
	}
	return out
}

// m7Answer reads one label column, refusing anything that is not yes or no.
func m7Answer(t *testing.T, path, v string) string {
	t.Helper()
	switch strings.ToLower(strings.TrimSpace(v)) {
	case m7Yes:
		return m7Yes
	case m7No:
		return m7No
	default:
		t.Skipf("%s still carries an unanswered or unreadable label (%q); it is not a labelled "+
			"fixture yet", path, v)
		return ""
	}
}

// m7Read returns the data rows of one TSV, skipping when the file is absent.
func m7Read(t *testing.T, path string, fields int) [][]string {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // G304: a fixed path under .capture/
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no %s yet - the pass that writes it has not been run, or it has not been labelled", path)
	} else if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var out [][]string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != fields {
			t.Fatalf("%s has a row with %d columns, want %d", path, len(parts), fields)
		}
		out = append(out, parts)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(out) == 0 {
		t.Skipf("%s has no data rows", path)
	}
	return out
}

// m7Write writes one TSV under .capture/.
func m7Write(t *testing.T, path string, header, rows []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("make the output directory: %v", err)
	}
	f, err := os.Create(path) //nolint:gosec // G304: a fixed path under .capture/
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	}()
	w := bufio.NewWriter(f)
	for _, l := range append(header, rows...) {
		if _, err := fmt.Fprintln(w, l); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush %s: %v", path, err)
	}
}

// m7Line collapses a value to one line so a TSV row stays one row.
//
// Six of the sixteen prompts .capture/ holds carry a newline and one carries a
// tab, so this is not defensive: a prompt written through unchanged splits its
// own row and every column after it reads as data.
func m7Line(s string) string { return strings.Join(strings.Fields(s), " ") }
