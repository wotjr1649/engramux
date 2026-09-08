# History-assisted code artifact, 2026-09-08

This experiment extends the configuration-answer comparison to a compiled Go maintenance
artifact. It is one public synthetic task, not an observed real user task or formal M7.
The production selector is unchanged; the experiment uses the existing continuation v2
test-only candidate and actual fenced output from a temporary store.

An independent designer supplied a slice-selection function, a prior tie decision and six
table-driven acceptance cases. Its original request also described the answer: earlier
equal-weight events must precede later ones. Before either solver ran, root removed that
redundant answer from the current request, retaining the signature, descending weight order,
exact limit, field preservation, input preservation and empty-result requirements. The
current request now refers to the earlier tie decision without revealing it. Root compacted
the test representation; the six case inputs and expected structs are the designer's.
This intervention is disclosed because the task is not an untouched independent sample.

The frozen task SHA-256 is
`35140409435f33c0107ab7d3bcd86b83c5b2086d853ea2d81960ee1705c15be5`;
the acceptance source SHA-256 is
`7cc61073d9118177c346c3ed3572d2e69664da198c2d69115abaa55bb80a97a9`.
The task JSON includes the starter. The acceptance source is preserved separately and was
not supplied to solvers. It checks complete struct equality, input values, result length
and non-nil empty results. It does not prove all possible input sizes or output non-aliasing.

## Observed sequence and result

`go test -p 1 -count=1 -timeout 1m -v ./.capture/code-history-2026-09-08/baseline`
compiled the reviewed, pure-slice starter and failed exactly the cutoff and equal-weight
ordering cases; the other four passed. No runtime, filesystem or network behavior is in
the supplied function, and the test imports only testing.

With `ENGRAMUX_CODE_HISTORY=1`,
`go test -p 1 -count=1 -timeout 2m -run '^TestMeasureCodeHistory$' -v ./internal/inject`
ingested the prior reply through the production store and exported one selected event,
614 bytes, to `code-history-output-2026-09-08.json`. Its output hash is
`8a451131112e7041988d2d833e91287ec70d6197403a365dfca8e1ae6273e881`.
The original configuration experiment files were not overwritten.

Two fresh solvers received the same request and starter, with either this exact output or
no recalled data. Both had the same instructions and inherited model configuration, one
response attempt, no acceptance tests, no other arm and no execution. Both were told not to
invent a missing compatibility decision. The off solver reported insufficient information;
it produced no implementation, so there is no off implementation test failure to count.
The on solver returned a one-comparison change from descending to ascending Sequence ties.
Both original answer objects are in `code-history-answers-2026-09-08.json`.

`go test -p 1 -count=1 -timeout 1m -v ./docs/evidence/code-history/testdata/on`
compiled that returned implementation and passed all six cases. Reversing its comparator
reproduced the same two assertion failures, rather than a build failure. Restoring it and
running `go test -p 1 -count=1 -timeout 1m ./docs/evidence/code-history/testdata/on`
passed again. The mutation is not present in the final artifact.

This establishes one executable fix made possible by supplied historical information under
an explicit no-guessing instruction. It does not measure time saved, superiority to asking
the user, or success on a real repository change. The history was deliberately made necessary,
was short enough not to be truncated and was the only stored candidate. Consequently this
case does not challenge candidate ranking or prove that latest-reply selection finds the
right decision among competing replies. The previously observed false-positive injection
remains unresolved. More constructed missing-fact tasks alone will not resolve that defect.

The exporter now shares the fixed experiment path through os.Root scoped to docs/evidence.
The first linter run rejected variable os.ReadFile/os.OpenFile paths; no exclusion was added.
After switching to rooted operations, the pinned linter exited 0. Existing continuation tests
and git diff checks passed. Full regression and race were not repeated for this opt-in test
and public fixture work. No product behavior, host configuration or installed binary changed.
