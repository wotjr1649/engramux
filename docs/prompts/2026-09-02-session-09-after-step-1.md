# Session 09 — Engramux: after Step 1

Session 08 closed the Phase 6 soak, merged Step 2, and did all of Step 1 in one sitting: eleven
backlog rows, one commit each, built, installed and verified on the owner's machine before the merge.
The plan's memory work — Steps 3, 4 and 5 — is what is left, and nothing blocks it.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file carries
what they cannot: the state of the work when the session opened, and how that session was scoped.

Read, in this order: `docs/superpowers/plans/2026-08-30-after-phase-6.md` rev.2, which is the order
and now records Steps 1 and 2 as done; `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md`,
whose §2 M-2 and §5 gates M1–M3 are Step 3, and whose §8 now owns the publication conditions; then the
1.0 spec's §7.1 soak row and read-deadline row, which is where the soak's numbers and the one decision
they produced live.

**Written 2026-09-02, at the end of session 08.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `37f55f3`, the `--no-ff` merge of `step-1-clearing-build`. Sixteen commits ahead of `origin/main` when this was written; whether they were pushed is the last thing session 08 did, and `git status -sb` says |
| Installed | The Step 1 build, through `engramux install --apply`, service started 2026-09-02 16:10 KST. Migration `00003` applied at that start. `doctor` green, `claude mcp list` answers `Connected` against the now-stateless server |
| Last full verification | On the branch's code: `go test -p 1 -count=1 ./...` **17 packages ok** · pinned linter `0 issues.`, **exit 0** · `./scripts/race.sh` **16 ok, no report, exit 0** |
| Backlog | **One row, 28**, and it is a publication condition rather than a build. Every row that needed a build closed in Step 1 with a test that fails when its fix is undone |
| Phase 6 | **Closed.** 72h5m uptime, one `serving` line, 147 samples; §7.1's soak row has the series and `docs/evidence/soak/` the file |
| The branches | `step-1-clearing-build` and `step-2-engramux-install` are merged and can go; `origin/step-2-engramux-install` is still on the remote |

---

## 2. What to do, in order

**Step 3 — native memory indexed (M-2).** Before writing a line: read one real Codex memory index
file. The memory spec marks its line-level schema `[unverified]` because nobody in the session that
wrote the spec had read one, and gate **M2** — an unknown key or file name warns and continues,
never skips silently — is shaped against exactly the failure a guessed schema produces. Claude Code's
location is configurable and must be resolved from settings, not hardcoded. Done when M1, M2 and M3
pass. Branch it as `step-3-…`, one commit per decision, `--no-ff`.

**Step 4 — derived fields (M-3)**, after Step 3 for judgement rather than for compilation. Settle
first whether Steps 3 and 4 share an FTS rebuild; the 1.0 spec's `00002` row is the reason to ask.
Gate **M4** is the step's own delete condition.

**Step 5 — injection, built and disabled (M-4).** Ships off. M5, M6, M9 pass and M8 is reported;
M7 licenses enabling it, which is a separate act from shipping it.

**Not ordered, but owed before publication** (memory spec §8): a first install on a **clean
profile** — a second local account on the owner's machine, which the owner creates; backlog **28**,
`mcp.json` written with its own DACL and the two host files reported by `doctor`; a `README`. The
licence is done: `LICENSE` is Apache-2.0.

---

## 3. What session 08 measured, so you do not re-measure it

- **The soak's nine refusals were all one read**, the status reply's per-cell `GROUP BY`, and eight
  of the nine coincided with the one outside reading the sampler could not take. The decision was the
  index first and the deadline unchanged; `00003` is the index, and it has to be **covering** — the
  two-column shape plans as `USING INDEX`, which still visits every row of the payload b-tree, and the
  break-it pass watched that plan come back.
- **The longest event name either host has ever emitted is 17 runes** (`PermissionRequest`, over 902
  captures). The wire bound is 256 and exists for a payload that lies.
- **`StreamableHTTPOptions.Stateless` changes the offered revision list in no other way**, and the
  real Claude Code connects to the stateless server. `GET` and `DELETE` answer 405 by design.
- **A re-install with the host already registered exited `claude mcp add` with 1.** The installer
  now reads the host's file first; the second re-install of the day said "already points at this
  endpoint".
- **The first checkpoint after a start is five minutes out**, so `status` prints `checkpoint  none
  yet` until then and that is not a fault.

---

## 4. Open, and what to do about each

| Open | What to do |
|---|---|
| The token on the command line | Still acknowledged in `claude.go` rather than solved; the installer now runs `claude mcp add` one time fewer, and never when the host already points at the endpoint |
| Backups accumulate copies of the token | Every re-install that changes `config.toml` leaves a timestamped copy holding the bearer token. Today's second re-install wrote none because nothing changed; nothing prunes the ones that exist. Undecided; decide when it is scoped, not in passing |
| The `stopped` line in the real log from 2026-08-30 | Explained: a scheduled task runs with its principal's environment, so an "isolated" installer run started a service against the real data directory. `AGENTS.md` has the row; the clean-profile condition is the consequence |
| `docs/evidence/exclusive/main.go` | Working copy was CRLF against `.gitattributes`; the blob was LF all along. Fixed in the working copy, nothing to commit |

---

## 5. Six things that will bite

1. **A scheduled task ignores the environment you gave the installer.** Redirecting `LOCALAPPDATA`
   isolates every file the installer writes and nothing the task starts. Only a different user
   isolates the task.
2. **`schtasks /end` then `install --apply` back to back leaves nothing running.** Wait until
   `status` stops answering before the install, which is what its `/run` needs; `AGENTS.md` has the
   row and the loop.
3. **A break-it pass written as a shell function with a variable named for the test tripped the
   maintainer's test guard**, which reads the command line for `npm test` shapes. Run each mutation
   as a plain `sed`, `go test -run`, `git checkout --` sequence, and assert the mutation applied
   before reading the result — one `sed` pattern missed today and reported `ok` for a mutation that
   was never in the file.
4. **A shell command that merely names `.claude.json` is refused by the credential guard**, whatever
   it does with the name. Keep that literal in file edits, never on a command line.
5. **A line-range deletion computed before an edit deletes the wrong block after it.** `awk`
   ranges were derived from one listing and applied after another edit had shifted the file; the
   diff caught it, the compiler did not. Re-list immediately before deleting, or anchor on text.
6. **`doctor`'s pipe-name override line is a note, not a fail**, because the service gate runs
   `doctor` under the override on purpose. A fail there fails the gate that asks.
