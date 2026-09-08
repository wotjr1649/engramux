# Credential delimiter fast path, 2026-09-08

The candidate preserves the text credential regex and skips its execution only when a string
contains neither ASCII `:` nor `=`. Every match of the current regex requires one of these
delimiters. The separate sensitive-JSON-key rule still examines the whole value, even when
that value has no delimiter. Other classes and whole-payload masking remain unchanged.

This addresses the returned-document processing cost investigated in
`search-cost-followup-2026-09-08.md`. It does not change search ranking or selection rules, excerpt
budgets, the 3.00 scaling ceiling, masking placeholders or redaction version. The optimization
is on `step-credential-delimiter-guard`; verification is required before integration.

The delimiter-condition test failed with an always-true initial implementation, then passed
with the necessary-character check. An unguarded reference walk checks exact ordered spans
for 1,600 combinations of keys, prefixes, separators and suffixes. They include Unicode case
folding, line breaks, embedded NUL, quotes, placeholders and fullwidth punctuation. A separate
exact JSON assertion protects a sensitive key whose value contains no assignment delimiter.
Reversing the production guard made the span oracle fail; restoration passed.

A registry test parses every production credential regex with `regexp/syntax`. It rejects any
possible accepting path that avoids both delimiters. Empty paths and zero-width assertions
are conservatively allowed, alternation uses any branch, concatenation requires every part,
and unknown operations fail the test. This guards future rule changes: widening the credential
language cannot silently invalidate the optimization. The security-review subagent reviewed
both this reasoning and the structured-key boundary.

`go test -p 1 -count=1 -timeout 3m ./internal/secret ./internal/secret/secrettest` passed, as did
`go test -p 1 -count=1 -timeout 2m -run '^TestPhase6RedactionAudit$' ./internal/service`.
`go test -p 1 -count=1 -timeout 3m -run
'^TestGateTheSearchDoesNotReadPayloadsItDoesNotReturn$' -v ./internal/search` then passed in
15.18 s: thin **33.805 ms**, fat **66.643 ms**, ratio **1.97**, ceiling **3.00**. The earlier
3.15 failure is preserved. These are different runs, so the ratio difference is not a paired
causal speedup estimate. No gate or query was changed to obtain the pass.

The full `go test -p 1 -count=1 -timeout 10m ./...` then exited 0, including the unchanged
scaling gate; the search package took 248.649 s. The registry guard was also mutation-checked:
making the real credential regex's assignment separator optional caused its test to fail,
and restoring the pattern passed. The explicit corpus rescan passed over **901 payloads,
900 tagged before masking**, without printing any payload. Final small test additions cover
fullwidth equals, any-character, negated/ranged character classes and case-folded literals;
the targeted tests and pinned linter subsequently passed.

Targeted `go test -race -p 1 -count=1 -timeout 3m` with the existing local compiler covered
the delimiter, registry and span tests plus service Phase 6 and session-resume redaction audits.
Both packages passed (secret 1.556 s, service 2.901 s). The hour-long complete race corpus was
not rerun for a stateless guard. The full normal regression is the broader matching check.
No host configuration, installed binary or release artifact was changed during validation.
The previously verified package names an older source revision; a new release build remains
necessary before any publication. This fixes the observed regression gate on the validated
source, not automatic-selection precision or task-success requirements.
