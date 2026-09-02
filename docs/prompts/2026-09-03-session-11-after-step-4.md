# Session 11 — Engramux: Step 5, and the install that Step 4 never got

Session 10 did what its brief asked in the order it asked: it ground the delivery cluster to the
end, wrote the answers into the spec and the plan, and then executed Step 4. What it could not do is
the last third of its own T6 — **the build was never installed**, because the CLI binary Windows
Defender removed on 2026-09-03 was still gone at the end of the session and rebuilding it is
recreating a denied effect rather than recovering from one.

So this session opens with a merged, verified, uninstalled Step 4. The first thing it owes is that
install, and the second is **Step 5**.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context — including the
two carve-outs about the service and about `install --apply`, and a `Commands` block that gained
gate **M4**'s line this session.

Read, in this order: `docs/superpowers/plans/2026-08-30-after-phase-6.md` **rev.3**, whose Step 4
now carries a Done paragraph and whose Step 6 no longer owns the delivery-channel question;
`docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` **rev.6**, and in it **M-3**'s
two sections for what Step 4 measured, **M-7**'s two sections for the whole delivery design, **M-4**
for what Step 5 is, and **§5** for gates **M5**, **M6**, **M7**, **M8** and **M9**; then **§6**,
which is not optional reading for Step 5 — it is the section that says injecting captured content is
an injection vector, and M9 is what it turns into a gate. Last, backlog rows **28**, **36** and
**37**.

**Written 2026-09-03, at the end of session 10.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Step 4 merged with `--no-ff`. **Ahead of `origin/main` and unpushed** — the owner was not asked. `git status -sb` is the answer |
| Installed | **Still the Step 3 build**, on migration `00004`, running. Step 4's `00005` has never run against the live 182 MB database, so its cost is `[unverified]` and §7.1 has no row for it |
| **Broken** | **`engramux.exe` is still absent from `dist/` and from the install directory.** Nothing changed since session 09 removed it; §3's first item was never done. Capture, indexing and MCP are all fine — the service binary was never touched — but `doctor`, `status`, `search`, `memory` and any install cannot run |
| Last verification | On the branch before the merge: the suite green at exit 0, the pinned linter `0 issues.` at exit 0 with both values read, `./scripts/race.sh` green on its **second** run — the first was red, and §6's first row is what it found. Thirteen deliberate mutations, every one asserted present before its suite ran and every one killed; one of the thirteen did not apply on the first attempt and the assertion is what said so |
| Gates | **M1**, **M2** pass in the normal suite. **M4** passes over the corpus and skips without it. **M3** still skips: its fixture is the owner's and does not exist yet |
| Backlog | **Four rows.** **28** a publication condition, **36** a memory item's title, **37** the Defender quarantine, and **38**, new this session: the Phase 5 contention gate's margin under `-race`, measured on a commit that predates Step 4 |
| Branches | `step-4-derived-fields` merged and deleted. `origin/step-2-engramux-install` is still on the remote; deleting it is the owner's remote change |

---

## 2. What to do, in order

**T1 — Read §1, then `git status -sb`.** Step 5 is **a branch per plan step, merged `--no-ff`**,
named `step-5-injection`. Only documentation goes to `main` directly.

**T2 — Unblock the CLI.** §3's first item, unchanged from session 10's brief because it was never
done. Start it and carry on; everything below except the install works without it.

**T3 — Install Step 4, the moment T2 clears.** This is session 10's unfinished T6 and it is not
optional bookkeeping: `00005`'s backfill runs over 17,043 events and three columns on the live
database and nobody has watched it. Record what it cost in the 1.0 spec §7.1 beside `00002`'s row —
the elapsed time and what the file and its WAL did — because §7.1 is where a migration's price
lives and Step 4's is the second one this product has that is not free. Then `doctor`, and a real
`engramux search` for a command line you remember running, which is the first time the boost will
have been asked anything by a person.

