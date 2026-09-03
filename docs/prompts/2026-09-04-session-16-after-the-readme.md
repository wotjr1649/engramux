# Session 16 — Engramux: M7 still waits, and the relay got heavy while nobody looked

Session 15 closed two of the four publication conditions' worth of work and could not run the one
gate the session was scoped around. It also found something larger than anything it was asked to do,
and did not fix it.

**The pattern worth carrying is the one session 14 established and session 15 repeated with a
better result.** Three adversarial reviews ran before a line was implemented, each on a different
axis, read-only. They killed the T4 design outright, corrected the T3 design in two places, and
found eleven false claims in the README's first draft. **Then a fourth review, run after the README
was written, found that five of its claims had been true when written and were falsified by this
same session's own code landing twenty-six minutes later.** That is the lesson: a status document
written mid-session is stale before the session ends, and the only defence is re-reading it against
the tree at the end rather than at the start.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context — including the
two carve-outs about the service and about `install --apply`.

Read, in this order: `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md`, and in it
*The second reading, over 150 prompts and per stratum* — which is new and contradicts the framing the
section above it was written under — then *What M7 will measure*, then **§8**'s publication
conditions. Then `docs/superpowers/backlog.md` rows **42** through **46**, which are all new, and
`README.md`, which is the first public-facing document this repository has ever had.

**Written 2026-09-04, by session 15.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | **12 commits ahead of `origin` and not pushed.** The owner was asked at the end of session 15 and said to keep it local: `README.md` is the repository's first public face and they want to read it first |
| Gates | M1–M6, M9, M10 pass. **M7 is built and still un-run**, waiting on 150 labels |
| Injection | Built, **off**, `MatchAll` unchanged. Untouched all session, deliberately |
| Publication | **Condition 2 closed.** Condition 3 has a `README` and **stays open** — it asks for the Defender exclusion steps and nobody has walked that route. 1 and 4 untouched |
| Suite | Green over 20 packages. Pinned linter `0 issues.` at **exit 0**. `scripts/race.sh` green, no data race. All three run in that order and not concurrently |

---

## 2. The one thing still waiting on the owner

**Label `.capture/m7/prompts.tsv`.** Still **150 of 150 `TODO`**, unchanged since session 14 wrote
it. Session 15 asked and was told the owner intended to label during that session; it did not
happen, and nothing about the fixture moved as a result — the file is byte-identical to how session
14 left it, and `.capture/m7/snapshot/` is intact.

Everything session 14's brief said about it still holds and is not restated here beyond the two
things that bite: **answer from the prompt alone**, and **do not delete the snapshot** — the fixture
is keyed to it, and re-taking it means re-labelling.

**One thing session 15 added.** The passes were exercised end to end against throwaway labels
through the harness's own `ENGRAMUX_M7_DIR` override, pointed at a scratch directory beside the same
snapshot. Pass 2 runs, reports, and writes a block file. So the machinery is known to work before
anyone's hours go into it, which is exactly what that override was built for. **The override is a
package-relative path** — `../../.capture/m7-scratch`, not `.capture/m7-scratch`; the first attempt
used the repository-relative form and the test skipped with a message that reads like a missing
file.

**A scratch directory is still on disk** at `.capture/m7-scratch/` and can be deleted. Session 15
tried and a recursive-delete guard refused; it was not worked around. Nothing is at risk —
`.capture/` is never committed — it is just clutter.

---

## 3. What to do, in order

**T1 — M7, if the labels exist.** Pass 2 writes `.capture/m7/blocks.tsv`; label those; run the gate.
**Change nothing about the injector while this runs.** If M7 goes green, **do not turn injection
on**: `inject.json`'s existence is the record of the user's consent and an agent writing it makes
that sentence false.

**One check that is free and worth doing first.** Session 15 measured pass 2 over all 150 prompts
and recorded the figures in the spec. A real run with the owner's labels must reproduce them exactly
— same injector, same snapshot, same 150 prompts, and pass 2 never reads a label. **28 injected,
122 abstained, 172 blocks, worst 254.03 ms, largest 4,944 B.** If those numbers move, something
changed that should not have, and finding out what matters more than the gate.

**T2 — backlog 42, and it is the largest thing on this list.** The relay binary links the SQLite
driver and goose, through `cmd/engramux`'s own `doctor.go` and `inject.go` importing
`internal/inject`. `dist/engramux.exe` is **8,703,488 B** against the **3,862,528 B** the 1.0 spec
§7.1 records twice. This is not tidiness: that binary is spawned once per hook event, which is what
made its size an invariant, and **two of the spec's recorded rejections are argued against the
figure that moved** — §5.9's rejection of an stdio proxy and `doctor`'s rejection of a `net/http`
probe at +93.7%. It is a design decision rather than a deletion: how does `doctor` report on
injection, and how is the `inject` CLI verb reached, without the reader half being in the relay's
dependency graph. Product change, so its own branch. **A test could own this** — a dependency
assertion over `go list -deps` — and none does.

**T3 — backlog 46, the abstention reasons.** `ReasonNoHits` covers at least three situations and
`ReasonTooBroad` absorbs a fourth. Splitting them makes M7's own `byReason` count the causes for
free, with no leak surface, where measuring from outside the package cannot — `candidates`,
`maxMatches` and `keepable` are all unexported, so an external replication measures a different
population. **Do this after T1, not before**: it changes a log line, and changing the injector while
the gate is being run makes pass 2 and the gate measure different treatments.

