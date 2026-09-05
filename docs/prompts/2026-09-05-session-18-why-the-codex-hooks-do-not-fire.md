# Session 18 — Engramux: the diagnosis was wrong, and the question it was hiding is still open

Session 17 was handed a grilling that had stopped at its third question: what should this product do
about the malformed `~/.codex/hooks.json` it had written onto every installed machine. **That
question no longer exists.** The file was never malformed. Checking the premise before grilling it
took four commands and refuted a finding that had already been written into a backlog row, a work
order and a cross-session hand-off.

What is still true is the observation the row was filed for: **no Codex event has ever been captured
through a hook Engramux wrote**, and `doctor` reported that host healthy throughout. Why the hooks
do not fire is open, and it is this session's work.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read, in this order: `docs/superpowers/backlog.md` row **50** as it now stands, then the 1.0 spec's
**§4.2** and **§4.3**. Read session 17's brief only to see what a settled-looking finding looked
like from the inside; **do not act on it** — its §2 and the first half of its §3 are withdrawn.

**Written 2026-09-05, by session 17.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Session 17 left four commits and one merge on top of `65a76ba`: the brief it was handed, the record correction, the doctor guard on `backlog-50-doctor-reports-what-arrived` merged `--no-ff`, and this brief. **Not pushed.** `origin` is public |
| Installed | Rebuilt from `dist/` and reinstalled through `scripts/reinstall.sh` after the merge, so the running service sends the per-host breakdown and `doctor` reports it. Before that it answered `unknown`, which is the branch that says a service predates the field rather than reading its silence as zero |
| Checks | Suite 21 packages exit 0, pinned linter `0 issues.` exit 0, `scripts/race.sh` 21 packages and no data race — in that order and not concurrently |
| Gates | M1–M6, M9, M10 pass. **M7 is still un-run**: `prompts.tsv` is 150 of 150 `TODO` and `blocks.tsv` does not exist |
| Publication | 1, 2 and 4 closed. **3 stays open** — Defender never fired on the clean profile, so nobody walked the exclusion procedure |
| Backlog | **36, 37, 38, 40, 46, 47, 48, 50** open. 50 is rewritten rather than closed |
| Injection | Built, off, untouched |

---

## 2. What was refuted, and how

Session 16 concluded that `MergeHooks` writes the eleven event names under a top-level `hooks`
member that Codex does not recognise, so Codex registers nothing. Four measurements, all taken on
the owner's own machine on 2026-09-05:

**The official reference shows the wrapper.** `developers.openai.com/codex/hooks` redirects to
`learn.chatgpt.com/docs/hooks`, and the file it documents has a top-level `hooks` member holding the
event names.

**Codex's own state file lists all eleven events against that exact path.** `~/.codex/config.toml`
carries a `[hooks.state]` table whose keys are source, event and entry index — the eleven events of
`~/.codex/hooks.json`, event names normalised to snake_case, one trust hash each. A host that could
not read the file could not have written that table.

**The wrapper predates this product.** The 454-byte file Engramux first backed up on 2026-08-28
already had it, holding somebody else's `PreToolUse` hook.

**The owner checked `/hooks` and reported all eleven trusted**, so the documented trust gate — a
changed hook is marked for review and skipped until trusted — is not the cause either.

The lesson is the session's own subject one layer out: the diagnosis came from one session's reading
of the vendor documentation rather than from the host's own state, and the state that refutes it was
two commands away. `AGENTS.md` now carries the row.

---

## 3. What was built

**`doctor` reports what each host has actually delivered.** The doctor reply carries the per-host
breakdown the service already computes for `cells`, and each host's configuration line is now
followed by a `<host> received` line: a count and the most recent arrival, or that nothing has ever
arrived, or that nobody can answer. It is a note and never a fail, for `permissions`' reason —
`doctor` cannot tell a host that is broken from a host the user has not opened since installing.

Two states are deliberately not read as zero: a service that is not answering, and a service too old
to send the breakdown, which `Events > 0` with no cells identifies. Reading either as "this host has
never delivered" would be this command inventing the evidence it exists to supply.

