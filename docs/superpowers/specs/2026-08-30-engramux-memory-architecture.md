# Engramux — memory architecture, after 1.0

**rev.15** - 2026-09-04 - rev.15 is three corrections and one closure, all from an adversarial review
of rev.14 taken before any label was written. **The three-term AND does not match nothing**: measured
over 150 prompts, the event index returns exactly one document in 115 of the 120 abstentions that had
a query at all, and that document is the prompt's own event, which the exclusion then removes - so the
abstention reason's own words were misread. **Nearly half the fixture is not English**: 40 mostly
Hangul and 29 mixed of 150, against a corpus rev.11 measured at 74% English and an injector that
deliberately carries no translator, so an unstratified abstention figure cannot tell capability P2
from a defect. **The first label was asking the wrong question**, and is reworded from "does history
plausibly exist" - which nobody can answer from a prompt - to "did you want context", with what that
does and does not support written into the table. And **gate M4's second measurement closes its own
delete condition**: improved in all three classes, regressed in none. rev.1 to rev.14 below.

**rev.14** - 2026-09-04 - rev.14 registers what gate **M7** will measure, before its harness ran and
before one prompt was labelled. Three findings forced a redesign of the obvious shape. The installed
corpus **grew from 20,075 to 20,993 events inside one session**, so a number pinned against it is not
re-derivable and the fixture becomes a frozen snapshot of the `.db` and `.db-wal` pair. The 16
distinct prompts `.capture/` holds **predate the corpus by 34 hours**, so their own working context
cannot be in it, and the 374 prompts inside the snapshot replace them. And the statistic is the
**relevant-byte share** rather than a count, because the budget being spent is bytes. The gate is
pre-registered at **strictly above 0.50**, with three non-vacuity arms that must fail, and M7's row
is narrowed in writing to what it actually measures. rev.1 to rev.13 below.

**rev.13** · 2026-09-04 — rev.13 records what installing rev.12 found. Search was reading a payload
for **every matching document to return twenty of them**, because `count(*) OVER ()` materialises the
whole result set and the payload sat in the same SELECT list. On the synthetic corpus §7.1 measures
it at 24.2 ms; on the owner's real 227 MB database it takes **4 s** and hits the service's read
deadline, and **5 of 10 ordinary questions came back as `context deadline exceeded`**. The defect
predates the selector — one common token triggers it — and the selector is what made an ordinary
question reach it. Fixed by joining the payload after the LIMIT, and gated by a ratio rather than a
duration. rev.1 to rev.12 below.

**rev.12** · 2026-09-04 — rev.12 is rev.11's selector, built. The implicit AND becomes a caller's
choice: **MCP and the CLI join a query's tokens with OR, the injector keeps the AND**, and gate
**M3 is pinned for the first time** at **claude-code 0.400 and codex 0.600** — 25 of 50 against
rev.11's pre-registered bar of 19 and an oracle ceiling of 37, where the AND returned 1. Two gates
were written and one of them was killed by its own break-it pass before it could be believed. rev.1
to rev.11 below.

**rev.11** · 2026-09-04 — rev.11 corrects rev.10 and settles what it left open. The same 50 queries
were re-asked in English: connectivity goes **16 → 47 of 50** and the oracle ceiling **13 → 37**, so
the class of question rev.10 said verbatim retrieval could not reach **does not exist**, and the
summariser stays closed. What is left is the **selector** — full-query recall is **1 of 50** in
English against that ceiling of 37 — and it is now the only thing between this product and a green
M3. Four decisions follow: language independence is guaranteed on **MCP and the CLI** and explicitly
not on injection; new memory is written in English **for token cost and predictability, not for
search**; M3's fixture becomes the English one; and the model-authored translation is carried as an
`[unverified]` flaw rather than resolved. rev.1 to rev.10 below.

**rev.10** · 2026-09-03 — rev.10 records the first time a person asked this product a question.
Gate **M3** ran against the owner's own 50 queries and returned **0 of 25 on each host**, and the
zero is not about ranking: the queries are Korean and the documents are English, and 34 of 50 share
no word with the answer at any rank. A second wall sits in front of it — a sentence handed to an
implicit AND matches nothing — and the ceiling with that wall removed is 26%. It is the condition
**M-1** named for reopening the summariser, arriving by a route M-1 did not predict, and the gate is
left **unpinned and red** because a floor of zero is a gate that is off. rev.1 to rev.9 below.

**rev.9** · 2026-09-03 — rev.9 records what building **M-4** settled. The decision rev.8 did not
name turned out to be the one that decides what the feature is: a prompt is not a query, and how it
becomes one is written here now. Gates **M5**, **M6**, **M9** and **M10** have their first numbers,
one of them found a defect that no other gate could have — a Go timer is not a clock — and the run
hands over two findings the design did not predict. Backlog **41** is closed by a test. rev.1 to
rev.8 below.

**rev.8** · 2026-09-03 — rev.8 settles what **M-4**'s one-line row never said: which hook, which
hosts, what injection may cost, and what it may not select. It adds gate **M10**, because M5, M6 and
M9 between them measure bytes, abstention and the fence and not one of them measures time — and
injection is the first thing this product does on the user's critical path. It also changes **M3**'s
shape, which a human-labelled fixture forced rather than a preference. rev.1 to rev.7 below.

**rev.7** · 2026-09-03 — rev.7 reverses one of rev.5's decisions on the owner's word: the
false-positive submission is **not** adopted, because the objection is who does it and when it
answers rather than whether it is worth doing. §8's fourth publication condition is unmoved — it is
an outcome with a documentation half — but that half is now the whole of it, which puts a named
requirement on condition 3. rev.1 to rev.6 below.

**rev.6** · 2026-09-03 — rev.6 records what building **M-3** settled: two of its seven fields have
nothing in the corpus to read, its error spans are prose rather than a field, gate **M4** passes
small, and the boost's weight is a measured plateau rather than a taste. rev.1 to rev.5 below.

**rev.5** · 2026-09-03 — rev.5 closes the one thing rev.4's **M-7** deliberately left open, the
delivery channel, and the seven decisions that hung off it: what the artefact is, what the plugin
does and does not carry, what Codex users get, a product version separate from the wire version, a
release process, the signing route, and what `doctor` compares. It also corrects one measured claim
in rev.4's M-7.

**rev.4** · 2026-09-03 — rev.4 adds **M-7**, the update path, and §8's fourth publication
condition, which an antivirus wrote for us mid-session. rev.1 to rev.3 below.

**rev.3** · 2026-09-02 — rev.1 was 2026-08-30; rev.2 and rev.3 are both 2026-09-02, and they are one
day's three states of the same section. **rev.2** read both hosts' memory on the owner's machine,
corrected two clauses of M-2 the reading falsified, answered the nine questions Step 3 could not be
written without, added a fifth MCP tool and named the 1.0 rows that moves. **rev.3** records what
building it then settled: three defects the real corpus found that no synthesised fixture did, and
the one estimate the built parser falsified.

**Scope.** What Engramux does about *memory* once Phase 6 closes, and how installation and diagnosis
change to match. It **supersedes nothing**. `2026-08-27-engramux-1.0-design.md` rev.4 remains the
document of record for everything through 1.0, and its §2 row *"Context re-injection — 1.0 is
pull-only, SessionStart emits nothing"* stays true of 1.0. This document decides what comes after,
and where it contradicts a 1.0 row it says so and names the row.

**Why it exists as its own document.** The owner's goal moved on 2026-08-30 from "a personal
capture-and-search tool" to "personal now, published once the memory feature is native-grade or
better". That is a change of product scope, not of implementation, and rev.4 was not written to hold
it. Every decision below was taken in one session against a four-report research round; the reports
are not in the repository and this document is what survives them.

Same marking rule as rev.4: **[verified]** with a reproduction, **[unverified]** otherwise, and no
unmarked claim. A figure taken from a paper is marked with its identifier so it can be re-read; a
figure a search engine summarised and nobody opened is **[secondary]** and decides nothing.

---

## 1. The finding that set the direction

Both hosts now ship memory of their own, and the market this product was reaching for is gone.

**[verified]** from Claude Code's own documentation: auto memory is **on by default**, writes typed
notes under a per-project memory directory, and loads the first 200 lines or 25 KB of its index at
every session start. The same page states what it deliberately does **not** record — *"anything it
can derive from the codebase, such as architecture, file paths, or debugging fixes"*.

**[verified]** from the `openai/codex` repository's own `memories` README: Codex consolidates
sessions into a memory directory that is itself a git repository, with a raw-memories file,
per-rollout summaries, an index and a consolidated summary artefact.

So the empty seat is not "remember what I like". It is the thing both hosts throw away: **verbatim,
searchable, cross-host history of what actually happened** — the error string, the command, the path,
the fix. That is what Engramux already stores, and the architecture below is built to retrieve it
rather than to replace it with a description of it.

---

## 2. Decisions

| # | Decision | Status |
|---|---|---|
| **M-1** | **No summarisation layer of our own.** Verbatim events stay the record. Nothing derives a natural-language summary, and no LLM is called at any point in this product | Decided |
| **M-2** | **Index both hosts' native memory directories, read-only.** One query covers Claude Code sessions, Codex sessions, and both native memories. Never write to them | Decided |
| **M-3** | **Derive search *fields*, never search *answers*.** Rule-based columns beside the payload — touched paths, commands and exit codes, error spans, tool name, success flag, session, timestamp. The payload is not rewritten (I-10) | Decided |
| **M-4** | **Hook-time injection is built, and ships disabled.** It is turned on per user only after §5's gates pass. This is the row that contradicts rev.4 §2's pull-only decision, and it contradicts it for 1.0-and-after, not for 1.0 | Decided |
| **M-5** | **Installation moves into the Go binary.** `engramux install` replaces `scripts/install-hooks.mjs`, and the Node dependency goes with it | Decided |
| **M-7** | **Replacing an installed build is its own command.** `engramux update` is `install --apply` minus everything that writes host configuration, and Engramux never fetches what it runs | Decided |
| **M-6** | **`doctor` judges by stage.** "Not installed yet" and "installed and broken" become different answers with different next commands; MCP becomes optional rather than required for a green result; the eleven hook entries are checked; and the output is masked by default, with `--full` for the real values | Decided |

### Why there is no summariser (M-1)

Three independent measurements point the same way, and the third is the one that decides.

**Verbatim beats extraction at recalling specific facts.** *Fidelity Before Structure*
(arXiv:2601.00821v3, 2026-06-12) compares verbatim chunks against LLM-extracted typed facts under
one retrieval stack: LoCoMo **43.9% vs 28.0%**, LongMemEval-S **67.4% vs 45.4%**, and on a synthetic
probe for whether a qualifier survives, **91.0% vs 14.0%** exact match. Its stated mechanism is that
extraction fixes relevance before the question is known. Adding artefacts to chunks measured 42.5%
against chunks alone at 43.9% — **the extraction earns nothing back**.

**The bottleneck is retrieval, not representation.** *Diagnosing Retrieval vs. Utilization
Bottlenecks* (arXiv:2603.02473v2, 2026-04-12) decomposes failures across three write strategies and
three retrieval methods: retrieval failure **11–46%**, utilisation failure **4–8%**. Changing the
retrieval method moves accuracy by about **20 points**; changing how memory is written moves it by
**3–8**. Effort spent on the selector is worth several times effort spent on the store.

**A summary you cannot select is worse than no memory at all.** *SWE Context Bench*
(arXiv:2602.08316v1, 2026-02-09) is the only controlled experiment here on a coding agent — Claude
Sonnet 4.5, 399 tasks. On the 99 related tasks: no-memory baseline **26.26%**, oracle-selected
summary **34.34%**, and **agent-retrieved summary 22.22%** — four points *below* having no memory,
at higher cost and longer runtime. The whole value of a summary sits in the selector, so a selector
has to exist and be measured before a summariser is worth discussing.

To which this product adds three constraints of its own: an LLM summariser needs either a sidecar
process, which the Windows thesis forbids, or an API key and an outbound call, which would put the
**entire capture corpus** through an egress that §6 of rev.4 spent a phase auditing.

**When to reopen this.** M-1 is falsifiable, not permanent. If §5's **M8** shows a class of question
that verbatim retrieval cannot reach at all — the shape is "why did we choose X", scattered over
many sessions with no literal span to match — then summarisation is back on the table. Even then the
first move is **M-2**, reading a summary someone else already paid for, not writing one.

### Why the hosts' own memory gets indexed (M-2)

It is the one capability neither host can have: each writes to its own directory and neither reads
the other's. Cross-host single search is therefore structurally ours, and the summarisation cost is
already paid.

The cost accepted is a dependency on two formats with no stability promise. Claude Code's is
publicly documented — location, index file, load limit, frontmatter fields — and its location is
configurable, so it must be **resolved and never hardcoded**. Codex's file names are in
its own repository but the line-level schema of its index was **[unverified]** when this revision was
written — nobody in that session read one. The parser therefore follows the rule rev.4's §4.4 already
imposes on `tool_response`: **preserve a shape you do not recognise, warn, and continue.** A silent
skip is a failure, not a fallback.

Two clauses of that paragraph are amended by the reading below and are marked here so nobody follows
them: **"resolved from settings" was the wrong mechanism** — no setting names a memory path, and what
must be resolved is the configuration home from the environment — and **Codex's line-level schema is
no longer `[unverified]`**, it is read out in full below.

**[verified] 2026-09-02 — both hosts' memory was read on the owner's machine, and the two clauses marked above are
corrected.** Shapes and counts only; these are the owner's private notes and no line
of one is quoted here, in a commit, or in a test. Reproduce by reading the two directories named
below and tabulating; the throwaway scripts that did it are not in the tree, so every count here is
`[verified once, no committed harness]` until M1 and M2 own them.

*Claude Code — the location is not a setting.* The memory of a project lives at
`<configuration home>/projects/<project key>/memory/`, and only the configuration **home** moves: it
is the `CLAUDE_CONFIG_DIR` environment variable, default `~/.claude`. The published settings schema
carries exactly **one** memory property, the boolean `autoMemoryEnabled`, and **no property names a
path** — so "resolved from settings" above is the wrong instruction and the right one is *resolved
from the environment, with the documented default*. The project key is the project path with the
drive colon and every separator folded to `-`, and the schema's own words are that it is *"derived
from the git repository, shared across worktrees"*: of the three keys present, **two decode to a
directory that is a git root and one does not**, so there is a fallback to the working directory and
a parser must not assume a repository. **94 project directories on this machine, 3 with a `memory/`**
— 21 files, 151,102 B, every one `.md`, every one flat with no subdirectory.

*Claude Code — the index and the notes.* Each memory directory holds one `MEMORY.md` and it has
**no frontmatter**. Its entries are markdown list items of the form link-then-em-dash-then-
description; **6, 11 and 1 entries, and 18 of 18 resolve to a file that exists, 0 missing**. One of
the three indexes carries a heading and two do not, and **one index line is a bullet that is not an
entry at all** — no link, no target — so M2's drift case is already in the corpus rather than
hypothetical. The 18 non-index notes each carry a YAML block with the top-level keys `name`,
`description` and `metadata`, 18 of 18. `metadata` is a **nested block and not inline JSON**, with
`node_type` (18 of 18, one value, `memory`), `type` (18 of 18, **four** values — 10 / 5 / 2 / 1 over
the taxonomy), `originSessionId` (18 of 18, UUID shape) and `modified` (**17 of 18**, ISO-8601 with
milliseconds and a trailing `Z`). One note therefore has no `modified`, which is the field §3's P3
compares against, and a parser that requires it fails on 1 of 18 here. Descriptions run 60–668
characters, median 173; note bodies 789–19,408 B, median 4,725. The three indexes are 239 / 2,466 /
18,326 B at 4 / 7 / 13 lines, all under the documented 200-line and 25 KB load limit, so **nothing on
this machine exercises that truncation** and a test for it needs a synthesised index.

