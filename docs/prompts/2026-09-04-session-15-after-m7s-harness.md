# Session 15 — Engramux: after M7's harness and the first real-corpus reading

Session 14 opened on the first brief in five that had nothing red and nothing waiting on the owner.
It closes with two things waiting on the owner and a finding that reverses one of the spec's open
questions.

The session was scoped by an adversarial review before any code was written — two subagents and
Codex, each on a different axis — and that review is why almost nothing in session 14's own brief
was built the way it described. Three of its findings killed a design, one killed a bar, and one
killed a claim about what M7 licenses. The work that followed is what survived them.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context — including the
two carve-outs about the service and about `install --apply`.

Read, in this order: `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` **rev.14**,
and in it *What M7 will measure, pre-registered before a label exists* and *What the first reading
over a real corpus said* — the second answers a question the first was written around. Then **§5**'s
**M7** row, and `docs/superpowers/plans/2026-08-30-after-phase-6.md` **rev.6**'s Step 6. Backlog
rows **28**, **36**, **37**, **38** and **40** are unchanged.

**Written 2026-09-04, by session 14.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Carries the memory ratio gate, spec rev.14 and M7's harness. **Not pushed** — session 14 did not ask |
| `step-6-update-path` | Carries Step 6's local half. Merge it `--no-ff` or continue on it; session 14 left it unmerged |
| Installed | Step 6's build, verified by `update` itself. `doctor` exit 0, both hosts on the endpoint |
| Gates | M1, M2, M3, M4, M5, M6, M9, M10 pass. **M7 is built and un-run**, and what it now waits on is 150 human labels rather than a harness |
| Injection | Built, **off**, still `MatchAll` |
| Suite | Green over every package. Pinned linter `0 issues.` at exit 0 |

---

## 2. What only the owner can do, and the first one is new

**Label M7's fixture.** Two files, in this order, and the order is the measurement:

1. `.capture/m7/prompts.tsv` — 150 rows, already written. Replace each `TODO` with `yes` or `no`:
   *given only this prompt, does earlier work on this machine plausibly hold something worth putting
   in front of the model?* **Answer from the prompt alone.** The file that shows the injector's
   output does not exist yet, deliberately — a label written with the answer visible measures the
   answer.
2. Then run pass 2, which writes `.capture/m7/blocks.tsv`, and label each block `yes` or `no`: *was
   this excerpt worth the bytes it spent?* Expect roughly 25 injecting prompts and perhaps 130
   blocks, because most prompts abstain.

Then the gate runs. The bar is pre-registered and moving it is a spec revision.

**Do not delete `.capture/m7/snapshot/`.** It is the frozen corpus the fixture is keyed to — 256 MB
plus its WAL — and the gate refuses loudly rather than quietly when an emitted block has no label,
which is what happens if the corpus moves. Re-taking it means re-labelling.

**Step 6's release half.** A tag, a GitHub Release, a marketplace entry, and the CI workflow. Every
one is a publication act. `.github/` does not exist.

**One line in their own `CLAUDE.md` / `AGENTS.md`: write memory in English.** Unchanged from session
14's brief. Spec rev.12 decision 2, and the reasons are token cost and predictability, not retrieval.

**Backlog 28 and the clean profile.** Both publication conditions, both unchanged.

---

## 3. What session 14 found, and what it changes

**Abstention is the common case on a real corpus, and nobody knew.** Measured over the snapshot: **5
of 30 prompts injected and 25 abstained**, where `.capture/fixtures-raw` gave 16 of 16 with no
abstention at all. **24 of the 25 matched nothing**, and the selectivity ceiling this design was
built around fired **zero times**. So the reduction to three ANDed terms is not too broad on a real
corpus, it is too narrow — the same narrowness that put native memory at 0 of 16.

Whether that is **P2 working** or a **miss** is exactly what the `should_inject` labels decide, and
that is the sharpest reason to write them.

**The 500 ms budget holds where it had never been tested.** M10 worst **42.05 ms** over documents
that are the machine's own, against every earlier reading being over documents a few dozen bytes
long. That is the half rev.13's finding demanded and it came back fine.

