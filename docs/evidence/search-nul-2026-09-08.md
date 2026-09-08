# NUL query refusal, 2026-09-08

Extending M8 P5 diagnostics to its 91 all-no groups exposed an existing search input bug.
One derived query contained 36 NUL bytes in 222 bytes and produced an FTS `unterminated string`
error. No original query text or capture identity was printed. A synthetic query containing
an embedded NUL reproduced the same failure through the injection path.

The shared query tokenizer now returns `ErrNULQuery` before SQLite receives such input.
It does not remove the byte, guess an encoding, search only a prefix, or change the stored
event. Injection handles this typed refusal like its existing invalid-query cases and emits
zero bytes. Other search consumers receive a descriptive input error without the query value.
This changes handling of malformed queries, not ranking or M7 thresholds.

Tests for embedded, trailing and standalone NUL first failed; the injection test also returned
the original SQLite error. After implementation both passed. Mutating the NUL check to inspect
a different control character made all three parser cases fail; restoration passed in the
subsequent checks. The negative-group diagnostic counts NUL refusals explicitly and retains
them in the denominator rather than silently skipping failed queries.

The focused command was `go test -p 1 -count=1 -timeout 3m -run
'^Test(QueryBounds|BuildAbstainsOnAPromptWithNothingToSearchOn|EvaluateM8P5AgentEstimates)$'
-v ./internal/search ./internal/inject`, with the existing M8 agent labels and turn expansion
enabled. The M8 evaluation passed in 9.04 s; the injection case passed as well. The full suite
subsequently passed with `go test -p 1 -count=1 -timeout 10m ./...`, exit 0; search took
250.875 s. A public-path test added while that run was in progress then separately verified
both event and native search return the sentinel before touching a nil database, and omit
synthetic query values from errors. The focused search/injection checks and pinned linter
passed afterwards. The same focused boundary checks passed under `-race` using the existing
development compiler. The full race corpus suite was not rerun for this stateless input check.

The failure-fix review agent confirmed typed refusal rather than silent normalization and
requested the public-path tests. Inspection of the service caller found the abstention reason
is logged and returned, not used to authorize or select work. The existing invalid-query
abstention category is retained. An attempted combined caller search was rejected by the
shell guard as a nested-shell command; it was not retried, and narrower direct reads of the
internal files supplied the needed caller evidence.
