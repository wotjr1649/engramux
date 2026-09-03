# Session 14 — Engramux: after the selector and what installing it found

Session 13 opened blocked on the same thing the two before it were — fifty queries only the owner
could write — and it stopped being blocked mid-session. The queries landed, gate **M3** returned
**zero**, and the rest of the session was spent finding out why. The answer was two walls, one of
which turned out to be the query builder, and **Step 7** is what came of it.

Installing it then found a second thing, and **Step 8** is what came of that: search was reading a
payload for every matching document to return twenty of them, and on the real database half of an
ordinary question set was timing out. It is fixed, gated and installed.

So this session opens with something none of the last four did: **no gate is red, nothing is waiting
on the owner, and the installation is the tree.**

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context — including the
two carve-outs about the service and about `install --apply`.

Read, in this order: `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` **rev.13**,
and in it the four sections that run consecutively — *What gate M3 measured on its first human
fixture*, *What the English arm settled*, and *What building the selector settled*. They are one
argument in three parts and the last one is the only one that changed code. Then **§5**'s **M3** and
**M7** rows, and `docs/superpowers/plans/2026-08-30-after-phase-6.md` rev.4's **Step 6** and
**Step 7**. Backlog rows **28**, **36**, **37**, **38** and **40**.

**Written 2026-09-04, by session 13.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Carries Step 7 and Step 8, both merged `--no-ff`, spec rev.11 to rev.13, plan rev.5. Pushed to `origin` on 2026-09-04 with the owner's word. `git status -sb` is the answer |
| Installed | **Step 8**'s build, which is the tree. `doctor` green at exit 0 on every section, both hosts pointing at the endpoint, spool 0 |
| Gates | **M1**, **M2**, **M4** pass. **M5**, **M6**, **M9**, **M10** pass. **M3 is pinned and green for the first time** — claude-code 0.400, codex 0.600. **M7 is un-run and un-built** and is now the only one |
| Injection | Built, **off**, and still on `MatchAll` by design (spec rev.12) |
| Suite | Green over every package. Pinned linter `0 issues.` at exit 0 |
| Backlog | **Five rows**, unchanged: **28**, **36**, **37**, **38**, **40** |

---

## 2. What to do, in order

**T1 — Build M7's harness and run the gate.** Unchanged from session 13's brief and now the only
un-run gate in the spec. It is what licenses turning injection on for anybody. The shape is
`TestWriteM3Candidates`'s: the prompts are already captured as `UserPromptSubmit` events, the
injector's output for each is mechanical, and the only column that has to be human is the verdict.
Two constraints from rev.9 before you start. The **corpus's own prompts** are the only honest source,
because a synthesised prompt measures the synthesiser. And the harness must not print an excerpt or a
prompt into a terminal — `internal/inject`'s gates are measured clean and this one has to be too.

**T2 — Step 6, the update path.** Memory spec **M-7** and the plan's Step 6. Independent of
everything above, and the only step left in the plan.

**T3 — Verify, close.** Suite, the pinned linter (check its exit code, never its summary line),
`./scripts/race.sh`, in that order and not concurrently. The plan gets a dated Done paragraph for
whatever closed, and a session 15 brief lands in this directory. **Push only after asking.**

---

## 3. What only the owner can do

**One line in their own `CLAUDE.md` / `AGENTS.md`: write memory in English.** Spec rev.12 decision 2.
It is not a product change — Engramux does not write native memory (M-2) — and `AGENTS.md` forbids an
agent editing `~/.claude` or `~/.codex`. The reasons are token cost and predictability, **not
retrieval**; the spec says why, and says that during the transition Korean queries get worse.

**Turning injection on, if they want to.** Still one file, still `doctor` prints its path, and M7
still has not run.

**Backlog 28 and the clean profile.** Both publication conditions and neither this session's.

---

## 4. Decided 2026-09-04 and not to be reopened

Every one is in spec rev.11 and rev.12 with its reasoning and its measurement. Nothing here repeats a
figure.

- **Language independence is guaranteed on MCP and the CLI and explicitly not on injection.** A
  Korean prompt receiving zero bytes from the injector is **P2 working**.
- **New memory is written in English, going forward only, for token cost and predictability.** The
  20 existing Korean items are not rewritten.