*Codex — the directory is the git repository its README describes, and it has one commit.*
`~/.codex/memories`, a fixed location. Branch `main`, **one** commit, **no remote**, working tree
clean against it, 60 tracked paths. The newest file mtime is a week after that commit and the tree is
still clean, so the bytes now on disk are the bytes of the baseline commit and **nothing has been
committed since**; whether the consolidation writes and commits, or writes only, is `[unverified]`
and one live consolidation on a machine with the feature on would settle it. Four artefacts, matching
§1's README reading: an index `MEMORY.md` (39,977 B, 386 lines), a consolidated `memory_summary.md`
(7,217 B, 83 lines), a raw-memories file (291,566 B, 3,181 lines) and **55** per-rollout summaries.
Two subtrees the README-level reading does not name are also there, one holding an instructions file
and one holding a skill.

*Codex — the line-level schema, which was the `[unverified]` this reading exists to close.* None of
the four artefacts has frontmatter; every field is a bare `key: value` line or an inline `key=value`
inside parentheses. The index's file entries are **22** bullets of the form path-then-parenthesised
pairs, with the key set `cwd`, `rollout_path`, `thread_id`, `updated_at` on **21 of 22** and one
carrying a fifth, `head` — a first drift case in a population of 22. **The index references 22 of the
55 rollout summaries; 33 are unreferenced**, so an indexer that walks the index alone reaches 40% of
what is on disk, and reaching the rest means walking the directory. Around those entries the index is
10 first-level headings, 46 second, 32 third, 96 further bullets and 20 `scope:` / `applies_to:`
field lines. The raw-memories file is **55** second-level sections keyed by thread id — one per
rollout summary — under 89 third-level task headings, and its field set is **not uniform**: the full
nine-key set on 41 of 55, 7 sections missing `keywords`, 5 missing `keywords` with two more, and 1
carrying only four keys. The per-rollout summaries are 3,092 / 7,129 / 19,698 B with `cwd`,
`thread_id`, `updated_at` and `rollout_path` on 55 of 55 and **`git_branch` on 52 of 55**. So on
Codex's side a required-field parser fails on 14 of 55 raw sections and 3 of 55 summaries; the M2
requirement to warn and continue is load-bearing on the corpus that exists today, not against a
future format change. Two value formats matter to a parser and are recorded as formats: `updated_at`
is ISO-8601 with a numeric offset and no milliseconds — a **different shape from Claude Code's**
`modified`, which is UTC with milliseconds — and the path fields are absolute Windows paths of which
**39 carry the `\\?\` extended-length prefix against 16 that do not**, in one file, so a path
comparison that does not normalise it will miss 39 of those 55 lines.

*Neither host's memory is live on this machine, and that is the reading's most consequential fact.*
Claude Code's auto memory is **off** — `autoMemoryEnabled` false in the user settings and
`CLAUDE_CODE_DISABLE_AUTO_MEMORY` set to 1 beside it — and its files span 2026-07-18 to 2026-08-04.
Codex's is **off** in three places at once: `memories` false at the top level of its configuration and
a `[memories]` table setting `generate_memories` and `use_memories` false; its files span 2026-08-01
to 2026-08-17 and the 55 summaries share a single mtime to the minute. So both directories are frozen
snapshots, and three things follow that the design has to take rather than assume away. A collection
strategy cannot be chosen by measuring change rate here, because the measured change rate is zero.
M3's fixture is drawn from a Claude Code population of **18 notes over 3 projects**, one of which is
not a repository and none of which is this one — whether 25 queries answerable from that host alone
even exist is now a question the gate has to answer before it is written, and it is `[unverified]`.
And the parser cannot be validated against a live format: it is being written against two snapshots,
which makes M2's warn-and-continue the whole of the defence rather than a courtesy.

*One risk this reading found and did not settle.* Claude Code resolves memory through an internal
storage layer that addresses it by namespace, project key and relative path rather than by file path
directly, and the binary carries a start-up warning about a **v5 storage backend** bound to the
configuration home. On this machine that layer is file-backed and the files above are what it holds.
Whether a future release can put the same namespace behind a non-file backend is **[unverified]**;
what would settle it is a release whose memory directory is empty while its memory works. Until then
the file walk is correct and it is not guaranteed to stay correct, which is one more reason the
warn-and-continue rule is a gate and not a style.

### Step 3's open questions, answered (M-2)

**Decided 2026-09-02**, against the reading above and not against a preference. Nine decisions; the
five the plan named, plus four the reading forced into the open. Each says what it costs, because a
decision whose cost is not written is one nobody can reopen.

**1 — Step 3 proceeds with both hosts' memory switched off, and is not blocked by that.** The
question was put the other way first and the framing was wrong: none of M1, M2 or M3 requires a live
format. M1 is over *"every native memory file present on the machine"* and 79 files are present. M2's
inputs are synthesised by the test, and the snapshot turns out to carry five real drift shapes
already — an index bullet with no target, an entry with an extra key, 14 of 55 raw sections missing a
field, 3 of 55 summaries without a branch field, 1 of 18 notes without the timestamp §3's P3 compares
against — which is better material than a live directory that happens to be well-formed. M3 needs a
population and not a stream, and the gate's own wording already reports *"against each native
memory's own ceiling"*, so a small ceiling is a number it prints rather than a failure. What is given
up is exactly two things and both are small: whether Codex commits each consolidation stays
**[unverified]** — one commit against a clean tree cannot separate write-and-commit from write-only,
and a single live consolidation would settle it — and a format change in a future host release
produces no new file here, so it is invisible on this machine. The second is what M2 exists for at
runtime rather than in a gate, which is the point of that gate's shape. The owner's own configuration
is left alone deliberately: it was set off twice on one host and three times on the other, and
Claude Code's is on by default for everyone else, which is the machine this feature ships for.

**2 — One memory item is one block the host's own format delimits, and one whole file where it does
not.** Codex's thread sections, its per-rollout summaries and its index entries; Claude Code's notes.
Where the delimiter is not recognised the file survives as a single document, which is M2's
warn-and-continue applied to the unit rather than only to the fields. This section said "about
150–200 documents" when it was written and the built parser says **303**, over 81 files — 38 Claude
Code and 265 Codex, 127 carrying a host timestamp and 240 a project — measured by
`TestGateM1EveryNativeMemoryFileParsesAndKeepsItsText`, which logs the figures on its passing path.
The estimate was low because it counted a Codex artefact's sections and not its own leading block.
303 against 17,043 events. The alternative that was rejected is file granularity, and one measurement rejects it:
Codex's raw-memories file is 291,566 B where the median document either side of it is about 5 KB, so
file granularity makes the bulk of Codex's content one document, and an excerpt cut from it answers
nothing.

**3 — Collection is the drain's ticker with a modification-time and size short-circuit.** Re-stat the
directories on the interval and re-read only what changed. It reuses a mechanism that is already in
the service, adds no handle and no thread, and therefore leaves the Phase 6 soak's baseline —
handles 198 → 240, threads 15–18, working set 61–75 MB — directly comparable to the next series,
which is the instrument that would see this decision go wrong. A scan at service start was rejected
because the service runs for days and memory written mid-session would not be searchable until a
restart. A file watcher was rejected on a measurement: Claude Code's memory directory is a
*subdirectory of the transcript directory*, whose siblings are 36, 104 and 4 transcript files and
whose parent holds 3,823, so a recursive watch fires on every session write this product's hooks are
already capturing, and a per-directory watch needs its watch set rebuilt every time a project appears.

**4 — M3's fixture is the owner's, lives outside the repository, and the gate skips when it is
absent.** The same shape `.capture/` and `TestPhase4Gate`'s corpus mode already have. Nothing is
promoted, nothing is redacted, and nothing leaks; what is given up is that the gate does not run on
anyone else's machine and its result is an observation of one. Whether 25 queries per host is
reachable against a Claude Code population of 18 notes is **deferred to the fixture's construction**
rather than decided here, because the gate already reports against the population's own ceiling.

**5 — `memory_items` keeps its name and loses its schema.** Migration `00004` drops it and creates it
again with the columns the reading says are needed: host; kind; the source file path; the entry key
within that file; a title; the body; the host's own timestamp, separate from ours; a project path;
`privacy_class` and `redaction_version`; and when we indexed it. Dropping is safe and checked rather
than assumed — no shipped code writes that table, and its only references are five lines of
`internal/store/migrate_test.go`. The present schema does not fit for two reasons that are not
matters of taste. It has no `host` column, so `UNIQUE (project_id, key)` makes the two hosts collide
on any key they share. And `project_id` is `NOT NULL` with a foreign key into `projects`, which
cannot express a Claude Code memory whose git root this database has no row for — the three that
exist include no project this repository is one of — or a Codex entry whose `cwd` names a directory
no event ever came from.

**6 — Native memory gets its own external-content FTS table, and its ranked list stays its own.**
The tokenizer clause is taken from the migration's own `CREATE` statement rather than written twice,
which is the discipline `TestEveryCandidateDocumentIsReachable` already uses. `events_fts` is not
touched, so the cost on an existing installation is a small index over about 950 KB rather than a
rebuild — and the rebuild is what is being avoided: `00002` cost 1.30 s and doubled the file at 8,177
events and 40 MB, and this database is now 17,043 events and 182 MB. The rejected alternative was one
FTS over a view unioning both — FTS5 does accept a view as `content=` — and it was rejected because
it pays that rebuild on every installation and couples two tables through one rowid space, which the
`00002` header already warns must never be renumbered. The two lists are **not merged**: bm25 is not
comparable across indexes whose document frequencies come from populations of about 200 and 17,043,
and a normalisation rule invented to merge them would be an unmeasured input to M3's own recall
number.

**7 — Step 3 and Step 4 do not share an FTS rebuild, and are two migrations.** The plan asked this
to be settled before either migration is written. M-3's own wording is that the derived fields are
*"rule-based columns beside the payload"* — filters and a ranking input, not indexed text — and M4
measures them with the boost on and off, which is a scoring change rather than an index change. So
the batching argument the `00002` row exists to make does not apply. If Step 4 turns out to need
indexed text after all, it earns its own rebuild then, on its own evidence.

**8 — A memory item is scoped by the path the host wrote, not by a foreign key.** The row carries the
project path — Claude Code's memory directory resolves to one, Codex's entries each carry their own
`cwd` — and a project-scoped request compares that path against the requested project's root. The
foreign key is filled when a `projects` row happens to exist and is otherwise empty; it is a
convenience and never the scoping mechanism. Keying on the foreign key alone was rejected because it
makes every unmatched memory unreachable through MCP, and creating a `projects` row per unmatched
path was rejected because it fills the project list and `list_sessions` with directories no event
ever came from.

**9 — One `search` call returns both lists, and `get_memory` is `get_event`'s equivalent.** P4 is
defined as *"one query reaches answers that exist only in the other host's sessions or memory"*, and
a separate search tool breaks that literally: the model has to know to make a second call, and a
model that does not is the agent-retrieved regression SWE Context Bench measured at four points below
no memory at all. So `SearchReply` gains a second array rather than the surface gaining a second
search. `get_memory` is added rather than `get_event` being taught a second kind of id, because a
tool whose name does not describe what it returns is the defect, and because §8's Phase 5 gate on
`get_event` checking the project with the id is written about events. Its reply bound is **measured**
before it ships and recorded in the 1.0 spec §7.1, on the same rule `get_event`'s bound was: the
bodies it will carry are 789–19,408 B for Claude Code notes, median 4,725, and 3,092–19,698 B for
Codex's per-rollout summaries.

**Forced by an existing invariant rather than chosen here, and recorded so no review reads them as
open.** A memory hit's source path is a user path, so it is masked on the MCP surface — `§8`'s Phase 5
egress clause sweeps a marshalled reply with the detector rather than naming fields, so nothing had
to be added for it to be caught, and it would be caught. The CLI prints masked by default with the
real values behind `--full`, which is M-6's rule and not a second decision. And the index is over the
original text, never a masked form, for the reason §5.7 gives: masking happens at egress, and an
external-content index of masked text disagrees with the table `rebuild` reads.

**What this contradicts in `2026-08-27-engramux-1.0-design.md` rev.4, named rather than left for a
reader to find.** §5.9's *"the four tools"*, §8's Phase 5 row and §8's Phase 6 row, which counts *"the
four MCP tool results"*, and §10's closed question 3. There are **five** tools after Step 3, and the
Phase 6 audit's sweep is over five results and five errors. Nothing else in those rows moves: each new
surface is swept by the same detector, in both modes, against the same definition of an egress.

### What building it settled (M-2)

**[verified] 2026-09-02, on `step-3-native-memory`.** The nine decisions above were taken against a
reading; this is what running the code corrected and what it added. Every figure here comes out of a
committed test that logs it, not out of a probe.

*The corpus is bigger than the estimate, and the estimate is corrected above.* 303 items over 81
files, 38 Claude Code and 265 Codex, 127 with a host timestamp and 240 with a project — so **63 of
303 belong to no project this database has a row for**, which is what decision 8's path scoping and
`get_memory`'s optional project are for. That was 148 with a project until the first live install
showed why: a Codex rollout summary writes its `cwd` once in the file's header and then a heading, so
every section below it read as belonging to nowhere — `project ""` on an item whose own file named one
two lines above. A section inherits its file's `cwd` now and 92 more items are reachable through a
scoped call. Only the path is inherited; inheriting a timestamp would date a section by its neighbour. The largest body is **20,156 B** and the largest masked body
is also 20,156 B, nothing in that item having matched a rule; `ipc.MaxMemoryBodyBytes` is 128 KiB,
6.6× the largest measured, on the ratio `MaxEventPayloadBytes` was set at.

*Three defects the real corpus found that no synthesised fixture did, all three now owned by a test.*

1. **A heading is not unique within a file.** Migration `00004` makes `(host, source_path, entry_key)`
   unique and a Codex artefact here repeats a heading, so the collector failed that constraint on the
   first pass over the machine's own memory. Only a repeat is perturbed, because an id is derived
   from the key and a caller holds one across a tick.
2. **The parser was destroying the context a credential rule matches on.** A line reading
   `secret: <value>` had its label stripped, and §6.1's credential rule matches on exactly that word —
   so the mask found nothing and the body reached a reply bare, *detected clean*, because the shape
   the detector matches on was the part the parser removed. Same for `password`, `passwd`, `token`
   and `api_key`. The label set is closed now: the thirteen field names the reading measured, and
   anything else is kept verbatim, which is M2's rule one layer down.
3. **A URL scheme parsed as a field label.** `postgres://user:pw@host` became the label `postgres`
   and the value `//user:pw@host`, which is the connection-string rule's own prefix removed. The
   closed label set fixes this one too.

