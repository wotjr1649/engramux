# Incremental history experiment boundary, 2026-09-08

The continuation replay establishes agent-estimated relevance, not improved task completion.
An independent reviewer proposed a paired visibility experiment using only a conceptual
description; it received no private corpus, prompts or excerpts and made no implementation
changes. This is an experiment design, not a result.

Freeze the continuation v2 selector. For each independently designed task, compare four arms:
selector off/on crossed with the decisive historical reply present/absent in the solver's
current context. Preserve all other task information and use fresh solvers with the same
configuration. Feed the selector the same stored history in all arms. Record actual selected
IDs and hashes, not an idealized hand-selected substitute. The no-output cases remain in
the denominator. Use executable acceptance checks fixed before any solver sees a case.

Start with public synthetic tasks so independent solvers can receive all necessary inputs
without disclosing captures. Include a missing historical constraint, an already visible
constraint, a self-contained task with a misleading continuation word, and a stale latest
reply conflicting with a newer current requirement. Synthetic results establish only the
tested mechanism; they cannot establish real-work usefulness or pass formal M7.

Record task/arm identity, visible message IDs/hashes, selected IDs/hashes, acceptance outcome,
and any observed correction turns. Compare on-minus-off acceptance separately for missing
and already-visible information. Contrary to one reviewer suggestion, improvement in the
already-visible arm is not automatically worthless: repetition may affect salience. Report
that effect separately rather than calling it recovery of missing context.

Do not add cue exceptions during the fixed experiment. Before running it, freeze exact task
artifacts, checks, ordering, finite run count and rejection criteria. Stop this candidate's
adoption on an observed task regression attributable to injected stale or irrelevant content;
an absence of measured benefit leaves usefulness unverified. Further experiments need a new
hypothesis, not relabelled failures. The existing 108-prompt private holdout is not consumed
by this preliminary mechanism test.

At design time, no four-arm experiment had run. Runtime-model exploration was not a
prerequisite for the following experiment.

## First fixed run

An independent task designer produced four public configuration tasks with exact expected
objects before any solver ran. Root preserved the tasks unchanged in
`incremental-tasks-2026-09-08.json`. These are configuration-answer tasks, not repository
implementation tasks. The scorer checks exact object keys, values and Python value types;
its SHA-256 before solving was
`87328f8271c4387d4429aba092de80a866c460a3481a93471ed1c8bd394ea86f`.

`ENGRAMUX_INCREMENTAL_HISTORY=1` enabled
`go test -p 1 -count=1 -timeout 2m -run '^TestMeasureIncrementalHistory$' -v ./internal/inject`.
It ingested each prior reply through the production store into a temporary database, then
ran the unchanged continuation v2 helper. Actual fenced outputs, selected IDs, output hashes
and the design hash are preserved in `incremental-output-2026-09-08.json`. Three tasks emitted
483, 470 and 462 bytes. The fourth emitted nothing because its prompt contains no cue.
This run passed in 0.06 seconds of test-body time; that is not a usefulness verdict.

`python -X utf8 docs/evidence/prepare-incremental-arms.py` wrote exclusive per-arm inputs
containing only public requests, the applicable prior replies and actual selector output.
Four fresh solvers received one arm each, all four independent tasks in the same fixed order,
the same instructions and inherited model configuration, without expected answers or other
arms. Each had one response attempt and no correction turn. They were told to report missing
information rather than invent a prior decision. Their returned JSON objects are preserved
in `incremental-answers-2026-09-08.json`.

`python -X utf8 docs/evidence/score-incremental-history.py` verified the design/output hashes
and evaluated all sixteen answers:

| Prior reply in current context | Selector off | Selector on |
| --- | ---: | ---: |
| Absent | 2/4 | 4/4 |
| Visible | 4/4 | 4/4 |

The two missing-history tasks failed only in absent/off, with explicit insufficient-information
answers. The self-contained task succeeded despite an irrelevant injection. The stale-history
task received no injection and therefore does not test resistance to injected stale content;
in its visible arms the solver did ignore the older conflicting settings. No selected output
was substituted to strengthen that arm. There was no observed answer regression in these
sixteen responses and no measured benefit when the prior reply was already visible.

This demonstrates a narrow information-recovery mechanism, not real-development usefulness,
robust admission, timing savings, or formal M7 acceptance. Four easy synthetic tasks and one
response per arm cannot support a general effect estimate. The same-session visibility gap
in the real replay remains unresolved, and the irrelevant self-contained injection remains a
selector failure even though this solver tolerated it. Next evidence must connect missing
history to realistic work with executable artifact checks; adding more easy configuration
questions would not close that gap. The private holdout remains unopened.

The pinned linter exited 0 and `git diff --check` passed. Full regression and race were not
repeated for this opt-in test exporter and public evidence. No production selector, model,
host configuration or installed binary changed.
