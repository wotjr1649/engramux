# Session 17 — Engramux: half the product has never worked, and the checks all said it had

Session 16 was scoped around a `README` and a gate. It closed three publication conditions, took
the SQLite driver out of the relay, and then followed one anomaly in a `doctor` output far enough to
find that **Codex has never captured a prompt on any machine** — and that every check this
repository has for it reports it healthy.

**You are being handed a grilling that stopped at its third question.** The first two are settled by
evidence and are written below as settled. The third is the owner's and is the reason this document
exists.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read, in this order: `docs/superpowers/backlog.md` row **50**, then rows **47** and **48**; then the
1.0 spec's **§4.3**, which was corrected on 2026-09-04 and carries the reasoning this row repeats
one layer out.

**Written 2026-09-05, by session 16, against `65a76ba`.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `65a76ba`, pushed, 0 ahead. `origin` is public |
| Checks | Suite 21 packages, pinned linter `0 issues.`, `scripts/race.sh` 21 packages and no data race — all exit 0, in that order and not concurrently, on `7f3118d`. **Nothing but `backlog.md` has changed since**, so they still describe this tree |
| Gates | M1–M6, M9, M10 pass. **M7 is still un-run**: `prompts.tsv` is 150 of 150 `TODO` and `blocks.tsv` does not exist |
| Publication | **1, 2 and 4 closed** on 2026-09-04, on a machine that had never run these binaries. **3 stays open** — Defender never fired there, so nobody walked the exclusion procedure, and that condition is satisfied by a recorded procedure and not by an absence |
| Backlog | **36, 37, 38, 40, 46, 47, 48, 50** open. 42, 45 and 49 closed this session |
| Injection | Built, off, untouched |

**This file is uncommitted, so the tree is dirty.** That matters before anything else: a break-it
pass that reverts with `git checkout --` deletes uncommitted work, and a new file has no `HEAD`
version to restore at all — session 16 hit the second form. Commit this file first.

---

## 2. The question you are resuming at

`~/.codex/hooks.json` on every machine this product has ever installed onto carries a document
shape Codex does not read. Two thirds of the repair are decided. The third is not:

**What should this product do about the file it has already written?**

Four shapes the answer could take, and none of them is obviously right:

- **Write the correct document and leave the wrong member where it is.** Nothing is removed from
  another product's configuration; a stray `hooks` member sits in the user's file for ever, and it
  is this product's litter.
- **Write the correct document and remove the member this product wrote.** Symmetrical with having
  written it, and the only outcome that leaves the file as it would have been. Removing a member
  from a file belonging to another product is a heavier act than adding one, and the installer
  cannot tell its own member from one a user wrote by hand under the same name.
- **Report it in `doctor` and let the person delete it.** Consistent with how the two host files'
  permissions are handled — reported as a finding rather than changed, because they are not this
  product's to change. Costs a step nobody will take.
- **Migrate on the next `install --apply` and say so in the output.** The installer already prints
  every backup path it takes, and `internal/host` now keeps a bounded number of copies, so the
  removal is recoverable by a route that already exists.

**What the answer has to survive.** `AGENTS.md`'s rule that an agent does not edit the user's host
configuration is about the agent and not about the product — Engramux writing hook configuration
with the user's consent is in scope, and `install --apply` is the consent. That rule does not settle
this. What might is that `unregister` and `register` are described there as always the user's,
because they write host configuration unconditionally.

**Grill it. Do not decide it here.** The frontier for that round is this question plus whatever
hangs off the answer; the two below are prerequisites that are already settled, so they belong to no
round.

---

## 3. Settled by evidence, and not to be reopened without new measurement

**`MergeHooks` needs a per-host root.** It writes the eleven event names under a top-level `hooks`
member and both hosts go through it, so `~/.codex/hooks.json` has exactly one root key. OpenAI's
Codex hooks reference puts the event names at the root of that file with no wrapper and draws the
contrast with Claude Code's `settings.json` explicitly. Claude Code's half is correct and is not
what changes.

**`doctor` has to report the last event actually received per host.** Today it answers
`codex 11 of 11 events point at the installed relay` by reading the file through the same member
name the installer wrote it under: this product validating its own output against its own
assumption. The service already computes a per-host per-event breakdown with first and last
timestamps for `cells`, so the line is cheap, and it is the one that would have caught this on the
first day rather than the ninth.

**Neither of these is the acceptance criterion.** See §6.

---

## 4. The evidence, so you do not measure it again

**Per host and event, from the owner's own database, 2026-09-05.** `claude-code` has all eleven
event names and 29,176 events, the newest arriving while the query ran. `codex` has two: **three
`SessionEnd`, the last on 2026-08-28 21:28**, and 54 `SessionStart`. There is **no**
`codex UserPromptSubmit` at all, in a database holding 468 Claude Code ones, and no `codex`
`PreToolUse`, `PostToolUse` or `Stop` either. The command is `engramux cells`.

