# Session 22 — Engramux: the instrument that was not there

**Session 20 ran long and executed its own work order for session 21.** The document beside this one,
`2026-09-06-session-21-what-is-left-of-the-first-run.md`, is a record of what was planned and is
correct about the moment it was written; everything it asks for is done. Read this one to start.

What it asked for was M11's non-vacuity arm, and what that arm found changed the shape of the work:
the ranking defect backlog 48 described is real and **larger** than the six hits it was filed on, and
the weight it proposed **fails its own gate at every weight in the sweep**. Both halves are measured,
both are pinned, and the row is closed. What replaces it is backlog **53**, which is a question
rather than a fix.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read, in this order: memory spec §5's **M11 section** — the two arms, the table and the paragraph
titled *What the two arms say together* — then `docs/superpowers/backlog.md`'s row **53**. Session
21's work order is superseded by this document.

**Written 2026-09-06, by session 20.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `60339da` plus this document; tree clean, four `--no-ff` merges this session. **Eighteen commits are unpushed** — `1faf88c` was already ahead when the session opened. `origin` is public and a push is a fresh ask |
| Checks | Suite **exit 0, 21 packages**; pinned linter **`0 issues.` exit 0**; `scripts/race.sh` **exit 0, 21 packages** — in that order and not concurrently. The first two ran on `7fb92bb` and race on `60339da`, whose trees `git diff` reports as identical |
| Installed | **`0.0.0-dev+60339da107a9`, which is `main`.** Reinstalled twice this session under `doctor`'s own precondition; spool 0, errors 0, both hosts at 11 of 11 events |
| Gates | M1–M6, M9, M10 pass. **M7 is un-run**: `prompts.tsv` is 150 of 150 `TODO`. **M11 ran and its answer is that no weight ships** |
| Publication | 1, 2 and 4 closed. **3 stays open** and is a human walking the Defender exclusion through the Windows Security UI |
| Backlog | **37, 38, 40, 46, 51, 52, 53** open. 36, 47 and 48 are closed |
| Injection | Built, off, untouched |

---

## 2. What session 20 closed

**47** — `sessions` with no argument is corpus-wide, matching `search`; the project root travels per
session because a listing spanning projects has no single one. **36** — a memory item's title is the
first line that is not a label this reader recognises, out of a set measured over the 55 rollout
summaries on this machine; the Codex call site moved with it, because a title taken from the indexed
body was the bare thread id on all 55 header blocks. **48's display half** — an excerpt's edges move
to a word boundary rather than cut a word in half.

**A flake that was not a flake.** `internal/host`'s backup test failed once mid-session and passed
200 reruns. `backup` named its file from the clock and wrote it with `os.WriteFile`, so two copies
taken inside one tick shared a name and the second destroyed the first. Measured: **200,000 samples
of that stamp expression produce 75 distinct names**, and in a loop shaped like `backup`, **128 of
2,000 consecutive pairs collide**. `AGENTS.md` carries the row; the fix is `O_EXCL` with a counter.

**48's ranking half — the part worth reading.** Two arms, both pre-registered before they were run,
both committed before their numbers existed.

*The non-vacuity arm.* Over the 901-document corpus, 82.2% of documents carry neither a `prompt` nor
a `last_assistant_message`. A person's prompt is outside the top ten for **its own most distinctive
word 64.7% of the time**, a reply 45.3% — and where a prompt is buried, the top ten is *entirely*
machinery in 11 of 11 cases. A perfect down-weight would rescue 41.2% and 42.4% of queries, computed
exactly and without choosing one.

*The gate.* Five classes — two gain, and M4's three unchanged as the harm arm — at 0, 1, 2, 3, 4, 5,
20 and 100. Both baselines reproduced their source gates exactly. **No weight qualifies**: `a touched
path` loses one document of 25 at weight 1 and never recovers, where weight 5 would have bought 31
replies of 139. A supplementary run over every candidate rather than 25 confirmed that document was
representative — the harm is monotone and reaches all three harm classes by weight 5.

**The finding is the sentence the table forces.** The machinery is not in the way by mistake. The
document that ran the command genuinely contains the path, so lifting the prompt above it costs the
command line its own place. **What is missing is a signal, not a coefficient.** That is row 53.

---

## 3. What is left, and what each one is blocked on