**T4 — Execute Step 5.** Memory spec **M-4**: hook-time injection, built, and **shipped disabled**.
Its gates are **M5**, **M6** and **M9**, with **M8** reported rather than passed, and **M7** is what
would license turning it on for anybody — which this session does not do. §6 is the design input, not
background: the corpus is attacker-reachable, Codex's own memory files carry model-directed
instructions, and **M9's nonce delimiter is the only mitigation in that list that does not depend on
a model behaving well.** For every test that guards an invariant: write it, watch it fail,
implement, watch it pass, break the implementation on purpose, watch it fail, revert. One commit per
decision. **Commit before every break-it pass** — §6's first row.

**T5 — Verify, install, merge, close.** Suite, the pinned linter (check its exit code, never its
summary line), `./scripts/race.sh`, in that order and not concurrently — the race script wants a
background run rather than a foreground timeout, and the suite alone now runs past two minutes.
Merge `--no-ff`, plan gets a dated Done paragraph, a session 12 brief lands in this directory.
**Push only after asking.**

---

## 3. What only the owner can do

**Unblock the CLI, and it is still two things.** `Add-MpPreference` was refused with HRESULT
`0xc0000142` — unelevated, or Tamper Protection, which is the feature that exists to refuse exactly
that — so the exclusion has to be added through the **Windows Security UI**, by a human, for the
build output directory and the install directory. And the binary should be **submitted to Microsoft
as a false positive**, which session 10 decided is a line in the release checklist rather than a
reaction. **Do not work around this**: an antivirus is a hard boundary and re-building to retry is
recreating a denied effect, not recovering from one. Session 10 did not build the CLI once for this
reason, and neither should the next one.

**Gate M3's fixture.** Unchanged, and still the thing that makes M3 a gate rather than a skip. 25
queries per host whose answer exists in only that host, human-labelled, at `.capture/m3/queries.tsv`
— outside the repository, never committed, the gate skipping with the format printed when it is
absent. Three tab-separated columns: the host, the query, and a run of text that is in the answering
item and in none of the other host's. The answer must not be a path or a credential, because what is
compared is the *masked* body, and it must be text rather than an id, because an id is derived from
the file's path and rots when the file moves.

---

## 4. Decided in session 10, and not to be reopened

The delivery cluster is closed. Every one of these is in the memory spec **rev.5**'s M-7 delivery
section with its reasoning and its measurement.

- **A GitHub Release is the substrate and a Claude Code plugin is the channel**, through a
  marketplace `archive` source carrying the zip's SHA-256. The host fetches and verifies; `update`
  reads the plugin cache. Whatever fetches is still not Engramux.
- **The zip is the plugin**, which the channel forces rather than anyone choosing: the manual
  `--from` door and the channel consume the same artefact, byte for byte.
- **The plugin delivers and does not configure.** No hooks, no MCP entry, although both fields
  exist — a plugin's hooks die with the plugin, and that is capture stopping silently.
- **Codex users get the same capability and less convenience**, and the `README` says so. Codex has
  a plugin system now and documents neither an archive source nor an update command, so a Codex
  plugin could carry the binaries and could not notice a new one.
- **Semantic versioning, `0.x`, injected at link time, `ipc.Version` left alone.**
- **GitHub Actions**: the three checks on push and pull request, a tag builds and uploads. The
  catalogue lives at the repository root so a version and its artefact's hash cannot be committed
  apart.
- **No certificate is bought.** Azure Artifact Signing is closed to this project by geography —
  individuals are limited to the USA and Canada — and SignPath Foundation is free for an
  Apache-2.0 project but wants a release and a trusted build system first. Signing is sequenced
  behind the release process, not rejected.
- **`doctor` compares three versions**; **`scripts/reinstall.sh` becomes a one-line wrapper.**
- **Every release is submitted to Microsoft as a false positive.**

And from Step 4, which is a measurement rather than a preference and should not be re-litigated
without re-measuring: **two of M-3's seven fields have nothing in this corpus to read**, its error
spans are prose rather than a field, and the boost's weight sits on a plateau the sweep found.

