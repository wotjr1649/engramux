# Session 09 — Engramux: Step 3, native memory indexed

Session 08 closed the Phase 6 soak, merged Step 2, and did all of Step 1 in one sitting: eleven
backlog rows, one commit each, built, installed and verified on the owner's machine before the merge.
What is left of the plan is its memory work — Steps 3, 4 and 5 — and nothing blocks it. This session
is Step 3, memory spec **M-2**, and it has three parts in a fixed order: read the hosts' real memory
files, decide what the spec left open, then build. **Do not write Step 3 code before the second part
is recorded in the spec.**

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file carries
what they cannot: the state of the work when the session opened, and how that session was scoped.
An earlier file of this name, written half an hour before this one, was replaced before anyone read
it; it never became a record of anything.

Read, in this order: `docs/superpowers/plans/2026-08-30-after-phase-6.md` rev.2, which is the order
and records Steps 1 and 2 as done; `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md`
whole — §1 for the two `[verified]` facts about the hosts' memory, §2's M-2 for the decision and its
cost, §3 for P4, §5 for gates M1–M3, §8 for the publication conditions; then in the 1.0 spec
`docs/superpowers/specs/2026-08-27-engramux-1.0-design.md`, §5.7 for how the search index is built,
§7.1's soak row and read-deadline row for the one decision the soak produced, and the `00002` row
for what an FTS rebuild costs; last, the header comment of
`internal/store/migrations/00002_events_fts.sql` and `docs/evidence/README.md`.

**Written 2026-09-02, at the end of session 08.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `2fc2f0d` plus the commit that carries this file. `2fc2f0d` is pushed; whether this file's commit is, `git status -sb` says |
| Installed | The Step 1 build, through `engramux install --apply`, service started 2026-09-02 16:10 KST. Migration `00003` applied at that start. `doctor` exit 0 with its two new lines; `claude mcp list` answers `Connected` against the now-stateless server |
| Last full verification | On the merged code: `go test -p 1 -count=1 ./...` **17 packages ok** · pinned linter `0 issues.`, **exit 0** · `./scripts/race.sh` **16 ok, no report, exit 0** |
| Backlog | **One row, 28**, a publication condition rather than a build. Every row that needed a build closed in Step 1 with a test that fails when its fix is undone |
| Phase 6 | **Closed.** 72h5m uptime, one `serving` line, 147 samples; the 1.0 spec §7.1's soak row has the series and `docs/evidence/soak/` the file |
| The live service | `status` reports `errors 1`: session 08 sent a UNC path to `sessions` on purpose to see a refusal's reason, and the counter counted it. It resets at the next start and is not a fault |
| Branches | `step-1-clearing-build` and `step-2-engramux-install` are merged and deleted locally; `origin/step-2-engramux-install` is still on the remote. Deleting it is a remote change and the owner's to make |

---

## 2. What to do, in order

**T1 — Read the list above.** Then `git status -sb`; if the tree is not clean, find out why before
anything else. Nothing here writes to `main` directly: Step 3 is **a branch per plan step, merged
with `--no-ff`**, and the name is `step-3-native-memory`.

**T2 — Facts before questions: read both hosts' real memory files on this machine.** The memory
spec marks the Codex index's line-level schema `[unverified]` because nobody who wrote it had read
one, and gate **M2** is shaped against exactly the failure a guessed schema produces. Everything
below is a reading, not a decision, and it is done before any question is asked, because the
questions in T3 are answerable only against these facts.

For each host: where the memory lives on this machine and how that location is resolved — Claude
Code's is documented as configurable and **must be resolved from its settings, never hardcoded**;
where the setting lives and what it is called is `[unverified]` and is the first thing to find. Then:
how many files, their sizes, whether they are per project or global and how a file maps to a project
path, the index file's line shape, every frontmatter key that actually occurs and how often, the
`modified` spread (how live the directory is), and whether the Codex directory really is the git
repository its README describes. Record shapes and counts only. **These files are the owner's
private notes: read them locally, never paste a line of one into the conversation, a commit or a
test, and never copy one under the repository** — a fixture made from one is a promotion, which needs
redaction and the user name substituted out of every path, the same rule `.capture/` is under.
Write the reading into the memory spec's M-2 section as a dated `[verified]` paragraph, and mark
what could not be read as `[unverified]` with what would settle it. That paragraph is committed on
`main` before T3, because it is a measurement and not Step 3's behaviour.