Two and three were found by the **Phase 6 redaction audit widened to five surfaces**, which is the
change that pays for itself in this build: the audit's literal-needle half caught both where its
detector half reported clean, which is the exact division that clause was written for. The audit's
own fixture was wrong first and the audit found that too — it wrote the event payload's *bytes* as a
memory body, and a payload's newlines are escaped, so the dotenv rule's multiline anchor never fired.
A memory body is markdown; the fixture now carries the samples as text, under the same premise the
payload half has.

*The host timestamp does not enter the indexed text, on either host.* It is written on every document,
so its parts — a year, a month, an offset — would be tokens of all of them, which is the defect §5.7
measured `cwd` at 900 of 901 documents. **M1 is what found that the two sides disagreed about it**:
Claude Code's `modified` was going to the column and Codex's `updated_at` was going to both. A
time-qualified query gets the column, which is §3's P3.

*M3 was verified once and then left to skip.* Against a fixture generated from this machine's corpus —
which is not the human-labelled one M3 asks for, and was deleted — **claude-code 1 of 1 over 38 items
and codex 11 of 11 over 265**. Replacing `ORDER BY rank` with an id order took codex to 10 of 11 and
failed the gate, so the gate is not vacuous. What remains is the fixture, and it is the owner's.

*Decision 9 has a cost the first live upgrade showed, and it is priced rather than fixed.* The SDK
derives an output schema for `search` from `ipc.SearchReply` and a client caches it at connect, so a
**session that was already open across the upgrade rejects the reply** the moment it carries memory
hits — *"Structured content does not match the tool's output schema: data must NOT have additional
properties"*. Observed at the terminal, from a real Claude Code session: the same call against a
project with no native memory succeeded, because `omitempty` left both new fields out and the reply
still matched the old schema exactly. The service logged nothing; it is the client's validation and
not the server's. A reconnect fixes it, which is what "one build is one compatibility event" already
means. The alternative is `get_event`'s: an `any` output type produces no schema and nothing to
validate, so a future field could never do this — and it costs the model the shape of the reply, which
`get_event` gave up under duress rather than by choice. **Kept typed**, and the next revision that
grows this document should know it is choosing again.

*M2 fires in production, and the three shapes are the ones the reading predicted.* The live service's
first pass logged two unknown `.md` names — the two subtrees §1's README reading does not name — and
one index bullet with no link. Warned, and all three still indexed.

*One test was fake and a break-it pass is what said so.* The memory hit's masking test searched for a
literal user name the *body* carried, and the source path a test writes to is under the machine's own
temporary directory — so it carries the **real** user name and the assertion never reached the field
it was named for. It sweeps the marshalled hit with the detector now, which is what §8's Phase 5
clause does and for this reason.

### What gate M3 measured on its first human fixture (M-2)

**Measured 2026-09-03**, the first time this product has been asked a natural-language question by a
person. The owner wrote **50 queries, 25 per host**, from memory rather than from the answers beside
them, against `.capture/m3/candidates.tsv`'s verified answer column. Every line is well formed and
every answer still verifies — the gate checks both — so what follows is about retrieval and not about
the fixture.

**Recall@10 is 0 of 25 on each host.** That number is not a statement about ranking, and pinning it
would have turned the gate off while leaving it green, which is why the gate now refuses to advise a
pin at zero.

**There are two walls, and both are load-bearing.**

**The first is the implicit AND.** §5.7's query builder turns a query into one quoted prefix phrase
per token joined by an implicit AND, which is exactly right for a known-item literal and wrong for a
sentence. The fixture's queries are **4 to 9 tokens, median 7**, and **2 of 50** return a single hit.
Reducing to the three longest tokens — the reduction M-4's injector already makes — takes that to
**7 of 50**, and crudely stripping a trailing Korean particle from each takes it to **15 of 50**.

**The second is language, and it is the larger one.** All 50 queries are **100% Hangul** by letter.
The documents they ask about are not: the median target item's body is **0% Hangul**, and **37 of 50**
are under 20%. Asking token by token whether any word of a query reaches its answer document *at any
rank* — the token itself, a Latin stem cut before an attached particle, and one or two syllables
trimmed — **34 of 50 connect to nothing at all**. The cross-tabulation is close to a clean split:
of 37 mostly-Latin targets **4** connect, and of 13 mostly-Korean targets **12** do.

**The ceiling, with the first wall removed entirely.** Handing the search exactly the tokens that do
connect — an oracle selector no implementation can have — puts the answer in the top 10 for **13 of
50**. So **26% is the ceiling for any lexical selector over this corpus and these queries**, and the
remaining 74% is not reachable by choosing better words from the question.

*Measured through a throwaway probe in `internal/search`, deleted with the run — the same standing
the 1.0 spec §7.1's `00002` migration-cost row has. What survives in the tree is the gate's own
same-script and cross-script split, which is the half that has to be visible on every run: measured
on the same fixture, **0 of 8** and **0 of 42**. The first wall is why the same-script arm is also
zero.*

#### What this decides, and what it does not

**It does not falsify the ranking, the tokenizer or the derived-field boost.** Gate M4 measured the
boost over literal known-item classes and it still holds; the Phase 4 gate's five classes still pass.
Those measure a literal a person pastes back. This measures a question a person asks, and the two are
different instruments.

**It narrows P4 as written.** *"One query reaches answers that exist only in the other host's
sessions or memory"* is true when the query and the document share a language and false when they do
not, and on this machine they usually do not — the owner asks in Korean and both hosts write their
notes in English. P4's claim is unchanged for a literal; it is `[unverified]` and currently measured
at zero for a question.

**It is *not* the condition M-1 named for reopening the summariser. Corrected 2026-09-04 — rev.10
wrote this and rev.10 was wrong.** M-1 says to reopen *"if §5's M8 shows a class of question that
verbatim retrieval cannot reach at all"*, and rev.10 read the zero above as that condition arriving
by a route M-1 did not predict. The English arm in the next section falsifies it: asked in the
language the corpus is written in, **47 of the same 50 questions reach their answer** at some rank.
There is no class of question verbatim retrieval cannot reach here. There is a query in one language
against an index in another, and behind that a selector that discards most of what does connect —
neither of which a summariser fixes, because a summariser writing in the language the source already
uses changes nothing about either. **The summariser stays closed.** M-1's own next move — **M-2
first, read a summary someone else already paid for** — was already made and still holds: these 303
items *are* both hosts' own summaries.

**Four options were named here and the next section chooses among them.** Translating the query,
translating the index, a multilingual embedding beside the FTS index, or narrowing P4 to same-language
retrieval and saying so. §7 rejected a vector index on the `CGO_ENABLED=0` boundary and noted that
`modernc.org/sqlite` v1.57.0 vendors sqlite-vec CGO-free, so half of that objection has already
lapsed; what has not is that an embedding needs inference, which is the sidecar or the API key §2
rejected. **The next section takes the fourth and none of the other three**, and what made that cheap
is that the wall is smaller than rev.10 measured it to be.

### What the English arm settled, and the four decisions it forced (M-2)

**Measured 2026-09-04.** rev.10 left the wall unmeasured in one direction. It knew the queries were
Korean and the documents were not; it did not know what those same questions would do asked in the
language the corpus is written in. The owner's 50 queries were translated to English — column 2
only, the answer column copied and never read — and re-run against the same corpus.

| Same 50 questions, same corpus | Korean | English |
|---|---|---|
| Connectivity — any word of the query reaches the answer at any rank | 16 of 50 | **47 of 50** |
| Oracle selector recall@10 — handed exactly the tokens that connect | 13 of 50 | **37 of 50** |
| Full-query recall@10 — what a person actually gets | 0 of 50 | **1 of 50** |
| Longest-three recall@10 — the reduction M-4's injector makes | 0 of 50 | **2 of 50** |

**Language was nearly all of the first row and none of the last.** The corpus holds the answers and
Korean words could not touch them: 16 → 47. But an English question does not reach them either. **The
selector discards 35 of the 37 answers that removing the language wall makes reachable.** That is the
same defect M-4 found in the injector — a three-term AND is too narrow — and this is the first time
it has a size. **The selector is the keystone: M3 cannot go green in any language until it moves.**

**The gate's own reading agrees, and adds one thing the probe does not.** Run over the English
fixture on 2026-09-04: **claude-code 1 of 25, codex 0 of 25** — the same 1 of 50 — and the script
split inverts, **same-script 1 of 42 against the Korean fixture's 8, cross-script 0 of 8 against its
42**. So **8 of the 50 targets are themselves Korean documents**: translating the query moved 42
lines out of the language wall and moved 8 into it. The wall is a property of the pair, not of the
query, and no single-language convention removes all of it.

*Measured through the same throwaway probe as the section above, deleted with the run, and
re-run first-hand on 2026-09-04 before being written here. What survives in the tree is the gate.*

#### Storage is already English, which reverses the obvious remedy

**Measured 2026-09-04** over the same 303 native items:

| | mostly Korean (≥50% Hangul) | mixed | mostly Latin (<20%) | median Hangul share |
|---|---|---|---|---|
| claude-code, 38 items | 11 | 6 | **21** | 1% |
| codex, 265 items | 9 | 52 | **204** | 0% |

**74% of native memory is already English.** So "write the memory in English" moves 7% of the corpus,
and that 7% is the only part currently working — **12 of the 13 mostly-Korean targets connect and 4
of the 37 mostly-Latin ones do**. Unifying storage on English deletes what works and leaves what
fails untouched. **The retrieval gap is entirely on the query side.**

#### The four decisions

**1 — "Ask in any language" is guaranteed on MCP and on the CLI, and explicitly not on injection.**
The three surfaces differ by whether a translator is already present. **MCP**'s caller is a model, so
one line of tool description — this corpus is mostly English, search in English — buys the
translation for nothing, and it is the best value per line available in this product today. **The
CLI**'s caller is a person and a README line is all there is. **Injection** has neither: it sees a raw
prompt at hook time with no translator, and giving it one opens **M-1** (no LLM) and **§2** (no
sidecar, no API key) for a feature that ships **off** and whose activation gate **M7 has not run**.
That is out of proportion. A Korean prompt receiving zero bytes from the injector is **P2 working**,
and it is documented as such rather than repaired.

One blocker that follows from the implicit AND and belongs to the selector: **a bilingual query is
not expressible today.** One Korean term added to an English query takes the result to zero, because
the terms are ANDed. "Search both languages" needs OR — more evidence for the keystone.

**2 — New memory is written in English, going forward only, and not for search.** The two reasons
that survive the measurement above are **token cost** — M5 already converts Korean at a conservative
2 bytes per token — and **predictability**, a rule that is true beating one that is true 74% of the
time. *"For better retrieval"* **is not one of them and must not be recorded as one**: during the
transition Korean queries get worse, because the 7% that works is what shrinks.

**The 20 existing Korean items are not rewritten.** They are the hosts' own data — the Codex memory
directory is itself a git repository — which is precisely why **M-2 is read-only**. The hosts' own
consolidation replaces them in time, and the recall gained by hand-editing 20 items is smaller than
the risk of writing into a host's store.

**This is not a product change and no agent performs it.** Engramux does not write native memory
(M-2), and `AGENTS.md` forbids an agent editing `~/.claude` or `~/.codex`. It is a line in the
owner's own `CLAUDE.md` / `AGENTS.md`, by the owner's own hand.

**3 — M3's fixture is the English one; the Korean original is kept as evidence, not as a gate arm.**
M3's definition is *cross-host* and language is not in it, so the Korean fixture mixes two failures
into one number. Under decision 1 the documented usage is English, and a gate should measure the
documented usage. With decision 1 taken, a Korean arm is **permanently zero** — and a permanent zero
is either a gate that is red forever or a pin that asserts nothing. The ceiling moves with the
fixture, **13 → 37**, which is what makes a selector target meaningful.

**4 — The translation is the model's, and that is recorded as a flaw rather than resolved.** The
assistant translated the queries, from column 2 alone, with the answer column never read — so M3's
circularity guard (a query written from memory, not cut from the answer beside it) survives. What
does not survive untouched is **register**: a model translating into the same register the documents
are written in may have made **47 of 50 optimistic**. **`[unverified]`.** Accepted because the gate's
purpose is regression detection, which a slightly optimistic absolute still serves, and because the
alternative spends the owner's time before the selector work can start.

#### What this leaves open

The selector, and it is now the only thing between this product and a green M3. **Nothing here has
been implemented** — decisions 1 and 2 are documentation the selector work will make true, and
decision 3 is the fixture the selector work will be measured against.

*Closed the same day by the next section.*

### What building the selector settled (M-2)

**[verified] 2026-09-04, on `step-7-selector`.** The section above named the selector as the keystone
and pre-registered **19 of 50** as the bar. This is what replacing it cost and bought.

#### The join is a caller's choice, and the injector does not get the new one

The implicit AND is right for what this index was built for — a literal somebody pastes back, where
every token is in the document by construction — and wrong for a question. So it stops being a
property of the builder and becomes a parameter: **MCP and the CLI ask with OR, the injector keeps
the AND**, and each call site says which.

**A two-phase shape was measured and rejected.** *"AND, and OR only when the AND came back empty"*
is the smallest change and it is worse: **20 of 50** against the plain OR's 25. Over the 8 questions
where the AND does match something, it finds **1** answer and the OR finds **6** — so on this fixture
the AND was not protecting the queries it answered either, and preserving today's behaviour where it
works costs 5 of 25 rather than nothing.

**The injector's exclusion is structural, not caution.** Its abstention is a threshold on the size of
the match set, which is what M6's zero-byte claim rests on; an OR there matches most of the corpus
and moves M5, M6 and M10 in one step, for a feature that ships off and whose gate M7 has not run.

#### Gate M3, pinned

**claude-code 0.400 (10 of 25) and codex 0.600 (15 of 25)**, 25 of 50, over 38 and 265 items and the
English fixture. Same-script **24 of 42** and cross-script **1 of 8** against the AND's 1 and 0. The
bar was pre-registered at 19 before the number was known, which is the only order in which a bar
means anything.

The pin is a floor over a corpus that grows, so a future fall is either a retrieval regression or a
corpus that gained distractors, and no constant can tell those apart. That was the cost the pinned
shape was chosen with.

#### The gate that was written first and deleted, which is the part worth keeping

rev.11 decided to pin the Phase 4 ranks before touching the selector, so that a precision loss could
not hide behind a recall figure that stays at 100%. The instrument chosen was the **fixtures-mode
rank ceiling** — every answer at rank 1, which is what the five in-repository documents return. It
was green, and it was **fake**.

The break-it pass is what said so. Reversing the entire `ORDER BY` left every class green: in that
mode a derived query matches **exactly one document**, and a set of one has no order to get wrong.
What survives a total inversion of the ranking is not measuring the ranking.

**The corpus cannot be pinned either**, and that is a separate finding. `.capture/` grows whenever
the owner uses the machine, and the upper medians had already moved from §7.1's recorded
**3 / 3 / 10 / 9 / 30** to **4 / 4 / 6 / 8 / 30** with no change to the package between. An absolute
rank is not a stable assertion here in either mode.

What replaced it is **gate M4's shape**: both arms of the same run over one corpus, which is immune
to corpus drift by construction. Recall is gated and MRR is reported — the two arms do not rank the
same population, so a lower MRR at equal recall is the shape of the trade rather than a regression.