---

## 5. Open

| Open | What it decides |
|---|---|
| **Whether the boost is worth its cost at all** | M4 passed and passed small: one class gained one document at k, the other two moved only in MRR. Its runtime cost is now partly measured and the measurement was not free — the first form of the boost put the Phase 5 contention gate over budget and the second is under it with margin. What is still unmeasured is what the three columns cost in database size on a real installation. If the answer is "a lot", M4's own delete condition is there to be applied on the second measurement rather than the first |
| **`00005`'s price on a real database** | `[unverified]` until T3. `00002` was 1.30 s and roughly double the file at 8,177 events; this one adds three columns and a backfill at 17,043 and touches no index |
| **Backlog 36, a memory item's title** | Still nobody's. Step 5 will surface it again the moment an injected excerpt carries one |
| **Whether Step 5 needs its own migration** | M-4 is about a hook-time path and not obviously about storage. Settle it before writing one, the way decision 7 settled it for Step 4 |

---

## 6. Things that will bite

1. **A red `scripts/race.sh` is not evidence of a regression until you have a baseline.** Backlog
   **38**: the Phase 5 contention gate fails about one run in five on this machine *without* Step 4,
   sitting at 692–852 ms against an 800 ms budget. Session 10's first race run was red, and the
   discriminating check was three arms rather than a retry — the merge-base in a throwaway worktree,
   the branch with the boost switched off, and the branch as written. It turned out to be both: a
   pre-existing 20% margin **and** a real regression on top of it, and only the second was this
   step's to fix. Measure the baseline before fixing anything, and never fix this one by moving the
   number.
2. **The suite is slow now, and `internal/search` under `-race` is 757 s.** Gate M4 runs 150
   searches over 901 documents twice and the race detector multiplies it; the whole race script is
   around fifteen minutes. A foreground run is moved to the background by the harness mid-run, so
   start it there and read the output file. Also do not pipe it through `tail` — nothing is written
   until the pipeline ends, and a fifteen-minute silence looks exactly like a hang.
3. **`git checkout -- <file>` restores HEAD, not the working tree.** Commit before every break-it
   pass. Session 10 committed before all thirteen of its mutations and lost nothing; session 09 did
   not and lost three files.
4. **A mutation that never applied is a third state that reads like a pass**, and session 10 hit it
   once in thirteen: a Python replacement whose backslashes the shell ate matched nothing, the
   suite ran unmutated, and `ok` was the honest answer to a question nobody asked. The assertion in
   front of it caught it. A build failure is the other non-kill, and is a discarded mutation rather
   than a killed one.
5. **A shell guard in this environment refuses compound syntax it cannot parse statically**, it
   refuses a nested shell outright, and it reads inside heredocs. A Python heredoc carrying
   backticks trips the first; `powershell.exe -Command` inside Bash trips the second; writing the
   words of a bare test command into a *document* through a heredoc trips a third. None of them is
   worked around: use the file-write tool for the edit, and ask the owner for the PowerShell.
6. **A relative path in a test resolves against its own package**, not the repository root.
7. **The service's collector runs on its own goroutine and `start()` does not gate it.**
8. **A live MCP client caches the reply schema.** A session open across an upgrade rejects a
   `search` reply carrying memory hits until it reconnects. The service logs nothing, because it is
   the client validating.
9. **Defender quarantines a fresh unsigned CLI in the build output and install directories, but not
   the test suite's temporary builds** — measured, the suite passes. Development is not blocked;
   only the install path is.

`AGENTS.md`'s own table carries the rest.

---

## 7. Done when

Step 4 is installed and its migration's price is in §7.1; **M5**, **M6** and **M9** pass and **M8**
is reported; injection is built and ships **off**, with **M7** un-run and un-claimed because
licensing the switch is a separate act; suite, pinned linter and race script are green;
`step-5-injection` is merged `--no-ff`; the plan says Step 5 is done with the evidence; and a
session 12 brief exists.
