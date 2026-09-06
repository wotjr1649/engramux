# Session 23 — Engramux: what the signal could not reach

Session 22 took the one step backlog 53 could take without a schema change, and the answer is worth
more than the step. Gate **M12** asked whether a ranking term keyed on *where the match fell* is a
different instrument from the event-class weight M11 had just rejected. It is — and it is powerless
in the exact class that rejected M11.

**Then a grilling round at the end of the session found three things that were stale, and one of
them is the gate the owner's own goal hangs on.** §2 is that, §3 is the order it produced, and both
were settled with the owner in the session rather than inferred. **Read §2 and §3 before anything
else**; the measurement that gave the document its title is §4 and it can wait.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

After §2 and §3, read memory spec §5's **M12 section** — the subset argument, the two tables and the
paragraph titled *What the run says* — then `docs/superpowers/backlog.md`'s row **53** with the two
paragraphs now standing above it. Session 22's work order is superseded by this document.

**Written 2026-09-06, by session 22, and amended by it the same day** — the amendment is §2, §3 and
the corrected rows of §1. A brief is a record and is not updated by a later session; this is the
session that wrote it finishing it before the hand-off, and it is marked so nobody reads it as the
other thing.

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `477c801` plus this amendment, one `--no-ff` merge on top of `f325162`; tree clean. **Twenty-seven commits are unpushed** — eighteen were already ahead when the session opened. `origin` is `github.com/wotjr1649/engramux` and **277 commits are already public there**, specs and session briefs included, so this batch is the same kind of disclosure rather than a new one |
| The push | **Not authorised, and the owner is reviewing the diff themselves first.** That is their answer and not a deferral: session 19 published this machine's account name in a fixture, so a scan somebody else ran is not evidence here. Session 22's scan, for what it is worth: 14 hits over 4,768 diff lines, every one a placeholder (`Users\<name>`, an already-redacted fixture path) or a synthetic test literal (`D:\work`, `C:\rollouts\one.jsonl`), and no real account name, SID or user path |
| Checks | Suite **exit 0, 21 packages**; pinned linter **`0 issues.` exit 0**; `scripts/race.sh` **exit 0, 21 packages, no data race** — in that order and not concurrently, all three on the merged tree |
| Installed | **`0.0.0-dev+60339da107a9`**, two merges behind `main`. Nothing was reinstalled and nothing needed to be: no shipped behaviour changed |
| Gates | M1–M6, M9, M10 pass. M11 ran and no weight ships. M12 ran and no schema change is licensed. **M7 is un-run** — 150 of 150 `TODO`. **M8 has no test and has never been reported**, and §2 says why that is the headline rather than a footnote |
| Publication | 1, 2 and 4 closed. **3 is open and is one paragraph**, not a document — see §2 |
| Backlog | **37, 38, 40, 46, 51, 52, 53** open. 53 is narrower than it was; nothing closed |
| Injection | Built, off, untouched |
| Decided by the owner, 2026-09-06 | The goal is **1.0 published to strangers**, not a personal tool with a public repo. **M7's 150 labels are the owner's own** and are not delegated. **Code signing is wanted** — and §2 has the answer to what that costs, which is not money |

---

## 2. Three things that were stale, and what they change

All three were read out of this repository rather than recalled, and each had survived at least one
hand-off in a compressed form that hid what was actually left.

