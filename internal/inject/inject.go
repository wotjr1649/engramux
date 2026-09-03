package inject

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/wotjr1649/engramux/internal/memory"
	"github.com/wotjr1649/engramux/internal/project"
	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/secret"
)

// MaxBytes is gate M5's cap on one injection, fence included.
//
// It is a conversion and not a measurement (memory spec rev.8, M-4). Codex
// documents a default additionalContext limit of about 2,500 tokens, past which
// it spills the text to a file and gives the model a preview and a path; Claude
// Code documents no limit at all. So the hosts' documented budget is Codex's, it
// is the stricter of the two by virtue of existing, and 2 bytes per token is the
// conservative end for a corpus carrying Korean. §6's third mitigation - small -
// wants the error in this direction.
const MaxBytes = 5000

// Budget is the 500 ms M-4 gives one injection, and gate M10's subject.
//
// It comes out of the 1 s the relay already has (1.0 spec §5.3) rather than
// being added to it, so the product's own budget does not move because a
// feature was added inside it. Twice the worst of the five measured
// `engramux search` runs against the installed service over a 227,954,688 B
// database - 93, 113, 185, 245 and 251 ms - each of which is process start,
// pipe dial, search and reply. What is unverified is the tail: all five are
// warm, and every read-deadline failure the 1.0 spec §7.1 records was a cold
// read after an idle period against a smaller database. M10 measures that
// rather than assuming it.
const Budget = 500 * time.Millisecond

// candidates is how many hits each of the two indexes is asked for before
// anything is filtered out.
//
// Well over what fits [MaxBytes] - about six to twelve excerpts do - because
// the filters below remove hits after the ranking rather than before it, and a
// prompt whose top ten are all this product's own search noise would otherwise
// inject nothing at all.
const candidates = 30

// The query derivation, and it is this session's decision rather than the
// spec's: rev.8's M-4 settles what injection may spend and may not select, not
// how a prompt becomes a query.
//
// [search.Search] joins its tokens with an implicit AND, and its cap is 32
// tokens - so a real prompt handed over whole is either refused outright or is
// an intersection of forty prefix phrases that matches nothing. The prompt has
// to be reduced to a query, and the reduction decides what this feature is.
//
// queryTerms is 3. An AND of three terms is a narrow query, and narrow is the
// right default for a feature whose enabling gate is precision (M7) and whose
// other gate is abstention (M6).
//
// **Which three is not a length ranking, and that was measured rather than
// assumed.** Length alone picks `세션에서` over `M3` and `checkpoint` over
// `00005`, which is backwards: the shortest tokens in this corpus are the most
// distinctive ones, because they are identifiers. So a token that carries a
// digit, a separator or any other non-letter, or that is written in capitals,
// is an identifier and sorts first whatever its length - that is P1's classes
// spelled as a rule - and only the remainder is ranked by length.
//
// minTermBytes drops the connective tissue that ranking would otherwise
// promote in a prompt with nothing longer - `this`, `with`, `그거`. It applies
// to words and not to identifiers, because `M3` and `WAL` are two bytes and
// three and are the whole of what a person is asking about.
//
// ponytail: no progressive relaxation. A prompt whose three terms intersect to
// nothing abstains rather than retrying with two, and a real answer may be one
// term away. The upgrade path is a second search with fewer terms when the
// first returns nothing, and it costs a second search inside a 500 ms budget
// whose tail is unmeasured (M10) - so it waits for M10 to say what the tail is.
const (
	queryTerms   = 3
	minTermBytes = 4
)

// maxMatches is the selectivity ceiling: a query that matched more documents
// than this is not a known-item query, and what it would inject is whatever
// bm25 put on top of a large undifferentiated set.
//
// This replaces guessing at the prompt with measuring the answer. "How do I fix
// this" reduces to one common word, and the thing that says so is not the
// word's length - it is that the word is in a fifth of the corpus. It is
// applied to each of the two lists separately because they are separate indexes
// over separate populations.
//
// ponytail: an absolute number, so it does not scale with the corpus. On a
// hundred events it never fires and on a million it fires late. The upgrade
// path is a fraction of each index's own population, which costs one count per
// injection; 200 is the number this ships with and M7 is the gate that would
// price a better one.
const maxMatches = 200

// Request is one injection: the prompt that triggered it and the scope it may
// select from.
type Request struct {
	// Prompt is what the user typed, verbatim. It is reduced to a query
	// here rather than by the caller, so that the reduction has one
	// definition (see [queryTerms]).
	Prompt string
	// Project is the absolute path of the worktree the prompt was typed in,
	// which both hosts send as `cwd`. Empty is every project, the same
	// meaning ipc.SearchRequest.Project carries.
	Project string
	// ExcludeID is the id the prompt's own event was ingested under.
	//
	// The relay delivers before it injects, so by the time this runs the
	// prompt is already a row whose text is the query - a document that
	// would otherwise be its own top hit every single time. It is exact
	// rather than heuristic: the relay minted the id, so this excludes one
	// known row and nothing that resembles it.
	ExcludeID string
}

