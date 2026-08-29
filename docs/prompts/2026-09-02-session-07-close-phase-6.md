# Session 07 — Engramux: close Phase 6

Session 06 did the `[auto]` half of Phase 6, had it reviewed, fixed what the review found, and built
the binary the soak runs on. **The redaction audit is written, gated and green, and it found
nothing.** What is left is the soak, and the soak is a clock: it wants 72 hours of one service on
one binary, and only time supplies that.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file
carries what they cannot: the state of the work when the session opened, and how that session was
scoped.

Read `docs/superpowers/specs/2026-08-27-engramux-1.0-design.md` **§8's Phase 6 row first** — it is
now long, because session 06's job was to make it say what the audit is over and what the soak
records. Then §7.1's `The redaction audit finds nothing` and `A read deadline shorter than a cold
read`, and §7.3's soak row, which is where the series lives until it is finished.

**If the soak has not reached 72 hours, it cannot be made to.** Read §2, check the series, and stop.
Everything else here is either already done or waiting on that clock.

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Session 06's commits, **pushed**. Working tree clean |
| Last full verification | `go test -p 1 -count=1 ./...` **16 packages ok** · pinned linter `0 issues.`, **exit 0** (the exit code, not the summary line) · `./scripts/race.sh` **exit 0, 16 ok, 0 `DATA RACE`** |
| `dist/` and the installation | **Rebuilt and reinstalled**, byte-for-byte identical to `dist/`. `engramux.exe` 4,285,440 B, `engramux-service.exe` 13,919,232 B |
| Smoke test on the new build | `status` and `doctor` exit 0, MCP listening, both hosts pointing at the endpoint, tokenizer agreeing — and the flow the `events.id` fix could have broken, `search` then `event` on a real id, round-trips |
| The soak clock | **Not started when this was written.** It starts at the user's reboot, and everything in the series before that reset is not the soak |
| Backlog | **22 rows**, down from 26. Session 06 closed 5, 11, 24 and 29 |
| Phases | 1–5 done and gated. **6's `[auto]` half is done and gated; its `[manual]` half is the soak** |

---

## 2. The soak, and the two things only the user can do

**The clock is the service's own uptime**, read off `engramux status`. It starts at the reboot and
ends 72 hours later. If the service restarted at any point in between — a logon, a crash, a
`schtasks` mistake — the clock restarted with it, and §8's gate is not met however good the series
looks. Check that before anything else.

**The record is `.capture/soak/soak.tsv`**, gitignored, one TSV line per sample, with a `pid` column
naming the run that wrote each row. Read the series from the **first row after the uptime resets**;
what precedes it is the pre-reboot tail, and `soak-pre-reboot.tsv` and `soak-shakedown.tsv` beside
it are earlier files that are not series at all.

Two things session 06 could not do:

1. **Register the sampler task.** `schtasks /create` is denied in that session's sandbox — `/query`,
   `/run` and `/end` are not, which is worth knowing — so the Task Scheduler entry is the user's to
   create. It runs `scripts/soak-sample-hidden.vbs`, which exists so the sampler does not flash a
   console window 144 times over three days. **Delete the task when the soak ends**; nothing else
   will.
2. **The reboot.** It is what starts the clock on the installed build.

**Start exactly one sampler.** Two loops appending to one log is not hypothetical — it happened
three times over in session 06, and `AGENTS.md` has the row on the two mechanisms. Check `ps -W` for
`/usr/bin/sleep` before starting anything, and `kill -9` both the loop and its sleep.

**The one thing the pre-soak samples turned up.** `status` lost to the 4 s read deadline twice, both
with `service: group events by cell: context deadline exceeded`, and both while a race-detector run
was saturating the machine — 2 of 2, no occurrence outside one. The correlation is with load, not
with the number. If you see one with nothing else running, that is the evidence: record it, and do
not change the deadline in the same breath, because that restarts the clock.

---

## 3. Why the binary was rebuilt before the soak and must not be after

The soak's precondition is that the binary stops changing. The user could afford **one** reboot, so
session 06 spent it: the four backlog rows that needed a shipped-binary change were done together
rather than trickled, and the reboot that installs them is the reboot that starts the clock.

That spends the budget. **Any further change to a shipped `.go` file, followed by a rebuild and a
reinstall, ends the soak and starts a new one at zero.** "Not a `_test.go` file" is the wrong test
for what is shipped; `go list -deps ./cmd/engramux-service ./cmd/engramux` is the right one —
`internal/secret/secrettest` is ordinary Go source that no shipped package imports.

Documentation, tests and scripts are free. §5 says what to spend the three days on.

---

## 4. What session 06 built, so you do not rebuild it

**The audit.** `TestPhase6RedactionAudit` in `internal/service` loads one event with a generated
sample of every shape in §6.1's table plus a user path in `hook_event_name`, `session_id` and `cwd`,
and sweeps all eleven documents that leave the machine: four reply documents, four MCP tool results
over an in-memory transport, and three tool errors. Two assertions per document — a `secret.Detect`
sweep and a literal search for each sample's `Needle` — because they fail on different bugs.
`TestPhase6TheMaskedCorpusIsCleanUnderARescan` in `internal/secret` masks every real capture and
rescans it. **§8's Phase 6 row decides the scope**: four surfaces in, five out with a different
reason each. Do not re-derive them.

**The pre-soak fixes.** Backlog 29, `events.id` masked on both replies that carry it, with the half
that matters held first — a real UUIDv7 is returned unchanged, so a hit's id still round-trips to
`get_event`. Backlog 24, the 512 KiB field cap withdrawn from spec §6 rather than implemented, with
`ipc.MaxFrameLen`'s justification rewritten from the measurement that outlived it. Backlog 5 and 11.

**Fifteen deliberate breaks, every one caught.** Three are worth carrying:

- The corpus rescan is **not** broken by changing the placeholder's spelling — `placeholder` and
  `isPlaceholder` read the same constant and move together. The mutation that isolates idempotence
  is `isPlaceholder` returning false.
- A break-it pass reverts with `git checkout --`, so it deletes uncommitted work in the files it
  touches. Three files went that way and the only symptom was two mutations reporting `NOOP`.
  `AGENTS.md` has the row.
- **The review found three inert assertions in the audit itself**, none of which the twelve breaks,
  the suite, the linter or the race detector had caught — because an assertion that cannot fail also
  cannot fail loudly. §7.1's redaction-audit row has all three. `secrettest.Sample.Needle` exists
  because of the first, and its doc comment is where that reasoning lives.

---

## 5. What to do, in order

**T1 — Check the clock, then the series.** `engramux status`'s uptime past 72 hours, with the sample
series behind it and no restart in the service log.

**T2 — Write the series up.** §7.3's soak row moves to §7.1 with what the 72 hours showed: the WAL's
range, the database's growth rate, the working set's trend or absence of one, the handle and thread
counts, and how many samples recorded each of `read-failed`, `down`, `unknown` and `parse-failed` —
four different things, and only the first is about the read deadline. The working set is the MCP
session map's only instrument, so its trend is the answer to the one question §5.9 left open; a flat
working set says nothing about the map's size, only that it is not growing without bound.

**T3 — The backlog, while the clock runs.** 17 of the 22 rows need no shipped-binary change and are
free during the soak: 1–4, 7–10, 12–15, and 19–23. Row 8 is one of them and is worth doing early —
it is `particleStem` in `gate_test.go`, not production code, and it costs the Phase 4 gate 22 Hangul
particle candidates. The five that are not free are 6, 16, 17, 27 and 28; they wait for the soak to
end and then go into one build, the same way this one did.

**T4 — Whatever the series turned up.** If it turned up nothing, Phase 6 is done and every phase gate
is green. That is **not** 1.0: the user's decision is that this stays a personal tool for now — no
tag, no release, and the backlog comes first. The repository has no `README` and no `LICENSE` while
`origin` is public, which is a fact worth knowing rather than a task in this file.

---

## 6. Open, and what to do about each

| Open | What to do |
|---|---|
| Backlog 28, the token in three files with inherited ACLs | A design change, not a bug fix; §5.9 already accepts the exposure. Needs a build, so it is not soak-compatible |
| Backlog 27, a refusal with no reason | Narrowed, not closed. The tool surface carries its reason; the CLI still reads a bare rejected `Ack`. A Phase 1 wire-contract change |
| Backlog 16 and 17, the event-name bound and its missing marker | Both wire-visible, both need a build |
| Backlog 6, `doctor` saying when `ENGRAMUX_TEST_PIPE_SID` is set | Small, needs a build |
| `doctor` cannot see a token mismatch | Accepted, deliberately. Unchanged |
| An MCP reply has no size ceiling | Still stated in three comments and §5.9's tool table, still left that way: a cap needs a number nobody has measured |
| The 4 s read deadline against a growing database | The database was 108 MB when 4 s was chosen and is about 149 MB now, and the budget goes on page-cache I/O, so this is a number that ages. §2 has the two observations |
| Codex `SessionEnd` past the clamp, §7.3 | Still unmeasured; needs a deliberately slow hook in a user's own configuration. Not agent work |
| No `README`, no `LICENSE`, `origin` public | The user's call and currently "later". A public repository with no licence grants nobody any rights |

---

## 7. Five things that will bite

1. **A rebuild ends the soak.** §3. Check `go list -deps` before you believe a change is free.
2. **`schtasks /create` is denied in the agent sandbox**; `/query`, `/run` and `/end` are not. Do not
   route around the denial — hand the command over.
3. **`schtasks /end` then `/run` back to back leaves nothing running.** There is a row in
   `AGENTS.md`. Wait for `status` to stop answering before starting.
4. **Check the linter's exit code, never its summary line.**
5. **`./scripts/race.sh` takes 10–20 minutes**, almost all of it `internal/search`. While it runs the
   machine is saturated, and a soak sample taken during it is not evidence about the read deadline.