**One statement was fixed and ungated.** Step 8 fixed two SELECTs and gated one; the memory statement
had no arm at all, and the injector calls both on the one path with a deadline. It has one now — and
writing it found that the event arm's ceiling of 3 is a number about its corpus as much as its
statement, because at limit 20 the per-returned-row cost dominates and only a big match set hides it.

---

## 4. What to do, in order

**T1 — merge `step-6-update-path`.** `--no-ff`. Nothing is blocking it; session 14 stopped before
merging because the branch's own step is not closed and that deserved a decision rather than a habit.

**T2 — the CI workflow.** It changes nothing the binary does, so by this repository's rule it lands
on `main` whenever somebody writes it, and rev.3 said it is worth writing *before* the release half
rather than during it. It is the one piece of Step 6's remainder an agent can do alone.

**T3 — whatever the M7 labels say.** If they are written, run pass 2, then the gate. If the gate is
red, read *What the number licenses* before concluding anything: it measures the precision of what
was emitted and cannot see what was missed.

**T4 — push, after asking.** `main` and the branch are both local.

---

## 5. Open

| Open | What it decides |
|---|---|
| **Whether the three-term AND is too narrow** | 83% of prompts receive nothing. `should_inject` is the instrument and it is unwritten |
| **Whether M7's retrieval arms can ever be conclusive** | Both report *inconclusive* at 7% and 8% labelled overlap, by their own honesty condition. Making them conclusive means labelling the whole candidate pool — roughly 30 blocks per prompt rather than the handful that fit the cap |
| **What a cold read costs at 256 MB** | Unchanged and still the honest gap. Every M10 reading is warm, including the new ones |
| **Whether the boost's three columns are worth 18.4 MB** | Unchanged. M4's own delete condition still wants a second measurement |
| **The translation flaw in M3's fixture** | Unchanged, `[unverified]`, and only the owner writing 50 English queries removes it |

---

## 6. Things that will bite

1. **A backslash escape through a script literal is not safe.** `AGENTS.md` has the row and session
   14 hit it **twice more** — once losing an edit silently and once with a Python `SyntaxWarning`
   that was the only clue. Anything with a backslash goes through a file-write tool. There is no
   third chance worth taking.
2. **`filepath.Dir` cleans its own result, and so does `filepath.Join`.** A test that builds an
   "uncleaned" path with either of them asserts nothing about a `filepath.Clean` around them. A
   break-it pass caught the first two attempts at one such case; the argument that actually
   distinguishes them is the one that did *not* come from a filepath call.
3. **`-trimpath` removes the `-ldflags` line from `go version -m` entirely.** Measured. The release
   build line is the one build where that diagnostic is blind, so a silently no-op `-X` is invisible
   there. Run the binary and read the version it reports.
4. **A flag's value survives as a positional.** `taskName` reads the first argument without a dash,
   so `--from <dir>` made the first real `update` ask Windows for a task named after a directory. A
   flag that takes a value has to consume both arguments. A test owns it now.
5. **A non-vacuity arm can be vacuous in either direction.** Session 14 registered three and two were
   worthless: a shuffle keyed by `(prompt, block)` always scores zero, and an arm that retrieves
   different documents scores unjudged — which is irrelevant — and passes trivially. Registering an
   arm is not the same as it measuring anything; make it report how much of what it emitted anyone
   ever looked at.
6. **The corpus moves while you measure it.** 20,075 to 20,993 events inside one session, on the
   machine doing the measuring. Anything pinned against the live database is not re-derivable. The
   snapshot exists for this.
7. **A mutation that removes the last reference to a variable does not compile**, so a break-it pass
   reports nothing and it reads like a weak test. `AGENTS.md` has the row; session 14 hit it on
   `want` in `updateTaskRefusal`. Change the answer, not the reference.
8. **`scripts/race.sh`'s contention arm is roughly one-in-five red on this machine** and has been
   since before any of this work. Backlog 38. Re-run once and say which arm before believing it.

`AGENTS.md`'s own table carries the rest.

---

## 7. Done when

Step 6's branch is merged or explicitly carried; the CI workflow exists or is explicitly deferred;
M7 has either run against a labelled fixture or is reported as still waiting on labels with the count
outstanding; the suite, the pinned linter and the race script are green; the plan says what closed
with the evidence; and a session 16 brief exists.
