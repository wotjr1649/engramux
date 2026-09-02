# Engramux — memory architecture, after 1.0

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

*One test was fake and a break-it pass is what said so.* The memory hit's masking test searched for a
literal user name the *body* carried, and the source path a test writes to is under the machine's own
temporary directory — so it carries the **real** user name and the assertion never reached the field
it was named for. It sweeps the marshalled hit with the detector now, which is what §8's Phase 5
clause does and for this reason.

### Why derived fields are not a summary (M-3)

The distinction is load-bearing and easy to lose. A derived field exists **to find a document**; a
summary exists **to answer instead of one**. The moment a derived value is what the reader is given
rather than what the reader is given a route to, M-3 has become M-1's rejected option. The evidence
for the boundary is the same 42.5%-versus-43.9% result above.

---

## 3. What "more precise than native" means, measurably

Native explicitly skips file paths, debugging fixes and anything derivable from the codebase. Five
capabilities, each with a definition that can fail.

| | Capability | Definition | Why native cannot |
|---|---|---|---|
| **P1** | **Exact-span recall** | For a literal that exists in the corpus — an error message, a stack frame, a command line, a path — a natural-language query for it puts a document containing that literal in the top *k*. Reported as recall@k and MRR per class | Those three classes are the ones native declines to store |
| **P2** | **Zero-cost abstention** | For a prompt with no relevant history, the injector emits **exactly zero bytes**. Required at 100% | Native loads its index every session **regardless of the query**, so its context cost is a constant. A query-dependent zero is structurally ours |
| **P3** | **Temporal resolution** | A time-qualified query is narrowed by real event timestamps and session boundaries | Native carries a `modified` field, which is when a note was written, not when the fact was true |
| **P4** | **Cross-host single search** | One query reaches answers that exist only in the other host's sessions or memory | Each host sees half |
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

M-4 does not turn on for anyone until M5, M6 and M9 pass and M7 clears its threshold. M1–M4 and M8
are conditions on the work that precedes it.

| | Gate | What it asserts |
|---|---|---|
| **M1** | Native parse fidelity | Over every native memory file present on the machine: no crash, frontmatter fields extracted exactly where they exist, body bytes preserved losslessly. One failure fails the gate |
| **M2** | Drift canary | An unknown frontmatter key, an unknown file name, a missing index — each **warns and continues**. A silent skip is a failure |
| **M3** | P4 recall | Queries whose answer exists in only one host, 25 per host, recall@10, reported against each native memory's own ceiling |
| **M4** | Field boost earns its place | P1's three new classes, recall@10 and MRR with the derived-field boost on and off. **No improvement means the code is deleted** |
| **M5** | Hard cap | The whole corpus through the injector, zero replies over the byte cap. The cap comes from the hosts' documented budget, not from an observed p95 |
| **M6** | Zero-byte abstention | Prompts with no relevant history emit zero bytes, **100%**. One failure fails the gate. This is the direct defence against SWE-ContextBench's free-summary regression |
| **M7** | Precision at budget | A human-labelled fixture of real prompts and what should have been injected, pinned once and then a regression test. Below threshold, the feature does not ship enabled |
| **M8** | Native coverage, reported | For P1 and P5, how many questions native memory alone could answer against how many verbatim retrieval can. **This pair of numbers is the honest form of "native-grade or better"** |
| **M9** | Data fence | Every injected payload sits inside a per-injection nonce delimiter, and the delimiter never appears unescaped inside the payload. Asserted over the whole corpus, zero occurrences |

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
2. **Backlog 28, the bearer token's file permissions.** The 1.0 spec §5.9 accepts the inherited DACLs
   on the owner's machine; on a stranger's machine it is not the owner's to accept. What is the
   product's to fix is `mcp.json`, written with a security descriptor of its own on the pattern
   `internal/pipe`'s listener already uses. The two host files are not this product's to narrow, and
   `doctor` reports their permissions as a finding rather than changing them.
3. **A `README`.** There is none, and a public repository with no `README` and no licence granted
   nobody anything. The licence half closed on 2026-09-02: `LICENSE` is Apache-2.0.
