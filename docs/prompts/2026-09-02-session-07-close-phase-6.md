# Session 07 — Engramux: close Phase 6

Session 06 did the `[auto]` half of Phase 6 and started the `[manual]` half. **The redaction audit
is written, gated and green, and it found nothing.** What is left is the soak, and the soak is a
clock: it wants 72 hours of one service on one binary, and only time supplies that.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file
carries what they cannot: the state of the work when the session opened, and how that session was
scoped.

Read `docs/superpowers/specs/2026-08-27-engramux-1.0-design.md` **§8's Phase 6 row first** — it is
now long, because session 06's job was to make it say what the audit is over and what the soak
records. Everything below assumes you have read it. Then §7.1's `The redaction audit finds nothing`
and `A read deadline shorter than a cold read`, and §7.3's soak row, which is where the series
lives until it is finished.

**If you are opening this before 2026-09-02 04:00 +09:00, the soak is not done and cannot be made
done.** Read §2, take a sample, and stop. Everything else in this file is either already done or
waiting on that clock.

---

## 1. Where the work stands

| | |
|---|---|
| Branch | `phase-6-hardening-and-soak`, off `main` at `26b5805`. **Not merged and not pushed** — session 06 left that to the user |
| Last full verification, on that branch | `go test -p 1 -count=1 ./...` **16 packages ok** · pinned linter `0 issues.`, **exit 0** (the exit code, not the summary line) · `./scripts/race.sh` **exit 0, 16 ok, 0 `DATA RACE`** |
| Shipped code changed | **None.** Zero non-test `.go` files differ from `main`. That is deliberate and it is the soak's precondition — see §3 |
| `dist/` and the installation | Untouched since session 05. `engramux.exe` 4,285,440 B, `engramux-service.exe` 13,919,232 B, installed and running |
| The live service | Started **2026-08-30 04:00:19 +09:00** and has not restarted since. One `ERROR` line since that start, at 04:59:51, and §2 says what it was |
| Phases | 1–5 done and gated. **6's `[auto]` half is done and gated; its `[manual]` half is the soak** |

---

## 2. The soak

**The clock ends 2026-09-02 04:00 +09:00**, 72 hours after the start above. It is the service's own
uptime that counts, not the sampler's — the sampler can stop and restart without costing anything.

**The record is `.capture/soak/soak.tsv`**, gitignored, one TSV line per sample. `bash
scripts/soak-sample.sh` takes one; `--every 1800` loops. The loop session 06 started **died with
that session**, so the first thing to do is check the last `ts` in the file and start another one.
Surviving a logoff needs a scheduled task, which is the user's to create and not an agent's.

**What it has already said, at about one hour in.** Events 12,587 → 12,792. `.db` 135.1 MB →
140.3 MB. WAL sawtoothing between 0.8 and 4.1 MB, so the checkpoint runs. Spool 0 throughout.
Working set 25–30 MB with no trend yet. Handles 201 → 211, threads 15 → 16. One hour is not a
series; do not read a trend into any of it.

**The one thing it did turn up**, and it is worth understanding before you see it again. The
04:59:51 sample recorded `read-failed`: the service was up — the sampler reads the working set
before it calls `status`, so the row proves it — and `status` still lost to the 4 s read deadline
with `service: group events by cell: context deadline exceeded`. The machine was saturated by the
race-detector suite at the time, and the same command five minutes later answered in full. §7.1's
read-deadline row carries it. **One occurrence under a load nothing else would produce is not
evidence the number is wrong.** A second one with nothing else running is, and if you see one:
record it, do not change the deadline in the same breath, because that restarts the clock (§3).

---

## 3. Why nothing was rebuilt, and what that constrains

The soak's precondition is that the binary stops changing. Session 06 therefore added tests, a
script and spec prose, and touched **no** shipped Go file — so the running service is still the
merged Phase 5 build, and its uptime is still the soak's clock.

This constrains you the same way. Any change to a non-test `.go` file, followed by a rebuild and a
reinstall, ends the soak and starts a new one at zero. That is not a reason to leave a real defect
unfixed; it is a reason to know what fixing one costs, and to batch the fixing rather than trickle
it. Documentation, tests and scripts are free.

