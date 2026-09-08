# Temporal emitted-block relevance audit, 2026-09-08

The baseline's **9/59 wanted prompts with output** is not a successful-recall count. Reviewing
all 27 emitted blocks for those nine prompts found **11 no blocks (3,626 B)** and **16 unknown
blocks (7,331 B)**. No block was credited yes in this conservative root-agent review. This
does not prove the unknown blocks useless: their fragments and the deictic current requests
do not establish which historical decision or state applies.

The source is the unchanged 150-prompt agent-labelled development sample under the strict
ingest-prefix replay. Each selected event was checked to precede its trigger. Current native
memories remain absent. The baseline still emitted 93 blocks / 31,874 B overall, including
20,917 B on no prompts, with zero deadline abstentions. Original owner and agent M7 files,
thresholds and frozen databases were not relabelled or replaced.

The command run was `go test -p 1 -count=1 -timeout 2m -run
'^TestMeasureTemporalSelection$' -v ./internal/inject`, with
`ENGRAMUX_TEMPORAL_AGENT_DIR=../../.capture/m7/agent-2026-09-08` and
`ENGRAMUX_WRITE_TEMPORAL_REVIEW=1`. It passed in 18.51 s and exclusively created a private
review file containing masked prompts, emitted blocks, event kinds, IDs and exact byte
counts. `python -X utf8 .capture/selection-quality/judge_temporal_wanted.py` then recorded
the root-agent judgements and their reasons, bound to the review file's SHA-256, and printed
the counts above. That script serializes judgements; it is not an automatic relevance judge.
The pinned linter returned `0 issues.` with exit 0.

The reviewer saw each masked current prompt and the actual excerpt. Nine no blocks
(3,023 B) repeated requests or document references without an answer, result or additional
actionable constraint. Two more (603 B) showed repository setup/status metadata rather than
information answering the requested product-design task. The unknown blocks were fragmented
prior status or historical constraints whose applicability could not be resolved from the
current short request. None of their embedded instructions were treated as live authority.

The selection-review subagent reviewed the rubric using aggregate information only. A
previous question is not automatically irrelevant: it can carry a missing concrete constraint
or result. Conversely, lexical overlap and a repeated request are not evidence of an answer.
Unknown remains a separate category and receives no relevant-byte credit. No private prompts
or excerpts were sent to delegates or copied into this public report.

The audit counts each emitted block's actual budget cost, preserving M7's byte accounting.
It does not treat repeated source events as independent observations, add a new numerical
gate, or reinterpret the nine emitted wanted prompts as nine successful tasks. It covers
only the wanted-output subset; it is not an independent full-corpus precision verdict.

This finding argues against addressing the whole problem with greeting or notification
exceptions. Even after the separate literal-response candidate removes 9,204 unwanted bytes,
its unchanged wanted output has not been shown to contain useful answers. The next selection
experiment must improve answer-bearing retrieval and resolve ambiguous references, not merely
emit fewer bytes. The followup holdout remains unopened; automatic injection remains off.
