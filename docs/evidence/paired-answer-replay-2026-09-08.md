# Explicit paired-answer replacement replay, 2026-09-08

The event-only experimental arm replaces a selected historical question with its uniquely
linked Stop. On the original 150-prompt development replay it emitted **90 blocks / 30,360 B**,
including **18,434 unwanted bytes (60.7%)**. Eighteen prompts received output, including 9/59
agent-labelled wanted prompts. There were zero deadline abstentions. This is still a failure
of the existing unwanted-byte constraint, not a selection-quality pass.

The original baseline remains 93 blocks / 31,874 B, 20,917 unwanted bytes (65.6%), 18 emitted
prompts and 9/59 wanted prompts with output. Original labels, corpus, thresholds and default-off
behavior were preserved. Neither the earlier 24-prompt nor 33-prompt exposed holdout is treated
as independent validation for this new candidate.

The test-only arm first uses the unchanged selector. For emitted event IDs, it resolves
explicit host keys from strictly earlier records, with one prompt and one Stop in the same
project/host/session. Duplicate keys are not resolved by recency. Unpaired events retain their
original excerpts; duplicate selected answer IDs are emitted once. Linked bodies are read with
an exact project and cutoff predicate, masked as whole payloads, and decoded only from
`last_assistant_message`. Oversized or missing bodies are omitted. The experiment takes the
first 240 Unicode runes of an answer, rather than centering an excerpt on the original query.
This can truncate useful content and is part of the experimental treatment.

The original selection, pair scan, resolution, body reads and assembly share one 500 ms
deadline. The existing assembly and fence helpers enforce the 5,000 B cap; the result is
discarded if the deadline has elapsed after CPU work. No offline pair-index cost is excluded
from that budget. The scan is bounded at 100,000 records and 1 MiB per payload. This is an
experimental scan, not a production indexing design or a scalability guarantee.

`go test -p 1 -count=1 -timeout 3m -run '^TestMeasureTemporalSelection$' -v ./internal/inject`
with `ENGRAMUX_TEMPORAL_AGENT_DIR=../../.capture/m7/agent-2026-09-08`,
`ENGRAMUX_TEMPORAL_PAIRED=1` and `ENGRAMUX_WRITE_TEMPORAL_REVIEW=1` passed in 18.93 s. The final
run invokes only the selected arm; an earlier diagnostic unnecessarily ran the baseline twice
and was corrected before this recorded run. Both runs yielded the same counts.

The root reviewed the eight changed wanted-prompt blocks in the exclusively created private
review file. They contain actual prior answers in place of question repetitions, including
status, results and recommendations. That structural improvement does not resolve which past
task a short current continuation request means. No new yes labels, relevant-byte precision
or actual-task success are claimed from this inspection.

The body test failed with the initial empty implementation, passed after implementation,
failed when masking was removed, and passed after restoration. Host-key and pair-integrity
tests also passed. A synthetic integration case confirmed the exact answer event replaced
the matching question, carried its accepted value, fit the cap and retained the fence. The
pinned linter then passed again with exit 0, and `go test -p 1 -count=1 -timeout 2m
./internal/inject` passed in 8.138 s. All files are test-only;
the shipped selector, installed service and MCP interface are unchanged.
