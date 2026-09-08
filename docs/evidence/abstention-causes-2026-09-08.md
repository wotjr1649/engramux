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
