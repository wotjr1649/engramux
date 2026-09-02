# Session 10 — Engramux: Step 4, derived fields

Session 09 did all of Step 3 in one sitting: the reading, the nine decisions, eleven commits, two
installs, and a merge. What is left of the plan is **Steps 4 and 5**, and nothing blocks either. This
session is Step 4, memory spec **M-3**, and the thing to know before you start is that its gate is
its own delete condition: **M4** says no improvement means the code is deleted, and that is not a
formality.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file carries
what they cannot: the state of the work when the session opened, and how that session was scoped.

Read, in this order: `docs/superpowers/plans/2026-08-30-after-phase-6.md` rev.2, which is the order
and now records Steps 1, 2 and 3 as done; `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md`
**rev.3** whole — §2's M-3 for the decision, the M-2 section for the nine decisions Step 3 was built
on and the three subsections recording what building it settled, §3 for P1 and P5, §5 for M4 and M8;
then in the 1.0 spec `2026-08-27-engramux-1.0-design.md`, §5.7 for how the index is built and what
`00002` cost, and §8's Phase 4 row for the shape a retrieval gate takes here. Last, the header
comments of `internal/store/migrations/00002_events_fts.sql` and `00004_memory_items.sql`, and
`internal/memory/memory.go`.

**Written 2026-09-02, at the end of session 09.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | The Step 3 merge plus this file. **Nothing is pushed**: `main` is 15 commits ahead of `origin/main` and the owner was not asked before the session ended. `git status -sb` is the answer |
| Installed | The Step 3 build, through `engramux install --apply` twice — the second carrying the `cwd` fix. Migration `00004` applied. `doctor` exit 0, every section green, both hosts' 11 hook entries pointing at the installed relay |
| Last full verification | On the merged code: `go test -p 1 -count=1 ./...` **17 packages ok** · pinned linter `0 issues.`, **exit 0** · `./scripts/race.sh` **17 packages, no report, exit 0** |
| Backlog | **Two rows.** **28**, a publication condition. **36**, raised by Step 3's first live install: a memory item's title is often the wrong line |
| Gates | **M1** and **M2** pass and run in the normal suite. **M3 skips**, because its fixture is the owner's and does not exist yet — §3 below |
| The live service | 303 memory items over 81 files, re-read every five minutes with an mtime and size short-circuit. Its log carries three M2 warnings per pass, which are the three shapes the reading predicted and are not faults |

---

## 2. What to do, in order

**T1 — Read the list above.** Then `git status -sb`; if the tree is not clean, find out why before
anything else. Nothing here writes to `main` directly: Step 4 is **a branch per plan step, merged
with `--no-ff`**, and the name is `step-4-derived-fields`.

**T2 — Settle what a derived field is, and what it is not, before writing one.** M-3's whole load is
the distinction: *a derived field exists to find a document; a summary exists to answer instead of
one.* The spec names the candidates — touched paths, commands and exit codes, error spans, tool name,
success flag, session, timestamp — and does not decide which of them earn a column. That is this
session's first decision and it is measurable: **M4 compares P1's three new classes at recall@10 and
MRR with the boost on and off**, so a field that moves neither is deleted rather than kept.

Two things are already settled and are not to be reopened. **Step 3 and Step 4 do not share an FTS
rebuild** (memory spec rev.3, M-2 decision 7): M-3's fields are base-table columns and a ranking
input, not indexed text, so this step is its own migration and pays no rebuild. And the memory index
is a **second** FTS table over `memory_items`; whatever Step 4 adds has to say whether it applies to
one index, the other, or both, because the two are ranked separately and their scores do not cross.

**T3 — Build, one decision at a time.** For every test that guards an invariant: write it, watch it
fail, implement, watch it pass, break the implementation on purpose, watch it fail, revert. One
commit per decision. **Commit before every break-it pass**: `git checkout -- <file>` restores HEAD,
and it silently deleted uncommitted work twice in session 09 before the lesson stuck.

**T4 — Verify, install, merge.** Suite, the pinned linter (check its exit code, never its summary
line), `./scripts/race.sh`, in that order and not concurrently — the race script takes about nine
minutes wall clock cold and needs a background run rather than a foreground timeout. Build both
binaries with the commands in `AGENTS.md`. The reinstall is the owner's hands, run in the session
with the `!` prefix: `schtasks /end /tn Engramux` with `MSYS_NO_PATHCONV=1`, then wait until
`engramux status` stops answering, then `dist/engramux.exe install --apply`. Then merge `--no-ff`.
**Push only after asking.**

**T5 — Close.** The plan's Step 4 gets a dated "Done" paragraph with the evidence; any backlog row a
test now owns is deleted; a session 11 brief is written in this directory.

---

## 3. What only the owner can do, and it is owed

**Gate M3 has no fixture.** It is 25 queries per host whose answer exists in only that host,
human-labelled, and it lives at `.capture/m3/queries.tsv` — outside the repository, never committed,
and the gate skips with the format printed when it is absent. Three tab-separated columns: the host,
the query, and a run of text that is in the answering item and in **none** of the other host's. That
second half the gate checks rather than trusts.