**Measured over the 901 captures, the OR costs nothing on the literals.** Every one of spec §8's five
classes returns identical recall@10 and identical MRR under both modes. The two-token class is the
only one that can tell them apart — the other four derive a single-token query, where the two
expressions are byte-identical — and there the match set widens **77 → 291** with recall@10 at 0.640
and MRR at 0.269 under both. bm25 puts the documents matching both terms above the documents matching
one, which is exactly the property the AND was being kept for. **The gate fails when no class
distinguishes the two modes**, because a run that compared a mode against itself priced nothing.

#### What the OR costs in time

**Measured 2026-09-04**, 19,503 synthetic events, limit 20, one query of two tokens chosen to be the
worst case — one term in 1 document in 100, the other in all of them: **MatchAll 3.30 ms over a match
set of 196, MatchAny 65.2 ms over 19,503**. About **20×**, and it is the match set that decides it,
which §7.1 already measured from the other direction. Acceptable on an interactive surface and the
reason the injector — the one path with a 500 ms deadline — keeps the AND.

**And that measurement was wrong about the real machine by two orders of magnitude**, which the next
section is about. It was not wrong about the ratio; it was taken over a corpus whose documents are
tiny, and what it could not see is that the cost scaled with the size of documents the query never
returns.

### What installing it found (M-2)

**[verified] 2026-09-04, on `step-8-window-cost`.** Step 7 was installed and its main surface half
stopped answering. **5 of 10** ordinary questions came back as `context deadline exceeded` against
the service's 4 s read deadline, and the successes took **1.3 s to 3.5 s**.

**The selector was not the cause.** A *single* token was enough — `bash` and `the` both timed out,
`engramux` took 3.6 s over 15,397 matches, and `deadline` took 424 ms over 838. A one-token query
builds a byte-identical expression under both modes, so **the defect predates the selector entirely**.
What the selector changed is how often an ordinary question reaches it: under the implicit AND a
sentence matched almost nothing, which is what made the search useless and is the whole reason
`MatchAny` exists.

**The cause is a window function sharing a SELECT list with a large column.** `count(*) OVER ()` is
computed over the whole result set, so SQLite materialises every matching row before returning the
first — and `events.payload` was in that list. A query matching 15,000 documents read 15,000 payloads
to return 20.

**Measured over two corpora identical but for payload size**, 20,000 events, one term in all of them,
limit 20: **64 B documents 45.2 ms, 8,192 B documents 1.178 s — a ratio of 22.26**. With the payload
joined after the LIMIT instead: **45.2 ms and 91.8 ms, a ratio of 2.03**. Some scaling is real and is
not the defect — the twenty rows that *are* returned get masked and excerpted — which is why the gate
is a ceiling of 3 rather than 1.

**Why nothing caught it, and what that says about the synthetic corpus.** §7.1 measured this exact
shape at **24.2 ms** over 19,503 synthetic events. The number is correct and the corpus is not the
machine's: its documents are a few dozen bytes where the real ones average about 11.5 KB. **A
performance measurement over a corpus that does not resemble the real one can be right and useless at
the same time**, and the gate written here is a ratio rather than a duration for that reason — two
corpora in one run, where the machine, the cache and the load cancel.

**Verified against the machine that failed.** The same ten questions, re-run on the installed build
after the fix, plus the two single tokens that had timed out: **0 of 12 exceeded the deadline**,
where 5 of 10 had. `bash` and `the` answer in **512 ms and 598 ms** against a timeout, and `claude
code hook event` — which matches 20,010 documents, essentially the whole corpus — answers in
**1.8 s**. The slowest of the twelve is 2.9 s and it is the first of the run.

### Why derived fields are not a summary (M-3)

The distinction is load-bearing and easy to lose. A derived field exists **to find a document**; a
summary exists **to answer instead of one**. The moment a derived value is what the reader is given
rather than what the reader is given a route to, M-3 has become M-1's rejected option. The evidence
for the boundary is the same 42.5%-versus-43.9% result above.

### What building it settled (M-3)

**[verified] 2026-09-03, on `step-4-derived-fields`.** M-3's field list was written against what a
capture ought to carry. This is what it does carry, and two of the seven are not there at all.

*Three of M-3's seven fields are already columns, two are unreachable, and the rest is three.* Tool
name, session and timestamp have been columns of `events` since `00001`, so M-3 adds nothing for
them. Against the 902 captures, **`tool_input.command` is present on 534, `tool_input.file_path` on
120 and `tool_response.filePath` on 54, and a non-empty `tool_response.stdout` on 220.** Against
that: **`tool_response.stderr` is present on 241 documents and non-empty on none of them,
`success` appears on 3, and exactly one key in the whole corpus matches
/exit|return.?code|errno/.** So M-3's *exit codes* and *success flag* have nothing to read and are
not built — recorded here rather than silently dropped, because a field nobody can fill is a
different thing from a field nobody wrote yet. And M-3's *error spans* are not a field either:
**227 documents carry error-shaped text and 62 of those carry it in `stdout`**, in prose. The three
columns are therefore a command line, a touched path, and what a tool answered.

*P1's four literals and M4's "three classes" are reconciled by the same measurement.* §3 names an
error message, a stack frame, a command line and a path; M4 says three. With no structured error
field in the corpus, an error message and a stack frame are one class here — both live in what a
tool answered — and the three classes are the three columns. `TestPhase4GateM4DerivedFieldsEarnTheirPlace`
carries that reasoning at its own head, and it is not the same as §8's Phase 4 class *a path
basename*: that one asks whether the tokenizer reaches a basename in any string leaf, this one asks
whether the ranking prefers the document that actually touched the file. 174 candidates against 900.

*M4 passes, and the honest reading of the pass is that it is small.* Measured over the corpus at the
weight below, boost off then on:

| Class | Candidates | recall@10 | MRR |
|---|---|---|---|
| a command line | 534 | 0.680 → 0.680 | 0.242 → 0.262 |
| a touched path | 120 | 0.480 → **0.520** | 0.134 → 0.170 |
| an error message | 96 | 0.760 → 0.760 | 0.555 → 0.613 |

Three of three classes improved and none regressed, which is what M4 asks. What the table also says
is that the boost **reorders the top ten and rarely reaches into it**: one class gained one document
at k, and the other two moved only in MRR. That is a real effect and a modest one, and it is written
here as the number rather than as "the gate passed" so that a later revision considering whether to
keep this code is arguing with a figure.

*The weight is a measured boundary and the sweep found two regimes rather than a continuum.* The
gate was run at 1, 2, 3, 4, 5, 20 and 100. Below 5 the boost only reorders inside the top ten — the
error class's MRR climbs 0.580, 0.587, 0.607, 0.613 — and recall@10 does not move in any class. At
**5** the touched-path class reaches 0.520, the only recall movement in the sweep. At **20 and at
100 every one of the six figures is identical to 5**: the boost dominates bm25 within the matched
set, the order becomes field matches first and bm25 among the rest, and there is nothing further for
a larger number to buy. 5 is therefore the smallest weight that reaches the plateau, which is where
this stands — a larger one changes no answer, and a smaller one leaves bm25 more say exactly where
the derived match is the weaker signal.

*The boost reorders and never filters, and that is asserted rather than intended.* Every token's
test is inside the `ORDER BY` and none of it is in the `WHERE`. A boost written into the filter
would pass every ordering assertion and quietly turn a ranking input into a feature nobody asked
for, so `TestTheDerivedBoostChangesNoResultSet` compares the sorted result sets of the two arms over
six queries — and a break-it pass that moved one predicate into the `WHERE` is what showed it fails
when it should.

*Migration `00005` costs no rebuild, and keeping it that way took one line nobody would have
missed.* Decision 7 settled that Step 3 and Step 4 are two migrations because these columns are a
ranking input rather than indexed text. But `events_fts` is external content with an update trigger,
so an `UPDATE` touching only the three new columns still fires it — deleting and reinserting every
row's `leaves` for no change at all, which is the rebuild this migration is defined by not doing
arriving through the back door. The trigger is dropped around the backfill and recreated after it,
and `TestTheDerivedBackfillLeavesTheFTSIndexAlone` asserts three things rather than one: that
`integrity-check` still passes, that the trigger is back, and that it still works — a trigger
recreated with the wrong body passes a count and fails an update probe.

*One divergence between the two walks existed only because the guard was looked for.* `Derive` and
the backfill answer the same question in Go and in SQL, and the shape that separates them is not a
rule but a limit: **SQLite stops at 1000 open containers where Go stops at 10000**, and the
backfill's `CASE` guards on `json_valid`. A payload carrying a shallow `tool_input.command` beside a
deeply nested sibling would therefore be derived on one side and not the other, for one row, with
nothing saying so. `sqliteWillParse` is what closes it and both sides of that limit are cases.
`TestTheTwoDerivedWalksAgree` compares **947 rows over three columns** — 4 fixtures, 22 derived
shapes, 19 validity shapes and 901 corpus captures — of which **675 derive something on both
sides**; the non-empty count is asserted too, because a backfill that wrote the empty string
everywhere would agree with a Go walk that also did and the comparison would pass having compared
nothing.

*A third walk exists because the first two cannot see the failure with the longest fuse.* A perfect
backfill beside an insert that never binds the columns gives a database whose old events rank and
whose new ones do not, and nothing reports it — a ranking input has no integrity check, and a boost
that stopped applying looks exactly like a boost that never helped.
`TestIngestWritesTheDerivedColumns` compares what `Ingest` stored against what `Derive` answers for
the same bytes.

*The boost has a runtime cost, the Phase 5 contention gate is what priced it, and the first form was
too expensive to ship.* `ORDER BY rank` makes FTS5 score every matching row before the first one is
returned, so anything in that clause runs per matching row per query token. The boost was first
written as `instr(lower(col), ?)`, and `lower()` **copies the column** before the comparison can read
it — over `derived_output`, which holds whole tool outputs. §8's Phase 5 contention clause measured
it: 20 ingests against 96 readers over 4,000 documents, slowest ingest **832, 845 and 928 ms** against
§5.3's 800 ms, none of three under. Rewritten as `LIKE ... ESCAPE`, which compares in place and whose
`OR` short-circuits, the same gate gives **375, 378, 481, 493 and 502 ms**, five of five. LIKE is
case-insensitive for ASCII by default and nothing sets `case_sensitive_like`, so it is the same
comparison minus the copy — and the escaping that was the reason to reject it is four lines, asserted
through queries somebody would type rather than through the pattern builder.

*The same run found that the gate was already marginal, which is not this step's to fix.* Measured at
the commit before any of Step 4: five runs of that gate alone gave 692, 784, 852, 777 and 751 ms —
**one of five over the budget**, and the other four inside it by less than 50 ms. So a red contention
gate is not evidence of a regression until an arm has been measured against a baseline, and a green
`scripts/race.sh` on that machine is roughly a four-in-five event. Backlog **38** carries it, names
the three readings that could explain the margin, and says that moving the number is not one of the
ways to settle it.

*The derived columns are never selected and never leave the machine.* They hold copies of payload
text, unmasked, which adds no exposure the database does not already have under I-10 — but it would
add an egress if anything read them out. Nothing does: they appear in the `ORDER BY` and in no
select list, and §8's Phase 5 clause sweeps a marshalled reply with the detector rather than naming
fields, so a future select that changed that would be caught rather than reviewed for.

### Where injection attaches, and what it may spend (M-4)

**Decided 2026-09-03**, before Step 5 rather than during it. M-4's row in §2 is one line and says
only that injection is built and ships off. What Step 5 cannot start without is a hook, a host, two
budgets and a boundary, and this document owns every one of those.

**`UserPromptSubmit`, and nothing else.** Codex documents `additionalContext` on seven of its
events and Claude Code accepts it here too, so the choice was available. Only this one carries a
query. `SessionStart` has none, so injection there is a constant — and a constant context cost is
exactly what **P2** says native already pays and this product structurally does not have to. The 1.0
spec §5.8's *"SessionStart emits nothing"* survives Step 5 unchanged, for a better reason than the
one it was written with.

**Both hosts, and that was never ours to choose.** **[verified]** 2026-09-03 against both current
references: the shape is identical — a hook writes `hookSpecificOutput.additionalContext` on stdout
and the text is added as developer context before the prompt is processed. What differs is that
Codex renders the injected text as a visible message in its transcript, which is §6's fifth
mitigation — *off, and visible* — arriving free on one host and owed on the other.

**500 ms, taken from inside the 1.0 spec §5.3's second rather than added to it.** **[verified]**
2026-09-03: five `engramux search` runs against the installed service over a 227,954,688 B database
took **93, 113, 185, 245 and 251 ms**, which is process start, pipe dial, search and reply — the
injector's whole path. 500 ms is twice the worst of the five, and it comes out of the 1 s the relay
already has rather than raising it, so the product's own budget does not move because a feature was
added inside it. What is **[unverified]** is the tail: all five are warm, and every one of the twelve
read-deadline failures the 1.0 spec §7.1 records was a cold read after an idle period, against a
database two thirds this size. **M10** exists to measure that rather than to assume it. When the
deadline is missed the answer is **zero bytes down M6's own path** — the abstention that is already
gated at 100%, not a second failure mode beside it.

**5,000 B, and only one host gave a number to convert.** **[verified]** 2026-09-03: Codex documents
a default `additionalContext` limit of about **2,500 tokens**, past which it spills the full text to
a file and gives the model a head-and-tail preview and that file's path; it is configurable per
handler. **Claude Code documents no limit at all.** So the "hosts' documented budget" M5 names is
Codex's, it is also the stricter of the two by virtue of existing, and what is left to decide is
bytes per token: **2**, the conservative end for a corpus that carries Korean, which costs more
tokens per byte than anything else in it. The figure is a conversion and not a measurement, and §6's
third mitigation wants the error in this direction.

**Engramux's own events are not injectable, and what identifies one is the binary rather than the
word.** Backlog **41** found a search returning its own capture as its own top hit. The pull path is
left alone — asking for a thing and getting your own last ask for it is an answer, and a ranking
function that special-cases a document class is how a ranking function starts to rot. The push path
is a different question with a different answer: M-4 selects from the same corpus, so a user's own
last search becomes a candidate for their next prompt, which is the distractor §6 cites *Context
Rot* for. The exclusion therefore lives in the selector and not in the ranking. **The test is that
the command line invokes the installed binary**, not that it contains the string: this repository's
own corpus is largely prose about `engramux`, and a string match would exclude the owner's work on
the product along with the product's own noise.

**No migration, and settled before one was written.** The injector reads. `memory_items`, `events`
and M-3's derived columns are all written already, and a hook-time path needs no column of its own.
§6's fifth mitigation also asks for a switch and a way to see what was injected; both are
configuration and a log, neither is schema.

### What building it settled (M-4)

**Built 2026-09-03**, on `step-5-injection`, and shipped **off**. Everything below is a decision the
section above does not make or a measurement it does not have.

