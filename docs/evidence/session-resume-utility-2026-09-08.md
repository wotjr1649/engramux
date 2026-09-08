# Session-resume synthetic task utility, 2026-09-08

Three fresh solver agents reconstructed three previously accepted configurations and one
self-contained control. With no history they solved **0/3 history-dependent tasks**; with
actual FTS excerpts or actual exact-session replies they solved **3/3**. Every arm solved
the control using the current prompt despite conflicting old settings. Exact-session read
did **not** outperform FTS on these tasks.

An independent fixture-design agent specified the prompts, arbitrary accepted values and
expected compact JSON before either retrieval path ran. Its conceptual host IDs and
`assistant_stop` event name were adapted to the actual `claude-code`/`Stop` schema; accepted
values were unchanged. The fixed synthetic project path avoids emitting temporary user paths.
The dataset has one accepted Stop per target session, no competing target-session history,
no future records and no ranking stress. It is deliberately a basic configuration recovery
probe, not a realistic coding benchmark or representative corpus.

`TestMeasureSessionResumeTaskContexts` ingests the synthetic Stop through the existing test
ingest helper and calls the actual `search.Search` (session ID, MatchAny, limit 100) and
`sessionResume` implementations. It exports their returned text. The command run was
`go test -p 1 -count=1 -timeout 2m -run '^TestMeasureSessionResumeTaskContexts$' -v
./internal/service` with `ENGRAMUX_RESUME_SYNTHETIC=1`; the final synthetic-path run passed
in 0.08 s. This command generates inputs; it does not run or grade agent task performance.

The solvers were separate `fork_turns=none` agents, one per arm, with no model override and
identical framing except context. They received only public synthetic prompts and their
arm's context, never the answer key or another arm's output. They were told to answer
`INSUFFICIENT` instead of guessing missing settings and to use no tools beyond any mandatory
operating-contract load. Each agent handled all four tasks, so the tasks within an arm are
not independent agent runs. No repeated trials, model comparisons or confidence intervals
were measured.

The returned answer strings were compared mechanically with the predeclared exact JSON,
including key order and compact formatting. `INSUFFICIENT` is an appropriate abstention
but does not count as successfully reconstructing the requested configuration.

| Context | History-dependent exact answers | Self-contained exact answer |
| --- | ---: | ---: |
| None | 0/3 | 1/1 |
| FTS excerpts | 3/3 | 1/1 |
| Exact session reply | 3/3 | 1/1 |

The adjacent JSON preserves the public prompts, actual retrieved contexts, answer key,
observed responses and scores. Its response strings are agent observations, not expected
answers substituted for an unrun solver. The pinned linter subsequently returned `0 issues.`
with exit 0; `git diff --check` also passed. No production behavior changed in this step.

This supports only a narrow conclusion: supplying captured accepted values enabled these
solvers to reconstruct settings they otherwise lacked. It does not prove repository edits,
build success, time saved, user acceptance, natural-language search quality, automatic
selection precision, or M7 completion. The earlier real-corpus exact-ID recovery result is
a separate retrieval measurement and must not be combined with this synthetic result into
an unobserved real-task success score. Automatic injection remains off by default.