Two rules for choosing an answer, both of which the gate will otherwise fail you for: it must not be
a path or a credential, because what is compared is the *masked* body; and it must be text rather
than an id, because an id is derived from the file's path and rots when the file moves.

It was verified once against a fixture generated from the corpus, then deleted because a generated
fixture is not a labelled one: **claude-code 1 of 1 over 38 items, codex 11 of 11 over 265**. Whether
25 per host is reachable at all against 38 Claude Code items is the open question, and the gate
reports the population rather than requiring the number.

---

## 4. Decided in session 09, and not to be reopened

- **Both hosts' native memory stays switched off** on this machine. None of M1, M2 or M3 requires a
  live format, and the snapshot carries better drift material than a well-formed live directory. What
  that costs is written down: whether Codex commits each consolidation stays `[unverified]`, and a
  format change in a future host release is invisible here.
- **One item is one block the host's format delimits**, and one whole file where it does not.
- **Collection is the drain's ticker** with an mtime and size short-circuit, five minutes. A watcher
  was rejected on a measurement: Claude Code's memory directory is a subdirectory of the transcript
  directory.
- **`memory_items` kept its name and lost its schema** in `00004`. Its foreign key to `projects` sets
  null rather than cascading, and it is the only one that does.
- **Scoping compares `project_path`**, not the foreign key, and asks about both forms a project can be
  filed under.
- **One `search` returns both lists**, ranked separately and never merged, and `get_memory` is the
  fifth tool. The 1.0 spec rows that counted four are named in the memory spec.

---

## 5. What session 09 measured, so you do not re-measure it

- **303 items over 81 files**, 38 Claude Code and 265 Codex, 127 with a host timestamp and 240 with a
  project. The largest body is 20,156 B and the largest masked body is the same, so `MaxMemoryBodyBytes`
  at 128 KiB is 6.6× the largest measured.
- **The index descriptions are not the notes' own**: median similarity 0.14 over 18 entries, identical
  on none. That is why an index entry is a document rather than navigation.
- **The whole memory corpus is about 950 KB**, so a full re-read is milliseconds and the interval is
  sized against how long a note may sit unsearchable rather than against the cost of a pass.
- **Four defects the real corpus found and no synthesised fixture did**, all four now owned by a test:
  a heading is not unique within a file; the parser stripped the word the credential rule matches on;
  a URL scheme parsed as a field label; and a rollout summary's sections did not inherit their file's
  `cwd`, which filed 92 items under no project.

---

## 6. Things that will bite

1. **`git checkout -- <file>` restores HEAD, not the working tree.** Commit before every break-it
   pass. It ate uncommitted work twice in session 09.
2. **A build failure is a discarded mutation, not a killed one.** Removing the last reference to an
   import makes a mutation that never runs; change the answer instead — `n > 99` rather than deleting
   the branch.
3. **A test can be fake and only a break-it pass says so.** The memory hit's masking test searched for
   a literal user name the body carried, while the field it was named for carried the machine's *real*
   one. Sweep a marshalled document with `secret.Detect`, which is what §8's Phase 5 clause does.
4. **A backtick in a Go string breaks a `python -c` inside `$( )`.** Write the mutation to a script
   file. Assert it applied before reading the result — session 09's did, and caught one that had not.
5. **A relative path in a test resolves against its own package**, not the repository root. The first
   M3 fixture landed in `internal/search/.capture/`, where nothing looks.
6. **The service's collector runs on its own goroutine and `start()` does not gate it.** A test that
   asserts straight after starting the service is racing it, and wins until the pass gets slower.
7. **A live MCP client caches the reply schema.** A session open across an upgrade rejects a `search`
   reply that carries memory hits, until it reconnects. The service logs nothing, because it is the
   client validating.
8. **`schtasks /end` then `install --apply` back to back leaves nothing running.** Wait for `status`
   to stop answering; `AGENTS.md` has the row.

---

## 7. Open, and what to do about each

| Open | What to do |
|---|---|
| M3's fixture | The owner's, §3 above. Step 4 does not wait on it |
| Backlog 36, a memory item's title | Which line should be the title is a decision no test can make. Cheap to fix once decided |
| Whether Codex commits each consolidation | `[unverified]`. One live consolidation settles it; the collection method works either way |
| Whether `search` should drop its output schema | Priced in the memory spec rev.3 and left typed. The next revision that grows the reply is choosing again |
| Every start rewrites all 303 rows | The short-circuit is per-process, so a restart re-reads everything. Cheap at this size and nobody has decided it is worth a persisted stamp |
| `origin/step-2-engramux-install` | Merged; deleting it is the owner's remote change |
| 15 unpushed commits | The owner was not asked before the session ended |

---

## 8. Done when

Step 4's decisions are a memory spec revision cited by the plan; gate **M4** passes *or* the code it
measures is deleted, which is the same outcome by that gate's own terms; suite, pinned linter and race
script are green; the build is installed and `doctor` is green; the branch is merged `--no-ff`; the
plan says Step 4 is done with the evidence; and a session 11 brief exists.
