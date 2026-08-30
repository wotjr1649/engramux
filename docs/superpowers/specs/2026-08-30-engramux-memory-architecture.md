# Engramux — memory architecture, after 1.0

**rev.1** · 2026-08-30

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
| **M-6** | **`doctor` judges by stage.** "Not installed yet" and "installed and broken" become different answers with different next commands; MCP becomes optional rather than required for a green result; the eleven hook entries are checked | Decided |

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
configurable, so it must be **resolved from settings and never hardcoded**. Codex's file names are in
its own repository but the line-level schema of its index is **[unverified]** — nobody in this
session read one. The parser therefore follows the rule rev.4's §4.4 already imposes on
`tool_response`: **preserve a shape you do not recognise, warn, and continue.** A silent skip is a
failure, not a fallback.

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

### `doctor` by stage (M-6)

Three changes. "Not installed yet" and "installed and broken" become different answers, each naming
the command that moves it forward — today a fresh machine gets four failing sections and no
instruction anywhere. MCP becomes **optional**: a deliberate capture-only installation is a supported
state and must be able to be green. And the eleven hook entries are checked — that they exist, and
that they point at the installed relay — which is the one thing a working install actually depends on
and the only major surface `doctor` does not look at.

Two things `doctor` already does that must not regress: it reports the tokenizer as a **verdict**
rather than as two strings to compare, and it explains a locked destination with the right remedy per
file. Two it does that should be reconsidered while it is open: it prints a Windows SID and the real
database path, in the output a user is most likely to paste into a public issue.
