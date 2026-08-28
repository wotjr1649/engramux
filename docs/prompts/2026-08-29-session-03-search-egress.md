# Session 03 — Engramux Phase 4: search, the egress half

Session 02 built the index and the gate; this session builds the read path over the pipe and
closes Phase 4. `CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your
context. This file carries what they cannot: the state of the work when the session opened, the
decisions session 02 took and what measured them, and what was left open on purpose.

Session 02's ledger — every ruling, every review verdict, every adjudicated finding, and the
per-task reports — is at `.superpowers/sdd/2026-08-28-session-02-search/progress.md`
(gitignored, local only). Read its `Ruling:` lines before reopening anything below.

---

## 1. Where the work stands

| | |
|---|---|
| Branch | `phase-4-search`, branched from `main` @ `743e364`. **`main` is untouched** and still 41 commits ahead of `origin/main`. Do not push |
| HEAD | `8e6116f` (T5, amended once — §6), 13 commits on the branch, working tree clean |
| Last full verification, at that commit, with the live service running | `go build ./...` ok · `go test -p 1 -count=1 ./...` **13 packages ok, `internal/search` FAIL** — `TestPhase4Gate`'s three corpus subtests at 24 of 25 (§2), nothing else red · pinned linter `0 issues.`, **exit 0** · `./scripts/race.sh -count=1` reported no data race (T5's run; its exit 1 is the same gate failure) |
| Tasks done | T1 pipe override · T2 the gate · T3 schema and triggers · T4 what gets indexed — each reviewed by a fresh subagent, with fix rounds where the review found something, plus a read-only Codex pass per task, folded in. **T5** query construction landed and was reviewed; its Critical was fixed at close (§6) and its two Importants are open (§4) |
| Tasks open | **T5 round 1** (two Importants from its review), **T4 round 2** (three Codex findings), **T6** search over the pipe and the egress, **T7** the gate green from clean plus the documentation pass, then **S1** (the user's Codex `SessionEnd` timeout request), then the merge decision |
| The live service | **Running the old binary**, task `\Engramux`. The 00002 migration has never touched the live database. Since T1 the suite runs beside it — do not stop it |

Merge only when Phase 4's gate passes from clean, the way session 01 did: fast-forward `main`,
delete the branch, hold the push.

---

## 2. What counts as done

Unchanged: spec §8's Phase 4 row — known-item retrieval gated per class, five classes, one
precision assertion, over the fixtures always and `.capture/` when present. The gate is
`TestPhase4Gate` in `internal/search` (package `search_test`).

Where the gate stands after T5, at `8e6116f`: **fixtures — all five classes and the precision
assertion pass.** Corpus — two tokens 25 of 25, path basename 25 of 25, precision 76 matched
against a bound of 103 with no stranger; **two-character Korean, particle and camelCase 24 of
25 each**, one miss apiece, so those three subtests are red and the suite exits 1 on
`internal/search` alone. The misses are not the builder's: `단계` cannot reach `0단계가`
because the Hangul run sits inside a digit-led token and a prefix anchors at a token start
(a derivation that takes a run mid-token, or a tokenizer limit — decide which); `replay`
cannot reach `replay를` because porter stems the ASCII query token to `replai` while the
mixed-script index token passes through unstemmed — the porter-plus-prefix hole the docs
review predicted, now measured in the gate. Session 02's ruling stands: a class below its
constant is investigated and either fixed or the constant lowered **with the reason written
into §5.7** — never silently. The controller's recommendation, unmeasured: derive the Korean
run only at a token boundary (the class tests prefix reachability, and §5.7 already says a
prefix index does not promise mid-token matches), and measure the gate with porter dropped
(`unicode61 remove_diacritics 2` alone) before deciding the tokenizer — the docs review found
porter inert on Korean and its real cost is precision on identifiers, which is most of this
corpus.

T6 adds the sixth clause — a runtime-generated secret findable by a term that is not the
secret and absent from the returned excerpt while the row still holds it — and T7 runs the
whole thing from clean, with the live service up, and writes the numbers into §5.7 and §8.

---

## 3. Measured this session — do not rediscover it

All observed, on modernc.org/sqlite v1.57.0 (SQLite 3.53.3) unless stated.

**The pipe override.** `ENGRAMUX_TEST_PIPE_SID` (`ipc.TestPipeSIDEnv`) stands in for the SID as
the hash input; read in exactly one place, `ipc.CurrentPipeName`; `pipe.ListenCurrent` derives
through it; the DACL keeps the real SID. Every test that listens or launches a binary calls its
package's `useTestPipeName(t)` first. `t.Setenv` forbids `t.Parallel` in those packages — Go
1.27's own panic, quoted in `AGENTS.md`. Whether `-p 1` can be dropped is `[unverified]`; the
guard that insists on it is a Claude Code `PreToolUse` hook on the maintainer's machine, so a
git-hook or shell-profile search finds nothing.

**Raw JSON is a precision disaster, measured by the gate itself.** With the index over
`events.payload`, a query for the key `cwd` matched 900 of 901 corpus documents (bound 103) and
4 of 4 fixtures (bound 0). Over the collected string leaves it matches 76 of 901, all of them
carrying the word in a leaf. Recorded in §5.7 as the amendment the brief asked for: the index
holds the **original** string leaves — not raw JSON, not a masked form — masked at egress.

**The leaves column.** `store.Leaves` walks the decoder's token stream in document order,
keys skipped, empty leaves kept, newline-joined, gated by `json.Valid` (a JSON *stream*
`{"a":"x"}{"b":"y"}` is refused, as `json_valid` refuses it). The 00002 migration adds the
column, backfills existing rows in SQL with `json_tree` (`group_concat(... ORDER BY id)` inside
the aggregate, `coalesce` for a payload with no string leaves, `json_valid` in front because
`json_tree` *raises* on malformed JSON and a raise inside the migration is a service that does
not start), then creates `events_fts` over it and `rebuild`s. `TestTheTwoWalksAgree` migrates a
version-1 database holding 917 payloads (fixtures, shapes, the corpus) and asserts the SQL text
equals the Go text per row. A malformed payload is reachable in production — the relay spools
what it cannot envelope and the drain hands the bytes to `Ingest` verbatim — which is why the
guard is load-bearing.

**Three known divergences between the two walks**, adjudicated: invalid UTF-8 and a lone
surrogate (Go yields U+FFFD, `json_tree` keeps the bytes — pinned by a test, and the
`fts5vocab` test shows unicode61 tokenises both spellings the same); a valid out-of-range number
such as `1e400` (Go's `Token()` fails without `UseNumber`, SQL keeps the strings); nesting
deeper than SQLite's limit of 1,000 (SQL says invalid, Go walks it). The last two are **T4
round 2**.

**`json_tree.id` is document order by observation, not by documentation.** SQLite guarantees
uniqueness only. The agreement test is what pins it; it belongs in `AGENTS.md`'s
re-verify-on-upgrade rows (T7).

**Newline is not a phrase boundary.** unicode61 drops it as a separator, so a phrase spans two
leaves. The builder never emits a multi-token phrase from user input (only a single token with
internal punctuation, `main_test.go`), so the exposure is a negligible cross-leaf false
positive; the comment in `leaves.go` that claims otherwise is wrong and is T4 round 2's third
item.

**The prefix index is gone, priced out.** Two-character Korean prefix queries: 129 µs without
it against 84 µs with it at 901 events; 792 against 643 at 18,020. The index costs 2.70–2.79×
(21.3 MB → 59.5 MB at 18,020 events). Harness: `BenchmarkPrefixIndex` in `internal/search`,
one cold run per process — a warm second run in the same process peaks at half a cold one.

**`project_id UNINDEXED` is gone, measured useless.** On an external-content table the
UNINDEXED value is read from the content table per candidate and the filter is applied
outside the vtab cursor: `EXPLAIN QUERY PLAN` shows `SCAN events_fts VIRTUAL TABLE INDEX
32:M2` with or without the filter, and the join form adds only `SEARCH e USING INTEGER
PRIMARY KEY (rowid=?)`. Phase 5 scopes a search by joining `events` and filtering
`events.project_id`.

**`integrity-check` catches a dropped trigger only with `rank=1`**, and on this driver the
error is `database disk image is malformed (267)` — SQLITE_CORRUPT_VTAB — asserted by code
through the driver's error type. The bare form and `rank=0` pass on the same desynced index;
both are committed as controls. FTS5's own `secure-delete` is set on the table.

**The rowid rule.** `events` keys the index on its implicit rowid; SQLite documents that
`VACUUM` may renumber a table without an INTEGER PRIMARY KEY. Nothing in 1.0 runs `VACUUM`; a
future `VACUUM` or textual restore must be followed by `rebuild`. Written in the migration
header; a gotcha row is T7's.

**The WAL bound after the triggers.** `TestTheWALStaysBoundedAcrossALongRun`'s bound is
`5*threshold` (655,360 B), derived from a measured table: pre-index peaks 2.0–2.2× the
threshold, with triggers 3.2–3.9×, a 2× miscalibration 4.4–5.2×, a 4× one 6.4–7.2×. It
resolves the 4× case and not the 2× one, and says so in the code.

**Query construction.** Quoted-prefix form, `"token"*` joined by spaces, measured over 25 shapes
with no syntax error: a token that tokenises to nothing is dropped from an AND rather than
zeroing it; an all-empty expression returns 0 rows, no error; `"AND"*` matches the word as
content; `foo:` quoted is content, not a column filter; porter's hole is real (`"correcti"*`
returns 0 against "corrected corrections" while `"correct"*` returns 1). T5 ships it as
`queryTokens` (the token list, which T6's excerpt needs) and `matchExpression` in
`internal/search`, with `ErrEmptyQuery`, `ErrTooManyTokens` and `ErrTokenTooLong` as the
sentinels and `maxQueryTokens` / `maxTokenBytes` as the bounds; ten exact expressions, six
bounds cases and a ten-row end-to-end test pin it. The two-token AND-containment check the
gate gained is vacuous over the fixtures (every fixture pair selects one document) and
discriminates 45 of 50 over the corpus; it logs the count rather than failing — one fixture
pair that discriminates would make it able to fail there (T7).

---

## 4. Tasks

Briefs for T6 and T7 already exist in the ledger's workspace (`task-6-brief.md`,
`task-7-brief.md`); they were written against session 02's rulings and are still right. Adjust
them where the code says otherwise, and say so.

**T5 round 1 — the gate's AND check, able to fail.** T5's review (in the ledger, verbatim)
left two Importants: the ruling-mandated two-token containment assertion is vacuous over the
fixtures — `sharp`, the count of terms whose result set exceeds the pair's, is logged and never
asserted, so the mode a contributor without `.capture/` runs reports a pass for a check that
gated nothing. Fix: assert `sharp > 0` beside the log, and give fixtures mode one extra
test-owned document (built the way `t5Payload` is) that repeats exactly one word of one
fixture's derived pair, chosen so the precision precondition still holds; and correct the
helper's doc, which names `maxDocsPerClass` as a reason for containment though both sides run
at `len(docs)`. The reviewer's minors are in the ledger under `Task 5`; the one worth taking
now is the §7.1 row for "a term that tokenises to nothing is dropped from the AND", reproduced
by `TestSearchSurvivesRawInput`.

**T4 round 2 — the two walks, closed.** `dec.UseNumber()` and an agreement case with `1e400`; a
depth guard mirroring SQLite's JSON limit of 1,000 with an agreement case one level past it;
the newline claim in `leaves.go` corrected and the measured behaviour pinned with a MATCH-based
test. *Gate:* `TestTheTwoWalksAgree` equal over its whole set, corpus present. Small; do it
first.

**T6 — Search over the pipe, and the egress.** As briefed. What the brief cannot know: the
builder's token list and sentinels are T5's (names in T5's report); the excerpt walks the
leaves of `secret.Mask(payload)` with `store.Leaves`; never `snippet()`, never `highlight()`,
never `secret.MaskString` on a fragment.

**T7 — Phase 4's gate, green, from clean.** As briefed, plus one thing session 02 queued:
**verify the upgrade path against a copy of the live database** before anyone installs the new
binary. Stop the service, copy `engramux.db` (a clean close leaves no WAL), start the service,
then migrate the copy in a throwaway harness under the pipe override and check `integrity-check`
with `rank=1`, the row and index counts, and a search. That is the only run of the 00002
migration over real data with real shapes, and the `json_valid` guard exists because real data
surprises. Record the outcome; do not install.

**S1 — the user's Codex `SessionEnd` timeout request** (queued 2026-08-29, verbatim in the
ledger). Codex warns *clamping SessionEnd hook timeout to 3s* on `~/.codex/hooks.json`, where
the engramux `SessionEnd` command hook carries `timeout: 5`; Codex's `SessionEnd` default is 1 s
and its maximum is 3 s. Asks: trace the code that writes or updates that file (candidate:
`scripts/install-hooks.mjs`; nobody has verified yet that it wrote this entry); set the Codex
`SessionEnd` hook's timeout to at most 3 — explicit 3 unless 1 s completion is certain; leave
every other event's timeout alone; make the upgrade path over an existing `hooks.json` correct a
wrong 5; a minimal regression test that the generated `SessionEnd` entry has `timeout <= 3`;
never work around it with `async`. Done means all of that plus "the warning does not reproduce at
Codex load" and a report of the changed files and the verification run. Boundary: an agent does
not edit `~/.codex` — the user runs the install command; verify the warning after that. Spec
§7.3's "Codex clamps `SessionEnd` at 3 s — still not measured" row is now observed (the warning
text is the evidence) and gets updated in the same change. The relay's own ceiling is 1 s (spec
5.3), p95 1.04 ms; 3 s is a ceiling it never approaches.

Then the merge decision, which is the user's.

---

## 5. Open, and what to do about each

| | |
|---|---|
| Deferred minors from every review | Listed in the ledger as `minor (deferred)` lines, one per finding, with file:line. The final whole-branch review triages them; do not fix them ad hoc |
| A UNC `cwd` in a captured payload reaches `os.Lstat` through `project.Identify` | Pre-existing Phase 1 ingest behaviour, raised by a Codex review of T2. Not Phase 4's. Carry it beside spec §10 question 2 |
| Invalid UTF-8 in a stored payload | Tokens agree, exact text does not; no live-database evidence either way. Accepted, pinned |
| `engramux doctor` could say when `ENGRAMUX_TEST_PIPE_SID` is set in its own environment | A leftover export in a shell moves the relay and the service apart (events spool and drain; nothing is lost). Deferred minor, not in scope |
| Four copies of the corpus loader across test packages | Accepted twice on review; the trigger to factor a test-support package is written in `leaves_test.go` |
| The corpus loader in `internal/store` returns nil without the corpus | The agreement test then covers 16 payloads, not 917, and says so in its log; the `[verified]` row states the count is with the corpus present |
| Codex reviews | Non-blocking, one per task, dispatched after the commit; the forwarder subagent times out at ten minutes and backgrounds the job — read the result with the companion script's `status`/`result` commands, never redispatch. T5's review was still running when session 02 closed: job `task-mtd9omx3-fjuo4l`. Read it before T6; the ledger says how |

---

## 6. How this session runs

Exactly as session 02: `superpowers:subagent-driven-development`, a fresh implementer per task,
a fresh task reviewer, fix rounds resumed on the same implementer, scoped re-reviews, the
ledger as the recovery map. Measure before dispatching — every task that went smoothly had its
load-bearing facts measured first (the ledger's `Measured before` blocks are the pattern), and
every task that needed a fix round had a fact nobody had checked. The break-it step is where
the findings came from: a WAL bound that only shrank its own divisor, an integrity check whose
short form cannot fail, a walk that accepted a JSON stream — none of them was found by reading.

Three things that bit this session and will bite again. A heredoc collapsed `\\u` and `\\n`
in a Go test literal mid-edit (caught by gofmt and a byte-level readback; `AGENTS.md` already
says to use a file-write tool, and it is right). A review finding was wrong on the fact and
right on the standard — the fix was to make the claim checkable, not to delete it. And **T5's
commit carried the author's real Windows username and project directory in five test
literals**, against the boundary in `AGENTS.md` and against the `C:\Users\fixture` placeholder
every other test uses; the task review caught it, the controller renamed the literals and
amended the unpushed tip so the name never enters history (the reflog keeps the old object).
Grep every commit for a username before you call it done — the implementer had "re-read the
committed blob" and still missed it, because it was checking escapes, not names.

---

## 7. First action

Read the ledger's `Ruling:` lines and T5's review verdict. Then T5 round 1 and T4 round 2 —
both small, both closing checks that cannot yet fail — then T6. When Phase 4's gate passes
from clean, T7; then S1; then stop and report. Do not push.