**T4 — backlog 43 and 44, the token copies nobody sweeps.** `mcpconf`'s temporary file holds a raw
token and has no equivalent of `internal/host`'s `staleTemps`; `internal/host`'s own backups
accumulate under timestamped names with no retention policy, in a directory people are asked to
attach to bug reports. **The count of those backups is `[unverified]`** — a credential-directory
guard refused the listing and it was not worked around. Whoever takes this needs the count first.

**T5 — push, after asking again.** Twelve commits. The owner declined once; that was about reading
the README, not a standing no.

---

## 4. Decided 2026-09-04 and not to be reopened without new evidence

**The injector's `MatchAll` stays**, and session 14's three reasons are unchanged. Session 15 adds a
fourth that is stronger than any of them: **latin prompts abstain most**, at 0.877 against hangul's
0.775 and mixed's 0.690. The script the corpus is written in is the script that abstains most, so
attributing this fixture's abstention rate to the language wall is unsupported by its own numbers.
The confound sits in the same table — a latin prompt is **394 runes at the median against hangul's
65** — and whether the figure measures script or length is open, because this fixture cannot vary
them independently.

**The replay-fidelity worry is settled for this fixture and stays a real limit.** **0 of 150**
prompts resolve to a project other than the one their event was stored under, so the live-filesystem
resolution the spec calls "not conservative" does not bite here. Carrying the stored `project_id` is
still the right fix; nothing measured now rests on it.

**Administrators is on `mcp.json`'s DACL**, where `internal/pipe`'s pattern has only SYSTEM and the
owner. A pipe's DACL dies with the process and this one outlives it, and this file is §5.9's only
documented rotation route. The memory spec §8 carries the argument.

**`GENERIC_ALL` and `FILE_ALL_ACCESS` are interchangeable through `ACLFromEntries`**, measured. An
adversarial review predicted the opposite and it is wrong on that path: `SetEntriesInAcl` maps a
generic mask before the ACE exists. A mutation swapping them is equivalent and must be **discarded
rather than counted as killed** — `GENERIC_READ` is the mutation that discriminates.

---

## 5. Open

| Open | What it decides |
|---|---|
| **Whether the abstention figure measures script or prompt length** | Needs the two varied independently, which this corpus cannot do. Not answerable by adding columns |
| **Whether each index hit its own ceiling** | Registered as reported and unreachable: `Result` carries neither total. The five `ReasonTooBroad` firings are an upper bound on ceiling firings, not a count. Backlog 46 |
| **Publication conditions 1, 3 and 4** | 1 needs a second local account; 4 needs 1; 3 needs somebody to walk the Windows Security UI and write down what happened. **None is an agent's** |
| **The `LICENSE` appendix and the copyright holder** | The placeholder is unfilled and no file in the repository records an owner. The README correctly does not invent one |
| **M8 and P5** | Unchanged |
| **M3's translation flaw** | Unchanged, `[unverified]` |

---

## 6. Things that will bite

1. **A status document goes stale inside its own session.** Five of the README's claims were true
   when written and false ninety minutes later, because this session's own merge closed the
   condition they described. Re-read anything you wrote about the state of the tree, against the
   tree, before you stop.
2. **`go test` without `-p 1` is refused by a guard**, and the guard is right. `-p 1` on the command
   line, not in `GOFLAGS`.
3. **A pipeline's exit code is the last command's.** `bash scripts/race.sh 2>&1 | tail -12` reports
   `tail`'s status and silently hides the first eight packages. Redirect to a file and read it.
4. **A mutation that does not change the answer is discarded, not counted.** Session 15 hit one and
   nearly recorded a fake test. The discriminating mutation is the one that changes the value the
   assertion reads.
5. **A break-it pass must not use `git checkout --` on a dirty tree** — it restores HEAD and deletes
   uncommitted work. Session 15 used file edits to revert instead.
6. **A backslash escape through a script literal is not safe.** Hit again in session 15, in Python
   heredocs writing `BUILTIN\Administrators` into a document. Verify the bytes afterwards; both
   times it landed correctly only because it was checked.
7. **`ENGRAMUX_M7_DIR` is package-relative.** See §2.
8. **Recursive delete and credential-directory listing are both guarded.** Two things session 15
   wanted were refused. Neither was worked around, and one measurement is `[unverified]` as a
   result. That is the correct outcome, not a workaround to find.
9. **`doctor`'s output is what people paste into public issues.** Any new line goes through
   `report.mask`, and `--full` turns that off — so a line that would carry a principal, a SID or a
   path must be a verdict rather than a value, with nothing to unmask.
10. **The corpus moves while you measure it.** Unchanged. The snapshot freezes the database and
    neither the clock nor project identity.

`AGENTS.md`'s own table carries the rest.

---

## 7. Done when

M7 has run against a labelled fixture, or is reported as still waiting with the count outstanding;
backlog 42 is closed or is a written decision about how `doctor` reaches injection; the suite, the
pinned linter and the race script are green in that order; the plan says what closed with the
evidence; and a session 17 brief exists.
