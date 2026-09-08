# Injection abstention causes, 2026-09-08

Backlog 46 described an observation defect: a matched event removed by self-exclusion was
reported as no match, and a ceiling on one index hid exclusions on the other. The fix retains
the existing searches, caps, filter order, assembly and deadline checks. keepable additionally
reports whether it actually removed the current prompt or a command invoking Engramux.

An empty result combines fixed messages in event-ceiling, memory-ceiling, prompt-exclusion,
command-exclusion order. No-hit is used only if none occurred. Successful injection keeps an
empty reason. The service already logs and returns this reason, so no additional log surface
or IPC field was added. No prompt, path, ID or command text is interpolated into the message.
The reason is a set of observed suppressions, not a proof that one observation alone caused
abstention. Filters still inspect only the existing bounded candidates; these messages do
not claim there is no relevant record outside that candidate set.

The new exclusion test failed before implementation in its prompt-only, command-only and
combined cases. After implementation, it passed alongside no-match and surviving-event
controls. The mixed-suppression test covers memory ceiling plus both exclusions, event ceiling
before filters, both ceilings, and surviving results from either index. Survivors are checked
by exact ID and index as well as nonempty context; abstentions must emit no context or IDs.
Both tests use synthetic temporary databases and preserve the 200-match ceiling.

An independent conceptual reviewer confirmed the fixed-phrase approach and warned against
claiming sole causality or reporting exclusions for candidates never passed to the filter.
The event-ceiling test verifies that the suppressed events do not acquire filter observations.

Observed checks:

- `go test -p 1 -count=1 -timeout 2m -run '^TestBuildReports' -v ./internal/inject` passed all ten subcases.
- `go test -p 1 -count=1 -timeout 3m ./internal/inject ./internal/service` passed both packages.
- Removing the prompt observation made three checks fail; removing the command observation separately made three fail. Neither mutant failed to compile. Each was restored, and the targeted tests passed again.

Historical evaluation exports and original label files were not rewritten. Future reason
histograms will have more categories and cannot be compared as identical strings with old
histograms; the emitted-content metrics retain their definitions. This is observability work,
not improved retrieval quality, a passed M7 gate or permission to enable injection.

The change was prepared on step-abstention-causes. The pinned linter exited 0.
`go test -race -p 1 -count=1 -timeout 3m -run '^TestBuildReports' ./internal/inject`
passed with the existing C compiler and CGO_ENABLED=1. No compiler was installed.
`go test -p 1 -run '^$' -timeout 2m ./...` passed with CGO_ENABLED=0; this was an all-package
compile check, not a full regression run. Full-repository regression and the hour-long race
suite were not repeated for this logging-only change. Final diff checks passed.

No installed binary, host settings, remote or release was changed. The earlier package at
4402f3e does not include these new diagnostic reasons; any release candidate must be built
from its final source rather than reusing that verification archive.

## Temporal replay after integration

At merged source 2ae073b, the existing temporal replay gained aggregate reason counts by
the already frozen agent prompt label. With ENGRAMUX_TEMPORAL_AGENT_DIR pointing at the
original agent fixture, and both candidate flags and both export flags empty,
`go test -p 1 -count=1 -timeout 3m -run '^TestMeasureTemporalSelection$' -v ./internal/inject`
passed in 18.71 seconds of test-body time. The source snapshot was opened read-only and
strictly earlier events were replayed into a temporary database. Native-memory history remains
absent because its versions at those triggers are unknown.

| Agent prompt label | Abstention observation | Prompts |
| --- | --- | ---: |
| yes | No corpus match | 50 |
| no | No corpus match | 75 |
| no | Event index above ceiling | 5 |
| no | No usable query term | 2 |

No prompt or command exclusions were observed in these abstentions. The prompt's own event
is absent by construction in strict-prefix replay, so this is not evidence that production
self-exclusion never matters. All 50 non-emitting wanted prompts failed before a useful
candidate was returned, not because these two filters removed one.

The replay still emitted on 18/150 prompts: 93 blocks, 31,874 excerpt bytes, 9/59 wanted
prompts, 20,917 bytes on unwanted prompts, and zero deadline abstentions. These aggregates
match the preserved temporal baseline; no block-by-block identity claim is made from counts.
All 52 completed-notification triggers emitted nothing. No new relevance label was assigned.

A second run enabled ENGRAMUX_TEMPORAL_TERM_DIAGNOSTIC=1, adding individual searches for the
unchanged selected query terms only on those 50 wanted no-match prompts, over the same prefix
and project. It passed in 18.42 seconds and retained the same injection aggregates. Of those
50 prompts, 45 had at least one individually matching term; 16 had every selected term match
individually. Thus 29 had partial individual matches and five had none. No term was refused
by the query parser. The extra diagnostic searches are outside the injector and do not form
a new selector or extend its budget. They report counts only, not terms, paths or excerpts.

This narrows the recall problem to query/candidate generation for this exposed population.
It does not establish that any individually matching record is relevant, that an OR query
would pass M7, or that reranking a broadened set would succeed. Earlier broadening failures
remain applicable. The unopened holdout was not used.

Before the first run and after each run, SHA-256 checks of the M7 snapshot, owner prompt
labels, agent prompt labels and preserved temporal-trigger metrics were identical. Both
export flags were disabled, so neither replay overwrote a prior evaluation artifact.
The pinned linter and final diff checks passed. Full regression and race were not repeated
for this opt-in diagnostic-only change.
