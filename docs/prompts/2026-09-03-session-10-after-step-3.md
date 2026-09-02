# Session 10 — Engramux: publish the delivery design, then Step 4

Session 09 did all of Step 3 and then kept going. What it left is a **half-decided cluster** — how a
built Engramux reaches a user and gets replaced there — and a plan whose next build is Step 4. This
session does both, in that order: **grill the cluster to the end, write it into the spec and the
plan, and then execute Step 4.**

The order is not arbitrary and is not a reversal. `engramux update` is scheduled *after* Steps 4 and
5 as a **build**; designing it first is what this repository did for M-2 before Step 3, and it is
worth doing now because half of it is already decided and the reasons are still in front of someone.

**This file replaces `2026-09-02-session-10-after-step-3.md`**, which was written before any of that
happened and is wrong about the reinstall procedure, the backlog, the spec revision and the state of
the machine. Session 09's own brief records the same move being made for the same reason; the stale
file is deleted rather than kept, because no session ever opened on it and it is a record of nothing.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context — including two
carve-outs that are new and that change how you work. Read them before you touch anything.

Read, in this order: `docs/superpowers/plans/2026-08-30-after-phase-6.md`, which records Steps 1, 2
and 3 done and now carries **Step 6**; `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md`
**rev.4** whole — **M-7** for what is already decided about the update path, **§8** for the four
publication conditions, **M-3** for what Step 4 actually is, **§5** for gate **M4**, and M-2's three
build subsections for what Step 3 settled; then in `2026-08-27-engramux-1.0-design.md`, **§5.5** for
the upgrade sequence, **§5.6** for the filesystem layout and **§5.7** for how the index is built.
Last, `scripts/reinstall.sh` and backlog rows **36** and **37**.

**Written 2026-09-03, at the end of session 09.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `a4caf51`. **Three commits ahead of `origin/main` and unpushed** — the owner was not asked before the session ended. `git status -sb` is the answer |
| Installed | The Step 3 build, running. Migration `00004` applied, 303 memory items over 81 files, five-minute collection ticks in the log |
| **Broken** | **`engramux.exe` is gone from the build output directory and from the install directory.** Windows Defender removed it mid-session as `Behavior:Win32/Execution.A!ml`. The service binary was untouched and is running; capture, indexing and the MCP endpoint are all fine. What is unavailable is the CLI — so `doctor`, `status`, `search`, `memory` and any reinstall cannot run until §3's first item is done |
| Last verification | On `a4caf51`: `go test -p 1 -count=1 ./...` **17 packages ok**. The pinned linter and `./scripts/race.sh` last ran green (`0 issues.` exit 0; 17 packages, no report, exit 0) two commits earlier, before two test-only changes — **re-run both before merging anything** |
| Gates | **M1** and **M2** pass in the normal suite. **M3 skips**: its fixture is the owner's and does not exist yet |
| Backlog | **Three rows.** **28** a publication condition, **36** a memory item's title, **37** the Defender quarantine |
| Branches | `step-3-native-memory` merged and deleted. `origin/step-2-engramux-install` is still on the remote; deleting it is the owner's remote change |

---

## 2. What to do, in order

**T1 — Read §1, then `git status -sb`.** If the tree is not clean, find out why first. Nothing writes
to `main` directly except documentation: Step 4 is **a branch per plan step, merged `--no-ff`**, named
`step-4-derived-fields`.

**T2 — Unblock the CLI.** §3's first item. It is the owner's hands and everything else in this
session works without it, so start it and carry on rather than waiting.

**T3 — Grill the delivery cluster to the end.** Use the `grilling` skill. §4 is what is already
decided and must not be re-opened; §5 is what is open. The owner prefers multiple choice through the
structured question tool, recommended option first and marked **(권장)**, and answers in Korean.
Ask nothing the environment can answer — several of §5's items are settled by reading a file or a
public document, and session 09 wasted a question by not checking first.

