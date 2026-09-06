# Session 23 — Engramux: what the signal could not reach

Session 22 took the one step backlog 53 could take without a schema change, and the answer is worth
more than the step. Gate **M12** asked whether a ranking term keyed on *where the match fell* is a
different instrument from the event-class weight M11 had just rejected. It is — and it is powerless
in the exact class that rejected M11.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read, in this order: memory spec §5's **M12 section** — the subset argument, the two tables and the
paragraph titled *What the run says* — then `docs/superpowers/backlog.md`'s row **53** with the two
paragraphs now standing above it. Session 22's work order is superseded by this document.

**Written 2026-09-06, by session 22.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `4fcdf34`, one `--no-ff` merge on top of `f325162`; tree clean. **Twenty-six commits are unpushed** — eighteen were already ahead when this session opened. `origin` is public and a push is a fresh ask |
| The leak scan | All twenty-six were scanned: **14 hits over 4,768 diff lines, and every one is a placeholder or a synthetic literal** — `Users\<name>` in prose, an already-redacted fixture path, `D:\work` and `C:\rollouts\one.jsonl` in table-driven tests. No real account name, SID or user path. **Read them yourself before asking for a push**; a scan somebody else ran is not evidence |
| Checks | Suite **exit 0, 21 packages**; pinned linter **`0 issues.` exit 0**; `scripts/race.sh` **exit 0, 21 packages, no data race** — in that order and not concurrently, all three on the merged tree |
| Installed | **`0.0.0-dev+60339da107a9`**, which is two merges behind `main`. Nothing was reinstalled and nothing needed to be: no shipped behaviour changed |
| Gates | M1–M6, M9, M10 pass. **M7 is un-run**: `prompts.tsv` is 150 of 150 `TODO`. M11 ran and no weight ships. **M12 ran and no schema change is licensed** |
| Publication | 1, 2 and 4 closed. **3 stays open** and is a human walking the Defender exclusion through the Windows Security UI |
| Backlog | **37, 38, 40, 46, 51, 52, 53** open. 53 is narrower than it was; nothing closed |
| Injection | Built, off, untouched |

---

## 2. What session 22 measured, and why the answer is not just "no"

M11's term keys on the **document** — `events.event_name IN (…)`, the column form of "this one
carries a prompt or a reply". So it lifts a `UserPromptSubmit` whether the query matched the
person's words or the `cwd` sitting beside them. A term keyed on **where the match fell** lifts only
the first, and that is what backlog 53's second-FTS-column candidate would put in the index.

The two rules can differ only on the 160 documents that carry both halves, and only on the queries
whose match falls in the machine half of one. **They do differ, and by a lot.** Over 534 command
lines the location rule recovers **eleven of the sixteen** M11 lost, and 310 of 530 of the documents
lifted past them matched outside their own human text.

**And on `a touched path` it recovers nothing, because there is nothing there to recover.** That
class's `machine-only` count is **0 of 31** sampled and **0 of 130** over every candidate: every
human-text document that M11's weight lifted above a touched-path target matched *inside its own
prompt or reply*. A person typed that file name. So the sampled arm's single lost document was never
a boundary artefact and its cause is a property of the class.

**The sentence the two gates now force together, and it constrains every remaining candidate.** A
file name is the one thing a person and a tool both write, and when a prompt contains a path the
prompt genuinely is about that path — so **no rule separating human text from machine text can
separate a document that is about a path from another document that is also about it.**

The condition was M11's own, unchanged, and it was committed in `f36b3c5` before the gate existed.
It is not met. What M12 removes from row 53 is one candidate, not the row.

**And the run found one thing nobody was looking for.** `scripts/race.sh` had passed `-timeout 30m`
since it was written, on the estimate that "`-race` adds 5-15x". It is a measurement now:
`internal/search` alone is **2,563 s, or 43 minutes**, and exits 0. The first race run of the session
died at exactly 30m0s naming `TestGateTheSearchDoesNotReadPayloadsItDoesNotReturn`, five seconds into
it — which reads exactly like that test hanging rather than like the package being out of time. The
budget is 90m now and the script's comment carries the figure.

---

## 3. What is left, and what each one is blocked on

**53's two remaining candidates, neither measured.** *How much of the document the query accounts
for* — a path in a two-line prompt is a far larger share of it than the same path in a 40 KB tool
output, and unlike provenance that is a property of the *pair* rather than of the document, which is
the one shape neither M11 nor M12 has tried. And *reserving places in the visible list* rather than
reordering it, which demotes nothing and therefore has no harm arm of M11's shape at all — but read
the harm classes' own weight-0 recall first, **17, 13 and 19 of 25**: those targets are themselves
often outside the top ten, so a reserved slot evicts marginal ones and the harm has to be measured
rather than argued away.

**The harness measures either one in an afternoon, and that is not a guess.** M12 reused M11's five
classes, populations, anchors and sample without touching them; what it added was one `[]string`
through three unexported functions and one test file. M12 is 38 s and the M11 pair is another 50 s.
`search.SearchAtHumanWeight` and `search.SearchAtHumanTextMatch` in `export_test.go` are the
template, and **`Search` passes 0 and nil**, so the shipped ranking is exactly what it was.

**What a new candidate has to beat is a number and not an argument.** `machine-only` is the exact
size of what a location signal was worth: 85 of 519 over the sampled classes, 330 of 760 over every
harm candidate. A candidate that cannot say what it would have moved is not ready to be built.

