# Session 08 — Engramux: the first build after the soak

Session 07 ran while the soak clock ran, so it did none of the work that needs a binary and all of
the work that does not. **The product's scope moved during it**, which is the thing to read before
anything else: Engramux stays a personal tool for now and is published later, once it improves on
claude-mem's function, works on Windows *measurably*, carries a memory feature that is native-grade
or better, and has an installation method that is decided and verified. Four research rounds and a
grilling session sit behind that, and what survives them is a new spec and a new plan.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file carries
what they cannot: the state of the work when the session opened, and how that session was scoped.

Read, in this order: `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` — it is
short and it decides what the product is now; then `docs/superpowers/plans/2026-08-30-after-phase-6.md`,
which is the order; then the 1.0 spec's §8 Phase 6 row and §7.3's soak row, which is where the series
lives until it is written up.

**Written 2026-08-30, with about 69 hours left on the soak.** Everything below about the series is
therefore what session 07 could see, and the numbers it quotes are two hours into a three-day run.

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Session 07's five commits, **pushed** (`bd469b9..0345370`). Working tree clean |
| Last full verification | `go test -p 1 -count=1 ./...` **17 packages ok** · pinned linter `0 issues.`, **exit 0** |
| `./scripts/race.sh` | **Not run in session 07, deliberately.** It saturates the machine for twenty minutes, which injects the exact confound the soak exists to rule out. It is the first thing to run when the clock stops, before any build |
| `dist/` and the installation | **Untouched since the pre-soak build.** No shipped `.go` file changed in session 07 — `go list -deps` against `git status` was empty at every commit |
| The soak clock | Started **2026-08-30T03:21:37Z**, ends **2026-09-02T03:21:37Z**. At the time of writing: uptime 2h31m, no restart, and **zero** `down`/`read-failed`/`unknown`/`parse-failed` rows in the live series |
| The sampler | A Task Scheduler entry, `\Engramux Soak Sample`, firing the VBS wrapper every thirty minutes. **Delete it when the soak ends; nothing else will** |
| Backlog | **10 rows, and every one needs a build.** That is stated in the file: a row appearing there that does not need a build is a sign something was filed rather than done |
| Phases | 1–5 done and gated. 6's `[auto]` half done and gated; its `[manual]` half is the soak |

---

## 2. What changed about the product, and why it is not a detail

Both hosts now ship memory of their own. Claude Code's is on by default and its documentation names
what it will not store — anything derivable from the codebase, *"such as architecture, file paths, or
debugging fixes"*. Codex consolidates sessions into a git-versioned memory directory. So the seat
Engramux was reaching for is taken, and the empty one is what both throw away: verbatim, searchable,
cross-host history of the error string, the command, the path and the fix.

The memory spec decides six things and rejects a seventh. The rejection is the one to understand
first, because it will look like timidity otherwise: **no summarisation layer of our own, ever, on
current evidence.** Not because summaries are bad but because of an ordering result — on the only
controlled experiment on a coding agent, an agent-retrieved summary scored below having no memory at
all, while the same summary handed over by an oracle scored well above it. The value is entirely in
the selector, so a selector has to exist and be measured before a summariser is worth discussing. The
spec writes the reopening condition down; it is falsifiable, not a mood.

Hook-time injection **is** being built, and it **ships disabled**. That contradicts the 1.0 spec's
`Context re-injection` row deliberately and after 1.0, not of it, and the 1.0 spec now says so at its
top.

---

## 3. What to do, in order

**T1 — Check the clock, then the series, then the machine.** `engramux status` uptime past 72 hours,
no restart in the service log, and the sample series behind it. If the service restarted at any point
the clock restarted with it and §8's gate is not met however good the series looks.

**T2 — Write the series up, and delete the scheduled task.** §7.3's soak row moves to §7.1. What it
owes: the WAL's range, the database's growth rate, the working set's trend or absence of one, the
handle and thread counts, and how many samples recorded each of `read-failed`, `down`, `unknown` and
`parse-failed` — four different things, and only the first is about the read deadline. The working set
is the MCP session map's only instrument, so its trend is the answer to the one question §5.9 left
open.

**T3 — `./scripts/race.sh`, then the plan's Step 1.** In that order. The race detector first because
it is the one check the soak was blocking, and Step 1 because its content is already settled and a
smaller queue makes the next build's failures easier to attribute.

**T4 — The plan, from Step 2.** It is order only and cites both specs for every value. One thing it
asks rather than assumes: before writing Step 4's migration, settle whether Steps 3 and 4 share an
FTS rebuild. If they do they are one migration and one backfill; if the derived fields are filters on
the base table and never enter the index, they are two and the batching argument disappears.