**A prompt is not a query, and the reduction is what the feature is.** This is the decision rev.8
left out and it turned out to be the load-bearing one. `internal/search` joins its tokens with an
implicit AND and caps them at 32, so a real prompt handed over whole is either refused outright or
is an intersection of forty prefix phrases that matches nothing. Three terms, then — and *which*
three is not a length ranking. Length picks the prose over `M3` and over `00005`, which is backwards
for this corpus: the shortest tokens in it are the most distinctive, because they are identifiers.
So a token carrying a non-letter or written in capitals sorts first whatever its length — that is
P1's classes spelled as a rule — and only the remainder is ranked by length. Words under four bytes
are dropped and identifiers are not, because `M3` is two bytes and `WAL` is three and they are the
whole of what a person is asking about.

**Selectivity replaces guessing at the prompt with measuring the answer.** "How do I fix this"
reduces to one common word, and what says so is not the word's length but that the word is in a
large share of the corpus. A query matching more than **200** documents is refused rather than
ranked, applied to each of the two indexes separately because they are separate populations. The
number is absolute and therefore does not scale with the corpus — on a hundred events it never fires
and on a million it fires late — and the upgrade path is a fraction of each index's own population,
which costs one count per injection. **M7 is the gate that would price a better one.**

**The fence is a nonce minted after the body exists and checked against it.** A fixed delimiter is a
string an attacker can write into a page the agent fetched three weeks ago; the captured bytes then
arrive inside the fence carrying their own closing marker, and everything after it reads as though it
came from outside. A nonce minted per injection cannot be in bytes captured before it existed, so the
close marker is unforgeable by anything already in the corpus — a structural property rather than a
heuristic, which is why §6 ranks it above the other four mitigations. `crypto/rand.Text` and not a
UUID, because a UUIDv7's leading bytes are the clock. A body that would collide is **refused**, not
escaped: there is no third answer. The lead line telling the model this is data sits **outside** the
fence, because inside it would be indistinguishable from an instruction the corpus carried.

**The switch is a file whose absence is off, and it is the relay that reads it.** `inject.json` in
the data directory, one key. The installer writes nothing, so a first install has no switch to find —
which is stronger than a default in code, because a user who has never heard of the feature cannot
have it on and a user who wants it makes one file whose existence is the record of their consent.
Every unreadable shape is off too. It is relay-side rather than service-side for two reasons: a
service-side switch needs a restart, since the service is a logon task; and a relay that never dials
is a shorter path to zero bytes than one that dials and is told no. `doctor` reports it either way
and prints the path on the off answer, which is the visible half of §6's fifth mitigation. The other
half is the service log: one line per injection with the masked ids and the byte count, one line per
abstention with the reason, and **never** the prompt or an excerpt.

**`Inject` is a request type of its own rather than a flag on `Search`, and that is what makes an old
service fail closed.** It answers an unknown type with a rejected ACK, the reply's `Verify` refuses
it, and the relay injects nothing; a boolean an old service ignored would have injected the whole
unfiltered result. It also keeps the field off the MCP tool surface, which is the pull path.

**The 1.0 spec §4.5 moves, and only for this event.** That section says the relay writes nothing on
stdout on any of the eleven events, and its own reasoning is *"since 1.0 is pull-only"* — which is
the row M-4 changes for after 1.0. So: `UserPromptSubmit`, with injection enabled, writes one
`hookSpecificOutput` document, and the other ten events still write nothing. §5.8's *"SessionStart
emits nothing"* is untouched.

**Capture is the invariant and injection is the feature.** Injection runs after delivery and never
touches the event's own error, so a failed injection cannot make the relay spool an event the service
already committed. Its budget is the 500 ms clamped by what is left of the relay's own second, so a
slow delivery costs injection time rather than pushing the process past its ceiling. The cost of that
order is that the prompt's own event is already a row whose text is the query, which is why the
request carries the id to exclude — exactly, rather than by resemblance.

#### A Go timer is not a clock, and only M10 could have found it

The deadline was first written as `ctx.Err()`, which is what every other read path in this product
uses. It is wrong here and the gate is what said so. **Measured 2026-09-03**: a call took **1.1445 ms
under a 1 ms budget** with `ctx.Err()` still nil, and a second run injected **640 B under a
one-microsecond budget**. A context deadline is a Go timer, Windows resolves one at about half a
millisecond, and a timer that has not fired yet leaves the context unexpired past the instant it
names — so an injection could be handed to the host after its budget with nothing having noticed.

The check now compares the instant as well as the context, and it sits **after the fence** rather
than after the reads: the two searches carry the context and fail themselves when it expires
(**[verified]** against `modernc.org/sqlite` v1.57.0 — a search taking 13 ms under a 1 ms budget
returns `context deadline exceeded` rather than its rows), but the masking, the assembly and the
fence after them carry no context at all, and a check before them leaves that stretch unguarded.

Two things follow for anyone reading the gate. The result carries **the elapsed time the injector
measured itself**, because a caller timing from outside cannot assert this without racing a decision
made a few hundred nanoseconds earlier inside. And the mid-flight arm asserts M10's own words — *no
injection exceeds its budget* — rather than "a small budget injects nothing", which timer resolution
can answer on its own; it counts the runs that did exceed, so an arm where nothing ran over cannot
pass by asserting nothing.

#### The gates, first numbers

**Measured 2026-09-03** over `.capture/fixtures-raw` — 902 captures, **16 distinct prompts**, with
this machine's **303** native memory items indexed beside them. Every reading is warm and this corpus
is not the installed 227 MB one, which no test may open (I-07).

| Gate | Asserted | Reported |
|---|---|---|
| **M5** | 16 of 16 injections inside the 5,000 B cap | largest **4,842 B**, median **750 B**. The cap is approached, so it gates something |
| **M6** | 25 synthetic prompts and every corpus prompt whose query matches nothing: **zero bytes, 100%** | **0** corpus prompts had a query this corpus does not answer, so that arm tested nothing and the synthetic one carried it |
| **M9** | 16 of 16 fenced, **0** bodies carrying their own nonce | — |
| **M10** | no injection over budget, on the injector's own clock; **32** runs across two shortened budgets did exceed and all emitted zero bytes; 16 of 16 zero bytes under a budget behind the clock | worst **29.19 ms**, p95 **29.19 ms**, median **3.11 ms** against **500 ms**; **0 of 16** abstained on time |

**M8 is not reported and nothing here should be read as it.** M8 is native memory's coverage of P1
and P5 against verbatim retrieval's, and it needs the labelled questions of both — P5's fixture does
not exist at all. What the run reports instead, under its own name, is where an injection's content
came from.

**The one over-budget reading that exists is the race run's, and it is the abstention path working.**
`./scripts/race.sh` puts the same gate over the same corpus at a median of **124.13 ms** against
3.11 ms without it, a worst of **523.01 ms**, and **1 of 16 abstained on time**. The race detector is
not a user's machine, but it is the only condition anyone has yet measured this feature under where
the deadline is reachable at all — every other reading is warm, unloaded and three orders of
magnitude inside the budget. What it says is that the abstention fires when the budget is genuinely
exceeded rather than only under a shortened one, and that the overshoot past 500 ms is the check's
own granularity: the two searches carry the deadline and the assembly after them does not. It also
corrected the gate, which had been asserting on the duration rather than on the injection and so
called a correct abstention a failure.

#### Two findings the design did not predict

**Native memory contributed to 0 of 16 injections.** The pull path reaches it — gate M3's own corpus
mode ranks memory items for targeted queries — but the three-term AND a prompt reduces to returns
nothing over 303 items. So **the push path does not reach P4 on this corpus**, and the alternation
that was built to stop events from eating the whole budget had nothing to alternate with. This is not
a defect of the alternation and it is not obviously one of the reduction either: it is the same
narrowness that makes M6 easy. What would settle it is M7.

**16 of 16 prompts injected and none abstained.** On a 902-document corpus the queries are already
narrow enough that the selectivity ceiling never fires — the largest matched 29 documents. So this
run says nothing about how often injection *should* stay silent on a real corpus, and P2's zero-cost
abstention is measured here only against inputs constructed to have no history. **M10 over the
installed database and M7 over a labelled fixture are the two instruments that would.**

*Both instruments ran on 2026-09-04, and the next section is what they said.*

#### What the first reading over a real corpus said (M-4)

**Measured 2026-09-04** over the frozen snapshot M7's harness is built against - about 21,000 events,
376 of them prompts - with 30 of those prompts put through the injector. It is the first time any of
these four figures has been read over a corpus that resembles the machine, and rev.13's finding is
that a figure taken over one that does not can be correct and useless at once.

| | over `.capture/fixtures-raw` | over the snapshot |
|---|---|---|
| injected / abstained | **16 of 16, none abstained** | **5 of 30, 25 abstained** |
| why it abstained | not reached | **24 of 25 matched nothing**; the selectivity ceiling fired **0 times** |
| M10 worst / median | 29.19 ms / 3.11 ms | **42.05 ms / 15.72 ms**, against 500 ms |
| M5 largest | 4,842 B | **4,744 B**, cap 5,000 B |
| M9 | 16 of 16 fenced, 0 carrying | **5 of 5 fenced, 0 carrying** |

**The open question above is answered, and the answer is the opposite of the shape it was asked in.**
Abstention is not rare on a real corpus, it is the common case: **83% of prompts receive zero bytes**,
and the selectivity ceiling this design built for never fired once.

**Why, corrected 2026-09-04 the same day, after an adversarial review caught the first reading.**
This section first said the three-term AND "matches nothing at all". That was read off the abstention
reason `ReasonNoHits`, whose text is *nothing in the corpus matched* - and the code returns it in two
different situations, because `keepable` runs between the search and the check. Measured through a
throwaway probe over 150 prompts of the same snapshot, deleted with the run: **122 abstained**, 2 of
them for having no usable term at all, and of the remaining 120 the event index returned **zero
matches in 0 of them, exactly one match in 115, and more than one in 5.** The memory index returned
nothing for any. **Both indexes were empty for none.**

So the AND does not match nothing. **It matches exactly one document, and that document is the
prompt's own event** - already ingested by the time the injector runs, matched by a query cut from
its own text, and then removed by the exclusion the request carries. What is left is empty, and the
reason string calls that "nothing matched".

**The corrected reading is a stronger statement about narrowness, not a weaker one.** On a corpus of
twenty-one thousand documents, the intersection of three words taken from a prompt is a fingerprint
of that prompt and reaches nothing else. That is the same narrowness that put native memory at 0 of
16, and it is now measured rather than inferred.