- **M3's fixture is the English one.** The Korean original is evidence, not a gate arm.
- **The join is a caller's choice.** MCP and the CLI use `MatchAny`; **the injector keeps
  `MatchAll`**, and that is structural — its abstention is a threshold on the match set's size.
- **A two-phase selector was measured and rejected.** It is worse than the plain OR, not safer.
- **M3's bar was pre-registered at 19 of 50 before the number was known.** It measured 25.
- **A window function does not share a SELECT list with a large column.** `count(*) OVER ()`
  materialises every matching row; the payload is joined onto what survives the LIMIT.

---

## 5. Open

| Open | What it decides |
|---|---|
| **Why native memory contributes nothing to injection** | Measured at **0 of 16** in Step 5, and the reason given was that the three-term AND is too narrow. Step 7 did not touch the injector, so the finding stands — but it now has a named remedy the spec has decided *not* to apply there. **M7** is what would say whether that decision costs anything |
| **Whether abstention ever happens on a real corpus** | **16 of 16** prompts injected in Step 5's run. Unchanged: the selectivity ceiling never fired, and **P2** is this product's sharpest claim measured only against inputs built to have no history |
| **The translation flaw in M3's fixture** | The English queries are the assistant's translation of the owner's Korean, answer column unread. The circularity guard survives; the register may make the numbers optimistic. `[unverified]`, and only the owner writing 50 English queries removes it |
| **What a cold read costs at 227 MB** | Unchanged and still the honest gap. Every M10 reading is warm |
| **Whether the boost's three columns are worth 18.4 MB** | Unchanged. M4's own delete condition wants a second measurement |

---

## 6. Things that will bite

1. **`git checkout -- <file>` restores HEAD, not the working tree.** Hit again in session 13, on an
   uncommitted `query.go`, while reverting a break-it mutation. `scripts/breakit.sh` refuses a dirty
   tree for exactly this; a hand mutation needs a `cp` backup, not a checkout.
2. **A test that survives a total inversion of the ranking is not measuring the ranking.** Session 13
   wrote a fixtures-mode rank ceiling, watched it pass, reversed the whole `ORDER BY`, and watched it
   pass again — in that mode a derived query matches exactly one document. Both its commit and its
   revert are on `main` on purpose.
3. **Four of spec §8's five classes derive a single-token query**, so they cannot tell `MatchAll` from
   `MatchAny` at all. Anything comparing the two modes over those classes is comparing a mode against
   itself; `TestGateSelectorEarnsItsPlace` fails when no class distinguishes them, and that guard is
   the only thing standing between it and a vacuous pass.
4. **An absolute rank cannot be pinned against `.capture/`.** It grows whenever the owner uses the
   machine, and the upper medians had already moved from spec §7.1's `3 / 3 / 10 / 9 / 30` to
   `4 / 4 / 6 / 8 / 30` with no code change between. Compare two arms of one run instead.
5. **M3's pin is a floor over a growing corpus.** A future fall is either a retrieval regression or a
   corpus that gained distractors, and no constant can tell those apart. Read the same-script and
   cross-script lines before concluding either.
6. **A backslash escape written through a script literal is not safe**, even inside `bash <<'EOF'`.
   Session 13 lost a `\t\n` to it in a table row. File-write tool, or match on a substring that
   carries no backslash.
7. **The installed service holds its database exclusively (I-07).** Every figure in rev.12 is over
   `.capture/` and native memory, not over the 18,000-event corpus the machine has.
8. **A live MCP client caches the reply schema.** Steps 7 and 8 change no schema and add no tool, so
   a session open across the upgrade keeps working — but it will start getting different *results*,
   which is the change nobody's cache protects them from.
9. **A benchmark over the synthetic corpus can be right and useless.** Spec §7.1 priced the search's
   worst shape at 24.2 ms and the real machine took 4 s, because the synthetic documents are a few
   dozen bytes and the real ones average about 11.5 KB. Anything measuring cost against document
   size has to say which corpus, and a ratio between two corpora in one run is what survives it.
   `AGENTS.md` carries the row.

`AGENTS.md`'s own table carries the rest.

---

## 7. Done when

M7's harness exists and its gate has either run or been reported as un-runnable with the reason;
Step 6 is done or explicitly deferred with what is left; suite, pinned linter and race script are
green; the plan says what closed
with the evidence; and a session 15 brief exists.
