# Session 18 — Engramux: Codex captures, and closing that opened two rows

Session 17 was handed a grilling that had stopped at its third question: what should this product do
about the malformed `~/.codex/hooks.json` it had written onto every installed machine. **That
question never had an answer, because the file was never malformed.** Checking the premise before
grilling it refuted a finding that had already reached a backlog row, a work order and a
cross-session hand-off. Then the replacement diagnosis was wrong too. Then the third attempt found
it, and Engramux captured its first Codex event.

**Codex hooks work now.** What is left is two rows that closing them created, and one of them cannot
be measured on this machine.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read, in this order: `docs/superpowers/backlog.md`'s closing note for **50** and rows **51** and
**52**, then the 1.0 spec's **§4.2**. Session 17's own brief is superseded by this one and is only
worth reading to see what two settled-looking findings looked like from the inside.

**Written 2026-09-05, by session 17.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Session 17's commits on top of `65a76ba`, including two `--no-ff` merges: the doctor delivery line, and the Codex quoting fix. **Not pushed.** `origin` is public |
| Installed | The binaries were rebuilt from `dist/` and reinstalled with `scripts/reinstall.sh` **before** the quoting fix, so the installed relay predates it. `update --from` does not touch host configuration, so `~/.codex/hooks.json` still holds the old spelling for ten of the eleven events |
| **The owner's Codex file is hand-edited** | `UserPromptSubmit` was rewritten by hand to the unquoted spelling and works; the other ten still carry the quotes and are dead. `~/.codex/hooks.json.manual-backup` sits beside it. **`engramux install --apply` is what repairs all eleven, and it is the user's to run** — the `AGENTS.md` carve-out that lets an agent run it was justified by the install writing no host configuration, and after this change it writes both host files |
| Checks | Suite 21 packages exit 0, pinned linter `0 issues.` exit 0, `scripts/race.sh` 21 packages and no data race — in that order and not concurrently |
| Gates | M1–M6, M9, M10 pass. **M7 is still un-run**: `prompts.tsv` is 150 of 150 `TODO` and `blocks.tsv` does not exist |
| Publication | 1, 2 and 4 closed. **3 stays open** — Defender never fired on the clean profile, so nobody walked the exclusion procedure |
| Backlog | **36, 37, 38, 40, 46, 47, 48, 51, 52** open. 50 closed |
| Injection | Built, off, untouched |

---

## 2. What it was, and what it took to see it

`CodexEntry` wrapped the relay path in quotes **inside** the value, so the command line Codex
received was one quoted token from its first character. Codex echoed it instead of running it.

**The echo is what found it, and it had been visible in the host's own UI all along.** Codex prints
a hook's stdout as `hook context:`, and for Engramux it printed the relay path — while this relay
writes nothing on stdout on any event (spec 4.5). One line of output that cannot exist is what a
whole day of correct-looking evidence could not produce. Every hook reported `completed`, the spool
stayed empty, and `doctor` said `11 of 11 events point at the installed relay`.

**Three candidates were declared dead on the strength of that `completed`, and one of them was the
answer.** `completed` is exit 0 of whatever the host spawned, and an echo exits 0.

---

## 3. What was built

**`doctor` reports what each host has actually delivered.** The doctor reply carries the per-host
breakdown the service already computes for `cells`, and each host's configuration line is followed
by a `<host> received` line: a count and the most recent arrival, or that nothing has ever arrived,
or that nobody can answer. A dead service and a service too old to send the breakdown are both
`unknown` rather than zero. It is a note and never a fail — `doctor` cannot tell a host that is
broken from a host the user has not opened since installing.

**`CodexEntry` writes the path plain.** Nine mutations across the two changes, all killed, each
changing an answer rather than removing a reference.

---

## 4. What is open

**Backlog 51 — a relay path with a space has no known spelling, and this machine cannot measure
one.** The quotes that were just removed are exactly what such a path would have needed. Unquoted
breaks on the space; quoted is the spelling just measured broken for a path without one. Neither
obvious answer is available, so a third has to be found and **measured** — a quote put back on
reasoning alone re-creates the defect for everyone else. It needs a profile whose Windows account
name has a space, which is the same shape of requirement as the memory spec §8's clean-profile
condition and is probably the same errand.

**Backlog 52 — every Codex turn ends with a red line.** `Stop hook (failed) — hook returned invalid
stop hook JSON output`. Codex wants JSON on a `Stop` hook's stdout and spec 4.5 has the relay write
nothing. It costs no capture and it costs a user a visible failure every turn. It is a **design
change**: 4.5 is an invariant, so the question is whether to move it, not how to quiet the symptom.

---

## 5. What is proved and what is not

**Proved.** A `codex UserPromptSubmit` reached the database at 2026-09-05 19:46:25 through a wrapper
that spawned the same binary unquoted, and a second at 19:51:45 through the entry itself with only
the quotes removed. The stored payload carries a `transcript_path` under the Codex session
directory, so host detection classified it correctly with no help. The relay, the pipe, the service,
ingest and detection were never the problem.

**Not proved, and do not write otherwise.** Only `UserPromptSubmit` has been observed arriving.
The other ten events carry the same spelling by construction and none of them has been watched. A
path containing a space is untested and is expected to fail. And the fix has not yet been through an
`install --apply` on any machine — the owner's file is still hand-edited.

---

## 6. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first. The override is
package-relative.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed.** Its 118 Codex captures are all from 2026-08-27 and were produced
by a capture probe, not by the relay.

---

## 7. Things that will bite

1. **A host reporting `completed` says a process exited 0, not that it was yours.** When a hook
   captures nothing, read its **stdout**: a relay with a documented empty stdout makes any output at
   all a contradiction worth chasing, and that one line was in the first paste.
2. **A settled-looking finding is still a finding somebody made.** Two of them were wrong here, and
   both were written down as settled before anything checked them.
3. **`~/.codex` holds a bearer token.** Read that directory structurally — key names, counts, sizes.
   `config.toml`'s section headers are safe; its `Authorization` line is not.
4. **A quoted heredoc still collapses a backslash here.** A `python - <<'PY'` block carrying a
   Windows path arrived with single backslashes and died on `\U`. Anything with a backslash goes
   through a file-write tool.
5. **A pipeline's exit code is the last command's.** Redirect to a file and read the code on its own
   line.
6. **`internal/search` takes about seventeen minutes under `-race`.** It is not hung.
7. **`go test` without `-p 1` is refused by a guard**, on the command line and not in `GOFLAGS`.

---

## 8. Done when

51 has a spelling measured on a path with a space, or a written decision that it waits for a machine
that has one; 52 has a decision recorded against spec 4.5 rather than a patch; whatever changes has
a test watched failing under a mutation that changes the answer; the suite, the pinned linter and
the race script are green in that order and not concurrently; and a session 19 brief exists.

**Say plainly what is still unobserved.** Ten of the eleven Codex events have never been seen
arriving, and no machine has yet installed this fix through `install --apply`.