**The 54 are almost all backlog 49's phantoms.** Until 2026-09-04 every Claude Code session minted a
Codex session that had never existed, because host detection read its key rules before its
`transcript_path` rule and Claude Code's `SessionStart` carries `model` and no `prompt_id`. That is
what kept `sessions` and `cells` looking alive for a host that was delivering nothing. **Closing 49
is what made this visible**, which is the strongest argument this repository has yet produced for
fixing a reporting defect rather than carrying it.

**The file.** `~/.codex/hooks.json`, 4,680 bytes, one root key: `hooks`, holding the eleven event
names. Read structurally — key names only, no values — because the directory beside it holds the
bearer token.

---

## 5. What is inference, and what is unexplained

**The three `codex SessionEnd` of 2026-08-28 are unexplained by measurement.** They are consistent
with a hand-written hooks file predating an `install --apply` that overwrote it, and `.capture/`
holds a `merge-hooks.mjs` and a `hooks.codex.NEW.json` that support the story. It is inference. Do
not write it down as measured, and do not build on it.

**Nobody has watched a Codex event arrive through a corrected file.** The documented shape and the
behavioural evidence agree, which is two independent things pointing the same way and is not the
same as having seen it work. `[unverified]` until an event lands.

**One session on the new machine is unaccounted for.** After the update, `sessions` showed a
`claude-code` session running 15:37:22 to 15:37:26 — four seconds, where every other Claude Code
session in that database runs for hours. It appeared between a `search` and a `sessions` in the same
terminal. Whether it was the Codex run landing under the wrong host, a real Claude Code session, or
something else is open, and it is worth resolving before concluding anything about what the
corrected file produces.

---

## 6. The acceptance criterion, and why it is the whole point

**A corrected file read back does not verify this fix.** Reading the file proves the writer wrote
what the writer meant, which is exactly the check that has been passing for nine days while the
feature was dead.

What closes it is **an event arriving**: a Codex session, a prompt, and then a
`codex UserPromptSubmit` in `engramux cells` with a timestamp after the fix landed. That needs a
person with Codex in front of them, so plan for the fix and its verification to be separated by a
turn.

**This is the third instance of one failure mode in two days**, and the pattern is worth naming
because a fourth will arrive:

1. Spec §4.3's host rule was recorded as 900 of 900 over a corpus holding **13 of the 22 host ×
   event cells**, and the two it lacked on the Claude Code side are the two that break the rule.
2. The first attempt to find that defect replayed detection over the corpus and compared the answer
   against each capture's recorded host — a label the same function had written. It came back clean
   and proved nothing.
3. `doctor` reads a host configuration through the member name the installer wrote it under.

Each is a check whose evidence was produced by the thing it was checking.
`internal/fixtures.TestTheCorpusCoverageIsWhatIsRecorded` now pins the first, and
`internal/host.TestDetectCorpusMeasurement` counts the disagreements that would have caught it. The
third has no guard yet, and building it is §3's second half.

---

## 7. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first — 28 injections,
largest 4,944 B, median 1,918 B, 122 abstained, 172 blocks, and the nine per-stratum figures,
identical line for line with only the timing normalised out. The override is **package-relative**.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent and an agent
writing it makes that sentence false.

**`.capture/` is never committed**, and the raw corpus and the live database are prompts, file
contents and paths.

---

## 8. Things that will bite

1. **A literal that equals a collection's size is a coupling nothing declares.** Adding a fifth
   fixture broke three of them, and two were tests that had been passing by asserting nothing —
   `internal/spool`'s Phase 1 gate read a fixture's row instead of its own because `Ingest` ACKs a
   duplicate as committed, and `internal/store`'s desync test collided at the same number.
2. **`git checkout --` on a dirty tree restores HEAD**, and on a file that has never been committed
   it restores nothing at all and reports success. Revert a mutation with an edit.
3. **A guard that refuses is a hard boundary.** Session 16 met three: a recursive delete, a
   credential-directory glob, and a `git mv` of a path containing `_test`. None was worked around;
   one measurement stays `[unverified]` as a result, and that is the correct outcome.
4. **A backslash through a script literal collapses**, including inside a quoted heredoc, because
   the second layer is the interpreter's own escapes. `char(92)` in SQL and `chr(92)` in Python
   avoid the question. Verify the bytes afterwards.
5. **`sessions` with no argument resolves the working directory as the project** and `search` is
   corpus-wide. Two sibling commands with opposite defaults, and the pair is what made a competent
   reader conclude that nothing had been captured. Backlog 47.
6. **`go test` without `-p 1` is refused by a guard**, on the command line and not in `GOFLAGS`.
7. **A pipeline's exit code is the last command's.** Redirect `scripts/race.sh` to a file and count
   the `ok` and `FAIL` lines.
8. **`internal/search` takes about seventeen minutes under `-race`.** It is not hung.

---

## 9. Done when

The third question has an answer the owner gave; `MergeHooks` writes a per-host root and `doctor`
reports the last event received per host, both with tests watched failing under a mutation that
changes the answer rather than removing a reference; backlog row 50 is closed or is a written
decision; the suite, the pinned linter and the race script are green in that order and not
concurrently; and a session 18 brief exists.

**The one thing that cannot be in that list** is a Codex event actually arriving, because that needs
a person. Say so rather than implying the feature is proved.