**One product defect falls out of it.** `ReasonNoHits` is written into the service log, where a
reader is meant to be able to tell recall from silence (§6's fifth mitigation), and it says "nothing
in the corpus matched" for a search that matched and was then filtered. The two cases want different
words. This is recorded rather than fixed here; it changes a log line, not a decision.

**Whether that is P2 working or the reduction being too narrow is what M7's `should_inject` labels
decide**, and they are the owner's to write. The two readings are not the same claim: a prompt with
no history receiving zero bytes is the capability, and a prompt with history receiving zero bytes is
a miss this instrument would otherwise never see.

**The budget holds, and that is the half rev.13 demanded.** 42.05 ms against 500 ms over a corpus
whose documents are the machine's own, where every earlier M10 reading was over documents a few dozen
bytes long.

**One fidelity limit, and it runs in the safe direction.** The snapshot is the whole history, so an
August prompt searches a corpus containing September. At hook time only the past exists, so the real
abstention rate is **higher** than 83% rather than lower, and the finding is conservative.

#### The second reading, over 150 prompts and per stratum (M-4)

**Measured 2026-09-04** over the same frozen snapshot, all 150 sampled prompts through the injector,
with the per-stratum breakdown the section below registers and this document had not yet produced.
The labels were throwaway - pass 2 and this report never read `wanted_context`, and every figure
below is the injector's own behaviour rather than anybody's judgement of it.

| | over the snapshot, 30 prompts | over the snapshot, 150 prompts |
|---|---|---|
| injected / abstained | 5 of 30, 25 abstained | **28 of 150, 122 abstained** |
| the selectivity ceiling | **fired 0 times** | **fires 5 times** |
| no usable term | not reported | 2 |
| M10 worst / median | 42.05 ms / 15.72 ms | **254.03 ms / 10.78 ms**, against 500 ms |
| M5 largest / median | 4,744 B | **4,944 B / 1,918 B**, cap 5,000 B |
| M9 | 5 of 5 fenced, 0 carrying | **28 of 28 fenced, 0 carrying** |

**Three of those rows correct a figure this document recorded**, and one of them corrects a claim
rather than a number. The ceiling that "never fired once" fires five times at five times the sample.
The worst injection is **six times** the earlier worst and still inside half the budget. And the
largest injection is 56 bytes under a 5,000 B cap, where the earlier reading sat 256 under it - M5
holds, and it holds with less room than either earlier reading suggested.

**The per-stratum table, and it does not say what this section was built expecting.**

| stratum | prompts | abstained | rate | sessions | projects | resolving elsewhere | median runes |
|---|---|---|---|---|---|---|---|
| hangul | 40 | 31 | 0.775 | 21 | 4 | **0** | 65 |
| mixed | 29 | 20 | 0.690 | 10 | 4 | **0** | 87 |
| latin | 81 | 71 | **0.877** | 30 | 4 | **0** | 394 |

**Latin abstains most.** The stratification was registered because this corpus is 74% English and
injection carries no translator, so a Hangul prompt receiving zero bytes is capability P2 rather than
a miss. That reasoning is untouched - it is about how to *read* an abstention, not a prediction of
rates - but the figure it was supposed to make legible does not separate in the direction it was
built for. The script the corpus is written in is the script that abstains most, and any sentence
attributing this fixture's abstention rate to the language wall is unsupported by its own numbers.

**A confound is visible in the same table and is the first thing to rule out.** A Latin prompt here
is **394 runes at the median against Hangul's 65**, six times longer, and the reduction to three
terms joined by AND gets narrower as the prompt gets longer, not wider. Whether the stratum figure is
measuring script or length is not settled by this table, and it is not settled by adding rows to it:
it needs the two varied independently, which this fixture cannot do because it is the corpus rather
than a design. **Recorded as an open question rather than resolved**, and it is a stronger reason to
distrust an aggregate than the one originally written down.

**Two things it does settle.** The replay-fidelity limit this section calls *not conservative* -
the injector re-resolving `cwd` against a live filesystem, so a moved worktree silently changes
scope - **does not bite on this fixture: 0 of 150 prompts resolve to a project other than the one
their event was stored under**. Carrying the stored `project_id` is still the right fix and is still
not made, but nothing measured here rests on it. And the effective sample is smaller than the
prompt counts: **21, 10 and 30 distinct sessions** behind 40, 29 and 81 prompts, over 4 distinct
projects in each stratum. A difference between two strata at these counts is not a difference many
independent observations support.

**What is still not reported, and why it cannot be from here.** The registration below asks whether
each of the two indexes hit its own ceiling separately. `Result` carries neither total, so the
figure is unreachable outside the injector - and `ReasonTooBroad` is returned when *either* index
exceeded the ceiling, including runs where the event side was emptied by the exclusion instead.
Those five ceiling firings are therefore an upper bound on ceiling firings rather than a count of
them. Backlog 46 owns it.

### What M7 will measure, pre-registered before a label exists (M-4)

**Registered 2026-09-04, before the harness had run and before one prompt had been labelled.** M3's
bar was pre-registered at 19 of 50 before the number was known, which is the only order in which a
bar means anything. This is that order for M7, and it lives here rather than in the harness so that
moving it is a spec revision somebody has to write down.

#### The corpus is a frozen snapshot, and that is what makes a pin re-derivable

**Measured 2026-09-04: the installed database went from 20,075 events to 20,993 inside one session**,
because the machine that would run M7 is the machine that generates the corpus - running the harness
changes what the harness measures. rev.12 already recorded that an absolute rank cannot be pinned
against a growing corpus; a precision number cannot either.

**I-07 is not in the way, and the snapshot is the pair.** I-07 forbids a second process opening the
service's database; a copy at another path is a different file, and the service does not hold it.
`AGENTS.md` already records that a snapshot taken after stopping the service is the `.db` and its
`.db-wal` together, because a hard-killed service never checkpoints. So: stop the service, copy the
pair, restart it in the same turn - which is `AGENTS.md`'s first carve-out, exactly as written.

Measuring offline also returns what the wire does not. The injector's own result carries the ids it
selected and the elapsed time it measured on its own clock, so **M9 and M10 become claimable over
this corpus instead of disclaimed**, and M6's asserted arm can run rather than being carried by
synthetic prompts.

#### The prompts come from inside the snapshot, and the captured ones cannot be used

**Measured 2026-09-04.** The snapshot holds **376 `UserPromptSubmit` events, every one of them
claude-code; codex contributes none** - 374 when the count was taken off `status` a few minutes
before the copy, which is the corpus moving even at that scale. `.capture/` holds 19 unique prompt captures carrying **16
distinct texts** - not the 21 a file count suggests, because two of them are byte-identical copies of
captures already in the fixtures.

Those 16 cannot be the fixture. **The capture window closed 2026-08-26 and the corpus's first event
is 2026-08-28**, so the strongest relevance class there is - the work the prompt was typed during -
is structurally absent, and a fixture drawn from them would measure precision against a corpus that
cannot hold the answers. Prompts drawn from inside the snapshot carry their own event id, so the
exclusion the relay makes is exact rather than empty, and their `cwd` is the one the relay actually
sent. Two of the 16 are also 30,945 B and 52,135 B pastes rather than typed questions.

**150 prompts, sampled systematically over time and with no random seed at all**: order every
`UserPromptSubmit` event in the snapshot by the instant it was received and take the *k*N/150*-th for
each *k*. Deterministic, so anyone with the snapshot re-derives the same 150 without being handed a
seed, and spread across the whole window rather than over one afternoon. A round-robin across sessions
was the first shape and is worse here - with more sessions than prompts wanted it can only ever take
each session's *first* prompt, and an opening prompt is not a typical one. A fixed integer stride was
the second and is worse for a different reason: it leaves the tail of the window unsampled whenever
the count does not divide, and the tail is the most recent work. **How many distinct sessions the 150
land in is reported** rather than assumed.

**150 and not 30, and the number came from a measurement rather than a preference.** The first draft
registered 30. Running the harness over the snapshot then measured what no reading before it had:
**5 of 30 prompts inject and 25 abstain**, so a fixture of 30 would score precision over five prompts.
At that rate 150 puts roughly 25 prompts through the part of the gate that needs them, and it costs
one extra blind yes-or-no per prompt rather than one extra excerpt to read - the block pass stays
small because most prompts emit nothing at all.

#### Two labels, and the first is written before the output is seen

The first label is judged from the prompt alone, with nothing from the injector visible. Only then
are the emitted blocks shown and judged one by one. That order is what makes an abstention scoreable
instead of undefined.

**The first label was reworded on 2026-09-04, before one was written, and the rewording matters more
than it looks.** It first asked whether earlier sessions *plausibly hold* something worth injecting -
which reads like M6's condition and is not it. **M6 and P2 are about whether relevant history
exists**; a person reading their own prompt is answering whether they *wanted* context, and nobody
can answer the first from a prompt alone. Conflating them would have let a coverage miss be read as
an M6 failure and the other way round. So the column is `wanted_context`, and this is what it
supports and what it does not:

| | injector emitted | injector abstained |
|---|---|---|
| **wanted_context = yes** | scored for precision | **coverage miss** - not an M6 failure, because nobody has said the history exists |
| **wanted_context = no** | **false positive** - bytes spent where none were wanted | correct silence |

**What is left unmeasured, named rather than hidden.** M6's own claim - *prompts with no relevant
history emit zero bytes* - needs the existence half, and the only instrument that reaches it is
relevance-labelling the candidate pool a prompt would have drawn from. That is a larger fixture than
this one and it is not built. M6 therefore continues to rest on its synthetic arm, which is what the
spec has said since the first reading.

Dropping abstentions from the average instead - the obvious shortcut - would leave a gate passable by
abstaining harder, which is the failure M6 exists to catch from the other side.

#### The sample is stratified by script, and that is not a detail

**Measured 2026-09-04 over the 150 sampled prompts: 40 are mostly Hangul, 29 are mixed, and 81 are
mostly Latin.** Nearly half the fixture asks in a language the corpus is not written in - rev.11
measured this corpus at **74% English** and a Korean query's connectivity to it at **16 of 50**
against an English query's **47** - and rev.12's decision 1 says injection deliberately carries no
translator, so **a Hangul prompt receiving zero bytes is capability P2 rather than a miss**.

So every abstention figure this fixture produces is reported **per stratum**. An aggregate that does
not say which stratum it came from cannot tell the capability from the defect, and the aggregate is
what the first reading of this corpus reported.

The stratum is computed from the prompt's letters and never labelled: half or more Hangul is
`hangul`, under a fifth is `latin`, the rest is `mixed` - the same boundaries rev.11's own table
uses.

#### Two fidelity limits, and only one of them runs in the safe direction

**The snapshot freezes the database and not the clock.** An August prompt searches a corpus holding
September, where at hook time only the past existed. That one is conservative: the real abstention
rate is higher, not lower.

**It does not freeze project identity either, and that one is not conservative.** The injector
resolves the request's `cwd` against the **live filesystem** every time, so a worktree that has been
moved, deleted, or has gained or lost a `.git` since the prompt was typed resolves to a different
project - and therefore a different scope - even against a frozen database. A replay is faithful only
for prompts whose worktree still resolves as it did. **Recorded rather than fixed here**; fixing it
means carrying the prompt's stored `project_id` instead of re-deriving it, which is a change to the
harness and not to the product.

#### What M7 cannot license, beyond what it does not measure

**Every prompt in the snapshot is Claude Code's.** 376 of them, and Codex contributes none, which is
what the corpus holds rather than a sampling choice. The switch is one boolean with no host in it, so
turning injection on turns it on for both. **M7 as built therefore says nothing about Codex**, and a
pilot taken on its number is a Claude Code pilot whatever the file says. Splitting the switch by host,
or building a Codex fixture, is what would change that.

#### The gate is the relevant-byte share, strictly above 0.50

**The budget is bytes, so the statistic is bytes.** A short relevant block beside a long irrelevant
one is 0.50 by block count while most of what the model actually received is distractor. Excerpts run
to 240 runes and blocks are assembled until 5,000 B is gone, so block counts and byte shares come
apart by design.

Strictly above, not at or above, because a tie is not evidence.

**The denominator is the assembled body, and the fence is outside it.** What the injector chooses is
which blocks to spend `MaxBytes` minus the fence on; the fence itself is fixed overhead that carries
no relevance in either direction, and putting it in the denominator would let a bigger fence improve
the score. So: relevant block bytes over all block bytes, both measured on the body `assemble`
returned.

*Both of the two paragraphs above were tightened on 2026-09-04 after the first draft of this section
and still before any prompt was labelled or the harness was run - which is what pre-registration
requires. The bar itself did not move.*

**Reported beside it and deliberately not gated**: pooled block precision, coverage - the share of
`should_inject = true` prompts that received at least one relevant block - the false-positive rate,
the abstention rate, whether each of the two indexes hit its own selectivity ceiling, and **M5, M6,
M9 and M10 over this corpus**. Those four have never been read over a corpus that resembles this
machine, and rev.13's whole finding is that a figure taken over one that does not can be correct and
useless at once.

The per-index ceiling matters more than it looks. The injector compares the event total and the
memory total against 200 separately and **injects anyway when one side is suppressed and the other
still has hits**, and nothing in the reply says so. An arm that did not record it would fold a
half-suppressed injection into the average as though it were an ordinary one.

#### The non-vacuity arms, and the two that were dropped for passing by construction

**This registration was corrected while the harness was being written and before one label
existed**, which is the only window in which correcting it is not tuning. The first draft named three
arms - a shuffled prompt-to-block assignment, a reversed ranking and the `MatchAny` selector - and
writing them is what showed that the first is vacuous and the other two can be.

**The shuffle always scores zero and is dropped.** A label is keyed by prompt *and* block, and a
block belongs to one prompt, so crediting prompt *i*'s blocks to prompt *i+1* misses every lookup
whatever the labels say. It would have passed on a fixture where a labeller marked every row
relevant, which is the one thing it was put there to catch.

**What replaces it has no such hole**: on the prompts the owner labelled `should_inject = no`, every
byte the injector emitted is a byte spent wrongly, and the share of all emitted bytes that went to
those prompts is gated at the same 0.50. It reads pass 1's labels, which are written before any
output is visible, so nothing about it can be answered by the injector's own choices - and it catches
what the precision figure structurally cannot, because that figure is scored only over prompts that
were injected into.

**The reversed and broadened arms stay, with an honesty condition.** Only what the shipped injector
emitted was ever judged, so an arm retrieving something else scores its blocks unjudged, and unjudged
is irrelevant - which makes a low score the pool talking rather than the ranking. Each arm therefore
reports **how much of what it emitted carried a label**, and below 20% it is recorded as
**inconclusive rather than passed**. It still fails the gate if it scores *above* the bar, because
that would be a real result.

**What would make them conclusive is labelling the whole candidate pool** - roughly `candidates`
blocks per prompt rather than the handful that fit the byte cap - and that is a larger fixture than
this one. It is named here so nobody reads more into a quiet arm than it carries.

**The reason any of this is here**: this repository has already written a gate that survived a total
inversion of the ranking, believed it, and deleted it. rev.12 records it, and both its commit and its
revert are on `main` on purpose.

**An unjudged block counts as irrelevant.** That is the null hypothesis - a block no one called
relevant is not evidence of relevance - and it is the convention pooled relevance judgements have
used since TREC. It is also what makes the overlap condition above necessary rather than fussy: the
pool was filled by one system, so the system that filled it scores best by construction.

#### What the number licenses, and what it does not

M7 as registered here measures **the precision of what the injector emitted**. It cannot see relevant
history that was never emitted, and that narrowing is recorded rather than hidden: **native memory
contributing to 0 of 16 injections would draw no penalty from this instrument at all.** So the number
licenses exactly one claim - that on this fixture the emitted bytes were mostly relevant - and it
licenses neither *"the necessary memory was recalled"* nor *"more precise than native"* nor anything
about task outcomes. Section 3 already puts the last of those out of reach for a one-developer corpus.

**And it licenses an owner pilot rather than a release default.** 30 prompts from one owner on one
machine over about one week is enough to let that owner turn injection on for themselves knowingly.
It is not enough to turn it on for a stranger, and any sentence saying M7 is what licenses that -
including the one session 13 wrote - is wrong by this section.

### Replacing an installed build is its own command (M-7)

**Decided 2026-09-03**, and scheduled after the plan's Steps 4 and 5 rather than into them. Nobody has
this product installed but its owner, so an update path is a feature with no users yet; what forced
the decision now is that the *developer* reinstall is the same sequence, and it was about to be a
bash script forever.

**`engramux update` is `install --apply` minus everything that writes host configuration.** It
replaces the two binaries and takes the service through the 1.0 spec §5.5 sequence — stop, wait for
the exclusive lock to be released, replace, start — and it restarts what was there when a copy fails.
It never touches `~/.codex/config.toml`, Claude Code's user configuration, or the hook entries.

That division is the point, and it is worth stating why it is not a second door to one room. Safety
here comes from **the definition of the command** rather than from a condition on the caller: an
agent may run `update` full stop, where `install --apply` needs `doctor` to have confirmed both hosts
are already registered (`AGENTS.md`). A narrower command with a guarantee attached is a better
boundary than a wider one with a rule beside it, and `scripts/reinstall.sh` becomes nearly empty when
this exists — which is the sign it is the right shape.

**Engramux does not get outbound network, and this decision is where that was tested.** Measured
2026-09-03: `net/http` is imported by exactly one shipped file, `internal/mcpserver/serve.go`, and it
uses it to *listen* on loopback; the three other `url.Parse` calls are string parsing. **This product
has never made an outbound call.** So "update itself when there is a newer tag" is not a small
request — it is a new capability class, and the binary an updater fetches is the classic supply-chain
surface. The answer is that **whatever fetches is not Engramux**: a delivery channel updates a local
marker, and `update` reads it. The noticing survives; the fetching stays with whoever already has the
user's trust for fetching.

`--from <directory>` survives as an **escape hatch and not the default** — it is how a developer
updates from a build tree and how an offline or proxied user updates from a folder they downloaded.
Until a delivery channel exists it is the only door, and that is accepted rather than hidden.

**What is rejected, with the reason, because each will be proposed again.**

*Hook-triggered automatic update.* Three independent things stop it, and any one is enough. The
relay's whole budget is 1 s (§5.3) and this sequence is a service stop, a lock release, 19 MB of
copying, a start and a migration. Windows cannot overwrite a running image — `AGENTS.md` has the row
— and the `SessionStart` relay *is* the binary that would be replaced. So it would need a detached
child process, which is exactly the installation architecture `AGENTS.md` forbids taking as a model.
Beyond the mechanics it is unattended work escalating itself, and a failure leaves the user with no
service at the moment a session starts, which is I-04 broken quietly.

*Hanging it on a plugin's update.* **[verified] 2026-09-03** from Claude Code's own plugin reference:
there is no install-time, update-time or removal-time lifecycle hook. A plugin manifest carries six
keys and its hooks are all session-runtime events. The one automated lifecycle step is a dependency
install with `npm ci` or `bun install`, which is the Node runtime **M-5** removed. So there is no
event to hang an updater on, and the nearest thing — a `SessionStart` hook — is rejected above.

*Building on the user's machine.* `go install` is not offered and source is not the primary path. The
product's argument is two statically linked binaries and no runtime, which is the same reason
`CGO_ENABLED=0` is a boundary; a Go toolchain is a heavier runtime than the C one that rule exists to
avoid. It is also worse for §8's fourth condition below rather than better: a binary built on the
user's machine has no publisher at all, so it starts from less reputation than an unsigned release,
not more.

### The delivery channel, and what it costs Codex (M-7)

**Decided 2026-09-03**, in the session after M-7 was written and against what the two hosts' own
references say today. M-7 left this open deliberately and named the consequence: until a channel
exists, `--from` is `update`'s only door. This closes it, and it does not move the schedule — the
plan's Step 6 still builds after Steps 4 and 5.

**A GitHub Release is the substrate and a Claude Code plugin is the channel.** One zip per release,
and the marketplace entry pointing at it carries that zip's SHA-256. Claude Code fetches it, checks
the hash, and unpacks it under its plugin cache; `engramux update` reads that directory. The party
that fetches is therefore the host the user already trusts for fetching, which is the whole of M-7's
rule about outbound network — this product still has never made an outbound call and this decision
does not give it one.

**[verified] 2026-09-03** from Claude Code's own plugin and marketplace references. A marketplace
entry accepts an `archive` source — a URL with an optional `sha256` — which needs neither git nor npm
on the user's machine, caps the archive at 256 MiB, and requires Claude Code v2.1.224 or later.
Installed plugins are cached one directory per version, keyed by marketplace, plugin and version,
with old versions kept about fourteen days for sessions still running against them. The host's own
plugin update command is what fetches, and a plugin's own version field pins what a user receives
until it is bumped.

**The zip is the plugin, and that is forced rather than chosen.** An `archive` source's zip has to be
a plugin directory, so it carries the manifest and the two binaries. The consequence is the part
worth writing down: the manual door and the channel consume **the same artefact**. Somebody who
downloads the zip and unpacks it by hand points `--from` at what they unpacked and gets bytes
identical to what the plugin cache holds. There is no second build and no second packaging step for
the two to drift apart in.

**The plugin delivers and does not configure.** Its manifest carries no hook entries and no MCP
server entry, although both fields exist and would fit what `register` writes. Two reasons, and the
first is I-04: a plugin's hooks are live only while the plugin is enabled, so disabling one would
stop capture silently, which is the failure this product exists not to have. And a plugin-provided
hook resolves against the plugin root rather than against the installed relay, so `doctor`'s check
that the eleven entries point at the installed binary would have to accept two answers, and two
relays of different versions could be live at once. `install` and `register` stay the only writers of
host configuration, which is also what keeps `AGENTS.md`'s rule about an agent not editing host
configuration meaningful.

**[verified] correction to the rejection above.** M-7's paragraph rejecting a plugin lifecycle hook
says the manifest "carries six keys". Re-read on 2026-09-03, it carries considerably more than six —
metadata, component paths for skills, commands, agents, workflows, hooks, MCP servers, output styles
and language servers, plus user configuration and plugin dependencies. The claim that decided
anything is unaffected and stands: **there is still no install-time, update-time or removal-time
lifecycle hook**, and every hook a plugin declares is a session-runtime event. The count was wrong;
the conclusion drawn from it was not.

**Codex users get the same capability and less convenience, and that is the decision rather than an
oversight.** The release zip is the contract for both hosts: a user of either can download it, unpack
it and run `update --from`, and what they get is byte-identical to what a plugin user gets. What is
unequal is the noticing. **[verified] 2026-09-03** from OpenAI's own plugin documentation, Codex does
now have a plugin system — a manifest of its own, local marketplace catalogues at a repository-scoped
and a personal path, and a cache under its configuration home — so the shape exists. What does not
exist there is the two things this channel is built out of: **no archive source is documented and no
update command is documented.** A Codex plugin could carry the binaries and could not fetch a new zip
or say that one exists, which is exactly the part being bought. Building one now would add a second
manifest to keep in step and deliver nothing the release page does not already deliver. What is owed
instead is that the README says this plainly rather than letting a Codex user find it out. Revisited
when Codex documents either of the two, and the revisit is cheap because the artefact is already the
same one.

**A product version, and it is not the wire version.** Semantic versioning, `0.x` until §8's four
publication conditions all close, injected at link time. The plugin manifest wants semver and so does
every package manager this decision leaves on the table; nothing wants a date. `ipc.Version` stays
what it is — a wire protocol version with exactly one consumer, the ack check, moving on a
compatibility event. Coupling them would raise the wire version on a release that changed a document,
and every relay and service pair a user had not restarted together would stop meeting. Two values
because there are two questions, and `doctor` prints both.

**The release runs on GitHub Actions, and that is a prerequisite rather than tidiness.** Push and
pull request run the three checks `AGENTS.md` names, in its order. A tag builds the two binaries,
packages the zip, computes its SHA-256, creates the release, and updates the marketplace entry in the
commit that carries the version. The catalogue lives at the repository root beside the code it
describes, so a version and the hash of the artefact it names cannot be committed apart. The build is
`-trimpath` over the pinned toolchain with `CGO_ENABLED=0`, which is what makes an artefact
attributable to a commit.

**A green CI is a weaker statement than a green local run, and the tagging rule is what that buys.**
**[verified] 2026-09-03**: a runner has no `.capture/` and no native memory, so **M1**, **M3**, the
Phase 4 gate's corpus mode and the Phase 6 audit's masked half all skip there, each by its own
explicit skip — the right behaviour and not a defect. What follows is a rule about tagging rather
than about CI: a tag is pushed only after those four have been seen green on a machine that has the
corpus, and what run that was is recorded with the release.

**One artefact, windows/amd64.** Assumed rather than decided, and written here so it is visible
rather than discovered: nothing in this product has been measured on Windows on arm64, and the 1.0
spec's argument is written against the platform it was measured on. Reversing it is one entry in a
build matrix and a second zip, on the day somebody asks.

**No certificate is bought, and what replaces buying one is named.** §8's fourth condition is an
outcome and stays one; this is the separate decision it left. **[verified] 2026-09-03** from
Microsoft's own comparison of code signing options, last updated 2026-08-29, which is wider than the
two costs §8 recorded. Azure Artifact Signing — formerly Trusted Signing, about ten dollars a month,
no hardware token, CI-native — is **unavailable to this project**: organisations are limited to the
USA, Canada, the EU and the UK, and **individual developers to the USA and Canada**. That is a
geographic bar rather than a price, and it removes the cheapest option before cost is discussed. An
OV certificate is 150 to 300 dollars a year worldwide and still carries the June 2023 hardware
requirement §8 records. EV is confirmed as no longer worth its premium for SmartScreen, which §8
already says. What §8 did not have is **SignPath Foundation**, which signs qualifying open-source
projects at OV level for free through a managed pipeline: an OSI-approved licence with no commercial
dual-licensing qualifies and `LICENSE` is Apache-2.0, but its other two bars do not clear today — the
project must **already be released in the form to be signed**, and it must build on a **trusted build
system**. Both are exactly what the release decision above creates. So signing is not rejected, it is
**sequenced**, and what unblocks it is a release process rather than a purchase.

**The false-positive submission is not adopted, and §8's fourth condition is carried by documentation
instead.** This clause said the opposite when it was written hours earlier — every release submitted,
on the argument that it is free and fixes the detection for everyone rather than for one machine. The
owner declined on **2026-09-03**, and the objection is on a different axis from the argument: not
whether it is worth doing but **who does it and when it answers.** The submission is an authenticated
web form, so it is a human's hands every release with no route an agent or a workflow can take; and
the same reading that recommended it says why it does not compound — without a signature there is no
publisher identity for anything to attach to, so each release is a fresh submission and a fresh wait
rather than a reputation being built. "Free" was true and "quick" was never established.

**What this does not do is weaken §8's fourth condition**, and that is worth stating because it is the
first thing a reader will assume. That condition is an outcome and it already reads *"a stranger's
first run works, **or** the documentation tells them exactly what will happen and what to do"*. With
the submission off the table the second half is the whole of it, which turns a vague intention into a
concrete requirement on publication condition 3: **the `README` has to name the detection by the
string a user will actually see, say that it fires on the CLI and not on the service, and give the
exclusion steps for the two directories.** A reader who meets `Behavior:Win32/Execution.A!ml` with no
warning has been failed by this project; a reader who was told in advance has not.

**What would reopen it.** A release process exists after Step 6, and a submission that a workflow can
make without a person is a different decision from this one. So is a signed release: with SignPath
there *is* a publisher identity, submissions start compounding, and the argument that was made here
becomes true rather than merely appealing.

**`doctor` compares three versions, because there are three and they fail differently.** The
installed binary, the service actually running, and the newest version present in the plugin cache.
Installed against running catches a replacement that was copied and never restarted, which is the
state an interrupted reinstall leaves on this machine today. Cache against installed is what M-7's
done-condition asks for, a newer binary beside the installed one. The first pair is answerable with
no channel at all, so a user who never installs the plugin still gets the more useful half.

**`scripts/reinstall.sh` becomes a one-line wrapper and is not deleted.** `update --from dist/`
replaces the sequence the script exists for, and what is left after it is `doctor` and `status`.
Three commands is still worth one, and the script keeps the place where `AGENTS.md`'s two carve-outs
are explained. M-7's own prediction, that it becomes nearly empty, is what happens — "nearly" is the
answer and not "entirely".

---

## 3. What "more precise than native" means, measurably

Native explicitly skips file paths, debugging fixes and anything derivable from the codebase. Five
capabilities, each with a definition that can fail.

| | Capability | Definition | Why native cannot |
|---|---|---|---|
| **P1** | **Exact-span recall** | For a literal that exists in the corpus — an error message, a stack frame, a command line, a path — a natural-language query for it puts a document containing that literal in the top *k*. Reported as recall@k and MRR per class | Those three classes are the ones native declines to store |
| **P2** | **Zero-cost abstention** | For a prompt with no relevant history, the injector emits **exactly zero bytes**. Required at 100% | Native loads its index every session **regardless of the query**, so its context cost is a constant. A query-dependent zero is structurally ours |
| **P3** | **Temporal resolution** | A time-qualified query is narrowed by real event timestamps and session boundaries | Native carries a `modified` field, which is when a note was written, not when the fact was true |
| **P4** | **Cross-host single search** | One query reaches answers that exist only in the other host's sessions or memory. **Narrowed 2026-09-04 to same-language retrieval, guaranteed on MCP and the CLI and explicitly not on injection.** Within the language it is **measured at 25 of 50** for a question, against an oracle ceiling of 37 and against 1 of 50 before the selector changed the same day. Still `[unverified]` in one respect the fixture cannot remove: the English queries were translated by a model, so the ceiling may be optimistic. See *What gate M3 measured on its first human fixture*, *What the English arm settled* and *What building the selector settled* | Each host sees half |
| **P5** | **Failure-fix pairs** | Querying with the text of a failure returns the edit or command that resolved it | *"Debugging fixes"* is on native's documented exclusion list |

P2 is the sharpest of the five and the least obvious. *Context Rot* (Chroma Research, 2025-07-14, 18
production models) measured that **a single distractor lowers accuracy against baseline, and the
effect grows with context length**. Injecting nothing when there is nothing to inject is therefore
not a saving, it is the feature.

---

## 4. Order

1. **M-2**, the native indexes. Smallest, immediately visible, and it is what makes P4 measurable at
   all.
2. **M-3**, derived fields. The cheapest precision lever, and the evidence says the selector is where
   the points are.
3. **M-4**, injection, built and left off until §5 passes.
4. **M-5** and **M-6**, installation and diagnosis. Independent of the three above and of each other;
   they gate publication rather than function.

---

## 5. The gates

M-4 does not turn on for anyone until M5, M6, M9 and M10 pass and M7 clears its threshold. M1–M4 and
M8 are conditions on the work that precedes it.

**M5, M6, M9 and M10 pass as of 2026-09-03**, over `.capture/fixtures-raw` with this machine's native
memory beside it — `TestGateInjectionOverTheCorpus`, `TestGateM6ZeroByteAbstention` and
`TestGateM10TheDeadlineIsEnforced` in `internal/inject`. The figures, what each arm actually asserted,
and the two things the run says nothing about are in M-4's own section; none of them is repeated here.
**M7 is un-run**, so nothing below licenses turning the feature on.

| | Gate | What it asserts |
|---|---|---|
| **M1** | Native parse fidelity | Over every native memory file present on the machine: no crash, frontmatter fields extracted exactly where they exist, body bytes preserved losslessly. One failure fails the gate |
| **M2** | Drift canary | An unknown frontmatter key, an unknown file name, a missing index — each **warns and continues**. A silent skip is a failure |
| **M3** | P4 recall | Queries whose answer exists in only one host, 25 per host, recall@10 against each native memory's own ceiling. **Measured once, pinned, and thereafter a regression test on the pinned number** — M7's shape, for M7's reason. A natural-language query a person wrote from memory is not a literal cut from the document the way P1's classes are, so a miss is not unambiguously a retrieval failure, and a gate that asserts 100% is answered by rewording the query until it passes | **Pinned 2026-09-04 at claude-code 0.400 and codex 0.600**, 10 and 15 of 25 over populations of 38 and 265 and the English fixture, same-script 24 of 42 and cross-script 1 of 8. It took three tries to get a number worth pinning: the Korean fixture returned **0 of 25 on each host** and a floor of zero is a gate that is off; the English fixture under the implicit AND returned **1 and 0**; and the selector is what moved it to 25 of 50, against a bar of 19 pre-registered before the number was known and an oracle ceiling of 37. Three sections carry it — *What gate M3 measured on its first human fixture*, *What the English arm settled*, and *What building the selector settled*.
| **M4** | Field boost earns its place | P1's three new classes, recall@10 and MRR with the derived-field boost on and off. **No improvement means the code is deleted** |

**M4's second measurement, 2026-09-04.** The gate's own delete condition asks for one, and this is it: **improved in all three classes and regressed in none**. Recall@10 moves in one class, 0.480 to 0.520 on `a touched path`; MRR moves in all three, 0.242 to 0.262, 0.134 to 0.170 and 0.555 to 0.613. The delete condition does not fire, migration `00005`, `store.Derive` and the ORDER BY term stay, and the 18.4 MB is paid for.

| **M5** | Hard cap | The whole corpus through the injector, zero replies over the byte cap, which is **5,000 B**. The cap comes from the hosts' documented budget rather than an observed p95, and M-4 below records which host documented one and how it became bytes |
| **M6** | Zero-byte abstention | Prompts with no relevant history emit zero bytes, **100%**. One failure fails the gate. This is the direct defence against SWE-ContextBench's free-summary regression |
| **M7** | Precision at budget | **The precision of the excerpt blocks the injector emitted, under the 5,000 B cap. Relevant history that was not emitted is not measured** - the narrowing is recorded rather than hidden. Measured over a frozen snapshot of the installed database, from prompts drawn out of that same snapshot, with the statistic, the bar, the reported figures and three non-vacuity arms all pre-registered on 2026-09-04 before a label existed. The gate is the **relevant-byte share, strictly above 0.50**. Below threshold the feature does not ship enabled, and above it what is licensed is an owner pilot rather than a release default. *What M7 will measure* carries all of it |
| **M8** | Native coverage, reported | For P1 and P5, how many questions native memory alone could answer against how many verbatim retrieval can. **This pair of numbers is the honest form of "native-grade or better"** |
| **M9** | Data fence | Every injected payload sits inside a per-injection nonce delimiter, and the delimiter never appears unescaped inside the payload. Asserted over the whole corpus, zero occurrences |
| **M10** | Injection's time | **The deadline holds, and the distribution is reported.** Over the whole corpus no injection exceeds the 500 ms M-4 gives it — asserted, and asserted against a search made deliberately slower than the budget as well as against the corpus, because a deadline that is never approached is not evidence that it is enforced. The p95, the worst, and the share that abstained on time are **reported**: nothing has measured what a cold read costs at this database's size, so a rate would be a number invented rather than found |

**What cannot be measured here, stated so nobody claims it.** Whether injection improves task
outcomes needs paired runs over hundreds of tasks, which is what SWE Context Bench did with 399. One
developer's corpus cannot support that claim and this project must not make it. What is reachable is
retrieval quality, precision at budget, abstention accuracy and injected-byte distribution — four
falsifiable things. Chasing a LoCoMo or LongMemEval score is explicitly out: both have documented
gold-label defects, small per-category samples, and a plain full-context baseline that beats most
published memory systems; two vendors have publicly contradicted each other's numbers on the same
benchmark. This project already owns a better instrument in its own known-item gate.

---

## 6. Injecting captured content is an injection vector

**The corpus is not "the user's own data".** It is everything the user's agent saw — prompts, tool
output, file contents, and **web pages the agent fetched**. That last item makes the corpus
attacker-reachable on a single-user machine, and unlike a session-scoped prompt injection the payload
is **temporally decoupled**: bytes captured today fire weeks later when a query happens to match.
The literature names this memory poisoning and has a 2026 survey of it (arXiv:2604.16548) plus work
on delayed-trigger variants (arXiv:2605.15338).

**One concrete instance, found rather than hypothesised.** Codex's own memory read-path stores
**model-directed instructions inside the memory files** — extract keywords, search the index, open
the file it points at, stop when nothing matches. So M-2 plus M-4 means literally injecting
instruction-shaped text. This is a property of the design, not a risk it might have.

Mitigations, strongest first:

1. **Not injecting is the default.** The MCP tool surface stays the first route. This buys no
   accuracy — §2's evidence says the pull path regressed too — it buys a smaller window.
2. **A structural data fence, enforced by M9.** The only defence here that does not depend on the
   model behaving well.
3. **Small.** A hard cap and short spans. Less payload is less surface, and Chroma's distractor
   result wants the same thing for accuracy.
4. **Provenance.** Every excerpt carries its event id and timestamp, so a reader can tell recall from
   instruction and an incident can be traced.
5. **Off, and visible.** A switch, and a way to see what was injected.

**Three things to be honest about.** None of this is safe against an adaptive attacker, and the
published position is that detection-based defences fail. The redaction work of rev.4 §6 is a
*different control* — it governs confidentiality and egress, and **masked content still carries
whatever instructions it contained**; the two must not be confused in review. And there is one
asymmetry in this design's favour: because the original event is never overwritten, a poisoned entry
can be audited and rolled back, which a store that consolidates its memories cannot offer.

---

## 7. Rejected, with the reason

| Rejected | Reason |
|---|---|
| LLM summarisation, ours | §2. Reopen only on M8's evidence, and even then M-2 first |
| A vector index — `sqlite-vec`, FAISS, chromadb | C extension or C library against a `CGO_ENABLED=0` boundary. Noted for the record: `modernc.org/sqlite` v1.57.0 **does** vendor sqlite-vec CGO-free, so half of this is already free if the question ever reopens — but nothing measured says it should |
| Local embedding inference — Ollama, llama.cpp, ONNX Runtime | Sidecar process or C runtime; both boundaries |
| mem0 / Zep / Letta / Graphiti as a service | Node or Python runtime plus a separate store, some needing a graph database. This is a reproduction of exactly why claude-mem fails on Windows |
| A knowledge graph as the primary store | Needs an LLM to build, and structure-first retrieval is reported to lose ground on simple factual questions **[secondary]** — which is the shape of a developer's question. **Take the idea of a validity interval and leave the graph**: it is a sortable column in SQL, not an edge |
| Reflection and consolidation passes | The measured gain is amortised cost and latency **[secondary]**, not recall of a specific fact — and consolidation discards first exactly what this product exists to return. Keep the scheduling idea, drop the LLM call inside it |
| Chasing LoCoMo / LongMemEval | §5 |
| Memory-as-action RL, multi-agent memory orchestration | No application to a single-user local service |

---

## 8. Installation and diagnosis

### `engramux install` (M-5)

The product's whole argument is two statically linked binaries and no runtime, and installation was
the one place a runtime survived. Moving it into the CLI closes that, and it closes the flow's own
defects at the same time: today the installer must be run **twice** — the first pass cannot register
the MCP endpoint because the service has not published one yet — and it never mentions `register`, so
a user who follows it end to end has a capture that stops working at the next logon.

What the Go command owes that the script did not: one pass, by starting the service and waiting for
the endpoint rather than asking the user to; naming the logon-task step; and refusing before the
first copy when a destination is locked, which is the script's one genuinely good diagnostic and must
survive the move.

**[verified] and load-bearing for the tests:** the script's own MCP registration shells out to the
host's CLI, which resolves its own configuration file and ignores the redirected environment a test
hands it. Any replacement inherits that hazard, and the seam that contains it — an empty `PATH` —
has to be reproduced or the tests write outside their temporary directory. `AGENTS.md` carries the
row.

**First run against the real hosts, 2026-09-02**, replacing the Phase 6 binaries with the merged
build on the owner's machine: both copies, the logon task, the service start, the Codex registration
and the eleven hook entries went through in one pass, and the four events that arrived while the
service was down came back through the spool. The one thing that did not go through was Claude
Code's `mcp add` against a registration the previous installer had already made — it exited 1 with
the existing registration intact and pointing at the live endpoint, which `doctor` confirmed.
Backlog 35: a re-install must not report a failure it did not cause. Closed in Step 1's build the same
day: the installer reads the host's own file first, with the check `doctor` already made, and a host
that points at the endpoint is said to and left alone.

### `doctor` by stage (M-6)

Three changes. "Not installed yet" and "installed and broken" become different answers, each naming
the command that moves it forward — today a fresh machine gets four failing sections and no
instruction anywhere. MCP becomes **optional**: a deliberate capture-only installation is a supported
state and must be able to be green. And the eleven hook entries are checked — that they exist, and
that they point at the installed relay — which is the one thing a working install actually depends on
and the only major surface `doctor` does not look at.

Two things `doctor` already does that must not regress: it reports the tokenizer as a **verdict**
rather than as two strings to compare, and it explains a locked destination with the right remedy per
file.

A fourth change, decided 2026-08-30 out of the two things this section previously left open. `doctor`
printed a Windows SID and the real database path, in the output a user is most likely to paste into a
public issue. **The default becomes masked and a `--full` flag prints the real values**, and the task
principal becomes a verdict — this user, or another one — for the same reason the tokenizer is a
verdict: the question is which user, not which number. Masking is applied to every line rather than
to a chosen set of fields, so the rule is one call site rather than a judgement repeated per value.
The real database path stays reachable, which is what §5.9 of the 1.0 spec asks of this command; it
moves behind the flag rather than out.

`--full` un-masks only what this command masks. A value the service already redacted before writing
it — a log line through I-10's filter — comes back redacted either way.

The stage judgement is **unanimous**: only a machine with no logon task, neither binary in the
install directory, and no Engramux hook entry in either host is told to install. Any one sign present
means the useful answer is what is broken, and a task or a host file that could not be **read** is not
one that is absent — both fall through to the full report, where each is a finding with its own line.
The direction matters: a report that said "not installed" to a half-installed machine would hide the
failure that half-install hit.

MCP being optional has a cost, and it is taken deliberately: an endpoint that is published and not
answering now exits 0. It is still printed and still loud. This is the same trade the service already
makes, where a failed endpoint is logged and ingest carries on rather than the service refusing to
start.

### Publication conditions

Decided 2026-09-02. §1 already makes publication wait on the memory feature being native-grade or
better; this list is what else it waits on, written here so that the conditions have one owner instead
of living in a session brief.

1. **A first install on a clean profile.** The 1.0 spec's Windows argument has been measured on one
   profile only, and that profile has had every build of Engramux on it. What the argument needs is an
   install by the two shipped binaries alone onto a profile that has never run them. A *profile* is the
   unit, not a machine, because it is the product's own unit of isolation: one service per user, a pipe
   named from the user's SID, a data directory, two host files and a logon task per user. A second
   local account on the owner's machine satisfies it, and it sees the one thing a disposable sandbox
   cannot — the logon task starting the service at that account's logon. What it does not see is
   another Windows build, and that is accepted. The condition read "a clean VM" from 2026-08-30 until
   this revision; neither Hyper-V nor Windows Sandbox is installed on the owner's machine, a sandbox
   could not see the logon half, and a VM's one extra answer is not worth its setup while publication
   is this far off. What does **not** satisfy it, measured: an isolated tree on the owner's own
   profile. Session 07 ran `install --apply` against one, and the service that run started went
   through its scheduled task — a task runs with its principal's environment, not with the redirected
   one the installer was given — so that instance found the real data directory and the real pipe,
   lost the pipe race to the running service (I-09), and wrote its `stopped` line into the real log.
   `AGENTS.md` carries the row; the 1.0 spec §7.1's soak row carries the line.
2. **Backlog 28, the bearer token's file permissions. [verified] 2026-09-04, and this condition is
   closed.** The 1.0 spec §5.9 accepts the inherited DACLs on the owner's machine; on a stranger's
   machine it is not the owner's to accept. Both halves are now built. `mcp.json` is written with a
   protected DACL of its own, granting SYSTEM, BUILTIN\Administrators and the user this process runs
   as, and nothing else - set on the temporary file before one byte of the token reaches it, so the
   token never sits on disk under an ACL the product has not replaced. The two host files are not
   this product's to narrow, and `doctor` reports their permissions as a finding rather than changing
   them: a verdict and a count, never a principal, in both the masked mode and `--full`.

   **Three things the build settled that the condition assumed, and two of them reverse it.**

   **Administrators is on the list, where `internal/pipe`'s DACL has only SYSTEM and the owner.** The
   condition said "on the pattern `internal/pipe`'s listener already uses" and the pattern does not
   transfer whole. A pipe's DACL dies with the process; this one outlives it, and this file is §5.9's
   only documented way to rotate the token - delete it, restart, re-run the installer. A user SID
   changes on a profile migration or a recreated domain account, and a DACL admitting only SYSTEM and
   a principal that no longer exists has removed the remedy along with the exposure. The ACE costs
   nothing: an administrator holds SeTakeOwnershipPrivilege and reaches the file either way.

   **The mask is FILE_ALL_ACCESS and not GENERIC_ALL, and the reason is not the one the arithmetic
   suggests.** They share no bit - 0x001F01FF against 0x10000000 - so copying the pipe's mask looks
   like it would store a number no file request can use. **Measured 2026-09-04: it would not.**
   `ACLFromEntries` wraps SetEntriesInAcl, which maps a generic mask to the object type's specific
   rights before the ACE exists; an entry written with GENERIC_READ reads back as 0x00120089 and one
   written with GENERIC_ALL is indistinguishable from FILE_ALL_ACCESS. The mask is spelled out
   because the mapping belongs to that call and not to the ACE, so a caller that builds an ACL by
   hand or writes SDDL gets no mapping and no error - just a file its owner cannot open.

   **The DACL survives `os.Rename`, and that is load-bearing rather than incidental.** The narrowing
   is applied to the temporary file and what a host reads is the renamed one. Go's `os.Rename` is
   `MoveFileEx` without MOVEFILE_COPY_ALLOWED and not `ReplaceFile` - which would have preserved the
   *destination's* DACL and inverted the whole design - and the temporary file is created in the
   destination's own directory, so the move is same-volume and carries the security descriptor with
   it. `TestWriteNarrowsTheFileItPublishes` asserts it on the published file rather than the
   temporary one, which is what makes it a check rather than a restatement.

   **What it removed, measured on this machine**: the inherited ACL on a fresh file under the user's
   own directory is four ACEs, one of which names a principal beyond SYSTEM, Administrators and this
   user. Spec 7.1 recorded the same shape.

   **What it did not close, closed the same day.** Both were carried as backlog rows rather than
   implied, and both are now built with tests that fail when the fix is undone. `mcpconf`'s
   temporary file — the one holding a *raw* token, where `internal/host`'s holds one inside another
   product's configuration — is swept before each write, under a name narrow enough to glob for.
   `internal/host`'s backup copies are **bounded and not swept**: three survive, because a backup
   here is the documented way back from a bad write and removing all of them would take the remedy
   away with the exposure — the same reasoning that put Administrators on the DACL above. `doctor`
   counts them beside the permissions line, as a count and a date, on the two branches where the
   file carries the token. How many stood on the owner's machine before the bound is
   `[unverified]` and stays so: a credential-directory guard refused the listing in both sessions
   that tried, and neither worked around it. That refusal is what the `doctor` line answers.
3. **A `README`.** There is none, and a public repository with no `README` and no licence granted
   nobody anything. The licence half closed on 2026-09-02: `LICENSE` is Apache-2.0. **It gained a
   named requirement on 2026-09-03** when the false-positive submission was declined (M-7): condition
   4 below is an outcome with two halves, and with the submission off the table this document is the
   whole of the second one. So the `README` has to name the detection by the string a user will see,
   say that it fires on the CLI and not on the service, and give the exclusion steps — which makes
   this condition partly a dependency of condition 4 rather than only a courtesy.
4. **A first run that survives the machine's own antivirus, and is documented.** Written as an
   outcome rather than as a mechanism, which the other three are too, and deliberately: signing is
   not a pass. **[verified] 2026-09-03**, on the owner's machine, mid-session: Windows Defender
   removed `engramux.exe` from both the build directory and the install directory as
   `Behavior:Win32/Execution.A!ml`, severity 5, `DidThreatExecute` False — blocked before it ran, so
   nothing was compromised. `Behavior:` and `!ml` are the finding: a behavioural machine-learning
   detection on executing a freshly built, rare, unsigned binary, not a signature on the bytes. The
   service binary was untouched and kept running, so capture never stopped. It was **not** the first:
   `Trojan:Win32/Commando.A!ml` fired on 2026-08-30 against the soak sampler's `schtasks /create`, so
   two of the four detections that machine has ever recorded are Engramux doing what it is designed
   to do — run a new unsigned executable, and register a scheduled task. A stranger's first install is
   those same two shapes. Backlog **37** carries the measurement.

   **Why the condition is not "sign the binaries".** Microsoft's SmartScreen change of March 2024
   removed the instant-bypass an EV certificate used to grant; OV and EV now both accumulate
   reputation through download volume. So signing is not a switch that turns the detection off — it
   is what makes reputation *accumulate across releases* instead of resetting on every build, which
   is a real and different benefit. A first release by a new publisher still has no reputation, so a
   condition that named signing would be satisfied by something that does not yet deliver the
   outcome. What satisfies this condition is that a stranger's first run works, or that the
   documentation tells them exactly what will happen and what to do — and with the false-positive
   submission declined in M-7, the documentation is the whole of it. Condition 3 carries what the
   `README` therefore owes.

   Two costs of signing are recorded so the decision is made against them rather than against a
   guess: a code signing key has had to live on FIPS 140-2 Level 2 hardware since June 2023, so there
   is a token as well as a certificate; and from March 2026 a publicly trusted certificate is valid
   for at most 460 days, which makes it a recurring chore rather than a purchase. This is not a small
   project's problem alone — `openai/codex`, one of the two hosts this product serves, has its own
   Defender false-positive issue on the same shape.

   **The route past those two costs was found on 2026-09-03 and is decided in M-7 rather than
   here**, because it is a decision and this is a condition: signing is sequenced behind a release
   process rather than bought, the cheapest paid option turns out to be closed to this project by
   geography, and the free one is closed only until a release exists. What that changes about this
   condition is nothing — the outcome is still a stranger's first run working or being documented.
   What it changes is that "sign the binaries" now has an answer instead of a price tag.