**Publication condition 3 is not "write a README". `README.md` exists, is 240 lines, and has a
`Windows Defender will quarantine the CLI` section.** The spec's condition text still opens "There is
none", which is prose nobody updated. Of the three things rev.7 named that the README owes — the
detection string a user will actually see, that it fires on the CLI and not on the service, and the
exclusion steps for the two directories — **the first two are written and the third is explicitly
`[unverified]` and deliberately blank**, because `Add-MpPreference -ExclusionPath` was refused with
HRESULT `0xc0000142` and nobody has walked the Windows Security UI route and recorded it. So the
shorthand every previous brief used — "condition 3 is a human walking the Defender exclusion through
the UI" — was **a fair compression and not an error**. What it hid is that this is one paragraph of
an existing document, and that the two directories are already named: `dist\` under the checkout and
`%LOCALAPPDATA%\engramux\bin`.

**Gate M8 is what §1 makes publication wait on, and it has never been reported.** §1's goal sentence
is "published once the memory feature is native-grade or better", and §5 says in as many words that
**M8 is the honest form of that claim** — native memory's coverage of P1 and P5 against verbatim
retrieval's. The spec also says, at §5's own results table, *"M8 is not reported and nothing here
should be read as it"*. There is no test named for it and no fixture directory for it. **It did not
appear in session 22's own Gates row at all**, which is how a publication gate goes missing: every
brief listed the gates that had run. What it needs is the labelled questions of both capabilities,
and **P5's fixture does not exist at all** — P5 is *failure-fix pairs*, "querying with the text of a
failure returns the edit or command that resolved it", which is on native memory's documented
exclusion list and is exactly the kind of thing this product should be able to answer and native
cannot.

**There is no CI, and code signing is sequenced behind it rather than priced.** `.github/workflows`
does not exist. That matters now because the owner wants signing, and the route was already
researched and is `[verified] 2026-09-03` against Microsoft's own comparison of code signing options:
**Azure Artifact Signing** (~$10/month, no hardware token, CI-native) is closed by **geography** —
individual developers are limited to the USA and Canada — which removes the cheapest option before
cost is discussed; an **OV certificate** is $150–300/year worldwide and still carries the June 2023
FIPS 140-2 Level 2 hardware requirement plus, from March 2026, a 460-day maximum validity that makes
it a recurring chore; and **SignPath Foundation** signs qualifying open-source projects at OV level
**for free**, whose licence bar `LICENSE` already clears at Apache-2.0. SignPath's other two bars are
what is missing: the project must **already be released in the form to be signed**, and it must build
on a **trusted build system**. **So the answer to "how do I sign this" is "build a release process
and a CI, then apply, and pay nothing"** — and the delivery channel is decided too: a GitHub Release
as the substrate, a Claude Code plugin as the channel, one zip per release carrying its SHA-256, the
plan's Step 6.

**What the three together change.** Signing is *not* a publication condition — condition 4 closed on
documentation, and the spec is explicit that signing is not a switch that turns the detection off. So
CI and the release process, for all that they unblock, are **not on the critical path to
publication**. M8 is. And M8 cannot be started by the person who has to label it, because there is
nothing to label.

---

## 3. What to do, and in what order

**Take Phase A. It is the only work on the critical path that an agent can do**, and the reason is
the sentence above: every remaining publication gate that needs judgement is the owner's, and the one
thing standing between the owner and starting theirs is a fixture that does not exist.

| | Who | What |
|---|---|---|
| **A** | agent | **Settle whether M8's P1 half needs any new labels at all**, then build P5's fixture |
| **B** | owner | Four things, parallel, none blocking another: M7's 150 labels; M8's P5 labels; the Security UI walk for condition 3's last paragraph; backlog 52's one Codex turn |
| **C** | agent | M7 pass 2, and M8 reported. **This is where the native-grade verdict comes from** |
| **D** | agent, then owner | CI and the release process (Step 6) → the 1.0 release → apply to SignPath Foundation → signed releases from then on |

**Phase A in detail, because two of its three parts are easy to get wrong.**

*First, the cheap thing that might already be done.* P1 is exact-span recall, and gate M4 already
derives its queries **mechanically** — the longest token of a command line, a path's basename, the
longest token of a failing output line — over three classes with no human judgement anywhere in the
derivation. `internal/memory` and gate M3 already read this machine's native memory files. So M8's P1
half may be a matter of running the native side over M4's existing candidates and reporting the pair
of numbers, with **no new fixture and no labelling**. The spec's own wording points that way — it
says P5's fixture does not exist *at all*, singling out P5 — but this is `[unverified]` and it is the
first thing to check, because if it holds, **the first number ever produced for §1's publication gate
comes out of a session with no labelling in it.**

*Second, the fixture.* P5's candidates are failure-fix pairs cut from the corpus: an event whose tool
output reads like a failure, and a later event in the same session that edited or ran something to
resolve it. The pairing is a judgement, which is why it needs a label rather than a rule. It goes to
`.capture/m8/`, which **is never committed**, in M7's shape and under M7's discipline — and one part
of that discipline is load-bearing here: M7's fixture header says a label written with the answer
visible measures the answer. **The labeller must not be shown what native memory or the index
returns.** Build the pairs, hide the answers, hand it over.

*Third, pre-register what M8 reports before any number exists.* M8 is *reported* rather than gated,
so there is no bar to register — but there is a **rule**, and it is exactly the kind that gets tuned
to the answer if it is not fixed first: what counts as native memory being able to answer a question.
Write that down in its own commit, the way M11's condition and M12's were, before the harness can
produce a figure.

**Phase D's consequence, and it is an assumption this session made rather than a fact.** SignPath's
first bar is that the project is **already released in the form to be signed**, so the first release
is necessarily unsigned: **1.0 ships without a signature and 1.0.1 onwards carry one.** The README has
to say so. If that is wrong, D moves earlier — but moving it earlier means cutting a release before
the native-grade verdict exists, which is what §1 forbids, so the alternative is a pre-release that
is explicitly not the publication. **The owner has not ruled on this; it is written here so that it
is ruled on rather than inherited.**

**What is deliberately not next, and why.** Backlog 53's two remaining candidates, and 38 and 40, are
all real and all **off the critical path**. 53 in particular is tempting because its harness is warm
and an afternoon buys a measurement — but the ranking it would improve is not what publication is
waiting on. Take it when Phase B is with the owner and there is nothing on the path to do.

---

## 4. What session 22 measured, and why the answer is not just "no"

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

## 5. What is left, and what each one is blocked on

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

**46 is behind M7 and that has not moved.** `ReasonNoHits` covers three situations and
`ReasonTooBroad` absorbs a fourth, so the service log cannot tell recall from silence. Its row says
what a fix is. But the gate re-injects and M7 is un-run against a frozen snapshot, so changing
`internal/inject` means reproducing pass 2's figures through the `ENGRAMUX_M7_DIR` override first.

**38 and 40 are low value and are both a choice rather than a repair.** 38's margin is already gone
and what survives is a design question about whether its load constants were sized against a machine
rather than against the clause. 40's own row says there is nothing to repair — Go takes a duplicated
JSON key's last value and SQLite's `json_extract` takes the first, and JSON does not say which is
right. **Both rows say in as many words: do not fix it by moving the number.**

**51 is blocked on hardware nobody has.** It needs a volume with 8.3 name generation disabled *and*
an account name with a space *and* a Codex session that snapshots a shell. `doctor`'s `codex received`
line says nothing has arrived on such a machine. With publication as the goal this stops being
hypothetical, but it still cannot be measured here.

**52 is one Codex turn**, reading the failing `Stop hook` line for whose hook it names.

---

## 6. What must not move

**The M7 fixture.** `prompts.tsv` is 150 of 150 unlabelled and `.capture/m7/snapshot/` is what it is
keyed to. Do not delete the snapshot, do not re-run pass 1, and do not change `internal/inject`
without reproducing pass 2's figures through `ENGRAMUX_M7_DIR` first.

**M7's labelling is the owner's, decided 2026-09-06 and not a default.** The fixture header asks a
second-person question — *would **you** have wanted earlier context here* — so an agent answering it
changes what M7 measures, from the owner's own recall to a careful reader's judgement of a prompt.
The same rule binds M8's P5 labels for the same reason. **Do not offer to do either.**

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
`Users\<name>` before asking for a push — and the push is the owner's, who is reviewing this batch
themselves.

**The owner's memory files are not fixtures**, and neither are their prompts. M12's output is safe to
paste — measured, **0 of its 22 `-v` lines** carry a drive-letter path, and it logs class names,
counts and figures only — but that was a design condition rather than an accident, and it is the same
condition M11's arms are under. **M8's P5 fixture inherits it before it has a single row.**

**`~/.codex/hooks.json.manual-backup`** is a hand-made leftover from session 18. The user's to delete.

---

## 7. Things that will bite

1. **Pre-register the bar in its own commit, and then do not move it.** M12's condition is M11's, and
   it was committed before the gate could produce a number. The gain it declined was two replies of
   139 against one touched path of 25 — a trade small enough that whoever was looking at the number
   would have been tempted to make it. **M8 has no bar, because it is reported rather than gated —
   which makes its *rule* the thing to register first.**
2. **A pin on a verdict is not a pin on the measurement**, and M12 is the clean demonstration.
   Inverting its `machine-only` counter left `licensed: false` exactly where it was while moving
   every mechanism figure in both tables — 0 of 31 became 31 of 31 — and only the per-class row pins
   caught it.
3. **A stale sentence in a document outlives the thing it described, and a hand-off compresses it
   into something unfalsifiable.** All three of §2's findings had survived at least one brief. The
   guard is the one `AGENTS.md` already names for hosts and applies here to this repository's own
   documents: **read the artefact, not the summary of it.** `README.md` existing took one `ls`.
4. **A test-suite timeout names the test that happened to be running, not the one that is slow.**
   `panic: test timed out after 30m0s / running tests: TestX (5s)` is a package out of budget, and
   `TestX` is innocent. Sum the package's own time before suspecting anything in it.
5. **A splice that matches nothing reports success.** `AGENTS.md` carries the row. The short version:
   a bare `/tmp/x` from the file-write tool lands on the current *drive*, `sed`'s `r` silently
   ignores a file it cannot read, and a `perl` substitution whose literal carries an em dash quietly
   matches nothing. Verify the inserted lines, never the exit code. A `perl -0pi -e '…'` whose
   replacement contains an apostrophe or a backtick is worse than a no-op — it ends the shell quote.
   Rewrite the whole file with a file-write tool instead.
6. **The shell guard refuses a nested shell**, which includes `bash -n script.sh` and a `$( )` inside
   a compound command. Run a script as `./script.sh`; use `wc -l file` rather than assigning it.
7. **A skip guard written against the function under test skips exactly when that function is
   broken.** Ask the environment, or the files, directly.
8. **`git add -A` sweeps in whatever the session left lying about**, and `origin` is public. Write
   scratch files outside the repository.
9. **The harness's heredoc collapses a backslash even with a quoted delimiter**, and Python's own
   escapes collapse a second time. Anything with a backslash goes through a file-write tool.
10. **A long run of letters in a search fixture is a secret.** `internal/secret`'s opaque rule is 40
    or more of `[A-Za-z0-9+/]`, and an excerpt is cut from the *masked* document.
11. **`internal/search` is 145 s plain and 2,563 s under `-race`**, measured on the merged tree, and
    a full `scripts/race.sh` is about 45 minutes with 43 of them in that one package. It is not hung.
    When the next gate lands, re-measure and raise the script's budget rather than skipping a gate.
12. `go test` without `-p 1` is refused by a guard, on the command line and not in `GOFLAGS`.
13. **Do not paste a `TestPhase4Gate` corpus-mode run anywhere.** One line of it carries a real path.
14. **Four orphaned processes from 2026-08-31 survive `taskkill /F` unelevated** — same account,
    higher integrity. WINPIDs 8916, 6804, 17272 and 2776 as of 2026-09-05 23:37.

---

## 8. Done when

Whatever is taken has a test watched failing under a mutation that changes the answer; each behaviour
change is a branch named for its step, merged `--no-ff`; the suite, the pinned linter and the race
script are green in that order and not concurrently; and a session 24 brief exists.

**Four things stay one owner action away and none is an agent's to take.** The twenty-seven unpushed
commits need the owner's own review and then their authorisation. M7's 150 labels and M8's P5 labels
are theirs by decision. Condition 3's last paragraph needs the Defender exclusion walked through the
Security UI once and recorded. Backlog 52 needs one Codex turn with the failing `Stop hook` line read
for whose hook it names.

**One question is written down unanswered**, in §3: whether 1.0 shipping unsigned, with signatures
from 1.0.1, is acceptable. It follows from SignPath's own bars rather than from a preference, and it
is a public-facing promise, so it is the owner's to rule on rather than a session's to assume.
