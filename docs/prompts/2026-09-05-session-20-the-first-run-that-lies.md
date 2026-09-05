# Session 20 — Engramux: the first run that lies

Capture works on both hosts. It has since 2026-09-05 20:22, and session 19 established *why* it had
not: a Codex hook command is source text for whatever shell the session snapshotted, and a quoted
path is a PowerShell string literal that prints itself. That is closed, measured and pushed.

**What is left is the other end of the product — what a stranger sees in the first minute.** Three
backlog rows describe one scene, and all three were blocked on a decision rather than on work. The
decisions were made in session 19's grilling and are in §3 below. **They are the reason this
document exists**: read them before touching the code, because each row's own text says no test can
say which answer is right, and a session that re-derives them will derive different ones.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read, in this order: this document's §3, then `docs/superpowers/backlog.md`'s rows **36**, **47** and
**48** in full. Session 19's brief covers what was built yesterday and is worth reading only if you
touch `internal/host`.

**Written 2026-09-05, by session 19.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `74478fb`, tree clean, **pushed** — `origin/main` and `main` agree. `origin` is public and the push was authorised once, for that push; a later one is a fresh ask |
| Checks | Suite 21 packages exit 0, pinned linter `0 issues.` exit 0, `scripts/race.sh` 21 packages and no data race — in that order and not concurrently, on `74478fb` |
| Installed | `dist/` and the installed binaries predate session 19's fix. Nothing needed installing: the change is reachable only by a path with a space and this machine has none |
| Gates | M1–M6, M9, M10 pass. **M7 is un-run**: `prompts.tsv` is 150 of 150 `TODO` |
| Publication | 1, 2 and 4 closed. **3 stays open**, and it is not writing — the `README` already names the detection and gives the steps, and says itself that the steps are `[unverified]`. What is owed is a human walking them through the Windows Security UI on a machine where Defender fires |
| Backlog | **36, 37, 38, 40, 46, 47, 48, 51, 52** open. 36, 47 and 48 are this session's |
| Injection | Built, off, untouched |

---

## 2. What session 19 closed, in one paragraph each

**Backlog 51.** A relay path with a space had no spelling. codex-rs at the installed tag hands a
hook's command to the session's snapshotted shell as one ordinary argument, and only falls back to
`COMSPEC /C` when there is none — so the value is a *script*, not a command line. Measured over
`cmd.exe`, `powershell.exe` and `pwsh.exe`: no quoting serves both, and the 8.3 short path is the
only spelling that ran in all six arms. `internal/host.spaceFree` writes it, over the shortest
prefix that holds every space and nothing below it. Spec §4.2 carries the matrix.

**Backlog 52.** The premise was wrong twice over. The installed Codex trims a `Stop` hook's stdout
and treats an empty result as an explicit do-nothing branch, so a hook that writes nothing cannot
produce `hook returned invalid stop hook JSON output`. And with the binary and `hooks.json` held
still, the symptom moved: two failures at 20:40, none across four `Stop` events at 22:13 and 22:16.
Spec §4.5 does not move.

---

## 3. The three decisions, and why

**These were decided on 2026-09-05 by the owner, in a grilling, after the rows were read in full.
Do not re-open one without saying which part of its reason failed.**

### 47 — `sessions` becomes corpus-wide, and `[project]` narrows it

Matching its sibling `search`. **The reason is the row's own evidence**: the mitigation that already
existed — printing the root the service resolved — was visible and was not read, because the
interesting word in a two-line answer is the second one. A signpost adds a third line to not read.
Making the two commands agree removes the class of error rather than annotating it.

**What the row implies and the code does not support.** `ListSessionsReply.ProjectRoot` is singular —
"the project the request named". A corpus-wide answer has no single root, so the reply moves whichever
option is taken; there was no cheap half to choose. Decide whether the root travels per session or the
field goes, and say which in the change.

**Once decided, a test can own this row** — that no-argument `sessions` is corpus-wide is exactly the
kind of thing a test pins. Delete the row when it does.

