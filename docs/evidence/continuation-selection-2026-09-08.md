# Current-session continuation experiment, 2026-09-08

This test-only candidate has a better development output distribution, but remains unadopted.
It is not an official M7 pass, independent holdout result or demonstrated task-success gain.
The shipped selector and its off-by-default setting are unchanged.

The candidate combines lexical admission with one prior assistant reply. Korean cues are
`이전`, `앞서`, `이어서`, `추천대로`, and `그럼` at a whitespace token's start, after stripping
surrounding ASCII sentence punctuation. English cues are whole tokens `previous`, `earlier`,
`recall`, `resume`, and `continue`. Matching is a heuristic, not semantic intent classification.
A canonical UUID in the prompt causes abstention rather than silently replacing a named source
or destination with the current session. Otherwise the source is the trigger's stored session,
exact project and known host, selecting its latest strictly prior Stop by receipt time and rowid.

The whole payload is masked before reading `last_assistant_message`. A body longer than 480
runes contributes its first and last 240 runes with an explicit middle-omitted marker. Assembly
uses the existing nonce fence and 5,000-byte cap. Lookup, counting, masking and assembly share
500 ms with a final deadline check. More than 200 prior Stop candidates abstains; oversized
payloads, unusable references and missing bodies are not emitted. The harness supplies stored
session metadata; this path has not been wired into the production request contract or checked
as a replacement for every native-memory and injection capability.

## Development correction before acceptance

The initial substring cue accidentally matched `이전` inside `서브에이전트`. Its original
150-prompt replay emitted on 20 wanted prompts, 20 blocks / 18,807 bytes, with no unwanted
bytes. That result is preserved in `continuation-temporal-wanted-review.json` but is not the
corrected candidate's result. A new test exposed the error. Token-start matching fixed it;
changing that match back to a substring made the test fail, and restoration passed.

The first retrieval fixture also lacked a recognised host marker. Supplying a synthetic
Claude transcript path corrected the fixture; the candidate's known-host check was retained.
The original builder did not retrieve the prior answer to the synthetic continuation request.
The candidate does, while tests confirm silence for equal-time events, other projects, other
sessions and unresolved named scopes.

## Corrected v2 replay and separate agent judgements

`go test -p 1 -count=1 -timeout 3m -run '^TestMeasureTemporalSelection$' -v ./internal/inject`,
with `ENGRAMUX_TEMPORAL_AGENT_DIR` naming the existing private agent labels,
`ENGRAMUX_TEMPORAL_CONTINUATION=1` and `ENGRAMUX_WRITE_TEMPORAL_REVIEW=1`, passed in 18.24 s.
The separate 23-prompt run used `ENGRAMUX_TRANSFER_REPLAY=1` and
`ENGRAMUX_TRANSFER_CONTINUATION=1` with `TestMeasureTransferDevelopment`; it passed in 11.87 s.

| Population / arm | Wanted output | Blocks | Bytes | Unwanted bytes | Deadline abstentions |
| --- | ---: | ---: | ---: | ---: | ---: |
| Original 150, recorded baseline | 9/59 | 93 | 31,874 | 20,917 | 0 |
| Original 150, continuation v2 | 14/59 | 14 | 13,034 | 0 | 0 |
| Transfer development 23, baseline | 0/6 | 24 | 9,317 | 9,317 | 0 |
| Transfer development 23, continuation v2 | 2/6 | 2 | 2,287 | 0 | 0 |

The original baseline's prompt count, bytes, wanted emissions and unwanted bytes were
recalculated from its preserved trigger metrics. Both corrected exports use new exclusive
filenames; neither overwrote v1 or any previous experiment. The two transfer excerpts were
compared with their previously inspected versions and were identical.

Root judged the displayed excerpts separately from the already frozen prompt labels:

| Population | Yes blocks / bytes | No blocks / bytes | Unknown blocks / bytes |
| --- | ---: | ---: | ---: |
| Original 150 | 10 / 9,788 | 1 / 586 | 3 / 2,660 |
| Transfer development 23 | 1 / 1,152 | 0 / 0 | 1 / 1,135 |

The no block concerns a different implementation step from the one requested. Unknowns lack
enough displayed decision or task detail to establish usefulness, including the transfer
request asking for an execution location. Yes excerpts give the relevant prior recommendation,
next task, status or evaluation conclusion. They do not verify those historical assertions as
current facts. Known relevant shares are 9,788/13,034 (75.10%) and 1,152/2,287 (50.37%), with
unknown not credited. There are ten and one wanted requests with known relevant output,
respectively. All are exposed development data judged by an agent, not owner assessments.

`python -X utf8 docs/evidence/label-continuation-v2.py` checks both output hashes and writes
separate private judgements without modifying any original labels. Its counts match the table.
Same-session relevance does not prove added value: the host may already have the answer in
its current context, and that visibility was not measured.

## Remaining failure and adoption boundary

The candidate still injects an unrelated prior answer for the synthetic request
`Explain the continue keyword`. Its test deliberately reproduces that counterexample rather
than claiming admission quality passed. The independent selection reviewer also identified
quoted cues, negation, topic changes inside one session and omitted middle decisions as risks.
Therefore no fresh holdout is consumed yet and no general automatic-selection claim is made.
The 108-prompt holdout remains unopened. Admission correctness and applicable injection gates
must be addressed before this candidate can be frozen for that evaluation.

Targeted continuation tests, the cue mutation check, the pinned linter and `git diff --check`
passed. Full regression and race were not repeated for these opt-in, test-only changes.