Six mutations were applied and all six killed, each changing an answer rather than removing a
reference.

**`MergeHooks` was not touched and must not be.** Its output matches the reference for both hosts.

---

## 4. The open question

**Why do the eleven hooks not fire?** Everything that can be checked from outside Codex says they
should: the shape is right, the path in the file exists and is the installed relay, the hooks are
trusted, and `[features] hooks` is `true`. Codex ran 83 sessions between 2026-08-29 and 2026-09-04
and delivered nothing.

Three candidates, all `[unverified]`, in the order the evidence supports them:

**Enabled is not trusted.** The `/hooks` browser can disable an individual non-managed hook, and
that is a different axis from the trust hash. Nobody has looked at it. Cheapest to check and it
would explain all eleven at once.

**The command spelling.** `CodexEntry` writes the program token quoted *inside* the value and passes
no arguments. Every hook that demonstrably runs on this machine spells an unquoted program token
followed by quoted arguments. Against it: a fully quoted absolute path is a valid Windows command
line, so this is a difference rather than a defect until something measures it.

**The matcher.** `"*"` is documented as matching everything, but `openai/codex#22847` reports `"*"`
matchers misbehaving. It cannot be the whole story: the six events Engramux gives a matcher to are
disjoint from the five it does not, and all eleven are silent.

**What would discriminate.** A Codex session with somebody watching, because a hook failure is
reported in the TUI and nowhere this repository can read — `~/.codex/logs_2.sqlite` records
`hook/started` and `hook/completed` as app-server notifications with no hook identity, and the
session rollouts carry no hook records at all. Changing one thing at a time in that file is the
next step after that, and **an agent does not make those edits** — `AGENTS.md`'s rule about the
user's host configuration covers it, and neither carve-out reaches a hand edit of `hooks.json`.

---

## 5. The acceptance criterion, and why it is still not a file read back

A corrected file read back does not verify anything. That is exactly the check that passed for nine
days while the feature was dead, and it is the check that produced the refuted diagnosis.

What closes this is **an event arriving**: a Codex session, a prompt, and then a
`codex UserPromptSubmit` in `engramux cells` with a timestamp after the change. Do not manufacture
it — a payload pushed through the relay by hand would satisfy the letter of that sentence and prove
nothing, which is the failure mode this whole row is about.

---

## 6. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first. The override is
package-relative.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed.** Its 118 Codex captures are all from 2026-08-27 02:30–04:44,
before Engramux first wrote that hook file, and their `_cap.argv` records a capture probe rather
than the relay — they are not evidence that Engramux's Codex hooks have ever run.

---

## 7. Things that will bite

1. **A settled-looking finding is still a finding somebody made.** Session 17 was told in writing not
   to re-litigate two of three conclusions. Checking one of them anyway is what produced this
   document. The instruction was reasonable and the check cost four commands.
2. **`~/.codex` holds a bearer token.** Read that directory structurally — key names, counts, file
   sizes — and never values. `config.toml`'s section headers are safe; its `Authorization` line is
   not.
3. **A quoted heredoc still collapses a backslash here.** A `python - <<'PY'` block carrying `\\L`
   arrived as `\L` and the replacement silently matched nothing. Write anything containing a
   backslash with a file-write tool, or match on a substring that has none.
4. **A pipeline's exit code is the last command's.** `go test ./... | head` reports `head`'s 0.
   Redirect to a file and read the exit code on its own line.
5. **`internal/search` takes about seventeen minutes under `-race`.** It is not hung.
6. **`go test` without `-p 1` is refused by a guard**, on the command line and not in `GOFLAGS`.

---

## 8. Done when

The three candidates in §4 have been narrowed by something measured rather than argued; backlog 50
is closed or carries a written decision; whatever changed in `internal/host` has a test watched
failing under a mutation that changes the answer; the suite, the pinned linter and the race script
are green in that order and not concurrently; and a session 19 brief exists.

**The one thing that cannot be in that list** is a Codex event actually arriving, because that needs
a person and a session. Say so rather than implying the feature is proved.
