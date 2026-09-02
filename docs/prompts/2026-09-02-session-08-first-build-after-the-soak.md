# Session 08 — Engramux: close the soak, then the first build

Session 07 ran for the whole first day of the soak. It did the work that needs no rebuild, which
turned out to be most of a plan step: **the Node installer is ported to Go and deleted**, and
`engramux install` replaces it. That work is on a branch and is **not finished** — `doctor` is the
other half of the same step.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file carries
what they cannot: the state of the work when the session opened, and how that session was scoped.

Read, in this order: `docs/superpowers/plans/2026-08-30-after-phase-6.md` — it is short, it is the
order, and it now carries the one ordering constraint the deletion created; then
`docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md`, which decides what the product
is now; then the 1.0 spec's §8 Phase 6 row and §7.3's soak row.

**Written 2026-08-30, with about 62 hours left on the soak.** An earlier file of this name was
written twelve hours before this one, was overtaken by six commits before anyone read it, and is
replaced rather than kept: it never became a record of anything.

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Nine commits from session 07, **pushed**. The Node installer is still there, and that is load-bearing — see §3 |
| `step-2-engramux-install` | Four commits, **pushed**, `4c89e1c`. 21 files, +1623 −1508 against main. The installer half of plan Step 2, plus the deletion |
| Last full verification | `go test -p 1 -count=1 ./...` **17 packages ok** · pinned linter `0 issues.`, **exit 0**, on the branch |
| `./scripts/race.sh` | **Not run in session 07, deliberately.** Twenty minutes of saturation is the confound the soak exists to rule out. It is the first thing after the series is closed |
| `dist/` and the installation | **Untouched.** Measured: a rebuild from the branch links none of the new symbols — deadcode elimination drops them all — and the relay comes out the same size as the installed one. Only the `DefaultGODEBUG` pin differs |
| The soak clock | Started **2026-08-30T03:21:37Z**, ends **2026-09-02T03:21:37Z**. At the time of writing: uptime 9h55m, no restart, **one** `read-failed` row |
| The sampler | Task Scheduler entry `\Engramux Soak Sample`, every thirty minutes. **Delete it when the soak ends; nothing else will** |
| Backlog | 10 rows, every one needing a build, plus what session 07 filed |
| `go.mod` | Now `go 1.27.0`, with two `godebug` lines pinning the 1.26 defaults. Both were measured; §5 says why |

---

## 2. What to do, in order — and the order is not the obvious one

**T1 — Check the clock, then the series, then nothing else.** `engramux status` uptime past 72
hours and no restart in the service log. Anything that touches the machine before this is measuring
the wrong thing.

**T2 — Write the series up, and delete the scheduled task.** §7.3's soak row moves to §7.1: the
WAL's range, the database's growth rate, the working set's trend, handle and thread counts, and how
many samples recorded each of `read-failed`, `down`, `unknown` and `parse-failed`. One
`read-failed` is already in the series and §7.1 has its analysis; do not re-derive it.

**T3 — `./scripts/race.sh`.** The one check the soak was blocking.

**T4 — Step 1's build, from `main`.** Not from the branch, and this is the ordering constraint the
deletion created: `main` still has `scripts/install-hooks.mjs` and the branch does not. Building and
installing Step 1 from main means the Node installer is still there as a fallback if anything goes
wrong.

**T5 — Then merge the branch, with `--no-ff`, and verify the new installer by reinstalling with
it.** Merging before T4 would make the new installer's first real run and a new binary's first
install the same event, with two candidate causes for anything that broke.

**T6 — `doctor`, which is the rest of plan Step 2.** Three things, decided on 2026-08-30: stage
judgement, so "not installed yet" and "installed and broken" are different answers with different
next commands; MCP demoted to optional, so a deliberate capture-only installation can be green; and
the eleven hook entries checked against the installed relay, which is the one surface `doctor` does
not look at. Plus one more: `doctor` prints a Windows SID and the real database path, in the output
a person is most likely to paste into an issue — **the default becomes masked and a `--full` flag
prints the real values**. `secret.MaskString` already does this for the status reply.

---

## 3. The installer, and the two things to know before you touch it

