# Session 31 — Engramux: Phase 2's execution design

You are continuing in Claude Code at D:/AI_DEV/engramux. CLAUDE.md contains @AGENTS.md and both load
for you already, so this brief restates none of their rules, commands or hazard table. Reply in
Korean; repository documents stay English.

## What this session is

**Phase 2's execution design, and M7.** It is item **B** in the priority order the owner set on
2026-09-09, and A is finished.

**"Phase 2" is not a term the spec or the plan defines** — checked, `grep -rn "Phase 2" docs/` finds
it only in briefs — so read **session 29's brief**, `docs/prompts/2026-09-08-session-29-the-release-before-the-injector.md`,
for what it contains. Its *Out of scope* section is the closest thing to a scope statement anyone has
written: **productionising the continuation candidate out of `internal/inject/continuation_test.go`
into a product path, the labelling CLI, any M7 run, and the 108-prompt holdout and its manifest.**
That is the only reason to open an old brief here; everything else in it is a record of that session.

Then read the memory spec's *What M7 will measure, pre-registered before a label exists* and
*What ends M-4, and not only what starts it*. This brief does not restate either, because the
spec owns decisions and a brief is never updated.

**Read that second section before you plan anything, because it changes what this work is.** M7 is
not only an activation gate. Since 2026-09-08 M-4 carries an abandonment condition: owner-labelled M7
runs over its pre-registered 150 prompts, and if the relevant-byte share does not clear the **0.50**
bar, the candidate is narrowed **once** and M7 re-runs over the same 150. **A second miss deletes
M-4 and the injection code** — `internal/inject`, the `inject.json` switch `cmd/engramux` reads, and
the row. Backlog 55 and 56 are pre-registered as that one retry rather than as work to do before it.
Three things are fixed by this having been registered in advance and each is a way to break it: **the
candidate is not narrowed a third time, the bar is not moved, and a fresh prompt population is not
drawn.** The 108-prompt transfer holdout stays **sealed** — `docs/evidence/transfer-split-2026-09-08.md`
records the split — and is a check for after the feature is on, never a second attempt at this gate.

**What deletion would not remove is the pull path.** M-4 is the push path alone; the CLI and MCP
serve retrieval whether or not injection ever ships.

**M7 costs the owner's own time and that is the scarce input.** Pass 1 is **150 blind prompt
judgements**; pass 2 is roughly **14 block judgements**. The stratified sample, the two labels and
the non-vacuity arms were all registered before a label existed. Do not redesign what is registered
because it is expensive; the expense is the point of a blind judgement. What is open is the
**execution design** — how those judgements are put to a person, in what order, with what shown and
what withheld — and that is what this session is for.

**The figures that already exist are agent estimates and must never be relabelled.** The continuation
candidate measured 75.10% known-relevant bytes, zero bytes on prompts labelled as not wanting
context, and 14 emitted blocks over the 150-prompt temporal replay — **agent judgements over exposed
development data, not an owner verdict and not an M7 pass.** The full-snapshot agent M7 failed at
**0.032** against a bar above 0.50. The spec's *Owner judgements and agent estimates are different
evidence* is the section on this. Quoting either set as a result is the specific mistake to avoid.

The thing standing behind all of it is backlog 57: **injection ships disabled, and the owner reports
never having called `search`, `get_session_resume` or the CLI once.** M8 P1 puts verbatim recall at
1.000 in all three classes over a corpus nobody queries. So the gap is delivery. M5, M6, M9 and M10
passed 2026-09-03; M7 is what is left, in both directions.

## Verified repository state, 2026-09-09

`git rev-parse HEAD` was `1944ef3` before `d23387c`, which is this brief's own commit; the
branch was `main`; `git status --porcelain` was empty apart from this file. Recheck before acting on
any of it. Session 30 pushed, so `origin/main` and `main` should agree — if they do not, find out why
before writing anything.

**Checks that last passed, 2026-09-09.** `internal/store` green. `internal/search` green at 856 s,
which is 218 s of suite plus gate M15. The pinned linter at `0 issues.` and **exit 0**, checked by
exit code rather than by the summary line. `scripts/race.sh` was **not** re-run this session — see
below, because that is a live obligation rather than an omission.

## What session 30 left you, in one paragraph each

**M15 answered 0.00% and the branch closed.** Session-level eviction removes no bytes at all: every
one of 46,806 events is reachable, because the `a path basename` class derives a candidate from all
of them and reaches all of them. The eviction code is not written. What is worth more than the number
is what it says about the instrument — "unreachable by a query cut from the document's own text" is
an upper bound that approaches zero whenever one class fires on everything, so it cannot separate a
valuable event from a worthless one. Backlog 59 is that question and its deferral has lapsed; the
owner's order still puts it fifth.

**M16 was refused by SQLite rather than by FTS5.** `leaves` cannot become a VIRTUAL generated column:
subqueries are prohibited in generated columns and the walk needs one. FTS5 — the risk the spec named
— accepts a generated column, rebuilds over it and matches through it. The duplicate stays, and the
36% with it.

**Two gigabytes came off the promise.** Compaction's deferral now ends on the owner's judgement. The
file size stays as a leading indicator that **nothing reports** — that is backlog 60, which carries
the commands that answer it and the two places the mechanism is already written — and `status`'s
`errors` is the lagging signal that already ships, with its two limits written down.

**Gate M15 carries `//go:build !race`.** Priced at the multiplier `scripts/race.sh` says to use it
would be about 250 minutes against a 90-minute guard. That script's comment block argues the
exception and it is narrow: the file has no goroutine, so the detector has nothing in it to find. **A
gate you add that does have one is covered by the original rule — re-measure and raise the guard in
the same commit.**

## The obligation session 30 did not discharge

**`scripts/race.sh` has not been run since gate M15 was added**, and the build tag is the reason it
would now pass rather than evidence that it does. The last full reading is 2026-09-07, 69m13s over 22
packages. If this session adds or changes anything in `internal/search`, run it and record what it
said; if it does not, say that it was not run rather than inheriting the old number.

## Where the rest of the decisions live

The order the owner set is **A** the M15/M16 measurement (done), **B** this, **C** the deletion and
inventory CLI, **D** the long-term selection rules, **E** backlog 59, **F** a viewer, **G** the exit
condition. Backlog 58, 59 and 60 are deferred deliberately and each row says by whom and when.
**Backlog 55 and 56 are not in that list**: they are inside the injector and are pre-registered as
this work's one retry, so they are read when the first M7 misses and not before.

## Out of scope, named so it is not drifted into

**The 108-prompt transfer holdout.** It is sealed and it is not a second attempt at this gate.

**Narrowing the candidate before M7 has missed**, moving the 0.50 bar, and drawing a fresh prompt
population. All three are the pre-registration, and each is a way to turn a registered gate into an
unregistered one.

**Deleting `internal/inject`** even if the second miss condition is met in this session. That is a
destructive change to a shipped component and it is the owner's to authorise; measure, report, stop.

Items C through G of the owner's order, and the eviction code M15 closed. Compaction and value
truncation, which the memory spec defers and which cannot be automatic for the reason recorded there.
Backlog 60's size reporting, which is a candidate to fold into C rather than a detour before B.

No raw capture, private history or `.capture/` content goes into a commit, a document, a report or a
delegate.

## Authority

Publication is the owner's. A tag, a release, or a tag push is not yours to take, and if M7 clears
its threshold the decision to turn injection on is a separate one that stops and reports. Stopping
and starting the installed service is a named carve-out and so is `install --apply` under `doctor`'s
condition; both are in AGENTS.md and neither is restated here.
