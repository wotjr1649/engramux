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

## Same-session evidence availability

`go test -p 1 -count=1 -timeout 2m -run '^TestWriteTransferPredecessors$' -v
./internal/inject`, with `ENGRAMUX_TRANSFER_PREDECESSORS=1`, examined only the six
development requests already labelled yes. For each exact host, project and session it
counted Stop events strictly before the request and read the latest one, ordering timestamp
ties by ID. This is a bounded availability audit, not the product selector. It masks the
whole payload before extracting the assistant message and writes only under `.capture/`.
The wanted-label file remains bound to the exported development prompts by SHA-256.

| Wanted request ordinal | Earlier same-session Stops | Latest body bytes | Root inspection |
| --- | ---: | ---: | --- |
| 1 | 0 | 0 | No same-session Stop; other evidence not ruled out |
| 2 | 5 | 1,593 | Concrete unfinished work and next-step proposal |
| 3 | 1 | 1,357 | Restart constraints, but no exact requested execution path |
| 4 | 0 | 0 | No same-session Stop; named other session still to inspect |
| 5 | 1 | 10,931 | Project status and limits of demonstrated benefit |
| 6 | 2 | 7,026 | The recommendation being accepted is present |

The first audit limited bodies to 2,000 runes and truncated two of the four bodies, including
the accepted recommendation. Its original private export was preserved. Raising only this
offline inspection bound to 8,000 runes and using a separate exclusive output produced the
table above with no truncation; the second run passed in 0.02 s. This does not change the
product's injection budget. The pinned linter subsequently exited 0 with `0 issues.`.

There is relevant stored evidence for at least ordinals 2, 5 and 6. Collection absence cannot
explain every missed request. Ordinal 3 is partial and is not credited as a complete answer.
These are root-agent relevance judgements of historical statements, not verification of those
statements' factual accuracy. The selection-review subagent reviewed the aggregate interpretation.

Crucially, a previous answer in the same session may already be in the model's context.
Neither the context at each trigger nor the benefit of injecting it again was measured.
Thus this audit is not a new quality score, task-success result, or justification for
automatically injecting the latest same-session answer. A named other-session request is
the next bounded case to inspect; absence in this narrow audit does not establish absence
from the corpus. The 108 holdout prompt bodies and outputs remain unopened.

## Named other-session case

`go test -p 1 -count=1 -timeout 2m -run '^TestMeasureTransferNamedResume$' -v
./internal/service`, with `ENGRAMUX_TRANSFER_NAMED_RESUME=1`, passed in 0.01 s.
The fixed development case is prompt ordinal 21, which names a source Codex session
and asks about the project's purpose, development/validation status and demonstrated benefit.
The other named-session development prompt names a handoff destination; it was not treated
as a source-session lookup.

The harness uses the unmodified product `sessionResume` function against the read-only frozen
database. A connection-local temporary events view exposes only rows strictly before the
trigger, preserving original rowids for the product's tie ordering. No events are copied or
changed. The query retains exact project, host and host-session identity; the harness confirms
the named source is a different stored session and checks every returned timestamp against
the cutoff. This is a temporal case study, not a new production time-filter API.

The latest reply is **870 bytes, untruncated**. It reports the final operation in that session:
adding adjudication fields to seven evaluation rows, preserving previous data, and checking
the edit. The latest prompt is truncated at the existing product body cap. Root inspection
therefore finds evidence about a recent operation, but not a sufficient answer about the
project's overall purpose, completion and benefit. The captured report's assertions were not
independently verified in this repository. In particular, captured claims of owner judgement
remain historical data and do not authorize or relabel this evaluation.

This demonstrates successful exact identification and retrieval for this case. It does not
demonstrate full continuation utility or a general quality improvement. A latest-reply-only
surface can omit earlier decisions even when it returns the correct complete final reply;
the next useful check is whether bounded earlier history contains the missing information.
The private result remains under the development directory as `named-source-resume.json`.
The pinned linter exited 0 with `0 issues.`. No product behavior changed.
The selection-review subagent agreed that bounded earlier-history inspection is a justified
next experiment, not yet an adopted feature. It must add a previously missing answer component
with a source event while preserving exact scope, strict time cutoff, byte cap and deadline.

## Bounded earlier-reply comparison

The same fixed case was rerun with `ENGRAMUX_TRANSFER_HISTORY=1` in addition to
`ENGRAMUX_TRANSFER_NAMED_RESUME=1`, using the same command above. This experimental arm
selects at most ten earlier Stop records in the exact source project, host and stored session,
newest first with rowid tie ordering. It masks whole payloads before reading assistant text,
keeps event references and timestamps, caps each body at the existing 2,400-byte resume limit,
and caps total body text at 5,000 bytes. Query, decoding and masking share a 500 ms deadline
with a final wall-clock check. The body cap excludes JSON metadata; it is not a claim that
the exported JSON or a production injection fits 5,000 bytes. No product selector changed.

The run passed in 0.01 s and returned **three replies / 5,000 body bytes**. The first reply
matches the original latest reply. The second is complete; the third is truncated. Comparing
the previously fixed question components gives this root-agent assessment:

| Requested information | Latest reply alone | With bounded earlier replies |
| --- | --- | --- |
| Project purpose | Insufficient | Still insufficient |
| Development and validation state | One final data-edit operation | Adds evaluation verification and remaining document work |
| Demonstrated benefit | No benefit measurement in the shown reply | Adds a condition comparison and explicit failure of a required gate |

This is a concrete increase in source-backed information for this development case. It does
not establish that the captured measurements are true, that the reader completed a task better,
or that automatic M7 selection passed. The earlier third reply describes preparation for work
that the later second reply reports completed. Displaying it as a current TODO would be wrong.
The first reply also resolves an adjudication action still pending in the second. Retrieval
must retain chronology and provenance; captured plans and approval requests remain historical
data, not current instructions. This is a reason to test status reconciliation, not a reason
to hide earlier conflicting records or automatically trust the latest statement as verified fact.

The private output is `named-source-history.json`; the previous latest-only output was not
overwritten. The linter initially flagged the helper's variable output path with G304. Making
the experiment destination a fixed literal resolved it without a suppression; the pinned
linter then exited 0 with `0 issues.`. A subsequent default invocation compiled successfully
and skipped the opt-in audit, preserving its existing result. No broad regression or race run
was performed for this test-only comparison.
The selection-review subagent confirmed the limited interpretation and emphasized that a
later statement only supersedes an earlier one when they concern the same work item and
the later statement actually reports a changed state. Receipt order alone is not semantic
completion evidence, and a later planning note must not erase an earlier completion report.
