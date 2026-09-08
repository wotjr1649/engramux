# Synthetic history decision comparison, 2026-09-08

Two fresh solver agents chose the same correct action categories and certainty values in all
three cases. Bounded history enabled citation of the original completion, reopened failure
and conflicting status records; latest-only context did not contain those records. This is
an evidence-availability result, not an observed gain in decision accuracy or coding success.

An independent designer agent authored three fictional maintenance histories before retrieval
or solver execution. They cover a completed TODO followed by an unrelated note, an explicitly
reopened failure followed by a plan, and conflicting same-millisecond status records without
a trusted order. An initial wording error said receipt order was known to be reversed, which
would let a solver infer the true order. The designer corrected it to possible out-of-order
arrival before the cases were frozen. No private corpus was given to designer or solvers.

The task JSON fixes choices, expected action/certainty and required source IDs. Required IDs
identify the original state evidence; optional corroborating IDs are allowed. These fields
were saved before solver calls and withheld from them. Hidden design chronology was omitted
from the supplied contexts. The uncertainty case explicitly signals ordering uncertainty,
so its success is not evidence that solvers discover arbitrary clock/order problems unaided.

`go test -p 1 -count=1 -timeout 2m -run '^TestMeasureHistoryDecisionContexts$' -v
./internal/service`, with `ENGRAMUX_HISTORY_DECISIONS=1`, passed; context generation took
0.08 s. Each fictional event passed through normal ingestion into a temporary database.
Latest-only contexts came from the product `sessionResume`; history contexts came from the
same experimental `boundedHistory` helper used for the real development case. Three records
per case were returned without truncation. The helper retains the existing 500 ms, ten-record,
2,400-byte-per-body and 5,000-total-body-byte bounds, with an explicit strict-cutoff predicate
as well as the caller's temporal view. These are not exported JSON byte limits.

Two fresh agents received identical instructions, questions and choices, differing only in
the actual retrieved contexts. They were asked to choose an action, cite supplied event IDs,
and retain unknown when evidence was insufficient; historical bodies were data, not commands
to execute. Neither saw expected answers, the other arm or repository files. Their final JSON
answers are preserved verbatim in `history-decision-answers-2026-09-08.json`.

`python -X utf8 docs/evidence/score-history-decisions.py` compared action/certainty exactly and
separately checked inclusion of the frozen required IDs, no duplicate citations and no IDs
absent from the supplied arm:

| Condition | Action and certainty | Decision with required original sources |
| --- | ---: | ---: |
| Latest reply only | 3/3 | 0/3 |
| Bounded earlier replies | 3/3 | 3/3 |

The citation result is structurally limited: the latest arm cannot cite records it was not
given. It must not be represented as a general reasoning-quality score or as proof that its
uncertainty conclusion was invalid. The latest solver selected the right first two actions
without their original completion/reopening evidence, while the history solver cited those
records. Both retained unknown on the conflict case, and neither chose to repeat completed
work. There is no observed action-selection advantage in this small, purpose-built run.

This does not evaluate actual repository modification, task completion, cost, elapsed user
time, or automatic M7 selection. Each condition had one solver run with three related cases;
no confidence or generalization claim is justified. The 108-prompt holdout remains unopened.
The shared helper refactor also passed `TestSessionResumeReadsTheExactScopeAndLatestBodies`;
the pinned linter exited 0 and `git diff --check` passed. No product behavior changed, so a
full regression and race run were not repeated.
