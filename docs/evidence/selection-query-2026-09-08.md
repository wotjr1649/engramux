# Selection query diagnosis, 2026-09-08

`TestDiagnoseSelectedQueryTerms` measures the existing selector against the same strict
ingest-prefix index as `TestMeasureTemporalSelection`. It changes no selection rule and
prints only aggregate counts. The 150 already-exposed prompts are development evidence,
not an independent holdout. Wanted-context labels are agent estimates.

The command run was `go test -p 1 -count=1 -timeout 3m -run
'^TestDiagnoseSelectedQueryTerms$' -v ./internal/inject`, with
`ENGRAMUX_QUERY_DIAGNOSTIC_DIR=../../.capture/m7/agent-2026-09-08`. It passed in 15.90 s.

| Agent label / script | Prompts | Selected terms | Absent terms | Terms matching over 200 events | Prompts with an absent term | All terms absent | Empty AND | Empty intersection although every term exists |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| no / hangul | 4 | 6 | 4 | 0 | 2 | 2 | 2 | 0 |
| no / latin | 75 | 205 | 125 | 5 | 58 | 26 | 61 | 3 |
| no / mixed | 12 | 36 | 21 | 0 | 12 | 1 | 12 | 0 |
| yes / hangul | 36 | 105 | 44 | 1 | 28 | 5 | 33 | 5 |
| yes / latin | 6 | 18 | 3 | 2 | 2 | 0 | 6 | 4 |
| yes / mixed | 17 | 46 | 5 | 3 | 4 | 0 | 11 | 7 |

Of 59 prompts estimated to want context, 34 select at least one term absent from the
earlier project corpus; another 16 have no AND intersection despite each term matching
individually. All six Latin-script wanted prompts have an empty intersection. The problem
is therefore not explained by a Korean/English boundary alone.

Relaxing the conjunction is not yet an improvement: 72 of the 91 prompts estimated not
to want context also contain an absent selected term, and relaxing them can introduce
unwanted results. Relevance, useful coverage and task success remain unmeasured by this
diagnostic. Any candidate must be fixed before a separate holdout is evaluated.