---

## 4. What session 07 measured, so you do not re-measure it

**Three proposals died on measurement, and that was the session's most useful output.**

- Backlog 8 claimed the particle rule's ASCII-only trim cost 22 Hangul candidates. It costs **none**;
  the 158 and the 136 in that row are answers to two different questions. `deriveParticle`'s comment
  carries it.
- The research round proposed script-boundary segmentation of the indexed text at **1.53× the index**.
  On the real 901 captures it buys nothing: 84 documents carry a Hangul run no prefix query reaches
  and **0 of those 84** carry no reachable Hangul at all. `TestHowMuchKoreanIsOutOfReach` reports it,
  and it reports rather than gates because the sweep already answers 2,262 of 2,262 — no widening of
  the index can improve five classes that are already whole.
- Backlog 14 feared a payload `encoding/json` accepts and SQLite refuses, other than depth. Over 21
  candidate shapes they agree on all 21.

**Three defects were found, and all three need a build.** The MCP server does not offer the revision
§5.9 reasons from — `server/discover` answers a list without `2026-07-28` in it, and an `initialize`
handshake can never show this because that is the legacy path and the SDK caps it. `health.json` is
in §5.6 and nowhere in the code. `ipc.Drain` is a declared request type with no handler. The second
and third were **withdrawn** rather than built, with the reasoning in the backlog.

**And one that was not on any list.** `TestCancellingTheContextStopsServe` failed once in a full run
and passed five times in isolation — the shape that gets written off. Reproduced: **7 failures in
1,200 runs**, all seven carrying `ERROR_OPERATION_ABORTED`. Closing a listener while an `Accept` is
already blocked cancels the I/O rather than refusing it, so "the listener closed" has two spellings
and the assertion took one. Zero in 1,200 after the fix. `AGENTS.md` has the row.

---

## 5. Open, and what to do about each

| Open | What to do |
|---|---|
| **The Windows claim has never been measured** | This is the sharpest one. `grep` finds no row anywhere in the spec about a clean machine, a fresh machine, or a first run — it is not even *classified* as unverified. The whole differentiator rests on architecture reasoning and six claude-mem issue numbers. Session 07 took the free proxy: `TMP` pointed at a directory whose name contains a space, so every `t.TempDir()` carries one and so do the redirected `LOCALAPPDATA`, `HOME` and `USERPROFILE` the installer tests use. **All 16 packages pass**, and the harness was checked for vacuity first — a default run reports no space, a redirected one reports one. §7.1 has the row and says plainly what it does not cover: the VBS wrapper, `schtasks` against a spaced executable path, the host configuration files the installer really writes, a space in the repository path, and a space in the OS user name. Those want a clean VM, and it belongs to Step 2 so that "installation is verified" is part of the publication condition rather than a hope |
| Backlog 33, a reply with no match count | Filed after trying to measure how often a model reaches for the MCP tools unprompted and finding the product cannot count its own corpus. `.capture/` holds 714 tool calls and 0 MCP calls of any server, but that corpus predates the MCP server and answers nothing |
| No `LICENSE`, and `origin` is public **now** | "Publish later" is about promotion; the absence of a licence is a present state. A public repository with no licence grants nobody any rights, including the owner's future self on another machine |
| Backlog 28, the token in three files | §5.9 accepts the exposure deliberately, so moving it is a decision nobody has taken rather than work nobody has done. Not scheduled |
| `doctor` cannot see a token mismatch | Accepted, deliberately. Unchanged |
| `docs/evidence/exclusive/main.go` | Working copy is CRLF against `.gitattributes`, and `gofmt` does not pass it. Pre-existing, outside every session-07 change, trivial |

---

## 6. Five things that will bite

1. **A rebuild before T1 passes ends the soak and starts a new one at zero.** `go list -deps` is the
   test for what is shipped, not a file name.
2. **`schtasks /create` is denied in the agent sandbox**; `/query`, `/run` and `/end` are not. Hand
   the command over rather than routing around the denial. `/run` is how session 07 verified the
   sampler end to end, and its last result was 0.
3. **The file-write path turns a `\uXXXX` sequence into the character it denotes.** That put a literal
   NUL and a literal byte-order mark into Go source in session 07 and broke the compiler twice.
   Assemble such values from bytes and read them back before believing the write. Same family: a
   Python script writing a `.md` gives it CRLF against `.gitattributes`, and git normalises the blob
   so nothing complains.
4. **`git status` can print a modified marker from a stale stat cache while `git diff HEAD` exits 0.**
   The exit code is the answer. Session 07 lost time to this.
5. **Check the linter's exit code, never its summary line.**
