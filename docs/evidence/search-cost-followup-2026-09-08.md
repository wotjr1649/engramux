# Search scaling failure and cost diagnosis, 2026-09-08

The full `go test -p 1 -count=1 -timeout 10m ./...` run at `7ec1137` exited 1.
The event payload scaling gate failed: thin 30.657 ms, fat 96.496 ms, ratio **3.15**
against the unchanged **3.00** ceiling. The search package took 229.270 s; every other
tested package passed. This failure remains unresolved. Earlier green runs do not replace it.

No query or gate was changed. `go test -p 1 -count=1 -timeout 3m -run
'^TestDiagnoseSearchCostPhases$' -v ./internal/search`, with
`ENGRAMUX_SEARCH_COST_DIAGNOSTIC=1`, passed in 20.65 s. The existing instrument uses the
actual query and masking/excerpt functions and checks exact equality with public results.
It retains 20 returned events from 20,000 matches. Post-warm-up samples were:

| Arm | SQL samples, ms | Postprocessing samples, ms | Returned payload bytes |
| --- | --- | --- | ---: |
| Thin events | 56.0343, 50.1803, 42.5292 | 0.3833, 0.3769, 0.3275 | 1,500 |
| Fat events | 73.3361, 58.6148, 54.5192 | 39.5216, 41.8904, 40.3514 | 164,060 |

The paired SQL ratios are 1.17–1.31. The returned-document processing cost is therefore a
useful next investigation target. This does not prove SQLite read no unreturned payload,
nor does the lower diagnostic ratio invalidate the original failure. The failure-review
subagent confirmed those limits. Separate execution, calibration, cache and sequential arm
ordering prevent substituting one run's timings for another's gate result.

The same synthetic diagnostic ran with `-cpuprofile` and `-o` pointing into a new private
`.capture/search-cost-followup-2026-09-08` directory; it passed in 22.543 s. `go tool pprof
-top -nodecount=12` showed that the whole profile includes substantial SQLite corpus-building
work. A second report with `-focus='internal/secret'` showed regexp machine and backtracking
work in the masking path. The profile is not a postprocessing-only sample, so its aggregate
percentages are not presented as search phase shares. No real captured payload was profiled.

`TestDiagnoseRuleCost` separately measures the existing regexes on an 8,192-byte public
synthetic nonsecret string. `go test -p 1 -count=1 -timeout 2m -run
'^TestDiagnoseRuleCost$' -v ./internal/secret`, with `ENGRAMUX_RULE_COST=1`, reported:

| Rule | Initial ns/op | Final instrument ns/op |
| --- | ---: | ---: |
| API key | 315,823 | 256,392 |
| Private key | 198 | 155 |
| Authorization | 339,802 | 280,229 |
| Credential | 796,630 | 459,017 |
| Connection string | 180,057 | 142,723 |
| Dotenv | 96,427 | 72,121 |
| Opaque token | 395,039 | 302,146 |
| User path | 105,860 | 68,306 |

The final instrument explicitly propagates a failed benchmark correctness check to its parent
test; both runs found zero matches as expected. The final run passed in 11.79 s. These are
separate measurements, not a before/after optimization: production rules did not change.

The credential regex is the largest cost in both runs and requires a literal `:` or `=`.
Testing a necessary-delimiter guard is the next bounded hypothesis. A structured sensitive
JSON key is handled separately and must still mask its entire value when no delimiter occurs
inside that value. Any optimization needs equivalence and adversarial masking checks before
activation; no pattern, privacy boundary or gate threshold is relaxed by this diagnosis.
The pinned linter and `git diff --check` passed for the new diagnostic. The full-suite scaling
failure remains open; automatic-selection quality and actual task utility remain separate
unfinished requirements.
