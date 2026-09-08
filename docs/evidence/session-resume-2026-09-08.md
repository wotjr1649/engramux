# Session resume measurement, 2026-09-08

The exact-session reader is specified in
`docs/superpowers/specs/2026-09-08-session-resume.md`. It addresses an observed workflow:
asking to resume a named session retrieves short metadata matches through FTS, while the
last main answer can fall outside the maximum returned hit count.

A frozen local evaluation pair was taken with the installed service stopped: a 465,436,672 B
database and its 766,352 B WAL. The same turn restarted the existing scheduled task.
The endpoint subsequently reported 41,734 events, zero queued spool events and zero errors.
This was a snapshot operation, not an installation or host-configuration change. Original M7
and M8 inputs were preserved. Private database, WAL and review text remain under `.capture/`.

`TestMeasureSessionResumeOverFrozenHistory` opens that pair read-only. Its reference population
is every known-host session with a captured main `Stop` in the requested project. It enumerates
all candidate metadata and selects the latest timestamp/rowid tuple in Go, independently of the
candidate SQL window. The baseline searches the literal host session ID with the shipped
`MatchAny` search and the maximum 100 hits. The candidate reads the exact session.

The command run was `go test -p 1 -count=1 -timeout 3m -run
'^TestMeasureSessionResumeOverFrozenHistory$' -v ./internal/service`, with
`ENGRAMUX_RESUME_SNAPSHOT` naming the private frozen database and `ENGRAMUX_RESUME_PROJECT`
naming the project worktree. It passed in 0.88 s.

| Latest captured main reply | FTS top 100 | Exact session read |
|---|---:|---:|
| Recovered event reference | 6 of 32 | 32 of 32 |
| Nonempty answer bodies | Not scored | 32 of 32 |
| Shortened bodies | Not scored | 14 of 32 |

The slowest candidate read was 11.6275 ms in this run. This is an observed warm local result,
not a latency percentile or a performance guarantee. Both arms read the same frozen history;
this is a retrospective known-item comparison, not a temporal M7 evaluation.

For the session named in the original resume request, the final captured answer was outside
the FTS top 100. The exact reader returned its 2,372 B body without truncation. Local inspection
confirmed it contained the prior session's completed packaging/correction state and the next
work list: reviewing and pushing the accumulated commits, M7 assessment, and release prerequisites.
That makes the historical handoff reachable for the actual initiating workflow. Its statements
are historical and require current repository verification before acting on them.

This does not establish general retrieval relevance or an agent task-success rate. No owner
labels were assigned, no M7 threshold was changed, and injection remains disabled by default.
The candidate's own mechanical checks cover scope, deterministic ties, allowed fields, null and
empty cases, UTF-8 bounds, masking, MCP arguments/results/refusals and the shared read gate.
Deliberately breaking project matching, widening event host matching, reversing the rowid tie
order, or bypassing whole-payload masking caused the relevant tests to fail. Each mutation was
removed, and the focused tests passed again.

The first full `go test -p 1 -count=1 -timeout 10m ./...` run failed only the existing event
payload-scaling gate: 20.862 ms for 64 B documents versus 63.398 ms for 8,192 B documents,
ratio 3.04 against the unchanged 3.00 ceiling. `git diff --exit-code HEAD -- internal/search`
returned 0. An isolated run of `TestGateTheSearchDoesNotReadPayloadsItDoesNotReturn` passed
at 20.979 ms versus 57.256 ms, ratio 2.73. These observations do not establish the cause of
the full-suite failure; the search implementation and threshold were not changed to clear it.

A second full run passed the event gate but failed its existing native-memory counterpart:
4.720 ms for 64 B bodies versus 13.027 ms for 8,192 B bodies, ratio 2.76 against 2.00.
No full-suite pass is claimed. Both runs passed the other packages. The session feature's
focused tests and focused race run passed; the pinned linter exited 0 with no issues after
adding the evaluation reader's deferred row cleanup. Separating SQL and returned-body
processing cost is the next diagnostic; changing a threshold is not the proposed remedy.