**53 is the interesting one and it is not a fix.** It asks for a way to tell a document that is
*about* the query from one that merely contains it — how much of the document the query accounts
for, where the match falls, whether it is in text a person wrote or a field a tool filled. None of
that is in the index: `events_fts` holds string leaves and nothing about their provenance. **So it
is a schema and indexing question before it is a ranking one**, and the honest first step is to
decide what could be indexed at all, not to reach for another coefficient. M11's harness is there to
measure whatever comes of it: two arms, five classes, every figure pinned.

**46 has a precondition and it has not moved.** `ReasonNoHits` covers three situations and
`ReasonTooBroad` absorbs a fourth, so the service log cannot tell recall from silence. Its row says
what a fix is. But the gate re-injects and **M7 is un-run against a frozen snapshot**, so changing
`internal/inject` means reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first —
session 15 deferred it for that reason and the reason still holds.

**M7 itself is the largest open thing in the project.** 150 of 150 prompts unlabelled, and nothing
about injection ships until it clears its threshold.

**38 and 40 are low value.** 38's margin is already gone and what survives is a design question about
whether its load constants were sized against a machine rather than against the clause. 40's own row
says there is nothing to repair, only a choice, and a test already pins the divergence.

---

## 4. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through `ENGRAMUX_M7_DIR` first.

**M11's weight does not reach the injector**, and the seam is written so it cannot by accident:
`Search` passes 0 and the only caller that does otherwise is a test-only alias. If a future session
ships a ranking term, that decision has to be made again explicitly, because `internal/inject` calls
the same function and M7 is measured against today's ranking.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed**, and `origin` is public. `git log -p` your own diff for
`Users\<name>` before asking for a push.

**The owner's memory files are not fixtures**, and neither are their prompts. `internal/memory`'s
corpus test and both M11 arms read private text and print counts only. A failure message that
carries a title, a prompt or a derived query carries a line of one — the M11 arms are under the
stricter of the two rules for that reason, since every query they cut comes from a prompt or an
assistant message.

**`~/.codex/hooks.json.manual-backup`** is a hand-made leftover from session 18. The user's to delete.

---

## 5. Things that will bite

1. **Pre-register a bar before you can see the number, and then do not move it.** M11 failed by one
   document out of 25, against a gain of 31 out of 139. Both figures were known only after the
   condition was committed, and the condition is why the trade was not quietly made. Where a
   measurement decides whether to build something, write the bar down in its own commit first.
2. **A pin on a verdict is not a pin on the measurement.** Both M11 arms pin every count and not just
   the answer, because the answer is "none" and three mutations that leave it "none" change every
   number behind it. The same hole exists in any closed-set test: deleting a member makes the thing
   it classified stop being *recognised*, so the corpus assertion goes quiet and only a unit test
   sees it.
3. **A skip guard written against the function under test skips exactly when that function is
   broken.** Ask the environment, or the files, directly.
4. **A killed `go test` run does not say it was killed.** It leaves
   `compile.exe: exit status 0xc0000142` and `[build failed]` per package, which reads exactly like a
   broken toolchain. Seen twice.
5. **`git add -A` sweeps in whatever the session left lying about**, and `origin` is public. Write
   scratch files outside the repository.
6. **The harness's heredoc collapses a backslash even with a quoted delimiter**, and Python's own
   escapes collapse a second time. Anything with a backslash goes through a file-write tool. The
   shell guard also refuses a compound command whose here-doc it cannot statically close — put the
   script in a file instead.
7. **A long run of letters in a search fixture is a secret.** `internal/secret`'s opaque rule is 40 or
   more of `[A-Za-z0-9+/]`, and an excerpt is cut from the *masked* document.
8. **`internal/search` is about twenty minutes under `-race` and about a minute without**, and M11's
   two arms add roughly 50 seconds to a plain run. It is not hung.
9. `go test` without `-p 1` is refused by a guard, on the command line and not in `GOFLAGS`.
10. **Do not paste a `TestPhase4Gate` corpus-mode run anywhere.** One line of it carries a real path.
11. **Four orphaned processes from 2026-08-31 survive `taskkill /F` unelevated** — same account,
    higher integrity. WINPIDs 8916, 6804, 17272 and 2776 as of 2026-09-05 23:37.

---

## 6. Done when

Whatever is taken has a test watched failing under a mutation that changes the answer; each behaviour
change is a branch named for its step, merged `--no-ff`; the suite, the pinned linter and the race
script are green in that order and not concurrently; and a session 23 brief exists.

**Three things stay one user action away and none is an agent's to take.** The eighteen unpushed
commits need a fresh authorisation. Publication condition 3 needs the Defender exclusion walked
through the Security UI. Backlog 52 needs one Codex turn with the failing `Stop hook` line read for
whose hook it names.