// Result is one injection's outcome. An empty Text is an abstention, which is a
// success and not a failure - it is capability P2, and §6's first mitigation.
type Result struct {
	// Text is the fenced payload, or "" for an abstention.
	Text string
	// Events and Memory are the ids that went into it, masked. They are what
	// the service logs, which is the second half of §6's fifth mitigation -
	// a switch, and a way to see what was injected. The excerpts themselves
	// are deliberately not here: they are corpus text, and a log is a file.
	Events []string
	Memory []string
	// Reason names why an abstention was one. It is for the log and for the
	// gates; nothing branches on it.
	Reason string
}

// The abstention reasons. Each is a different question a reader of the log has.
const (
	ReasonNoTerms   = "the prompt has no term to search on"
	ReasonNoHits    = "nothing in the corpus matched"
	ReasonTooBroad  = "the query matched too much of the corpus to be recall"
	ReasonDeadline  = "the search did not finish inside the budget"
	ReasonNoRoom    = "no excerpt fits the byte cap"
	ReasonNoFence   = "no fence nonce was free of the payload"
	ReasonInjecting = ""
)

// Build selects, assembles and fences one injection, or abstains.
//
// It never returns an error for an empty corpus, an unmatched query or a blown
// deadline: those are abstentions, and an abstention is the feature. What it
// does return an error for is a database that will not answer at all, because a
// caller that cannot tell that from "nothing matched" would report a broken
// service as an empty history.
func Build(ctx context.Context, db *sql.DB, req Request) (Result, error) {
	return build(ctx, db, req, Budget)
}

