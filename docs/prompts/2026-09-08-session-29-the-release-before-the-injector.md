# Session 29 — Engramux: the release before the injector

You are continuing in Claude Code at D:/AI_DEV/engramux. CLAUDE.md contains @AGENTS.md and both
load for you already, so this brief restates none of their rules, commands or hazard table. Read
them from the harness rather than from here. Reply in Korean; repository documents stay English.

This session is **Phase 1 of a two-phase plan the owner settled on 2026-09-08**. It is four pieces
of agent-side work plus one spec revision, and it ends at a list of owner actions you present
rather than perform. **Do not start Phase 2.** Phase 2 is the injection work and it is scoped
below only so that you do not undo its prerequisites.

## The decisions behind this, settled by the owner and not open here

Session 28 ran a five-round grilling interview and the owner answered every branch. Re-opening any
of these costs the session; if evidence forces one open, say which answer it contradicts and stop
rather than deciding it yourself.

1. **The goal is to turn hook-time injection on for the owner's own machine**, on a horizon of
   weeks rather than indefinitely. Everything else is subordinate to that.
2. **The route is a narrow injector, not repair of the broad selector.** The broad path needs
   0.032 to reach 0.50 and the one natural experiment on this corpus says widening retrieval makes
   precision worse. Backlog rows 55 and 56 carry what session 28 measured about the broad path and
   are explicitly deferred, not scheduled.
3. **M7 runs over its pre-registered 150 prompts.** The 108-prompt transfer holdout stays sealed
   and is for a check after the feature is on, not for the gate.
4. **The continuation candidate is productionised exactly as measured**, defects included, so that
   the owner's M7 number stays comparable to the agent estimate it is being compared against.
5. **A labelling CLI gets built** rather than editing the label TSV by hand.
6. **The stop condition is one retry.** If owner-labelled M7 misses 0.50, narrow the candidate once
   and re-run; if it misses again, M-4 and the injection code are deleted. Registering that rule in
   the memory spec is this session's work, item 5 below.
7. **§1's coupling is released.** 0.1.0 ships now with injection still disabled; the injector is
   0.2.0. Publication no longer waits on the memory feature being native-grade.
8. **0.1.0's scope** is the four items below: the search skill, publication condition 3 closed, a
   release note naming backlog 54 as a known limitation, and a re-verified package.
9. **The owner will disable claude-mem for some weeks after 0.1.0 ships** and run on Engramux
   alone. That is deliberate and is the incremental-utility measurement; do not treat it as a bug
   report when the session-start view goes quiet.

## Verified repository state

At the time this brief was written, `git rev-parse HEAD` was 08e8e0fb046f906ecb87e59feb657de72529fd2a,
`git branch --show-current` was `main`, and `git log --merges --oneline | wc -l` was 36.
`git rev-list --count origin/main..HEAD` returned 40 against the local tracking ref with no fetch
performed, so that is not a live remote claim — recheck it before acting on it.

Three files are uncommitted and all three are session 28's output. Nothing is committed, on
purpose: 40 commits already wait for owner review and session 28 was not asked to add a 41st.

- `docs/superpowers/backlog.md` — rows **55, 56 and 57** added.
- `docs/prompts/2026-09-08-session-28-selection-review-handoff.md` — the brief session 28 was
  handed, never committed by its author.
- this file.

Commit them with your own work, or leave them for the owner; either is fine, but say which you did.

**No Go source changed in session 28.** Two throwaway diagnostic tests were written, run and
deleted. So the check state is what session 28 inherited: the abstention-causes change passed the
full `internal/inject` and `internal/service` tests, a targeted race run, the pinned linter,
mutation checks and an all-package compile. The all-package compile was not full regression, and
the hour-long race suite has not been rerun since the credential-delimiter change.

## Phase 1, the five items

### 1. A skill that makes the search surface reachable

This is the item with the most unverified surface, so do it first and check it against the host
rather than against documentation.

**The problem, established 2026-09-08.** The Engramux MCP tools reach the host model as deferred
names with no descriptions, so nothing prompts the model to call them; the session that found this
had to fetch a tool schema by hand before it could ask the service anything. The owner has never
called `search`, `get_session_resume` or the CLI once, and believed the product worked
automatically — what they were actually reading at session start is `thedotmack/claude-mem`
13.15.3's output. Backlog row 57 carries the whole finding and its evidence.

**What to build.** A skill under a new `skills/` directory at the repository root, whose name and
one-line description tell a model when to search this corpus rather than guess: past errors,
commands, touched paths, and decisions from earlier sessions on either host. The MCP tool names to
point at are in `internal/mcpserver/tools.go`; read them there rather than copying the list from
here.

