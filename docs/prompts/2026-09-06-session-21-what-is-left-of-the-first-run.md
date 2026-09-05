# Session 21 — Engramux: what is left of the first run

Session 20 took the three rows session 19's grilling had decided — 47, 36 and 48 — and closed two of
them and half of the third. What a stranger's first minute shows is different now: `sessions` with
no argument answers about every project, a memory item's title is the first line that is not a label,
and an excerpt no longer begins in the middle of a word.

**What is left of that scene is one question, and it is a measurement rather than a change.** Whether
tool-plumbing events should rank below documents carrying human text is backlog 48's remaining half,
and its gate arm is now designed and un-run: **memory spec §5's M11**. Read that section before
touching the ranking, because it contains a way to close the row with no code at all.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read, in this order: memory spec §5's **M11 section** (*What M11 will measure, and why it is not
simply done*), then `docs/superpowers/backlog.md`'s row **48**. The 1.0 spec's §5.9 carries what
changed about `list_sessions` and why. Session 20's work order is superseded by this document.

**Written 2026-09-06, by session 20.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Session 20's eight commits on top of `1faf88c`, two of them `--no-ff` merges; tree clean. **Nine commits are unpushed** — `1faf88c` was already ahead when the session opened. `origin` is public, and the last push was authorised once, for that push |
| Checks | Suite **exit 0, 21 packages**; pinned linter **`0 issues.` exit 0**; `scripts/race.sh` **exit 0, 21 packages** — in that order and not concurrently, on `9978f04`, which is this document's parent and differs from it in no code |
| Installed | `dist/` and the installed binaries predate session 19. Nothing has needed installing since; this session's changes are reachable through the CLI and the service, so a run against the installed build shows the old behaviour until one is |
| Gates | M1–M6, M9, M10 pass. **M7 is un-run**: `prompts.tsv` is 150 of 150 `TODO`. **M11 is designed and un-run**, and gates a ranking change rather than a feature |
| Publication | 1, 2 and 4 closed. **3 stays open** and is not writing: the `README` names the detection, gives the steps and says itself they are `[unverified]`. What is owed is a human walking them through the Windows Security UI |
| Backlog | **37, 38, 40, 46, 48, 51, 52** open. 36 and 47 are gone, 48 is narrowed to its ranking half |
| Injection | Built, off, untouched |

---

## 2. What session 20 closed, in one paragraph each

**Backlog 47.** `sessions` with no argument resolved the working directory while its sibling `search`
has always been corpus-wide, and the first person to run both read six hits and `no sessions` as
capture being broken. Empty now means every project on both, which is what §5.2's request set already
meant on the wire — `list_sessions` was the one handler that refused it. `ListSessionsReply.ProjectRoot`
was singular and could not survive the change, so **the root travels per session**: `store.Sessions`
joins `projects`, the service masks it per row, and the CLI prints it as a column. The MCP schema is
untouched, where a project stays required because the caller is a model with no working directory.
Spec §5.9 carries the decision, including which half of "an existing CLI invocation must not change
what it returns" is withdrawn and why the direction is what makes it safe.

**Backlog 36.** A title is now the first line that is not a label this reader recognises. The set is
closed and measured rather than listed: `Outcome`, `Rollout context` and `Reusable knowledge` open a
line of 55 of the 55 rollout summaries on this machine, `Preference signals`, `References` and
`Key steps` of 54, and nothing else capitalised reaches more than one file. **One thing was not in the
row and had to move with it**: `codexBlock` replaces each recognised field label with its bare value
before the body is indexed, so a title taken from the body was the thread id — measured, a UUID on
all 55 header blocks. The title is chosen from the block as it was written instead.

**Backlog 48's display half.** The excerpt window was centred on the match and aligned to nothing, so
it began in the middle of whatever word the arithmetic landed in. Each edge now moves to the nearest
word boundary within 32 runes and stays put when there is none — a UUID, a Windows path and a
language that does not space its words all look the same to a space-separated rule. The two edges are
decided from the *unaligned* window, so the excerpt gives runes back rather than sliding down the
text, and 240 becomes a ceiling. The Phase 4 egress gate's width assertion became a range for exactly
that reason; the exact geometry belongs to `internal/search`'s own excerpt tests.

---

## 3. What is left, and what has already been decided about it

### 48's ranking half — the arm is designed, and it can close the row without code

**Read M11 before writing anything.** Its non-vacuity arm is the part to run first: measure how often
a plumbing document outranks the human-text document a query was cut from, over `.capture/`'s corpus,
at the current weight. **If that is rare, the row closes on the number and no lever is built** — the
first run's six hits were a property of a corpus of a few dozen events rather than of the ranking,
which is exactly the failure §7.1's own row warns about, a figure taken over a corpus that does not
resemble the real one.