Two tooling facts for this part. A shell command that so much as names `.claude.json` is refused by
the maintainer's credential guard whatever it does with the name — keep that literal out of command
lines. And the `claude` binary resolves its own configuration and ignores a redirected `HOME`, so
nothing in T2 runs it against a test tree.

**T3 — Grill, scoped to what the spec leaves open, and record the answers as a spec revision
before any code.** Use the `grilling` skill. The owner prefers questions as multiple choice through
the structured question tool, with the recommended option first and marked **(권장)**, and answers
in Korean. Ask nothing the environment can answer — T2 exists so that these five can be put
narrowly:

1. **How native memory is collected.** A scan at every service start, a timer like the drain's, or a
   file watcher. The soak's instruments — handles, threads, working set — are what a watcher shows
   up on, and the 1.0 spec §7.1's soak row is the baseline any choice is compared against.
2. **Where a memory item is stored.** The Phase 1 schema carries an unused `memory_items` table with
   a `project_id` foreign key and a `key`; whether that shape fits, and what `project_id` means for a
   memory that belongs to no project, is decided here and not by reusing the table because it is
   there.
3. **How it enters search.** The same `events_fts` index, a second FTS table, or the base table
   filtered. The 1.0 spec's `00002` row is the cost of an index rebuild, and plan Step 4 asks whether
   Steps 3 and 4 share one — answer both together, so there is one migration and not two if that is
   what the shapes say.
4. **What the wire and the tool surface say.** How a hit says it is a memory item and not an event;
   whether `search` covers both or a tool of its own does; what `get_event`'s equivalent is for a
   memory item. Wire changes ship as one compatibility event — Step 1's rule — so this is one
   decision, not three.
5. **Where M3's fixture comes from.** Twenty-five queries per host whose answer exists in only that
   host, human-labelled, from the owner's own memory. Who labels them, how they are redacted, and
   whether they live in the repository or only on this machine. **This one only the owner can
   answer.**

Each answer lands in the memory spec, one revision bump, cited by the plan; the plan gains no
values. Where an answer depends on a T2 fact that could not be read, the spec says so and the code
waits.

**T4 — Build, one decision at a time.** For every test that guards an invariant: write it, watch it
fail, implement, watch it pass, break the implementation on purpose, watch it fail, revert. Assert
the mutation applied before trusting a result, and run each mutation as plain `sed`, `go test
-run`, `git checkout --` lines — a shell function with a variable named for the test tripped the
maintainer's test guard in session 08. One commit per decision. Gates **M1**, **M2** and **M3** are
tests, and M2 is the one to watch: a parser that skips quietly passes a review and fails a format
change silently.

**T5 — Verify, install, merge.** Suite, the pinned linter (check its exit code, never its summary
line), `./scripts/race.sh`, in that order and not concurrently. Build both binaries into `dist/`
with the commands in `AGENTS.md`. The reinstall is the owner's hands, run in the session with the
`!` prefix: `schtasks /end /tn Engramux` with `MSYS_NO_PATHCONV=1`, then wait until
`engramux status` stops answering, then `dist/engramux.exe install --apply`. Verify with `doctor`,
`status`, a `search` and an `event`, and — if the tool surface changed — Claude Code's own
`claude mcp list`. Then merge `--no-ff` into `main`. **Push only after asking**, once; the owner
answered yes at every push point in session 08 but asked to be asked.

**T6 — Close.** The plan's Step 3 gets a dated "Done" paragraph with the evidence; any backlog row
a test now owns is deleted; a session 10 brief is written in this directory, named for what session
10 opens on, and is a record from the moment it is committed.

---

## 3. Decided, and not to be reopened

- **Order:** Steps 3, 4, 5, in that order; the publication conditions are not ordered and none of
  the steps waits on them (plan rev.2).
- **`readDeadline` stays at 4 s** until a series measures the covering index's effect; changing
  both in one build was refused on purpose (1.0 spec §7.1, read-deadline row).
- **The MCP handler is stateless** and offers revision `2026-07-28`; the session map it dropped was
  kept through Phase 6 only for the soak to watch (1.0 spec §5.9).
