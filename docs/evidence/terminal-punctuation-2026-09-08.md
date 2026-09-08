# Terminal punctuation selection experiment, 2026-09-08

The development replay shows no improvement. The candidate remains only on local branch
`step-query-terminal-punctuation` at `f47f42a`; it was not merged, installed or pushed.

The original identifier classifier treats any non-letter as an identifier signal. Consequently,
`M7 checkpoint? threshold? migration?` selects the three longer punctuated words and drops M7.
The candidate removes terminal ASCII `.`, `,`, `?` and `!` only while classifying a token.
It retains the original token in the query and leaves internal punctuation, drive colons,
digits and acronym recognition unchanged. This is a ranking heuristic, not a universal parser
correction: punctuation can be part of a literal identifier, and changing classification can
change which literals survive the three-term selection.

`go test -p 1 -count=1 -timeout 2m -run
'^TestTerminalProsePunctuationDoesNotDisplaceAnIdentifier$' -v ./internal/inject` first failed,
showing M7 displaced. After the candidate, it passed for all four punctuation characters.
The existing identifier and search-punctuation tests also passed. Mutating the trimming set
to empty made the new test fail again; restoring it passed. The pinned linter exited 0 with
`0 issues.` and `git diff --check` passed.

`go test -p 1 -count=1 -timeout 3m -run '^TestMeasureTransferDevelopment$' -v ./internal/inject`,
with `ENGRAMUX_TRANSFER_REPLAY=1` and `ENGRAMUX_TRANSFER_TERMINAL=1`, passed in 11.87 s.
The candidate's separate exclusive output is `development-terminal-replay.json` under the
private transfer directory. All 23 development prompts remained in the replay, including
abstentions; original prompt labels were not changed and the 108-prompt holdout stayed unopened.

| Arm | Emitted | Wanted output | Blocks | Bytes | Unwanted bytes | Deadlines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Previously recorded baseline | 2/23 | 0/6 | 24 | 9,317 | 9,317 | 0 |
| Terminal punctuation candidate | 2/23 | 0/6 | 24 | 9,317 | 9,317 | 0 |

These aggregate results are identical; no block-by-block identity claim is made. The synthetic
ranking repair did not produce useful history for any wanted request in this development set.
The selection-review subagent independently identified the literal-punctuation ambiguity.
Broader literal, Unicode, regression and race checks were not run because the candidate was
rejected for integration on the available quality evidence. Passing a small ranking test is
insufficient reason to change the shipped selector.

The registered M7 threshold remains relevant-byte share **strictly above 0.50**, with its
non-vacuity checks and owner-label requirement, as specified in the memory architecture's
"What M7 will measure" section. Unwanted-byte and coverage figures in these development
experiments are diagnostics, not newly invented activation thresholds. None of these agent
experiments replaces the official owner evaluation or proves real task success.