**46 has a precondition and it has not moved.** `ReasonNoHits` covers three situations and
`ReasonTooBroad` absorbs a fourth, so the service log cannot tell recall from silence. Its row says
what a fix is. But the gate re-injects and **M7 is un-run against a frozen snapshot**, so changing
`internal/inject` means reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first.

**M7 itself is the largest open thing in the project, and it may not be an agent's to do.** 150 of
150 prompts unlabelled, and the judgement is "was this injected block relevant" over 150 prompts of
the owner's private text. **Ask before starting.** Nothing about injection ships until it clears its
threshold.

**38 and 40 are low value.** 38's margin is already gone and what survives is a design question
about whether its load constants were sized against a machine rather than against the clause. 40's
own row says there is nothing to repair, only a choice, and a test already pins the divergence.

---

## 4. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through `ENGRAMUX_M7_DIR` first.

**Neither ranking term reaches the injector, and now there are two of them.** `Search` passes 0 and
nil; the only callers that do otherwise are aliases in `export_test.go`. `internal/inject` calls the
same function and M7 is measured against today's ranking, so a future session shipping a ranking
term makes that decision again explicitly.

**The id set is a measuring instrument and cannot become an implementation.** Knowing it means
reading every matching payload outside the query, which is §7.1's four-second shape twice over.
`orderExpr`'s doc comment carries the reason; `TestTheHumanIDSetReachesTheStatement` is what says the
predicate reaches the statement at all, since no figure in either table could tell a term that never
arrived from one that arrived and changed nothing.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed**, and `origin` is public. `git log -p` your own diff for
`Users\<name>` before asking for a push.

**The owner's memory files are not fixtures**, and neither are their prompts. M12's output is safe to
paste — measured, **0 of its 22 `-v` lines** carry a drive-letter path, and it logs class names,
counts and figures only — but that was a design condition rather than an accident, and it is the same
condition M11's arms are under.

**`~/.codex/hooks.json.manual-backup`** is a hand-made leftover from session 18. The user's to delete.

---

## 5. Things that will bite

1. **Pre-register the bar in its own commit, and then do not move it.** M12's condition is M11's, and
   it was committed before the gate could produce a number. The gain it declined was two replies of
   139 against one touched path of 25 — a trade small enough that whoever was looking at the number
   would have been tempted to make it.
2. **A pin on a verdict is not a pin on the measurement**, and M12 is the clean demonstration.
   Inverting its `machine-only` counter left `licensed: false` exactly where it was while moving
   every mechanism figure in both tables — 0 of 31 became 31 of 31 — and only the per-class row pins
   caught it.
3. **A test-suite timeout names the test that happened to be running, not the one that is slow.**
   `panic: test timed out after 30m0s / running tests: TestX (5s)` is a package out of budget, and
   `TestX` is innocent. Sum the package's own time before suspecting anything in it.
4. **A splice that matches nothing reports success.** `AGENTS.md` now carries the row. The short
   version: a bare `/tmp/x` from the file-write tool lands on the current *drive*, `sed`'s `r`
   silently ignores a file it cannot read, and a `perl` substitution whose literal carries an em dash
   quietly matches nothing. Verify the inserted lines, never the exit code. A `perl -0pi -e '…'`
   whose replacement text contains an apostrophe or a backtick is worse than a no-op — it ends the
   shell quote. Rewrite the whole file with a file-write tool instead.
5. **The shell guard refuses a nested shell**, which includes `bash -n script.sh` and a `$( )` inside
   a compound command. Run a script as `./script.sh`; check a file's line count with `wc -l file`
   rather than assigning it.
6. **A skip guard written against the function under test skips exactly when that function is
   broken.** Ask the environment, or the files, directly.
7. **`git add -A` sweeps in whatever the session left lying about**, and `origin` is public. Write
   scratch files outside the repository.
8. **The harness's heredoc collapses a backslash even with a quoted delimiter**, and Python's own
   escapes collapse a second time. Anything with a backslash goes through a file-write tool.
9. **A long run of letters in a search fixture is a secret.** `internal/secret`'s opaque rule is 40 or
   more of `[A-Za-z0-9+/]`, and an excerpt is cut from the *masked* document.
10. **`internal/search` is 145 s plain and 2,563 s under `-race`**, measured on the merged tree, and
    a full `scripts/race.sh` is about 45 minutes with 43 of them in that one package. It is not hung.
    When the next gate lands, re-measure and raise the script's budget rather than skipping a gate.
11. `go test` without `-p 1` is refused by a guard, on the command line and not in `GOFLAGS`.
12. **Do not paste a `TestPhase4Gate` corpus-mode run anywhere.** One line of it carries a real path.
13. **Four orphaned processes from 2026-08-31 survive `taskkill /F` unelevated** — same account,
    higher integrity. WINPIDs 8916, 6804, 17272 and 2776 as of 2026-09-05 23:37.

---

## 6. Done when

Whatever is taken has a test watched failing under a mutation that changes the answer; each behaviour
change is a branch named for its step, merged `--no-ff`; the suite, the pinned linter and the race
script are green in that order and not concurrently; and a session 24 brief exists.

**Three things stay one user action away and none is an agent's to take.** The twenty-six unpushed
commits need a fresh authorisation. Publication condition 3 needs the Defender exclusion walked
through the Security UI. Backlog 52 needs one Codex turn with the failing `Stop hook` line read for
whose hook it names.