If it is not rare, the gate is M4's shape: both arms of one run over one corpus, the gain arm over
classes cut from human text and **the harm arm over M4's own three classes unchanged**. The harm arm
is the point. A weight that lifts prompts by burying the document that actually ran the command has
moved the defect rather than fixed it, and a gate with only the gain arm cannot see that.

### What nobody has looked at

Rows **37, 38, 40, 46, 51 and 52** are untouched by this session. 46 is the one with a shape a
session could take on directly: `ReasonNoHits` covers three situations and `ReasonTooBroad` absorbs a
fourth, so the service log cannot tell recall from silence — its row says what a fix is and why
session 15 deliberately did not do it.

---

## 4. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed**, and `origin` is public. `git log -p` your own diff for
`Users\<name>` before asking for a push: session 19 published this machine's Windows account name in
a test fixture and it had to be taken out of the history before the push, and the already-published
tree had zero occurrences, which is the standard.

**`~/.codex/hooks.json.manual-backup`** is a hand-made leftover from session 18. The user's to delete.

**The owner's memory files are not fixtures.** `internal/memory`'s corpus tests read
`~/.claude/projects/*/memory` and `~/.codex/memories` and print counts only. A failure message that
carries a title carries a line of a private note — the tests are written the awkward way they are for
that reason, and a new assertion over that corpus inherits the rule.

---

## 5. Things that will bite

1. **A skip guard written against the function under test skips exactly when that function is
   broken.** Session 19's first 8.3 test passed against a `spaceFree` that did nothing. Session 20's
   corpus title test asks the *files* how many blocks open with a label, which is a fact the code
   under test does not get a vote on.
2. **A closed-set test cannot catch a value being removed from the set it reads.** Deleting `Outcome`
   from `codexProseLabels` leaves the corpus test green, because the title it then produces is no
   longer *recognised* as a label. The two unit tests are what own the set's contents. Any closed set
   added later has the same hole and needs the same second test.
3. **The harness's heredoc collapses a backslash even with a quoted delimiter, and Python's own
   string escapes collapse a second time.** Both were hit again this session: a `\\` in a Go fixture
   arrived as `\` and did not compile, and a `python - <<'PY'` carrying a Windows path literal died
   on `\U`. Anything with a backslash goes through a file-write tool.
4. **A long run of letters in a search fixture is a secret.** `internal/secret`'s opaque rule is 40 or
   more of `[A-Za-z0-9+/]`, and the excerpt is cut from the *masked* document — so a filler of 1,000
   `a`s makes the fixture measure the mask instead of the window. Break the run.
5. **A shell guard denies more than you expect, and it is right to.** `git checkout -B`,
   `git branch -f`, `git checkout -- <path>`, `rm -rf`, `git rebase -i`, and any nested shell
   (`bash -c`, `cmd //c`). History rewriting is the user's to run, so plan a scrub as a command you
   hand over rather than one you execute. A break-it pass therefore reverts by editing the file back,
   not by `git checkout`, which is the safer habit anyway.
6. **`internal/search` takes about seventeen minutes under `-race`, and about fifty seconds without.**
   It is not hung.
7. **A killed `go test` run does not report that it was killed.** Stopping a backgrounded suite
   mid-compile leaves one line per package reading
   `compile.exe: exit status 0xc0000142` and `[build failed]`, which is a process terminated during
   DLL initialisation and reads exactly like a broken toolchain. Observed twice this session. Before
   diagnosing a build failure, check whether you stopped the run that produced it.
8. **`git add -A` before a commit sweeps in whatever the session left lying about.** A scratch log of
   test output reached a commit that way, and `origin` is public. Write a scratch file outside the
   repository, and read `git status` before staging rather than after committing.
7. `go test` without `-p 1` is refused by a guard, on the command line and not in `GOFLAGS`.
8. **Do not paste a `TestPhase4Gate` corpus-mode run anywhere.** One line of it carries a real path.
9. **`wmic` is gone on this Windows build.** Use `tasklist /FI "PID eq N" /V` for a process's owner.
10. **Four orphaned processes from 2026-08-31 survive `taskkill /F` unelevated** — same account,
    higher integrity, so an elevated terminal is the only route. They hold handles, which is what
    `t.TempDir()` cleanup failures and `database is locked` are made of. WINPIDs 8916, 6804, 17272
    and 2776 as of 2026-09-05 23:37.

---

## 6. Done when

M11's non-vacuity arm has run and row 48 is either closed on its number or has a measured weight
behind it; whatever changes has a test watched failing under a mutation that changes the answer; each
behaviour change is a branch named for its step, merged `--no-ff`; the suite, the pinned linter and
the race script are green in that order and not concurrently; and a session 22 brief exists.

**Three things stay one user action away and none is an agent's to take.** The seven unpushed commits
need a fresh authorisation. Publication condition 3 needs the Defender exclusion walked through the
Security UI. Backlog 52 needs one Codex turn with the failing `Stop hook` line read for whose hook it
names.
