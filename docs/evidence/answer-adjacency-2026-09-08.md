# Receive-order answer adjacency rejected, 2026-09-08

Adding the first main Stop after a matched historical prompt, before the next prompt in
the same host/project/session, cannot establish that it answers the matched prompt. The
selection-review subagent identified delayed-ingest counterexamples; a local SQLite
diagnostic reproduced both without changing the production selector.

The oracle says S1 answers P1 and S2 answers P2. All synthetic rows share one conceptual
host/project/session, and all precede the current trigger. The query sees only event kind
and receipt order, not the oracle relationship.

| Receipt order | Heuristic result for P1 | Failure |
| --- | --- | --- |
| P1, S2, P2, S1 | S2 | Wrong answer association |
| P1, P2, S1, S2 | Empty | Correct answer missed |

`go test -p 1 -count=1 -timeout 2m -run
'^TestReceiveOrderDoesNotEstablishPromptAnswerLinkage$' -v ./internal/inject` passed in 0.04 s,
asserting the exact wrong association and omission. This is a counterexample to a proposed
query, not an observed corruption in shipped behavior. The diagnostic table is temporary
test data; it does not add a migration or a product query. It tests two reorder cases, not
every possible host ordering or timestamp tie.

The proposal is rejected as an answer-linking mechanism. Strictly-before-trigger filtering
prevents future ingestion from entering a replay, but cannot reconstruct host conversation
order. The explicit session reader remains an accurately named latest-captured-record read;
it does not claim prompt/answer pairing. A future paired-answer design needs verifiable host
turn or parent linkage, or must explicitly measure ambiguity rather than present adjacency
as a known answer relationship. No activation or selection-quality improvement is claimed.
