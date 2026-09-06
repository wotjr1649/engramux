# Session 24 — Engramux: the half that needed no labels

Session 23 took Phase A, and the inference session 22 marked `[unverified]` held. **Gate M8's P1
half needs no label anywhere**, so §1's publication gate has its first number ever — produced by a
session with no labelling in it. The pair is stark and the reason it is stark is not the one that
was registered, which is §2.

**Phase B is now entirely with the owner and nothing on the critical path is an agent's.** That is
not a stall: it is what Phase A was for. §3 says what to do with a session that opens into it.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read the memory spec's **What M8 will measure** — the rule, then *P1's half, measured 2026-09-06* —
before anything else. It is the section session 23 wrote and it is where the numbers below live.
Session 23's work order is superseded by this document.

**Written 2026-09-06 by session 23.** A brief is a record and is not updated by a later session.

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `2c188af`, tree clean. **Thirty commits are unpushed**; three are session 23's and the other twenty-seven were already ahead. `origin` is `github.com/wotjr1649/engramux` and is public |
| The push | **Not authorised, and the owner is reviewing the diff themselves first.** Unchanged from session 23, and their answer rather than a deferral. Session 23's own three commits add 676 lines and were scanned on their own: **0 hits** for a drive-letter path, a `Users\` fragment, a SID or an email address. That is a scan of three commits and not of the thirty |
| Checks | Suite **exit 0, 21 packages**; pinned linter **`0 issues.` exit 0**; `scripts/race.sh` **exit 0, 21 packages, 0 data races** — in that order, not concurrently, all three on the final tree. The first two were run twice: once before the last commit and once after it, because a fix landed after the first pass and a check on a superseded tree is not evidence. **In the race run two packages re-ran and the rest came from the content-addressed cache** — `internal/search` at **2,110.8 s** and `internal/inject` at 94.9 s. A cached `ok` under `-race` is a result for that package's exact inputs, and nothing outside `internal/search`'s own test files changed, so the cached rows hold on this tree; they are not a fresh execution and are written down as what they are |
| Installed | **`0.0.0-dev+60339da107a9`**, unchanged. Nothing was reinstalled and nothing needed to be: **no shipped behaviour changed this session** — every line is a test file or a document |
| Gates | M1–M6, M9, M10 pass. M11 ran and no weight ships. M12 ran and no schema change is licensed. **M8's P1 half is reported** — §2. **M8's P5 half is un-run and its fixture now exists**, 94 failures and 281 candidate rows, every label `TODO`. **M7 is un-run** — 150 of 150 `TODO` |
| Publication | 1, 2 and 4 closed. **3 is open and is one paragraph** — the Defender exclusion walked through the Windows Security UI once and recorded |
| Backlog | **37, 38, 40, 46, 51, 52, 53** open. Nothing closed and nothing opened |
| Injection | Built, off, untouched |
| Open question, unanswered | Whether 1.0 ships unsigned with signatures from 1.0.1. It follows from SignPath's own bars, it is a public-facing promise, and session 22 wrote it down so it is ruled on rather than inherited. **Still unruled** |

---

## 2. What Phase A settled, and the one thing it did not build

**The cheap thing was already done, and it holds.** M8's P1 half needs no new label. Its questions
are gate M4's own three classes over M4's own candidates and its own 25-per-class sample, derived
mechanically; the native side is gate M3's collector over this machine's 303 memory items. So the
first figure for §1's publication gate came out of one run:

| Class | Candidates | Verbatim answers | Native answers |
|---|---|---|---|
| a command line | 534 | 25 of 25, **1.000** | 4 of 25, **0.160** |
| a touched path | 120 | 25 of 25, **1.000** | 1 of 25, **0.040** |
| an error message | 96 | 25 of 25, **1.000** | 11 of 25, **0.440** |

**The rule was committed before the number existed**, in `3836756`, and it predicted the verbatim
side would read *near* 1.00 because a prefix-phrase match already implies containment. It came out
**1.000 three times**. That is the pre-registration working, and it is why the row is reported
without being the interesting one.

**What the run added, and it is the sentence to carry forward.** Of the 75 sampled queries, **18
returned any native item at all**, and **16 of those 18 carried the literal**. So the native side is
near-tautological *conditional on matching*, the way the verbatim side is unconditionally. **M8's P1
pair therefore measures what each index holds, not how either ranks** — the event index holds every
literal by construction and native memory holds between 4% and 44% of them. That was not predicted
and the spec records it rather than the tidier reading.