**Two things are `[unverified]` and both are checkable this session.** Whether Claude Code
discovers a `skills/` directory at a plugin root without a manifest field, and whether a skill
delivered through a plugin surfaces its name and description the way a marketplace skill does.
AGENTS.md's own row applies: read the host's own state after installing, and prefer it to the
documentation when the two disagree. If discovery needs a manifest field, `.claude-plugin/plugin.json`
declares none today.

**A verified fact that will bite you.** `scripts/package.sh` stages exactly `.claude-plugin/plugin.json`,
`README.md`, `LICENSE` and the two binaries. **A `skills/` directory does not reach the archive
without a change to that script.** Change it there, and remember that item 4 re-verifies the
archive afterwards.

### 2. README's Defender paragraph, which closes publication condition 3

Publication conditions 1, 2 and 4 are closed. Condition 3 is a README plus the Defender exclusion
procedure written down, and session 27 recorded the README half done and the exclusion steps open.

**Ask the owner before writing this.** Whether they actually walked the Windows Security exclusion
path, and by which route, is a fact only they hold and session 28 did not have. Do not infer it and
do not write a procedure you have not been told was performed. Everything else in Phase 1 proceeds
independently of the answer, so ask when you reach this item rather than at the start.

### 3. A release note naming backlog 54

Backlog 54 is the `CLAUDE_CONFIG_DIR` path-resolution defect, and the owner decided it ships as a
known limitation rather than delaying 0.1.0. Name it plainly: on a machine where that variable is
set, `doctor` reports the eleven hook entries missing and `install --apply` writes where the host
will not read. Do not fix it here — it changes product behaviour and AGENTS.md wants its own branch
for that.

### 4. Re-verify the package at 0.1.0

The existing package verification is at 4402f3e and predates the abstention-reason change, so it is
not a current artefact. Run the release build through `scripts/package.sh` and confirm it is
reproducible the way `docs/evidence/package-verification-2026-09-08.md` records. Do this **after**
item 1, so the archive under test is the one with the skill in it.

The script's own refusals do real work here — it asks the built binary its version and refuses a
mismatch, and it checks the marketplace rewrite. Report what it printed; do not paraphrase a pass.

### 5. Register M-4's delete condition in the memory spec

`docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md`. M4, M11, M13 and M14 each carry
an explicit delete condition; **M-4 carries an activation gate and no abandonment condition at
all**, and session 28 identified that asymmetry as the structural reason this line of work has not
ended. Write decision 6 above into the spec as M-4's own condition, in the same pre-registered form
the other gates use, and date it. This is a deliberate design change and not a correction, so say so
where the spec records it.

## What you present at the end and do not do

Three owner actions, in this order. Give them as commands the owner can run, and stop.

1. Review, commit if you left anything uncommitted, push, tag and publish 0.1.0. This is a remote
   write and it is the owner's.
2. Disable claude-mem, after 0.1.0 is published and the skill is installed.
3. Later, in Phase 2: 150 blind prompt judgements in pass 1, then roughly 14 block judgements in
   pass 2. Mention it so the owner can plan; it is not this session's to start.

## Out of scope, named so it is not drifted into

Phase 2 entirely: productionising the continuation candidate out of `internal/inject/continuation_test.go`
into a product path, the labelling CLI, and any M7 run. The 108-prompt holdout and its manifest.
M8 P5's 281 owner rows. Backlog 53 and the fix for 54. Backlog rows 55 and 56, which are the broad
selector and are deferred by decision 2. No raw capture, private history or `.capture/` content
goes into a commit, a document or an independent reviewer.

## What Phase 1 owes for evidence

No Go source changes, so no regression run is owed by blast radius. What is owed is item 4's
package output as it was printed, the host's own state for item 1's two unverified questions, and
the pinned linter if any Go file does change. Documentation-only work verifies the facts, paths and
formatting it changed — the README, the release note and the spec revision each need that and
nothing more.

## The figures Phase 2 rests on, for context only

The continuation candidate measured 75.10% known-relevant bytes, zero bytes on prompts labelled as
not wanting context, and 14 emitted blocks over the 150-prompt temporal replay. Those are **agent
judgements over exposed development data**, not an owner verdict and not an M7 pass; the whole point
of Phase 2 is to replace them with owner labels. The full-snapshot agent M7 failed at 0.032 relevant
bytes against a bar above 0.50, with 0.531 of bytes spent on prompts marked as not wanting context.
Do not quote either set as a result, and never relabel an agent estimate as an owner judgement.
