# Session 19 — Engramux: the shell nobody had asked about

Session 18 closed backlog 50 and opened two rows it could not finish: 51, a relay path with a space
and no known spelling, declared unmeasurable on this machine; and 52, a red `Stop hook (failed)` on
every Codex turn, declared a cost of spec 4.5 and therefore a design change.

**Both were answered by asking the host a question nobody had asked it: which program actually runs
a Codex hook command.** 51 has a spelling now, measured over three shells and implemented. 52 turned
out not to be this product's defect at all, so 4.5 does not move.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read, in this order: the 1.0 spec's **§4.2** — which now carries the shell matrix — then its **§4.5**,
then `docs/superpowers/backlog.md`'s rows **51** and **52**. Session 18's brief is superseded by this
one.

**Written 2026-09-05, by session 19.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Session 19's five commits on top of `1e7bcd4`, including one `--no-ff` merge; tree clean. **Nothing since `65a76ba` has been pushed** - sixteen commits across three sessions. `origin` is public |
| Installed | Unchanged. `dist/` and the installed binaries are session 18's, from before this session's fix. Nothing here needed an install: the change is reachable only by a path with a space and this machine has none |
| Checks | Suite 21 packages exit 0, pinned linter `0 issues.` exit 0, `scripts/race.sh` 21 packages and no data race — in that order and not concurrently |
| Gates | M1–M6, M9, M10 pass, unchanged. **M7 is still un-run**: `prompts.tsv` is 150 of 150 `TODO` |
| Publication | 1, 2 and 4 closed. **3 stays open** — nobody has walked the Defender exclusion procedure |
| Backlog | **36, 37, 38, 40, 46, 47, 48, 51, 52** open. 51 and 52 are both much narrower than they were |
| Injection | Built, off, untouched |

---

## 2. What backlog 51 turned out to be

**A Codex hook command is source text for a shell, and which shell is not this product's decision.**
Read end to end in codex-rs at tag `rust-v0.150.1`, which is the version `codex --version` reports
here:

1. the session builds its hook configuration from the turn environment's shell, when it has one;
2. that shell's own exec argv becomes the hook runner's program and arguments — for PowerShell,
   the executable, `-NoProfile` and `-Command`;
3. the hook runner wraps the value in a further pair of quotes **only** when it is running `COMSPEC`
   with `/C`, and otherwise passes it as one ordinary argument;
4. so under a snapshotted PowerShell the value is not a command line at all. It is a script.

This machine gets step 4: `features.shell_snapshot` is on and its Codex session rollout records
`"shell":"powershell"`.

**That is why the pre-fix entry was echoed.** A fully quoted path is a PowerShell string literal, and
a string literal at the top of a script evaluates to itself and is printed. `cmd.exe` runs that same
spelling perfectly — measured — so the nine days of silence backlog 50 chased were never a `cmd.exe`
problem, and no reading of `cmd.exe`'s quoting rules could have found it.

**Then the measurement 51 asked for.** A stub that writes nothing on stdout and records that it ran,
spawned exactly the way each of the two shapes spawns it — the raw command line for the `cmd.exe`
shape, an ordinary argument for the other — over `cmd.exe`, `powershell.exe` and `pwsh.exe`, with and
without a space. Four spellings; the matrix is in spec §4.2. The plain path fails PowerShell when it
has a space, the quoted path is printed by both PowerShells, the call operator is a syntax error in
`cmd.exe`, and **the 8.3 short path ran in all six**.

---

## 3. What was built

**`internal/host.spaceFree`.** A relay path carrying a space is written in its 8.3 spelling, over the
shortest prefix that holds every space and nothing below it. The narrowness is not tidiness: the
installer recognises its own hooks by the word engramux in the command, so a short name that reached
`engramux\bin\engramux.exe` could collide its way to something that no longer contains it, and the
next install would add a second hook beside the first. `PointsAt` gained the same function rather
than a second rule, so the writer and the reader cannot drift.

`Install` plans both host files before it copies a binary, so the relay does not exist when the entry
is written and the prefix asked about is the account directory, which does. Both arms are tested.

Four mutations, all killed, each changing an answer rather than removing a reference.

**Nothing else changed.** `spaceFree` returns before it touches the disk for a path with no space, so
every machine measured so far receives the byte-for-byte spelling it was measured with.