**The 11× spread across the classes is the shape §3 predicted.** Native covers error messages best
and touched paths worst: a person writing a note records what went wrong in prose they wrote, and
does not record which file a tool opened.

**M-1's reopen condition is checked and not met.** Verbatim reaches all three classes at 1.000, so
there is no class of question it cannot reach. The summariser stays closed, now for a second reason.

**The P5 fixture exists and the labels are the owner's.** `.capture/m8/pairs.tsv`, written by
`TestWriteM8Pairs`: 94 failures anchored of 901 documents, 281 candidate rows, three candidates per
failure, every label `TODO`. Nothing in that file is a search result — the pairs come from a
mechanical rule and never from a ranking, which is M7's discipline inherited before the fixture had
a row.

**What Phase A deliberately did not build is P5's measuring half.** Its labels do not exist, and a
measurement that cannot be run cannot be watched failing under a mutation either — which is the
`[unverified]` claim §8 forbids shipping. What stops it drifting in the meantime is that **its rule
is already committed**: the query, the answer literal, the population, and what leaves the
population are all in the spec, written while the fixture was empty. Phase C writes the code against
a rule it did not choose.

**The verdict is not registered and that is the owner's decision.** M8 reports a pair; §1's
*"native-grade or better"* is a reading of that pair. It is the one place §7's warning about a rule
tuned to its answer still applies, and it is left open on purpose.

---

## 3. What to do, and in what order

| | Who | What |
|---|---|---|
| **B** | owner | Four things, parallel, none blocking another: M7's 150 labels; M8's 281 P5 labels; the Security UI walk for condition 3; backlog 52's one Codex turn |
| **C** | agent | M7 pass 2, and M8's P5 half against the committed rule. **This is where the native-grade verdict's second number comes from** |
| **D** | agent, then owner | CI and the release process → the 1.0 release → SignPath Foundation → signed releases |

**If you open into Phase B with no labels yet, take backlog 53 and not Phase C.** That is session
23's own reading of session 22's order and it has not changed: 53's two remaining candidates are
real, off the critical path, and the harness measures either in an afternoon. *How much of the
document the query accounts for* is the one shape neither M11 nor M12 has tried — it is a property
of the **pair** rather than of the document. *Reserving places in the visible list* demotes nothing
and so has no harm arm of M11's shape, but read the harm classes' own weight-0 recall first — 17, 13
and 19 of 25 — because those targets are often outside the top ten already and a reserved slot
evicts marginal ones.

**What a new candidate has to beat is a number.** `machine-only` is the exact size of what a location
signal was worth: 85 of 519 sampled, 330 of 760 over every harm candidate. A candidate that cannot
say what it would have moved is not ready to be built.

**Do not offer to do Phase B.** M7's labels and M8's P5 labels are the owner's by decision of
2026-09-06 — the fixtures ask a second-person question and an agent answering one changes what the
gate measures. The push waits on their own review. Condition 3 needs a human at the Windows Security
UI. Backlog 52 needs one Codex turn.

---

## 4. What must not move

**M8's rule.** Committed in `3836756` before any figure existed, and `62e0324` produced a figure
under it. Phase C implements P5 against that rule; if it turns out to be wrong, that is a spec
change made deliberately and recorded as one, not a harness that quietly measures something else.

**The M7 fixture.** 150 of 150 unlabelled, keyed to `.capture/m7/snapshot/`. Do not delete the
snapshot, do not re-run pass 1, and do not change `internal/inject` without reproducing pass 2's
figures through `ENGRAMUX_M7_DIR` first.

**The M8 fixture.** `.capture/m8/pairs.tsv` is what the owner is labelling. Regenerating it renames
nothing and rewrites everything: the rows are keyed by corpus file name, so a regeneration over the
same corpus is stable — but a regeneration **while it is half-labelled discards the labels**, because
`TestWriteM8Pairs` writes `TODO` in every row. Do not run it again once a label exists.

**Neither ranking term reaches the injector.** `Search` passes 0 and nil; the only callers that do
otherwise are aliases in `export_test.go`.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed**, and `origin` is public.

**The owner's memory files are not fixtures**, and neither are their prompts. M8's P1 gate output is
safe to paste — measured, **0 of its 8 log lines and 0 of the 12 a `-v` run emits** carry a literal,
a body or a path, and it logs class names, counts and figures only. That was a design condition and
the file's doc comment says so.

**`~/.codex/hooks.json.manual-backup`** is a hand-made leftover from session 18. The user's to delete.

---

