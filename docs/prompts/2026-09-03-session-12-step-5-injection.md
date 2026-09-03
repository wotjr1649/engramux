# Session 12 — Engramux: Step 5, and the gate M3 was never going to pass

Session 11 did its brief's T2 and T3 and then went somewhere its brief did not name. It unblocked
the CLI, installed Step 4, measured what `00005` costs over the live database, built a generator for
gate **M3**'s fixture, and reversed one of rev.5's delivery decisions. What it did not do is **Step
5**, which was its T4, so it closed without the T5 that would have written this file.

A short session between the two read that transcript and grilled what comes next rather than
building it. Six things were decided; one of them — the two sentences in §8 that still recommended
what rev.7 had just declined — was the only thing it changed in the tree, and it changed it because
a document contradicting itself is a defect and not a decision. The other five are in the memory spec
**rev.8** and in backlog **41**, which is where values live. **This brief owns none of them.**

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context — including the
two carve-outs about the service and about `install --apply`.

Read, in this order: `docs/superpowers/plans/2026-08-30-after-phase-6.md` **rev.3**, whose **Step 5**
is what this session executes and whose Step 4 now carries a Done paragraph;
`docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` **rev.8**, and in it the new
**M-4** section, which is the whole of what Step 5 may spend and may not select, then **§5** for
**M5**, **M6**, **M7**, **M8**, **M9** and the new **M10**, then **§6**, which is design input and
not background: it is the section that says injecting captured content is an injection vector, and
**M9** is the one mitigation in its list that does not depend on a model behaving well. Last, backlog
rows **36**, **38** and **41**.

**Written 2026-09-03, by the short grilling session that followed session 11.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | One commit ahead of `origin/main` at the moment this was written — the spec rev.8, backlog 41, the §8 repair and this file. Nothing in it changes the binary. **The owner was not asked to push.** `git status -sb` is the answer |
| Installed | **Step 4**, on migration `00005`. `doctor` green on every section: both binaries present, 11 of 11 hook entries per host pointing at the installed relay, the MCP endpoint listening and both hosts pointing at it, spool 0, errors 0 |
| Last verification | Run in the grilling session, not inherited: the suite **17 packages at exit 0, zero `FAIL`**; `TestGateM3CrossHostRecall` **skips**; five `engramux search` runs against the installed service took **93, 113, 185, 245 and 251 ms**. The pinned linter and `./scripts/race.sh` were **not** re-run, because nothing shipped changed |
| Gates | **M1**, **M2** pass in the normal suite. **M4** passes over the corpus and skips without it. **M3** still skips, and rev.8 changed what it will assert when it stops |
| M3's fixture | `.capture/m3/candidates.tsv` exists, **120 candidate lines**, 60 per host, every answer column filled and verified and every query column still `TODO-WRITE-THE-QUERY`. The owner writes those **in this session** |
| Backlog | **Six rows.** **28** a publication condition, **36** a memory item's title, **37** the Defender quarantine, **38** the contention gate's margin under `-race`, **40** the duplicated-key divergence, **41** now decided and waiting on Step 5 to implement it |
| Branches | `origin/step-2-engramux-install` is still on the remote; deleting it is the owner's remote change |

---

## 2. What to do, in order

**T1 — Read §1, then `git status -sb`.** Step 5 is **a branch per plan step, merged `--no-ff`**,
named `step-5-injection`. Documentation and test-only work goes to `main` directly, and T2 and T5
below are both that.

**T2 — Change gate M3's shape, on `main`, before anything else.** rev.8's M3 row is the decision and
this is the work: the gate measures recall@10 against each host's ceiling, and asserts against a
**pinned** number rather than against 100%. Until a fixture exists it still skips, so the pin is
absent and the constant that holds it says so. The reason is in the spec and is not to be
re-litigated: the gate as written could only be green or red, because its own documentation named the
*query* as the thing to change when a query missed. Do T3 the moment this lands, not after Step 5.

**T3 — Hand the owner `candidates.tsv` and do not wait on it.** They fill the query column and save
what they keep as `.capture/m3/queries.tsv`. This is the only thing in the session that blocks on a
human, and everything below is independent of it — start T4 while it is out.

**T4 — Execute Step 5.** Memory spec **M-4**: hook-time injection, built, and **shipped disabled**.
rev.8's M-4 section already settles which hook, which hosts, the two budgets, the self-reference
exclusion and the absence of a migration, so none of those is a decision this session takes again —
what it takes is the ones rev.8 does not name, and **M9**'s delimiter is the first of them. Its gates
are **M5**, **M6**, **M9** and **M10**, with **M8** reported rather than passed, and **M7** is what
would license turning it on for anybody — which this session does not do. For every test that guards
an invariant: write it, watch it fail, implement, watch it pass, break the implementation on purpose,
watch it fail, revert. One commit per decision. **Commit before every break-it pass** — §6's third
row.

**T5 — Pin M3's number, on `main`, the moment `queries.tsv` lands.** Run the gate, read the recall it
reports, write that number into the test's constant and into the spec's M3 row, and say in the spec
what corpus and what date it was measured over. This is the act that turns M3 from a skip into a
gate, and it is the one Step 3's done-condition has been waiting on since 2026-09-02.

