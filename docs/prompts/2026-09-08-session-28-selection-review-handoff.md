# Session 28 — Engramux: independently review selection and choose the next step

You are continuing in Claude Code at D:/AI_DEV/engramux after an excessively long Codex
session. The user explicitly chose this session's scope: independently reconsider the evidence,
select one approach, and define the first implementation task. Stop there. Implementation,
installation, Defender work and publication belong to later work.

CLAUDE.md contains @AGENTS.md, verified at handoff. Use the guidance your harness actually
loads; do not import Codex-specific tool availability or guard behavior as Claude capabilities.
Reply in Korean. Repository documents remain English.

## The decision to make

The continuing product objective is useful historical recall and better automatic selection,
with independent evidence and explicit evaluation provenance. CI/evaluation integration and
release preparation are part of the larger work. The user keeps Defender as the final separate
brainstorm and postpones the first publication until its prerequisites are resolved.

The host-model/MCP proposal is an unapproved alternative, not the chosen architecture:
docs/superpowers/specs/2026-09-08-host-mediated-retrieval-proposal.md.
The user specifically chose independent reconsideration before deciding. Assess this proposal
against a materially different route within the current architecture; do not assume either wins.
Select a recommended route and identify any actual owner decision it requires. This handoff
does not approve moving selection out of the hook, adding an own-model runtime, or lowering gates.

Codex marked its persistent goal blocked while waiting for that workflow decision. That is
session state, not proof that hook-time selection is impossible. Challenge the reasoning behind
the proposed change as well as the implementations. Avoid repeating synthetic success examples,
diagnostic-only commits, and status reports that do not change the next decision.

## Verified repository state

At handoff, main is 08e8e0fb046f906ecb87e59feb657de72529fd2a. The tree was clean before this
new, deliberately uncommitted brief. `git status --short`, `git rev-parse HEAD`, and
`git branch --show-current` established this. `git rev-list --count origin/main..HEAD` returned
40 against the local tracking ref; no fetch was performed, so this is not a live remote claim.
Recheck before acting. Historical session 27 describes an older state, not current permissions.

Implemented and locally integrated:

- CI fix ec4d8a0 and M7 provenance separation 058e9ea are ancestors of main.
- Exact session reader: merge 7f2fbc2, evidence docs/evidence/session-resume-2026-09-08.md.
  get_session_resume returns latest session material, not a complete decision history.
- NUL query refusal: merge 1372fb6, evidence docs/evidence/search-nul-2026-09-08.md.
- Credential-delimiter masking optimization: merge b580abb. The unchanged payload-cost gate
  passed after a preserved failure; docs/evidence/credential-delimiter-2026-09-08.md.
- Actual abstention causes: merge 2ae073b. Empty replies distinguish index ceilings and
  prompt/own-command exclusions. Backlog 46 was removed because tests now own it.

The production selector remains the three-term AND selector; experimental broadening,
rare-anchor and continuation selectors were not adopted. Injection remains off by default.

## Read these first, then inspect the corresponding code

Start with docs/evidence/abstention-causes-2026-09-08.md, especially its temporal replay.
The preserved event-only, strictly-before-trigger replay has 150 prompts, 18 injections,
93 blocks and 31,874 excerpt bytes. It emits on 9/59 agent-labelled wanted prompts and spends
20,917 bytes on unwanted prompts. All 50 wanted non-emissions have no search match. Of those,
16 have every selected term match individually, 29 have some terms match, and five have none.
Individual matches are not relevance evidence. The replay omits native memory because its
historical versions are unknown; it is not the full live-product population.

Then read docs/evidence/continuation-selection-2026-09-08.md and
docs/evidence/rare-anchor-2026-09-08.md for rejected candidates. Latest same-session reply
retrieval has some relevant exposed examples, but injects on keyword explanations and loses
older decisions behind unrelated newer replies. Rarest-word broadening increases irrelevant
context. These failures do not prove every lexical approach or every model approach will fail.