// build is [Build] with the budget made explicit, so that gate M10 can drive
// the deadline past a real search over a real corpus.
//
// A budget shorter than the search is the same inequality as a search longer
// than the budget, and it is the one an ordinary test can produce: making
// SQLite slow on demand needs a progress handler this driver does not expose,
// while making the budget small needs a parameter. What it proves is the same
// thing - that the deadline is enforced mid-flight rather than only checked
// before the work starts.
func build(ctx context.Context, db *sql.DB, req Request, budget time.Duration) (Result, error) {
	terms := queryFor(req.Prompt)
	if len(terms) == 0 {
		return Result{Reason: ReasonNoTerms}, nil
	}
	query := strings.Join(terms, " ")

	var projectID string
	var projectKeys []string
	if req.Project != "" {
		p, err := project.FromArgument(req.Project)
		if err != nil {
			// A cwd this product will not walk is not a reason to fail
			// the user's prompt. Scope to every project instead, which
			// is what an absent cwd already means.
			projectID, projectKeys = "", nil
		} else {
			projectID = p.ID
			projectKeys = memory.ProjectKeys(p.Root)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	hits, total, err := search.Search(ctx, db, query, projectID, candidates)
	if err != nil {
		return abstain(ctx, err)
	}
	mem, memTotal, err := search.SearchMemory(ctx, db, query, projectKeys, candidates)
	if err != nil {
		return abstain(ctx, err)
	}
	// The deadline is checked here as well as inside the two reads. Whether
	// this driver cancels a statement already running is not this package's
	// to rely on, and an injection that arrives after the budget is one the
	// relay has already stopped waiting for - so the check that makes M10's
	// assertion true is this one, and the context is what makes the reads
	// stop early when they can.
	if ctx.Err() != nil {
		return Result{Reason: ReasonDeadline}, nil
	}

	broad := total > maxMatches
	if broad {
		hits = nil
	}
	if memTotal > maxMatches {
		broad = true
		mem = nil
	}

	hits, err = keepable(ctx, db, hits, req.ExcludeID)
	if err != nil {
		return abstain(ctx, err)
	}
	if len(hits) == 0 && len(mem) == 0 {
		if broad {
			return Result{Reason: ReasonTooBroad}, nil
		}
		return Result{Reason: ReasonNoHits}, nil
	}

	// The overhead is measured rather than computed: the fence's own
	// constants are its business, and [rand.Text] documents that it may
	// return longer strings in a future release. Fencing an empty body costs
	// one mint and makes the arithmetic exact rather than approximate -
	// len(Fence(body)) is len(probe)+len(body) for a body that does not end
	// in a newline and one less for one that does, so a body built to
	// MaxBytes-len(probe) cannot produce a payload over MaxBytes. There is
	// no second check on the finished bytes for that reason: it would be a
	// branch no input reaches, and a break-it pass says so.
	probe, err := Fence("")
	if err != nil {
		return Result{Reason: ReasonNoFence}, nil
	}
	body, events, memories := assemble(hits, mem, MaxBytes-len(probe))
	if body == "" {
		return Result{Reason: ReasonNoRoom}, nil
	}
	text, err := Fence(body)
	if err != nil {
		return Result{Reason: ReasonNoFence}, nil
	}
	return Result{Text: text, Events: events, Memory: memories, Reason: ReasonInjecting}, nil
}

// abstain turns a failed read into an abstention when the budget is what ended
// it, and into an error otherwise.
//
// The distinction is the whole of M10's failure mode: a deadline miss is
// designed behaviour and emits zero bytes down M6's own path, while a database
// that will not answer is a broken service and has to be visible as one.
func abstain(ctx context.Context, err error) (Result, error) {
	if ctx.Err() != nil {
		return Result{Reason: ReasonDeadline}, nil
	}
	// The three query refusals are not failures either: they are what a
	// prompt that reduces to nothing usable looks like from inside
	// internal/search.
	switch {
	case isQueryRefusal(err):
		return Result{Reason: ReasonNoTerms}, nil
	default:
		return Result{}, fmt.Errorf("inject: %w", err)
	}
}

func isQueryRefusal(err error) bool {
	return errors.Is(err, search.ErrEmptyQuery) ||
		errors.Is(err, search.ErrTooManyTokens) ||
		errors.Is(err, search.ErrTokenTooLong)
}

// queryFor reduces a prompt to the terms the search runs on. See [queryTerms]
// for why it is a reduction at all and why identifiers outrank length.
func queryFor(prompt string) []string {
	kept := make([]string, 0, queryTerms)
	seen := map[string]bool{}
	for _, f := range strings.Fields(prompt) {
		low := strings.ToLower(f)
		if seen[low] {
			continue
		}
		if !identifier(f) && len(f) < minTermBytes {
			continue
		}
		seen[low] = true
		kept = append(kept, f)
	}
	if len(kept) == 0 {
		return nil
	}
	// Stable, so equal terms keep the order the prompt put them in and one
	// prompt always produces one query.
	order := slices.Clone(kept)
	sort.SliceStable(order, func(i, j int) bool {
		a, b := identifier(order[i]), identifier(order[j])
		if a != b {
			return a
		}
		return len(order[i]) > len(order[j])
	})
	if len(order) > queryTerms {
		order = order[:queryTerms]
	}
	// Back into prompt order: the tokens are ANDed, so the order changes no
	// result, and a query a person reads in the log should read as their own
	// words did.
	out := make([]string, 0, len(order))
	for _, f := range kept {
		if slices.Contains(order, f) {
			out = append(out, f)
		}
	}
	return out
}

// identifier reports whether a prompt token is the kind of thing this corpus is
// made of rather than the prose around it: `M3`, `00005`, `bm25`, `--project`,
// `internal/search`, `WAL`.
//
// Two signals, and each is one a word does not have. A non-letter rune - a
// digit, a separator, any punctuation - is what an identifier, a path, a flag
// and a version have in common. Capitals are the other: a token written all in
// capitals is an acronym in every language this corpus carries, and lower-case
// prose never is. A single letter is neither, which is what the length guard
// below it is for.
func identifier(tok string) bool {
	if len([]rune(tok)) < 2 {
		return false
	}
	upper := true
	for _, r := range tok {
		if !unicode.IsLetter(r) {
			return true
		}
		if unicode.IsLower(r) {
			upper = false
		}
	}
	return upper && unicode.IsUpper([]rune(tok)[0])
}

// keepable drops the hits this injection may not carry: the prompt's own event,
// and every event that is this product running itself.
//
// # Why the self-exclusion is here and not in the ranking
//
// Backlog 41 found a search returning its own capture as its own top hit, and
// the pull path is left alone deliberately - asking for a thing and getting your
// own last ask for it is an answer, and a ranking function that special-cases a
// document class is how a ranking function starts to rot. The push path is a
// different question: this selects from the same corpus, so a user's own last
// search becomes a candidate for their next prompt, which is exactly the
// distractor §6 cites Context Rot for.
//
// # The test is the command position, not the word
//
// This repository's own corpus is largely prose about `engramux`, so a string
// match would exclude the owner's work on the product along with the product's
// own noise. What is tested is whether a command line *invokes* the binary:
// each segment of the shell line, first token, quotes stripped, base name
// compared. `grep -rn engramux .` keeps its hit; `cd d:/x && ./dist/engramux.exe
// search foo` does not.
func keepable(ctx context.Context, db *sql.DB, hits []search.Hit, excludeID string) ([]search.Hit, error) {
	out := make([]search.Hit, 0, len(hits))
	ids := make([]any, 0, len(hits))
	for _, h := range hits {
		if h.ID == excludeID {
			continue
		}
		out = append(out, h)
		ids = append(ids, h.ID)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// derived_cmd rather than the payload: it is migration 00005's column
	// and it holds tool_input.command and nothing else, so a document that
	// merely talks about a command line has nothing in it.
	q := `SELECT id, coalesce(derived_cmd, '') FROM events WHERE id IN (?` +
		strings.Repeat(",?", len(ids)-1) + `)`
	rows, err := db.QueryContext(ctx, q, ids...)
	if err != nil {
		return nil, fmt.Errorf("read the commands: %w", err)
	}
	defer func() { _ = rows.Close() }()
	self := map[string]bool{}
	for rows.Next() {
		var id, cmd string
		if err := rows.Scan(&id, &cmd); err != nil {
			return nil, fmt.Errorf("scan a command: %w", err)
		}
		if InvokesEngramux(cmd) {
			self[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the commands: %w", err)
	}
	// Closed before the caller's next read, which takes the same single
	// connection (1.0 spec §5.4).
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close the commands: %w", err)
	}

	kept := out[:0]
	for _, h := range out {
		if !self[h.ID] {
			kept = append(kept, h)
		}
	}
	return kept, nil
}

// engramuxBase is the installed relay's file name without its extension. The
// installer writes exactly this name, so a base name is a stronger test than
// the installed path would be: the owner's own `./dist/engramux.exe` is this
// product's noise too, and a path comparison would keep it.
const engramuxBase = "engramux"

// shellBreaks are where one command line becomes two. A first token after any
// of these is a command position, which is what makes `cd x && engramux search`
// an invocation rather than a mention.
var shellBreaks = []string{"&&", "||", ";", "|", "\n", "&", "("}

// InvokesEngramux reports whether a captured command line runs this product.
//
// Exported because it is the whole of backlog 41's decision and the gates
// measure it directly over the corpus; nothing else calls it from outside.
func InvokesEngramux(cmd string) bool {
	segments := []string{cmd}
	for _, br := range shellBreaks {
		var next []string
		for _, s := range segments {
			next = append(next, strings.Split(s, br)...)
		}
		segments = next
	}
	for _, s := range segments {
		fields := strings.Fields(s)
		if len(fields) == 0 {
			continue
		}
		tok := strings.Trim(fields[0], `"'`)
		base := strings.ToLower(filepath.Base(strings.ReplaceAll(tok, `\`, "/")))
		base = strings.TrimSuffix(base, ".exe")
		if base == engramuxBase {
			return true
		}
	}
	return false
}

// assemble builds the fenced body from the two ranked lists, newest ranking
// first, until budget bytes are spent.
//
// The two lists alternate rather than running one after the other. They are
// ranked by separate indexes and their scores are not comparable (spec 5.2), so
// concatenating them would spend the whole budget on whichever list happened to
// be first and P4 - one query reaching what only the other host knows - would be
// the one that never fits. Alternating keeps each list's own order and gives
// neither the whole budget.
//
// Every excerpt carries its id and its timestamp, which is §6's fourth
// mitigation: a reader can tell recall from instruction, and an incident can be
// traced back to the event that carried it.
func assemble(hits []search.Hit, mem []search.MemoryHit, budget int) (body string, events, memories []string) {
	var b strings.Builder
	add := func(block, id string, into *[]string) bool {
		if b.Len() > 0 {
			block = "\n" + block
		}
		if b.Len()+len(block) > budget {
			return false
		}
		b.WriteString(block)
		*into = append(*into, id)
		return true
	}

	i, j := 0, 0
	for i < len(hits) || j < len(mem) {
		if i < len(hits) {
			h := hits[i]
			i++
			// Masked here rather than by the caller, for the reason
			// backlog 29 gave: events.id is TEXT with no CHECK, so a
			// path-shaped id is storable and this is an egress.
			id := secret.MaskString(h.ID)
			if !add(fmt.Sprintf("[event %s %s %s]\n%s", id, h.Host, stamp(h.ReceivedAtMS), h.Excerpt),
				id, &events) {
				break
			}
		}
		if j < len(mem) {
			m := mem[j]
			j++
			id := secret.MaskString(m.ID)
			if !add(fmt.Sprintf("[memory %s %s %s]\n%s", id, m.Host, stamp(m.HostModifiedMS), m.Excerpt),
				id, &memories) {
				break
			}
		}
	}
	return b.String(), events, memories
}

// stamp formats a captured millisecond count for a provenance line. Zero is a
// host that wrote no timestamp - 1 of the 18 Claude Code notes read on
// 2026-09-02 carries no modified key - and printing the epoch for one would be
// a date nobody wrote.
func stamp(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04Z")
}