### 48 — fix what a hit shows; the ranking waits for a measurement

The excerpt window is a rune window centred on the match and aligned to nothing, so it begins
mid-word — `lUse` is the observation that started the row. That is a defect with no second opinion,
and it is display.

**Down-weighting tool-plumbing events is not, and it does not get done here.** It is a lever on
every search every user ever runs, pulled on one first-run observation. This repository requires a
ranking change to be measured: `TestPhase4GateM4` is the shape — the boost on and off, three
classes, recall@10 and MRR, with the row's own delete condition. **Design that arm first, then take
the ranking half.** The row stays open, narrowed to the ranking question.

### 36 — a title is the first line that is not a recognised label

`firstLine` is eight lines and already strips `#-*> `, so skipping label lines is the smallest change
that exists. **Two conditions on it.** The label set has to be closed, and it has to be written down
somewhere a reader finds — spec §6.1's credential field names are the precedent for a closed set in
this codebase, and they got there by measurement over the corpus rather than by listing what came to
mind. Derive the set from the captured summaries; do not invent it.

**36 and 48's display half are one piece of work** — the row says so, and they are the title and the
body of the same hit.

---

## 4. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed**, and `origin` is public and now carries session 19's work. Session
19 published this machine's Windows account name in a test fixture and had to have it removed from
the history before the push — `git log -p` your own diff for `Users\<name>` before asking for a push,
because the already-published tree had zero occurrences and that is the standard.

**`~/.codex/hooks.json.manual-backup`** is a hand-made leftover from session 18. The user's to delete.

---

## 5. Things that will bite

1. **A skip guard written against the function under test skips exactly when that function is
   broken.** Session 19's first 8.3 test passed against a `spaceFree` that did nothing, because the
   skip asked `spaceFree` whether it had an answer. Ask the environment the question directly.
2. **The harness's heredoc collapses a backslash even with a quoted delimiter.** A `python <<'PY'`
   carrying a Windows path literal died on `\U`, twice in this repository's history. Anything with a
   backslash goes through a file-write tool.
3. **A shell guard denies more than you expect, and it is right to.** `git checkout -B`,
   `git branch -f`, `git checkout -- <path>`, `rm -rf`, `git rebase -i`, and any nested shell
   (`bash -c`, `cmd //c`) inside the Bash tool. History rewriting is the user's to run, so plan a
   scrub as a command you hand over rather than one you execute.
4. **`wmic` is gone on this Windows build.** Use `tasklist /FI "PID eq N" /V` for a process's owner.
5. **A same-user process can still refuse to die.** Four orphaned `tail`/`grep` from Aug 31 survive
   `taskkill /F` unelevated: same account, higher integrity, so an elevated terminal is the only
   route. They hold handles, which is what `t.TempDir()` cleanup failures and `database is locked`
   are made of. WINPIDs 8916, 6804, 17272 and 2776 as of 2026-09-05 23:37.
6. **`gosec` reads `uint32(len(buf))` as an overflow** whatever the slice was made from. Pass the
   constant. It cost session 19 a linter exit 1 on code that had already been merged.
7. `internal/search` takes about seventeen minutes under `-race`. It is not hung.
8. `go test` without `-p 1` is refused by a guard, on the command line and not in `GOFLAGS`.
9. **Do not paste a `TestPhase4Gate` corpus-mode run anywhere.** One line of it carries a real path.

---

## 6. Done when

47 answers corpus-wide with `[project]` narrowing, and a test pins it so the row can go; 36 and 48's
display half land together with the label set derived from the corpus rather than invented; 48's
ranking half is either measured through a gate arm of its own or left open with that arm designed;
whatever changes has a test watched failing under a mutation that changes the answer; each behaviour
change is a branch named for its step, merged `--no-ff`; the suite, the pinned linter and the race
script are green in that order and not concurrently; and a session 21 brief exists.

**Two things stay one user action away and neither is an agent's to take.** Publication condition 3
needs the Defender exclusion walked through the Security UI. Backlog 52 needs one Codex turn with the
failing line read for whose hook it names.