## 5. Things that will bite

1. **A fixture cut from tool output is arbitrary bytes, and `strings.Fields` is not a filter.** The
   first `.capture/m8/pairs.tsv` carried **63 control characters over two of its 281 rows, 36 of them
   NUL** — NUL is not Unicode whitespace, so it survived masking and flattening both. The file was
   valid UTF-8, and **`grep` answered `Binary file … matches` while `grep -c` went on counting
   correctly**, so the two disagreed and the fixture read as malformed when it was the tool declining
   to print it. `m8Field` and its test own the fix; what no test owns is the diagnostic — **when a
   `grep` pipeline and a `grep -c` over one file disagree, suspect the file's bytes before the
   data.** `python`, `od -c` and `awk -F'\t'` all read it correctly throughout.
2. **`corpusDocs` returns a name sort and a name sort is not a time sort.** A corpus file is named
   `host__Event__nanos__pid.json`, so `os.ReadDir` groups a host's `PostToolUse` together and puts
   every `PermissionRequest` before them. Anything reasoning about *a later event in the same
   session* off that order pairs a failure with something that happened before it. The nanosecond
   field is the ordering key; `events.received_at` cannot do it, because for a corpus replayed into
   a temporary database that is the moment the test ran. `m8Pairs` and its first test case own this
   for M8 only — the next thing to read the corpus in order has to know it too.
3. **Stopping a backgrounded `scripts/race.sh` leaves exactly one `*.test.exe` alive.** Observed
   twice in one session, both times a single child of the package that was running. `ps -W` finds it
   and `taskkill /F /PID <winpid>` ends it. Left alone it contends with the next run and holds a
   temp directory. This is AGENTS.md's row about a backgrounded loop surviving its wrapper, in a
   second shape.
4. **`taskkill //F` is not `taskkill /F`.** AGENTS.md's MSYS row offers `MSYS_NO_PATHCONV=1` *or* a
   doubled slash, and the doubled form works for a subcommand word like `//query` and **not for a
   single-letter flag**: `//F` reached taskkill as `//F` and was refused as an invalid argument.
   `MSYS_NO_PATHCONV=1` with a single slash is the form that works for both.
5. **A mutation that removes the only reference to a local does not compile, and reads like a test
   that does not care.** Hit again: mutating `m8Pairs`'s session *grouping* left `sid` unused and the
   mutant was discarded rather than killed. Mutating the *lookup* instead changed the answer and the
   test caught it. AGENTS.md carries the row; this is the second session to meet it.
6. **A check on a superseded tree is not evidence.** The suite and the linter passed, a fix landed
   after them, and both had to be re-run. If a run finishes and the tree then changes, the run
   measured something that no longer exists — the ordering rule in *Done when* is about the tree as
   much as about the sequence.
7. **`internal/search` is 151.8 s plain**, up from 145 s: M8's P1 gate adds about 3.5 s and its two
   unit tests are milliseconds. **Under `-race` it was 2,110.8 s this run against the 2,563 s session
   22 measured the same day** — 18% apart, with *more* tests in the later run. Do not read that as
   the gate making the package faster: nothing about the two runs was controlled except the tree, and
   the machine's own state is the free variable. What it does mean is that a `-race` figure is a
   range and not a constant, so the 90 m budget is sized against the worse of the two and neither
   reading is the number to plan a schedule on.
8. **Do not paste a `TestPhase4Gate` corpus-mode run anywhere.** One line of it carries a real path.
9. `go test` without `-p 1` is refused by a guard, on the command line and not in `GOFLAGS`.
10. **Four orphaned processes from 2026-08-31 survive `taskkill /F` unelevated** — same account,
    higher integrity. WINPIDs 8916, 6804, 17272 and 2776 as of 2026-09-05 23:37.

---

## 6. Done when

Whatever is taken has a test watched failing under a mutation that **changes the answer** rather than
removing a reference; each behaviour change is a branch named for its step, merged `--no-ff`; the
suite, the pinned linter and the race script are green in that order, not concurrently, **and on the
tree that is being handed over**; and a session 25 brief exists.

**Four things stay one owner action away and none is an agent's to take.** The thirty unpushed
commits need the owner's own review and authorisation. M7's 150 labels and M8's 281 P5 labels are
theirs by decision. Condition 3 needs the Defender exclusion walked through the Security UI once and
recorded. Backlog 52 needs one Codex turn.

**One question is still written down unanswered**: whether 1.0 ships unsigned with signatures from
1.0.1. It is the owner's to rule on.