**T4 — Write the answers into the spec and the plan, and stop there.** One memory spec revision
extending **M-7**, cited by the plan's **Step 6**; the plan gains no values. This is documentation and
lands on `main`. **Do not build the update path in this session** — it is scheduled after Steps 4 and
5 and the reason is in the plan.

**T5 — Execute Step 4.** Memory spec **M-3**, derived fields. Its gate **M4** is its own delete
condition: no improvement in P1's classes at recall@10 and MRR with the boost on and off means the
code is deleted, and that is not a formality. For every test that guards an invariant: write it,
watch it fail, implement, watch it pass, break the implementation on purpose, watch it fail, revert.
One commit per decision. **Commit before every break-it pass** — §6's first row.

**T6 — Verify, install, merge, close.** Suite, the pinned linter (check its exit code, never its
summary line), `./scripts/race.sh`, in that order and not concurrently — the race script needs about
nine minutes cold and wants a background run rather than a foreground timeout. Build both binaries
with the commands in `AGENTS.md`, then reinstall: `AGENTS.md` now permits you to do this yourself,
under a condition it states. Merge `--no-ff`. Plan gets a dated "Done" paragraph; a session 11 brief
is written in this directory. **Push only after asking.**

---

## 3. What only the owner can do

**Unblock the CLI, and it is two things.** `Add-MpPreference` was refused with HRESULT `0xc0000142` —
unelevated, or Tamper Protection, which is the feature that exists to refuse exactly that — so the
exclusion has to be added through the **Windows Security UI**, by a human, for the build output
directory and the install directory. And the binary should be **submitted to Microsoft as a false
positive**, which is free and fixes it for every user rather than for one machine. Both were decided
on 2026-09-03 and neither has been done. **Do not work around this**: an antivirus is a hard boundary
and re-building to retry is recreating a denied effect, not recovering from one.

**Gate M3's fixture.** 25 queries per host whose answer exists in only that host, human-labelled, at
`.capture/m3/queries.tsv` — outside the repository, never committed, and the gate skips with the
format printed when it is absent. Three tab-separated columns: the host, the query, and a run of text
that is in the answering item and in none of the other host's. Two rules the gate will otherwise fail
you for: the answer must not be a path or a credential, because what is compared is the *masked* body;
and it must be text rather than an id, because an id is derived from the file's path and rots when the
file moves. The workflow is the product itself — search for something you remember, and copy the
answer out of the printed excerpt, which is already masked and therefore safe by construction.

---

## 4. Decided in session 09, and not to be reopened

Every one of these is in the memory spec rev.4 with its reasoning; this list is so that the grilling
does not start from zero.

- **`engramux update` is its own command**, and it is `install --apply` minus everything that writes
  host configuration. Safety comes from the definition of the command rather than a condition on the
  caller, which is why an agent may run it and `install --apply` needs a precondition.
- **Engramux never fetches what it runs.** It has made no outbound call in its life — measured,
  `net/http` is imported by one shipped file and used to listen on loopback. A delivery channel
  updates a local marker; `update` reads it.
- **`--from` is an escape hatch, not the default.** It is how a developer updates from a build tree
  and how an offline user updates from a folder they downloaded.
- **Prebuilt binaries only.** No `go install`, no source as the primary path: a Go toolchain is a
  heavier runtime than the C one `CGO_ENABLED=0` exists to avoid, and a locally built binary has no
  publisher, so it starts from *less* reputation rather than more.
- **No hook-triggered automatic update.** Three independent reasons, any one sufficient: the relay's
  whole budget is 1 s, Windows cannot overwrite a running image and the `SessionStart` relay *is* the
  binary being replaced, and the detached child that would follow is the architecture `AGENTS.md`
  forbids as a model.
- **There is no plugin lifecycle hook to hang an updater on.** Verified against Claude Code's own
  plugin reference: no install-time, update-time or removal-time hook exists, and the one automated
  lifecycle step is the Node dependency install M-5 removed.