**T6 — Build M7's harness, and cut this first if the session will not close.** The same shape
`TestWriteM3Candidates` has and for the same reason: M7's fixture is real prompts and what should
have been injected, the prompt side is already captured as `UserPromptSubmit` events, the injector's
actual output is mechanical, and the only column that has to be human is the verdict. It cannot be
built before T4, and it is the one piece here that moves to its own session without leaving anything
half-built.

**T7 — Verify, install, merge, close.** Suite, the pinned linter (check its exit code, never its
summary line), `./scripts/race.sh`, in that order and not concurrently. Merge `--no-ff`, the plan
gets a dated Done paragraph, and a session 13 brief lands in this directory. **Push only after
asking.**

---

## 3. What only the owner can do

**Gate M3's fifty queries.** Unchanged in kind from the last two briefs and changed in one way that
matters: the file to write them into now exists, with the answer column already filled and verified
the way the gate verifies it. Write the query **from memory rather than from the answer beside it** —
a query cut from the answer's own words measures the tokenizer, which another gate already measures.
The owner said 2026-09-03 that this happens inside this session, which is why T5 exists.

**Backlog 28 and the clean profile.** Both are publication conditions and neither is this session's.

---

## 4. Decided 2026-09-03 and not to be reopened

Every one of these is in the memory spec **rev.8** with its reasoning and, where there is one, its
measurement. Nothing here repeats a figure from it.

- **M3's gate changes shape** — measured once, pinned, then a regression test on the pin, which is
  M7's shape and for M7's reason.
- **The false-positive submission stays declined**, and §8's fourth condition is carried by
  documentation alone. rev.7 decided it; the two sentences that still recommended it are gone.
- **Injection attaches to `UserPromptSubmit` and to nothing else**, on **both hosts**, whose
  documented shape turns out to be identical.
- **Injection's time comes out of the relay's existing second, not on top of it**, and gate **M10**
  is what measures it. The number and what is still unverified about it are in M-4.
- **M5's cap is a number now**, and rev.8 records which host documented one, which documented none,
  and how tokens became bytes.
- **Engramux's own events are not injectable**, excluded in the selector and identified by the
  binary rather than by the string. Backlog **41** points at the decision.

---

## 5. Open

| Open | What it decides |
|---|---|
| **What a cold read costs at 227 MB** | The five latency readings behind M-4's 500 ms are all warm, and every read-deadline failure the 1.0 spec §7.1 records was a cold read after an idle period against a smaller database. **M10** is the instrument. If the abstention rate it reports is high, the answer is not to raise the budget — it is that injection is a feature that fires when the machine is warm, which is a finding and not a fault |
| **M9's delimiter** | rev.8 settles what injection may spend and select, not what the fence looks like. It is the only mitigation in §6's list that does not rest on a model behaving well, so it is worth more design than the rest of Step 5 put together |
| **Backlog 36, a memory item's title** | Still nobody's, and Step 5 surfaces it the moment an injected excerpt carries one |
| **Whether the boost's three columns are worth 18.4 MB** | M4 passed and passed small. Its own delete condition is written to be applied on a second measurement rather than a first, and the size figure now exists |

---

## 6. Things that will bite

1. **`TestGateM3CrossHostRecall` cannot pin a number it has not measured**, and a gate that asserts
   against an absent pin is a gate that passes. T2 lands the shape with the pin explicitly unset and
   the skip intact; T5 sets it. Getting that backwards produces a green gate that checks nothing,
   which is the exact failure the shape change exists to remove.
2. **A red `scripts/race.sh` is not evidence of a regression until you have a baseline.** Backlog
   **38**: the Phase 5 contention gate fails about one run in five on this machine without Step 4 or
   Step 5 in it. Measure the baseline before fixing anything, and never fix this one by moving the
   number.
3. **`git checkout -- <file>` restores HEAD, not the working tree.** Commit before every break-it
   pass. A mutation that never applied is a third state that reads like a pass, and a mutation that
   removes the last reference to an import or a local does not compile and is discarded rather than
   counted as killed.
4. **The suite is slow and `internal/search` under `-race` is around 757 s.** Start the race script
   in the background and read its output file; do not pipe it through `tail`, because nothing is
   written until the pipeline ends and a fifteen-minute silence looks exactly like a hang.
5. **`TestPhase4Gate`'s corpus mode logs one line carrying a real path.** Redact everything after
   `candidate documents: ` before any of its output goes into a report, a commit message or a chat.
   The M3 and M4 gates and the sweep are measured clean; that one is not.
6. **A relative path in a test resolves against its own package**, not the repository root.
7. **A live MCP client caches the reply schema**, so a session open across an upgrade rejects a reply
   whose shape changed until it reconnects. The service logs nothing, because it is the client
   validating.
8. **Injection is the first thing this product does on the user's critical path.** Everything the
   relay does today is fire-and-forget: a relay that cannot reach the service spools, and the drain
   replays. A relay that waits for an injection has no such door, which is why M10 asserts the
   deadline against a deliberately slow search and not only against the corpus.

`AGENTS.md`'s own table carries the rest.

---

## 7. Done when

M3's gate has its new shape and a pinned number measured over the owner's own fixture; **M5**, **M6**,
**M9** and **M10** pass and **M8** is reported; injection is built and ships **off**, with **M7**
un-run and un-claimed because licensing the switch is a separate act; suite, pinned linter and race
script are green; `step-5-injection` is merged `--no-ff`; the plan says Step 5 is done with the
evidence; and a session 13 brief exists.
