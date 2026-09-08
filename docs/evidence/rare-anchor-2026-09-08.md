# Available rare-term experiment, 2026-09-08

The 23-prompt development replay missed all six requests labelled as needing history.
This test-only experiment asks whether choosing an available term repairs retrieval.
It does not change the shipped selector. Prompt labels remain the previously frozen
root-agent estimates, not official owner judgements; the 108-prompt holdout stays unopened.

The candidate considers at most the first 32 whitespace tokens, deduplicated case-insensitively.
Tokens must survive the existing query eligibility rule. It searches each token within the
project and chooses the smallest positive total document count, keeping input order on ties.
The count is the total match count, not the one returned row. Selection then uses the existing
builder on that single token. Frequency queries and selection share the existing 500 ms budget
and final wall-clock deadline check. The replay supplies strictly earlier events only and
retains project, fence and byte-limit checks. Broadening three-term conjunction to one term
is the treatment, not an equivalent query rewrite.

`go test -p 1 -count=1 -timeout 3m -run
'^Test(RareAnchorUsesAnAvailableTerm|MeasureTransferDevelopment)$' -v ./internal/inject`
with `ENGRAMUX_TRANSFER_REPLAY=1` and `ENGRAMUX_TRANSFER_ANCHOR=1` completed the replay in
13.50 s. Its exclusive private output is `development-anchor-replay.json` under the existing
transfer experiment directory. Recalculating counts from that JSON with PowerShell agreed:

| Arm | Emitted prompts | Wanted output | Blocks | Bytes | Unwanted bytes | Deadline abstentions |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Baseline | 2/23 | 0/6 | 24 | 9,317 | 9,317 | 0 |
| Available rare term | 13/23 | 5/6 | 89 | 31,258 | 25,166 | 0 |

The candidate fails: 80.5% of its bytes occur on requests labelled as not needing historical
context. Output on five wanted requests is not five successful retrievals. Inspection found
metadata and repeated old questions among these outputs; one contained a relevant prior
explanation, but no complete block-level relevance score or actual task-success gain is
claimed. This inspection must not retroactively change the prompt labels.

The synthetic counterexample makes the confound explicit. A unique session metadata token
has document frequency one, whereas a repair term occurs in two answer records. The candidate
chooses the metadata record. `TestRareAnchorCanPreferMetadataOverAnswers` passing means that
this defect was reproduced, not that relevance passed. Scarcity alone is not evidence that a
record answers the request. The candidate is rejected for product integration.

The available-term test first failed with the original builder and passed with the experimental
helper. Changing the positive-frequency predicate to zero made it fail again; restoration
passed. Final `go test -p 1 -count=1 -timeout 2m -run
'^Test(RareAnchor|MeasureTransferDevelopment)' -v ./internal/inject` passed both diagnostic
tests and skipped the opt-in replay, preserving its existing output. The pinned linter first
rejected the new arm dispatch with QF1003; using a tagged switch fixed it and the subsequent
run exited 0 with `0 issues.`. `git diff --check` passed. No full regression or race run was
needed for these test-only changes; no installation, push or release occurred.

## Provenance and excerpt relevance followup

`python -X utf8 docs/evidence/transfer-block-provenance.py` queried only event types
for IDs already selected by the development replay. It printed fixed event enums and
counts, with no payloads or identifiers. Of the 25,166 unwanted bytes, 14,726 were
UserPromptSubmit, 4,065 PostToolUse, 3,328 Stop, 1,924 PreToolUse and 1,123 other types.
Of the 6,092 wanted bytes, the corresponding values were 3,041, 1,547, 576 and zero;
SubagentStop contributed 324, SessionStart 266, and other types 338.

Deleting all returned question events would leave 10,440 unwanted bytes out of 13,491
(77.4%). Keeping only Stop and PostToolUse would leave 7,393 out of 9,516 (77.7%).
These are post-hoc deletion calculations on existing results, not new selector replays.
They neither account for backfilling removed hits nor demonstrate causal gate effects.
They do show that removing question events alone cannot make these outputs acceptable.
The selection-review subagent independently reviewed these aggregate limits.

Root then judged all 15 displayed excerpts returned for the six wanted requests:

| Excerpt relevance | Blocks | Bytes |
| --- | ---: | ---: |
| Yes | 1 | 576 |
| No | 10 | 3,631 |
| Unknown | 4 | 1,885 |

The one yes excerpt gives a concrete prior explanation of absent benefit guarantees and
implementation limits relevant to the request. This labels its relevance, not the truth of
its claims or success on the task. Excerpts labelled no are the target session's metadata, a generic
continuation message, or questions that supply none of the requested answer. The four unknown
excerpts contain older policy or project decisions, but the displayed context does not establish
that they are the recommendation currently being accepted. They are not relabelled as irrelevant.

`python -X utf8 docs/evidence/label-anchor-wanted.py` checked the reviewed replay SHA-256,
exclusively wrote private agent judgements keyed by prompt and event, and reported exactly
1/10/4 blocks and 576/3,631/1,885 bytes. It does not alter original wanted labels.
Known relevant bytes are 576/31,258 (1.84%); even crediting every unknown wanted excerpt gives
2,461/31,258 (7.87%). Only one of the six wanted requests has a known relevant excerpt.
This remains development evidence, not official M7 or an independent task-success evaluation.
It strengthens rejection of this candidate without opening the holdout or lowering a threshold.
Repeating the judgement writer exited 1 with `judgements already exist; refusing overwrite`.