---

## 4. What backlog 52 turned out to be

**The premise was wrong.** The row said Codex requires a `Stop` hook's stdout to be JSON and that
empty is not JSON. The installed Codex's stop handler trims stdout and treats an empty result as an
explicit do-nothing branch; both the parse and the `hook returned invalid stop hook JSON output`
message live inside the `else` of it. **A hook that writes nothing cannot produce that message**, and
on `Stop` this relay writes nothing under every configuration there is — the one thing that writes on
stdout is injection, and injection writes on `UserPromptSubmit` and nowhere else.

So spec §4.5 does not move, and it now records that it was challenged and why it stands.

**What is not settled is whose hook it was.** This machine runs three products' hooks on the same
eleven events. `claude-mem` is still installed and registers its own Codex `Stop` hook that spawns
its worker with stdio inherited, so anything that worker prints becomes that hook's stdout — a
candidate, not a finding. Nobody has read the attribution off the failing line.

---

## 5. What is proved and what is not

**Proved.** The shell identification is read, not inferred: four files of codex-rs at the installed
tag, plus this machine's own configuration and rollout. The four-spelling matrix is measured, and the
reproduction is faithful argument for argument — the probe ran `-NoProfile -Command` because that is
what the vendor's own `derive_exec_args` returns for PowerShell.

**Not proved, and do not write otherwise.** The 8.3 spelling has **never been through Codex**. It was
measured against the three shells Codex hands hooks to, outside Codex, because no account on this
machine has a space in its name and an agent does not edit the user's host configuration to make one.
What is untested end to end is the whole path: installer to `hooks.json` to Codex to the relay, for a
user whose profile carries a space.

**Still unobserved**, unchanged from session 18: `SubagentStart`, `SubagentStop`, `PreCompact`,
`PostCompact` and `PermissionRequest` have never been seen arriving, because no session has done the
things that fire them. Six of the eleven have.

**8.3 is not guaranteed.** Name generation is per-volume and can be turned off. Then `spaceFree`
answers the path it was given and a Codex whose snapshot is PowerShell captures nothing — which is
all backlog 51 has left, and which needs a machine that can be measured rather than an argument.

---

## 6. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed.**

**`~/.codex/hooks.json.manual-backup`** is still there. It is a hand-made leftover from session 18's
diagnosis and it is the user's to delete — this session read it, which is how the pre-fix value was
confirmed to be exactly 53 characters of path wrapped in two quotes.

---

## 7. Things that will bite

1. **A hook failure in a host's UI names nobody.** Three products register hooks on the same Codex
   events here. The event a failure arrives on is not evidence about whose hook it was, and 52 was
   a whole row built on the assumption that it is.
2. **Reproducing a host is not the same as reasoning about it.** The first attempt at this
   reproduction ran the `cmd.exe` shape only, found that both spellings worked, and looked exactly
   like a defect that had never existed. What was missing was the shell, and the shell was two
   greps away in the host's own state.
3. **Passing a string to a shell through `os/exec` measures Go's quoting, not the shell's.** The
   `cmd.exe` shape needs the raw command line — `SysProcAttr.CmdLine` here, `raw_arg` there.
4. **A skip guard written against the answer under test skips exactly when that answer is wrong.**
   The first version of the 8.3 test did that and passed against a `spaceFree` that did nothing.
5. **`gosec` reads `uint32(len(buf))` as an overflow** whatever the slice was made from. Pass the
   constant.
6. `internal/search` takes about seventeen minutes under `-race`. It is not hung.
7. `go test` without `-p 1` is refused by a guard, on the command line and not in `GOFLAGS`.

---

## 8. Done when

51 has been through Codex on a machine whose account name carries a space, or the row is closed by a
decision that it waits for one; 52 has an attribution read off a failing line rather than a candidate;
whatever changes has a test watched failing under a mutation that changes the answer; the suite, the
pinned linter and the race script are green in that order and not concurrently; and a session 20
brief exists.

**Two things are one user action away and neither is an agent's to take.** A Codex turn, with the
`Stop` failure's own text read for whose hook it names, settles 52. And sixteen commits are sitting
on `main` unpushed against a public `origin`.