**It is verified end to end, and here is exactly how far.** Session 07 ran `install --apply` against
an isolated tree — `LOCALAPPDATA` redirected, the three host files redirected, a test task name, and
a `PATH` holding only System32. It copied both binaries, skipped them on a re-run as identical,
backed up and reported both backups, wrote eleven events into each host with the user's own keys and
their order intact, registered the logon task **against the installed service**, started it through
the task, and reported the missing MCP endpoint without failing the run. The endpoint was missing
because the temporary service lost the pipe race to the real one, which is I-09 working.

**What that run did not cover**: the MCP half end to end, because there was no endpoint to register;
and `claude mcp add`, because `PATH` had no `claude` in it. Both are covered by unit tests with a
stub, and neither has been seen against the real Claude Code.

**Two reviews found things, and one of them found three assertions of mine that could not fail.** A
security subagent traced the token through every path; Codex compared the script line by line against
the port. Between them: a url of `--help` passed validation and made the installer report a
registration that never happened; an `Endpoint` printed its token under `%v`; a machine with no Codex
got a FAILED line on every install; `--remove` had been lost from the orchestration entirely; and
three test assertions were inert, one of which was measured inert by deleting the code it claimed to
guard and watching the test stay green. All fixed. The commit messages carry the detail.

---

## 4. What session 07 measured, so you do not re-measure it

- **Script-boundary segmentation buys nothing** on this corpus: 84 documents carry a Hangul run no
  prefix query reaches, and 0 of those 84 carry no reachable Hangul at all. §7.1 has the row.
- **The MCP server does not offer revision `2026-07-28`.** `server/discover` answers a list without
  it, and an `initialize` handshake can never show this because that is the legacy path. Backlog 30.
- **A `.cmd` shim runs under Go but mangles a quoted argument**, which is why only a `.exe` is
  accepted for `claude` — the argument carries a credential.
- **A running image answers `ERROR_SHARING_VIOLATION` and a read-only file answers
  `ERROR_ACCESS_DENIED`**, and `errors.Is(err, fs.ErrPermission)` is false for the first.
- **Raising the `go` directive moved two GODEBUG defaults**, one of them governing certificate
  verification, and the linker keeps the x509 verification path — so "inert here" could not be
  claimed. Both are pinned.
- **A path with a space breaks nothing the suite covers** — all 16 packages pass with every temp
  path carrying one, and the harness was checked for vacuity first.

---

## 5. Open, and what to do about each

| Open | What to do |
|---|---|
| The Windows claim, still half-measured | The space-path proxy passed; a clean VM is what is left, and on 2026-08-30 it moved from being a condition of merging Step 2 to being a condition of **publishing**. Nothing has ever been installed on a second machine |
| The token on the command line | Acknowledged in `claude.go` rather than solved: Windows makes a command line readable by any same-user process and process-creation logging forwards it off the machine. Not avoidable without writing Claude Code's live state file, which is refused for a reason that still holds |
| Backups accumulate copies of the token | Every re-install leaves a timestamped copy of `config.toml` from before the write, and after the first install that file holds the bearer token. They are reported now; nothing prunes them |
| Backlog 33, a reply with no match count | The product cannot count its own corpus, which is why "how often does a model call these tools" is still unmeasured |
| No `LICENSE`, and `origin` is public **now** | A public repository with no licence grants nobody any rights |
| `docs/evidence/exclusive/main.go` | Working copy is CRLF against `.gitattributes` and `gofmt` does not pass it. Pre-existing |

---

## 6. Six things that will bite

1. **Build from `main` before merging the branch.** §2's T4. The fallback installer only exists
   there.
2. **A rebuild before T1 passes ends the soak.** `go list -deps` decides what is shipped, not a file
   name.
3. **The file-write path turns a `\uXXXX` sequence into the character it denotes**, and a heredoc
   collapses `\\` to `\`. Both put broken bytes into Go source in session 07, more than once.
   Assemble such values from bytes and read them back.
4. **A break-it that reports no failing test may not have compiled.** Go makes an unused import a
   compile error, so a mutation removing the last reference produces a program that never runs, and
   a harness grepping for a failing test sees nothing. Change the answer, not the reference.
5. **`git status` can print a modified marker from a stale stat cache** while `git diff HEAD` exits
   0. The exit code is the answer.
6. **Check the linter's exit code, never its summary line.**
