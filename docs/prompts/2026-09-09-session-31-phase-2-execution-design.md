# Session 31 — Engramux: Phase 2's execution design

You are continuing in Claude Code at D:/AI_DEV/engramux. CLAUDE.md contains @AGENTS.md and both load
for you already, so this brief restates none of their rules, commands or hazard table. Reply in
Korean; repository documents stay English.

## What this session is

**Phase 2's execution design, and M7.** It is item **B** in the priority order the owner set on
2026-09-09, and A is finished. Read the memory spec's *What M7 will measure, pre-registered before a
label exists* and the sections around it — that is the design, and this brief deliberately does not
restate it, because the spec owns decisions and a brief is never updated.

**M7 costs the owner's own time and that is the scarce input.** 150 blind judgements, and the
stratified sample, the two labels and the non-vacuity arms were all registered before a label
existed. Do not redesign what is registered because it is expensive; the expense is the point of a
blind judgement. What is open is the **execution design** — how the 150 are put to a person, in what
order, with what shown and what withheld — and that is what this session is for.

The thing standing behind it is backlog 57: **injection ships disabled, and the owner reports never
having called `search`, `get_session_resume` or the CLI once.** M8 P1 puts verbatim recall at 1.000
in all three classes over a corpus nobody queries. So the gap is delivery, and M7 is the gate
between here and turning delivery on. Nothing about M-4 turns on for anyone until M5, M6, M9 and M10
pass and M7 clears its threshold; the first four passed 2026-09-03.

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
condition. Backlog 55, 56, 58, 59 and 60 are all deferred deliberately and each row says by whom and
when.

## Authority

Publication is the owner's. A tag, a release, or a tag push is not yours to take, and if M7 clears
its threshold the decision to turn injection on is a separate one that stops and reports. Stopping
and starting the installed service is a named carve-out and so is `install --apply` under `doctor`'s
condition; both are in AGENTS.md and neither is restated here.