Only if needed to assess usefulness claims, read docs/evidence/incremental-history-probe-2026-09-08.md
and docs/evidence/code-history-2026-09-08.md. Their synthetic controls show missing information
can help configuration answers and one compiled Go fix. They do not establish real-work time
savings, robust ranking, or added value when the host already sees the same history.

Read the relevant M-1, M-4, M7 provenance/threshold and M8 P5 sections of
docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md rather than loading all archived
plans and evidence. Preserve the existing pure-Go/runtime, privacy and budget decisions.
Do not equate the spec's own-LLM prohibition with a verified impossibility of every semantic
technique. Pure-Go inference was inspected statically only; no model build, download, inference,
latency or Korean-quality result exists. The versioned review is in
docs/evidence/admission-position-2026-09-08.md if that alternative becomes relevant.

## Evaluation provenance and protected material

Owner M7 is not evaluated: the latest read-only count found 150/150 prompt rows TODO.
Owner M8 P5 has 281/281 rows TODO; its actual gate reported NOT EVALUATED and skipped.
The completed agent estimates are separate. Never relabel them as owner judgments or count a
skip as a pass. The original full-snapshot agent M7 failure and later temporal diagnostics are
different measurements; do not combine their denominators. M8 P5 details, including the
same-session control that also recovers the two positive examples, are in
docs/evidence/m8-turn-expansion-2026-09-08.md and the memory spec.

The transfer holdout is reserved: 108 prompts / 10 sessions, versus exposed development
23 prompts / 10 sessions. It is same-user and session-disjoint, not a new-user study; two
holdout sessions contain 75/108 prompts. See docs/evidence/transfer-split-2026-09-08.md.
Do not open holdout prompt bodies or outputs during this review or reroll the split.
The private manifest is .capture/selection-quality/transfer-2026-09-08/manifest.json.
Original M7 data is under .capture/m7; preserve all labels, snapshots and earlier exports.
No raw capture or private history goes into the brief, public commits or independent reviewers.

## Other work and validation limits

Backlog 54 remains open. docs/evidence/config-home-2026-09-08.md reproduces ignored
CLAUDE_CONFIG_DIR settings/cache paths. The installed host's exact global MCP-state relocation
rule was not verified. Codex's attempted executable inspection was denied by a credential-path
guard; no bypass occurred. Do not repeat a denied action through another route or guess that
rule. This review need not fix the path defect to decide the selection approach.

The most recent production change, abstention causes, passed full internal/inject and
internal/service tests, targeted race, pinned linter, mutation checks and all-package compilation.
All-package compilation was not full regression. The earlier full normal suite passed at the
credential optimization; the hour-long race suite was not rerun for subsequent changes.
Validation commands and limits are in the corresponding evidence files. Rerun only checks
needed to settle a material question in this review.

Package verification at 4402f3e produced equal archives twice, but predates the new abstention
reasons. It is not a current release artifact: docs/evidence/package-verification-2026-09-08.md.
Installed-build and live remote-CI status are unverified here; check them only if needed.
Preserve task-created .capture worktrees and rejected branches; no cleanup is part of this scope.

## First action and completion

Recheck the tree, read the three primary evidence documents above, and inspect queryFor,
Build and the matching replay code. Separate three questions: candidate availability, relevance,
and incremental task utility. Compare at most three materially different approaches against
the observed failures and existing constraints. Use an independent reviewer on a bounded,
public or synthetic description when that materially challenges the decision; do not send it
private captures or ask several agents to redo the same review.

Finish with one recommendation, its strongest counterexample and uncertainty, and one concrete
first implementation task: files/contracts affected, fixed acceptance criteria, discriminating
test, required evidence, rollback and stop condition. If one unavailable fact or owner decision
prevents selection, name exactly that dependency and what would settle it. Do not manufacture
progress by repeating failed candidates or opening the holdout. Do not implement in this session.
The larger product goal remains unfinished regardless of whether this bounded review completes.
