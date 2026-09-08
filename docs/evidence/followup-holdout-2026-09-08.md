# Frozen literal-response candidate: followup holdout, 2026-09-08

The literal-response candidate did **not** improve the session-disjoint followup holdout.
Both implementations emitted 17 blocks / 6,570 B into seven of 33 prompts. Of the 12 prompts
labelled as needing history, two received output. No relevance or task-success pass follows
from that output count.

The candidate was frozen at `04a74a3` before opening holdout prompts. Root-agent labels were
then fixed from prompts alone: **12 yes / 21 no**, with reasons and the original prompt-file
SHA-256 in a separate private `agent-labels.json`. The unlabelled source TSV was preserved.
Yes means that prior procedure, progress, decisions or handoff context was not fully specified
in the request; no covers self-contained work orders, notifications, literal responses and
fully specified configuration tasks. These are agent estimates, not owner judgements.

The original split excluded previously exposed sessions and assigned whole sessions before
prompt inspection, as recorded in `selection-followup-2026-09-08.md`. It remains a small,
same-project followup sample rather than a new-user study. It is now exposed evaluation data
and cannot serve as an untouched holdout for future candidate tuning.

`TestMeasureFollowupHoldout` verifies the label source, prompt-file hash, complete IDs, valid
labels and project identity. It replays the same frozen database with only events strictly
earlier than each trigger and checks every emitted event's timestamp. Current native-memory
versions do not enter either arm. The frozen database and WAL hashes still matched the split
manifest after both runs.

Both commands were `go test -p 1 -count=1 -timeout 3m -run
'^TestMeasureFollowupHoldout$' -v ./internal/inject`. The untracked evaluation harness was
preserved while switching between clean product revisions. The arm environment variable
labels a run; it does not select an implementation:

| Product revision | ENGRAMUX_FOLLOWUP_ARM | Replay duration | Emitted prompts | Wanted output | Excerpt bytes | Unwanted bytes | Deadline abstentions |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `26b3213`, original selector | baseline | 12.55 s | 7/33 | 2/12 | 6,570 | 2,926 | 0 |
| `04a74a3`, frozen candidate | literal | 16.30 s | 7/33 | 2/12 | 6,570 | 2,926 | 0 |

These durations are single whole-replay observations, not performance comparisons. The
unwanted-byte share is about 44.5% in both arms; satisfying that one constraint does not
establish relevant-byte precision, recall or useful work. Block relevance was not scored
in this comparison, and identical aggregate counts alone do not prove identical block IDs.

The candidate's narrow grammar was not widened after seeing the new response-only phrasings.
Its development noise reduction remains recorded, but this holdout provides no evidence of
improvement. It stays on its experimental branch and is not a main integration candidate on
this evidence. The pinned linter passed with exit 0 after both runs; no shipped behavior was
changed in the evaluation branch. Original M7 failures and owner TODO labels remain intact,
and automatic injection remains off by default.