---

## 4. What session 06 built, so you do not rebuild it

- **`TestPhase6RedactionAudit`** in `internal/service`. One event carrying a generated sample of
  every shape in §6.1's table, plus a user path in `hook_event_name`, `session_id` and `cwd`, swept
  through all eleven documents that leave the machine: the four reply documents, the four MCP tool
  results over an in-memory transport, and three tool errors — `status` has no argument for a
  caller to put a path in. Two assertions per document, a `secret.Detect` sweep and a literal search
  for each sample's own bytes, because they fail on different bugs.
- **`TestPhase6TheMaskedCorpusIsCleanUnderARescan`** in `internal/secret`. Every real capture
  masked and rescanned. Skips itself when `.capture/` is absent.
- **`scripts/soak-sample.sh`.** Reads everything from outside the service.
- **§8's Phase 6 row** now decides the audit's scope: four surfaces in, five out with a reason each.
  `doctor`, the installer's output, the CLI's own printing, the spool and the relay's stderr are the
  five, and each reason is different. Do not re-derive them.

**Eleven deliberate breaks, one per mask, every one caught.** One of them is worth carrying: the
corpus rescan is **not** broken by changing the placeholder's spelling, because `placeholder` and
`isPlaceholder` read the same constant and move together. The mutation that isolates idempotence is
`isPlaceholder` returning false.

---

## 5. What closing Phase 6 needs

**T1 — Confirm the clock.** The service's uptime past 72 hours, read off `engramux status`, with
the sample series behind it. If the service restarted at any point — a logon, a crash, a `schtasks`
mistake — the clock restarted with it, and §8's gate is not met however good the series looks.

**T2 — Write the series up.** §7.3's soak row moves to §7.1 with what the 72 hours actually showed:
the WAL's range, the database's growth rate, the working set's trend or absence of one, the handle
and thread counts, and how many samples recorded `read-failed`. The working set is the MCP session
map's only instrument, so its trend is the answer to the one question §5.9 left open — and a flat
working set says nothing about the map's size, only that it is not growing without bound.

**T3 — Whatever the series turned up.** If it turned up nothing, Phase 6 is done and 1.0's phase
gates are all green, which is a decision about what happens next rather than a task in this file.

---

## 6. Open, and what to do about each

| Open | What to do |
|---|---|
| Backlog 28, the token in three files with inherited ACLs | Unchanged from session 06's brief: a design change, not a bug fix, and §5.9 already accepts the exposure. It also costs a rebuild, so it is not soak-compatible |
| `doctor` cannot see a token mismatch | Accepted, deliberately. Unchanged |
| Backlog 27, a refusal with no reason | Narrowed, not closed. The tool surface carries its reason; the CLI still reads a bare rejected `Ack` |
| Backlog 24, the unimplemented 512 KiB cap | Still a row |
| The 23 remaining backlog rows | Untouched unless a task is already standing in that file |
| An MCP reply has no size ceiling | Still stated in three comments and §5.9's tool table, still left that way |
| The 4 s read deadline against a growing database | §2's observation is the first data point at 4 s. The database was 108 MB when 4 s was chosen and is 140 MB now, and the budget goes on page-cache I/O, so this is a number that ages. Watch it; do not move it on one loaded-machine sample |
| Codex `SessionEnd` past the clamp, §7.3 | Still unmeasured; needs a deliberately slow hook in a user's own configuration. Not agent work |

---

## 7. Four things that will bite

1. **A rebuild ends the soak.** §3. Check what you are about to change before you change it.
2. **`schtasks /end` then `/run` back to back leaves nothing running.** There is a row in
   `AGENTS.md`. It also ends the soak, twice over.
3. **Check the linter's exit code, never its summary line.**
4. **`./scripts/race.sh` takes about twenty minutes**, almost all of it `internal/search` at 840 s.
   Start it early and do something else. While it runs the machine is saturated, and §2's
   `read-failed` sample is what that looks like from the soak's side — so a sample taken during a
   race run is not evidence about the deadline.
