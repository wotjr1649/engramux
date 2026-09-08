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

No four-arm experiment has run yet. The immediate next artifact is the independent task set
with executable checks, followed by actual selector output generation and isolated solver
runs. Runtime-model exploration is not a prerequisite for this experiment.