- **§8's fourth publication condition is an outcome, not "sign the binaries".** Microsoft's March 2024
  SmartScreen change removed EV's instant bypass, so signing makes reputation accumulate across
  releases rather than switching a detection off, and a first release by a new publisher still has
  none.
- **The update path is built after Steps 4 and 5** (plan Step 6). Designing it now is not a reversal
  of that.

---

## 5. Open — this is the grilling

Each of these is a decision, not a fact to look up, except where it says otherwise.

| Open | What it decides |
|---|---|
| **The delivery channel** | The one thing M-7 deliberately left. A Claude Code plugin, a package manager, a downloaded folder, or several. It decides what local marker `update` reads, and until one exists `--from` is its only door |
| **Codex users get nothing from a Claude Code plugin** | Half of what this product exists for is the *other* host. A channel that serves one host serves half the audience, and the grilling has to confront that rather than discover it later |
| **A product version** | There is none. `ipc.Version = "1"` is the wire protocol version and nothing else; there is no `--version`, no tag in the build. An ldflags line is easy; what the scheme is, and whether the wire version stays separate from it, is the decision |
| **A release process** | No tags, no artefacts, no pipeline. Whether one is built, where it runs, and whether the build is reproducible |
| **Signing: buy a certificate or not** | §8 left the *condition* outcome-based and this is the separate, unmade decision. Costs recorded in §8: a key on FIPS 140-2 L2 hardware since June 2023, and from March 2026 a certificate valid for at most 460 days, so it is a recurring chore rather than a purchase |
| **The false-positive submission as a repeatable step** | Whether every release is submitted, or only when something is reported |
| **What `doctor` says about versions** | It needs something to compare against, which the channel decides |
| **Whether `scripts/reinstall.sh` survives** | M-7 says it becomes nearly empty once `update` exists. Wrapper, or gone |

---

## 6. Things that will bite

1. **`git checkout -- <file>` restores HEAD, not the working tree.** Commit before every break-it
   pass. It silently ate uncommitted work twice in session 09 before the lesson stuck.
2. **A build failure is a discarded mutation, not a killed one.** Removing the last reference to an
   import makes a mutation that never runs; change the answer instead — `n > 99` rather than deleting
   the branch.
3. **A test can be fake and only a break-it pass says so.** Session 09's memory-hit masking test
   searched for a literal user name the *body* carried, while the field it was named for carried the
   machine's real one. Sweep a marshalled document with `secret.Detect`, which is what §8's Phase 5
   clause does.
4. **Write a mutation to a script file and assert it applied.** A backtick in a Go string breaks a
   `python -c` inside `$( )`; session 09's assertion caught one that had not applied and would
   otherwise have reported `ok` for a suite that ran unmutated.
5. **A relative path in a test resolves against its own package**, not the repository root.
6. **The service's collector runs on its own goroutine and `start()` does not gate it.** A test that
   asserts straight after starting the service is racing it, and wins until the pass gets slower.
7. **A live MCP client caches the reply schema.** A session open across an upgrade rejects a `search`
   reply carrying memory hits until it reconnects. The service logs nothing, because it is the client
   validating.
8. **Defender quarantines a fresh unsigned CLI in the build output and install directories, but not
   the test suite's temporary builds** — measured, the suite passes. So development is not blocked;
   only the reinstall path is.

`AGENTS.md`'s own table carries the rest, and gained one this session: a PowerShell one-liner run
through bash loses `$_` before PowerShell ever sees it.

---

## 7. Done when

The delivery cluster's answers are one memory spec revision extending M-7, cited by the plan's Step 6,
with the channel decided and its consequences for Codex users written down; gate **M4** passes or the
code it measures is deleted, which by that gate's own terms is the same outcome; suite, pinned linter
and race script are green; the build is installed and `doctor` is green; `step-4-derived-fields` is
merged `--no-ff`; the plan says Step 4 is done with the evidence; and a session 11 brief exists.