- **The event name bound is the reply's**, 256 runes, with a flag when cut; the `Drain` request type
  and the upgrade's drain step are withdrawn; a refused request carries a masked reason; the status
  reply carries an error count and the last checkpoint result (1.0 spec §5.2, §5.6, backlog closures
  of 2026-09-02).
- **Publication conditions** live in the memory spec §8: a first install on a clean **profile** — a
  second local account on the owner's machine, which the owner creates — rather than a VM; backlog
  28; a `README`. The licence is Apache-2.0 and is done.
- **The soak is closed and its instruments stay:** `scripts/soak-sample.sh` samples,
  `scripts/soak-summary.sh` reduces, and a new series is what would license changing the deadline.

---

## 4. What session 08 measured, so you do not re-measure it

- **The soak's nine refusals were all one read**, the status reply's per-cell `GROUP BY`; eight of
  the nine coincided with the one outside reading the sampler could not take. `00003` is the
  covering index and it has to cover `received_at`: the two-column shape plans as `USING INDEX`,
  which still visits every row of the payload b-tree, and the break-it pass watched that plan come
  back.
- **The longest event name either host has ever emitted is 17 runes** (`PermissionRequest`, over
  902 captures); both hosts draw from a fixed list.
- **`StreamableHTTPOptions.Stateless` changes the offered revision list in no other way**, and the
  real Claude Code connects to the stateless server.
- **A re-install with the host already registered exited `claude mcp add` with 1**; the installer
  now reads the host's file first and the next re-install said "already points at this endpoint".
- **The first checkpoint after a start is five minutes out** — `status` printed `none yet` until
  16:20:34 and then a dated `ok`.
- **A scheduled task runs with its principal's environment**, not the caller's: session 07's
  "isolated" installer run started a service against the real data directory and lost the pipe race.
  That is why the publication condition is a profile and not a redirected tree.

---

## 5. Open, and what to do about each

| Open | What to do |
|---|---|
| Claude Code's memory location and the setting that moves it | `[unverified]`. T2's first reading; the spec says configurable and nothing more |
| The Codex memory directory's index, line by line | `[unverified]`. T2's second reading; the README-level shape is in memory spec §1 |
| Whether `memory_items` fits | Not a fact, a decision — T3's second question, taken against T2's facts |
| The token on the command line | Acknowledged in `claude.go`, not solved; the installer runs `claude mcp add` one time fewer since backlog 35 |
| Backups holding the token | Every re-install that changes `config.toml` leaves a timestamped copy; nothing prunes them. Undecided; not this session's |
| `origin/step-2-engramux-install` | Merged; deleting it is the owner's remote change |

---

## 6. Things that will bite

1. **A shell command naming `.claude.json` is refused**, whatever it does. Keep the literal in file
   edits; never on a command line.
2. **A line-range deletion computed before an edit deletes the wrong block after it.** Re-list
   immediately before an `awk` or `sed` range, or anchor on text; a diff caught this once in session
   08 and the compiler did not.
3. **A break-it pass reports `ok` for a mutation that was never in the file.** One `sed` pattern
   missed in session 08. Assert the mutation applied — count the mutated text — before reading the
   result, and read a build failure as "discarded", never as "killed".
4. **`schtasks /end` then `install --apply` back to back leaves nothing running.** Wait for `status`
   to stop answering; `AGENTS.md` has the row.
5. **`doctor`'s pipe-name override line is a note, not a fail**, because the service gate runs
   `doctor` under `ENGRAMUX_TEST_PIPE_SID` on purpose.
6. **The memory files are private and the guard cannot tell a fixture from a credential.** Shapes
   and counts leave the machine; content does not. A test that needs content needs a redacted
   fixture with the user name out of every path, and that promotion is its own commit.

---

## 7. Done when

T2's reading is in the memory spec as a dated `[verified]` paragraph on `main`; T3's five answers
are one memory spec revision, cited by the plan; gates M1, M2 and M3 pass as tests on
`step-3-native-memory`; suite, pinned linter and race script are green; the build is installed
through `engramux install --apply` and `doctor` is green; the branch is merged `--no-ff`; the plan
says Step 3 is done with the evidence; and a session 10 brief exists.
