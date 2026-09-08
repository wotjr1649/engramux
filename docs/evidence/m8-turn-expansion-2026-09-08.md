# M8 P5 additional turn context, 2026-09-08

The original M8 P5 agent evaluation still recovers an actual labelled fix for **1/2 positive
failures** at k=10. A test-only expansion of those search hits recovered **2/2**, after adding
ten event IDs per query (20 total). This is an extra-context result, not a same-k ranking
improvement, a production latency result, or evidence of general task success.

Before the experiment, a private metadata audit read the capture envelopes' `payload` members
for all three positive failure/candidate rows. Each pair shares a session, cwd and nonempty
`prompt_id`; none shares `tool_use_id`. Thus a single tool-call identity cannot connect these
repairs, while the broader host prompt identity can supply candidate context. No capture names,
key values or payload text are reproduced here.

The diagnostic starts from the unchanged ten MatchAny hits. For each hit in rank order it adds
later captured events sharing exact detected host, session, cwd and the host's linkage key.
Candidates are ordered by capture timestamp and file name; already returned IDs are excluded,
and the whole expansion stops after ten added IDs. Anchor selection does not see the labelled
failure ID, fix IDs or judgement values. Labels are consulted only to score the resulting IDs.
This is a retrospective reading operation: later repair records are intentionally available,
unlike M7's as-of-trigger automatic-injection evaluation.

`go test -p 1 -count=1 -timeout 3m -run
'^Test(M8TurnExpansion|EvaluateM8P5AgentEstimates)' -v ./internal/search`, with
`ENGRAMUX_M8_AGENT_LABELS=../../.capture/m8/agent-2026-09-08/pairs.tsv` and
`ENGRAMUX_M8_TURN_EXPANSION=1`, passed. The corpus evaluation took 2.66 s and reported:

| Measurement | Result |
| --- | ---: |
| Positive failures | 2 |
| All-no candidate groups | 91 |
| Unknown groups | 1 |
| Original event literal coverage | 2/2 |
| Original native literal coverage | 0/2 |
| Original actual fix-event coverage | 1/2 |
| Original failure echoes | 2/2 |
| Actual fix-event coverage with extra turn context | 2/2 |
| Added event IDs | 20 |

The scope/cap test returned the exact expected event. Removing its scope predicate returned
the other session's earlier event and failed; restoration passed. The pinned linter returned
`0 issues.` at exit 0. Production queries, migrations, MCP tools and owner labels were unchanged.

The two positives are already exposed agent-labelled development examples. The extension was
not evaluated on the 91 all-no groups or on independent failures, so false-context cost and
generalization remain unmeasured. It scans test documents rather than using a production index,
and does not prove bounded body bytes, masking or endpoint latency for a new tool. The result
supports designing a bounded explicit turn-context read for validation; it does not authorize
automatic injection or replace the original M8 report.

## Broader diagnostic and control

The same-session control, which drops only the turn-key equality condition while retaining
the ten-extra-ID cap, also recovered **2/2** positive fixes. This prevents attributing the
observed gain to turn identity alone; reading extra nearby records explains this small result
equally well.

Across the 91 all-no candidate-window groups, turn expansion added context for **90 groups**:
**900 added IDs**, of which **181** were explicitly labelled no for that failure and **719**
were outside its labelled candidate window. The latter are unknown, not automatically false.
The remaining query contained NUL bytes and was explicitly refused; it remains in the 91-group
denominator. The original two positive results and the one unknown group were not relabelled.

The expanded evaluation passed in 9.04 s using the same command and environment above. Its
initial run exposed an existing FTS query-parser error, documented separately in
`search-nul-2026-09-08.md`. This evidence does not justify automatically expanding every hit:
the additional context is common on negative windows and its benefit outside two positives is
unmeasured. A bounded explicit read remains a possible design, not an implemented release claim.
