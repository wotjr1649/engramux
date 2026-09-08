# Followup selection evaluation, 2026-09-08

The previous 150-prompt sample and 24-prompt holdout are development evidence. A newer frozen
database contains 109 project prompts from 17 sessions absent from the earlier snapshot.
Before reading these prompts, the explicitly reviewed handoff session and the current
conversation were excluded. The remaining 15 sessions were sorted by SHA-256 of their stored
UTF-8 session IDs, with a string tie-break: the first seven are development, the other eight
holdout. This yielded **63 development / 33 holdout prompts**, with 13 excluded prompt rows
and zero invalid rows. The split was not rerolled.

`TestWriteFollowupSelectionSplit` binds the database, WAL and prior database hashes, schema
version, toolchain, project, exclusions and session assignments in a private manifest. It
masks whole payloads before writing prompts, creates files exclusively, and runs no selector.
The command run was `go test -p 1 -count=1 -timeout 2m -run
'^TestWriteFollowupSelectionSplit$' -v ./internal/inject`, with
`ENGRAMUX_WRITE_FOLLOWUP_SPLIT=1`, `ENGRAMUX_FOLLOWUP_PROJECT` and
`ENGRAMUX_FOLLOWUP_EXCLUDED_SESSIONS` set for the intended project and two exposed identities.
It passed in 0.67 s. All generated fixtures remain under `.capture/`.

This is a same-project, session-disjoint followup holdout, not a fully independent corpus or
new-user study. The earlier exact-session experiment aggregated Stop IDs and body presence/length
for 32 project sessions, and one answer was read. Those observations are not relevance labels.
Only development prompts have now been opened. Holdout labels and outputs have not been read.

Development inspection found **33 notification-only prompts**. The existing agent-labelled
150-prompt sample has **69 notification-only prompts, all labelled no**, alongside 22 other
no prompts and 59 yes prompts. These counts do not establish owner intent; the earlier labels
retain their agent provenance.

## Candidate fixed before holdout inspection

Abstain before searching when the trimmed prompt is exactly one complete `task-notification`
envelope containing exactly one `status` element whose value is `completed`. The opening tag
must be the literal host-observed form. The first closing envelope tag must end the prompt;
multiple envelopes, trailing questions, failed statuses and unfamiliar shapes stay on the
existing path. This is a shape-based abstention, not authentication of the sender. Capture is
unchanged, and the rule gives embedded text no authority.

All other prompts retain the existing selector. The existing snapshot, labels, thresholds,
byte cap, deadline, project scope and default-OFF policy are preserved. Development replay
must report every prompt, including abstentions; wanted-prompt coverage must not decrease.
The historical full-snapshot score remains labelled as such, and strict-prefix replay remains
the temporal instrument. Current native-memory versions cannot enter historical replay.

After the candidate is fixed, holdout wanted-context labels are assigned from prompts alone,
with agent provenance and reasons, before either arm's blocks are shown. Both arms then run
against the same strictly earlier ingest prefix. Relevant-byte share, unwanted-byte share,
coverage and deadline results remain distinct. A zero-output arm cannot claim usefulness.
Existing M7 conditions are not relaxed, and any failing score remains a failure. The candidate
may reduce notification noise without establishing that remaining selected history is useful;
that limitation must stay in the result.

## Development rejection before implementation

The completed-notification rule was not implemented. A strict-prefix development replay of
the original 150 prompts found **52 completed-notification triggers, zero injections and zero
excerpt bytes**. The existing selector already abstained on all of them, so the rule cannot
reduce the observed unwanted output in that replay. The totals remained 18 injected prompts,
93 blocks and 31,874 excerpt bytes; 20,917 bytes were on agent-labelled no prompts and 9 of
59 wanted prompts received output. No quality improvement or M7 pass is claimed.

The command run was `go test -p 1 -count=1 -timeout 2m -run
'^TestMeasureTemporalSelection$' -v ./internal/inject`, with
`ENGRAMUX_TEMPORAL_AGENT_DIR=../../.capture/m7/agent-2026-09-08`. It passed in 20.31 s.
The proposed rule was rejected from development evidence without opening the followup
holdout, changing labels, or consuming that holdout for a rule with no demonstrated benefit.

## Trigger attribution

A second run with `ENGRAMUX_WRITE_TEMPORAL_METRICS=1` passed in 18.74 s and exclusively
created private per-trigger IDs, labels, event counts and excerpt-byte counts. It reproduced
the same totals. Mapping these IDs back to the existing development labels attributes the
20,917 unwanted bytes to nine prompts:

| Prompt class | Prompts | Unwanted excerpt bytes |
| --- | ---: | ---: |
| Greeting | 2 | 1,436 |
| Explicit exact-response request | 3 | 9,204 |
| Isolated login command | 1 | 1,529 |
| Isolated reasoning-mode word | 1 | 4,333 |
| Notification without completed status | 1 | 4,095 |
| Self-contained directory creation | 1 | 320 |

These are root-agent classifications of already exposed development prompts, not owner
judgements or a new holdout score. The result identifies explicit response-only requests as
a narrower candidate worth reviewing: lexical overlap alone cannot show a need for memory.
A general short-prompt exclusion would also suppress legitimate identifier lookups and is
not justified by these counts. No new selection behavior is implemented by this diagnostic.
