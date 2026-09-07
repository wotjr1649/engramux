# Session 27 — Engramux: the packaging half, and the hash the owner commits

Step 6's local half is built, verified against the real installation, and merged `--no-ff`. What is
left of Step 6 is a tag and a release page, and both are publication acts that need the owner's own
hands.

**Two corrections, written in by the session that wrote this document** — session 25's own precedent,
marked so nobody reads it as a later session editing a record. Both were found by a grilling pass
after this was committed, and both are the same failure: a claim taken from a document rather than
from the thing the document describes.

**This sentence said the merge was this repository's first, and it is the twenty-fourth.** `git log
--merges --oneline | wc -l` answers 24, eighteen of them already on `origin`, the earliest
`42d102a` on 2026-09-02. The claim came from `AGENTS.md`'s branch-policy paragraph, which said the
repository had none — true when it was written on 2026-08-30, stale three days later, and repeated
as fact by session 26's hand-off and by this document before anyone checked. `AGENTS.md` is
corrected, and the rule that would have caught it is written into *How we work* rather than left as
this session's lesson.

**§3 said the plan's Steps 7 and 8 were available and blocked by nothing. Both were done on
2026-09-04**, on `step-7-selector` and `step-8-window-cost`. There is **no unstarted step in the
plan** — Steps 1 through 8 are done except Step 6's publication half, which is a publication act.
What that changes about §3 is its conclusion rather than its shape: the work available to an agent
is the backlog and row 53's reserved slot, and nothing else.

One decision in it was not an agent's to make and was put to the owner rather than assumed. It is
§2, it changed the shape of the release process, and it is why the archive had to become
reproducible.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context.

Read the memory spec's **The delivery channel, and what it costs Codex (M-7)** before anything else,
and the plan's **Step 6** after it. Session 26's work order is superseded by this document.

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Tree clean, and it now carries a merge commit. **The unpushed batch is over fifty commits.** No count is named here and that is session 23's correction inherited rather than a style: a document that states its own depth is wrong the instant it is committed. **`git log --oneline origin/main..main` is the answer that stays true.** `origin` is public |
| The push | **Still not authorised, and the owner is reviewing the diff themselves first.** Unchanged since session 22, and their answer rather than a deferral. This session's commits were scanned for a drive-letter path, a `Users\` fragment, a SID and the owner's email address: **0 hits.** That is a scan of this session's range and not of the batch; `git log -p origin/main..main` is the one that covers it and it is the owner's to read |
| Checks | Suite **exit 0, 22 packages** — `scripts/mkzip` is the twenty-second — `internal/search` at 217.7 s; pinned linter **`0 issues.` exit 0**, checked on the exit code and never on the summary line; `scripts/race.sh` **exit 0, 22 packages, 0 data races**, `internal/search` at **3807.7 s, 63.5 minutes of the 90-minute per-binary guard**, the whole run **69m13s**. That is 335 s above session 26's reading with **no gate added** and the ordinary run flat at 217.7 s against 218.3 s, so all of it is the machine - and this run had a known confound, documents edited and one `go run` started while it went. It is recorded in the script as a third point on the spread rather than as a new figure. In that order, not concurrently, on the merged tree. **What was committed after them changed no `.go` file** — measured, `git status` names none: two documents, the backlog, one shell invocation in each workflow, one refusal added to `scripts/package.sh`, and eleven comment lines in `scripts/race.sh` recording that run's own measurement, verified as 11 insertions of which 0 are executable |
| Installed | A development build of the merge commit, put back by `scripts/reinstall.sh` at the end of the session. Between those two states the machine ran the **release archive itself**: `update --from` the unpacked archive replaced both binaries and restarted the service, `doctor` reported `0.1.0-test` installed and running, and the cache line flipped from *is newer* to *not newer*. That is AGENTS.md's stop-and-start carve-out used and restarted in the same turn |
| Gates | Unmoved. M1–M6, M9, M10 pass. M11–M14 ran and nothing is licensed to ship. M8's P1 half is reported; **its P5 half is un-run and its fixture carries 281 `TODO` rows**. **M7 is un-run** — 150 of 150 `TODO`. Verified again 2026-09-07 |
| Publication | 1, 2 and 4 closed. **3 is open and is one paragraph** — the Defender exclusion walked through the Windows Security UI once and recorded. The README's signing sentence that the 2026-09-07 ruling owed it is now written |
| Backlog | **37, 38, 40, 46, 51, 52, 53** open, and **54 opened** by this session: every Claude path this product derives ignores `CLAUDE_CONFIG_DIR`, the plugin cache added here inherits that rather than introducing it, and the fix is one shared resolution rather than one path at a time. Nothing closed |
| Injection | Built, off, untouched |
| The release | **The process exists and the release does not.** `git tag` is empty, there is no release page and no archive on one. The catalogue names version `0.0.0`, which is the no-release state written down rather than a placeholder nobody explained |

---

## 2. What this session settled, and the one thing that was the owner's

**The decision.** M-7 says a tag "updates the marketplace entry in the commit that carries the
version", and that sentence has two readings which differ on **authority** rather than on mechanism.
One has the workflow commit the archive's hash to `main` after the release, which is standing write
access to a public repository's default branch and a commit the owner never reviewed. The other has
the **owner** run the packaging locally, commit the version, the URL and the hash together, and tag
that commit; the workflow re-builds and **refuses the release when the hash it gets is not the one
the tagged commit already names**. Put to the owner, who chose the second. It is recorded in the
memory spec, because that is where decisions live.

**What it costs, and it is the interesting part.** The second reading only works if two builds of one
commit agree byte for byte. Three things make them, and one is not guessable: the archive is written
by `scripts/mkzip`, which sorts its entries and stamps every one with the zip epoch rather than with
the file's real modification time; the manifest carries **no `version` field**, so no rewrite at
packaging time can put a `jq` version into the archive's bytes; and the build line adds
**`-buildvcs=false`**. Without that flag the binary records `vcs.revision`, and the release order is
build, then commit the hash, then tag — so the owner's build would carry the parent commit and the
workflow's would carry the tag. **Measured**: the same package with the flag and without reports 0
and 3 `vcs.` lines and two different hashes. The archive itself hashed identically across seven runs
and three commits, one of them on a dirty tree.

**What was measured off the installed host rather than read in a reference**, because a document
about another product is not evidence about the binary on this machine. The `archive` marketplace
source and its optional `sha256`. The plugin cache layout, `<cache>/<marketplace>/<plugin>/<version>`,
read out of the cache this machine already has. That `strict: false` is required only of an entry
declaring a `headersHelper`, which ours does not. And the update signal's precedence — plugin
manifest, else marketplace entry, else digest — which is what licensed the manifest carrying no
version at all. Both manifests pass `claude plugin validate`, with one warning naming exactly that
absent field.

**What `doctor` says now.** Three versions rather than two. The third is the newest version in
Claude Code's plugin cache that has **both** binaries in it, because the line names `update --from`
and that command replaces the pair — a half-removed old version would otherwise be reported as an
available upgrade, and a cache that keeps versions for a fortnight will eventually hold one. It was
read off the real installation three times, and the third is the one that matters. With the release
archive unpacked into a cache-shaped directory it names the version and the exact argument; against
the real cache it says there is nothing there; and after `update --from` that same directory, it
says `0.1.0-test, installed and running agree` and *not newer than what is installed*. So the link
-time version survived the archive, and the comparison flipped on a real machine rather than in a
fixture.

**Eight break-it mutations, all killed, none `NOOP` and none `BUILD`.** Every one changes an answer
rather than removing a reference — the numeric ordering inverted, a release no longer outranking its
pre-release, the both-binaries condition dropped, the leading-zero check dropped, equal counting as
newer, the archive taking its timestamp off the clock, the entry order reversed, and an empty
directory accepted.

---

## 3. What to do, and in what order

| | Who | What |
|---|---|---|
| **B** | owner | Four things, parallel, none blocking another: M7's 150 labels; M8's 281 P5 labels; the Security UI walk for condition 3; backlog 52's one Codex turn |
| **C** | agent | M7 pass 2, and M8's P5 half against the committed rule. **This is where the native-grade verdict's second number comes from**, and it cannot start before B |
| **D** | owner | What is left is the tag, the release page and the archive on it. The SignPath submission comes after a release exists, which is that bar's own first requirement |

**Phase B is verified unmoved on 2026-09-07** — `.capture/m7`'s fixture is 150 of 150 `TODO` and
`.capture/m8/pairs.tsv` is 281 of 281 — so **Phase C is still blocked and is not available to take.**

**The release procedure, in the order it has to happen, because nothing else writes it down as a
sequence.** Run the three checks locally on a machine that has `.capture/`, because M-7's tagging
rule says a tag is pushed only after the gates that skip on a runner have been seen green here. Then
`scripts/package.sh <version>`, which builds, checks the binary's own reported version, writes the
archive under `dist/` and rewrites `.claude-plugin/marketplace.json`. Review that diff, commit it,
tag that commit `v<version>`, and push the tag. The workflow does the rest and refuses a mismatch.

**Ruled by the owner on 2026-09-07, in a grilling pass after this document was first committed, and
written in by the session that wrote it.** Four answers, and they order everything above:

1. **The owner's queue comes first and no agent session runs until the batch is pushed.** Every
   agent session grows what has to be reviewed, and the push is the only one of the five that
   unblocks an agent at all — it is also the only one whose cost grows with waiting.
2. **The review stays what session 22 decided**: read in full, pushed once. Not delegated, not
   staged.
3. **Publication condition 3 closes before the first release**, not after. The condition is that a
   stranger is told in advance, and a release is the moment strangers can meet the binary. The cost
   is accepted: SignPath needs a release to exist, so signing moves back by however long the
   Security UI walk takes.
4. **M7's 150 labels before M8's 281.** Smaller, and finishing them hands half of Phase C to an
   agent.

So the order is: this correction commit, then the owner's review and push, then M7's labels, the
Defender walk and backlog 52 in any order, then 0.1.0. **What a session 28 takes is answerable only
once the push has happened** — it depends on whether the labels are done by then — so it is
deliberately not fixed here.

Nothing on the critical path
is an agent's while Phase B is open. What is available if the owner wants it: row 53's reserved-slot
candidate, which stays unmeasured and unassigned, and backlog **38**, **40**, **51** and the **54**
this session opened. **Not the plan**: this paragraph named Steps 7 and 8 and both were done on
2026-09-04, which the correction at the top of this document covers.
**54 is the one to weigh against the others rather than to take on sight** — it is read from the
code and nobody has met it, so its priority is a guess about how many users move a configuration
home, which is exactly the kind of number this repository normally refuses to act on without.
**Backlog 46 is still
disqualified** for the reason it was — it changes `internal/inject`, which is the package M7 is about
to measure.

**Do not offer to do Phase B.** M7's labels and M8's P5 labels are the owner's by decision of
2026-09-06: the fixtures ask a second-person question, and an agent answering one changes what the
gate measures. The push waits on their own review. Condition 3 needs a human at the Windows Security
UI. Backlog 52 needs one Codex turn.

---

## 4. What must not move

**The reproducibility chain, and it is four things rather than one.** `scripts/mkzip`'s sorted
entries and epoch timestamps; `-trimpath`, `CGO_ENABLED=0` and `-buildvcs=false` on the release
build; the toolchain pinned by `go.mod`; and the manifest carrying no `version` field. Break any one
and the release workflow refuses what the owner tagged, with a message about two hashes and no
explanation of which of the four moved. `scripts/package.sh` owns the build line — **do not hand-type
a release build**, and AGENTS.md's row says so beside the command.

**The catalogue's `0.0.0`.** It is not a placeholder awaiting a fill-in; it is "there is no release"
in the field's own vocabulary, and it sorts below every real version. `scripts/package.sh` is what
changes it.

**`plugin.json` declares no hooks and no MCP server**, and that is M-7's own rule rather than an
omission. A plugin's hooks are live only while the plugin is enabled, so a plugin that configured
capture would let somebody stop capture by disabling a plugin — the failure this product exists not
to have. `install` and `register` stay the only writers of host configuration.

**M14's and M13's conditions and pins, and both ladders.** Inherited from session 26 unchanged: every
count is pinned, a corpus that grows makes them wrong, and the answer is to re-measure and correct
the figures rather than to relax a comparison. A rung added to either ladder now is chosen from an
answer already seen.

**The M7 fixture and the M8 fixture.** 150 of 150 and 281 of 281 unlabelled. `TestWriteM8Pairs`
writes `TODO` in every row, so a regeneration while it is half-labelled discards the labels. Do not
re-run pass 1, do not delete `.capture/m7/snapshot/`, and do not change `internal/inject` without
reproducing pass 2's figures through `ENGRAMUX_M7_DIR` first.

**No ranking term reaches the injector or any caller.** `Search` still passes 0 and nil.

**Injection stays off.** `inject.json`'s existence is the record of the user's consent.

**`.capture/` is never committed**, and `origin` is public.

**`~/.codex/hooks.json.manual-backup`** is a hand-made leftover from session 18. Still the user's to
delete.

---

## 5. Things that will bite

1. **`jq` on Windows writes its stdout in text mode.** Measured: the catalogue it produced came back
   with `{\r\n` as its first line. `.gitattributes` normalises the blob on `add`, so the commit is
   right and only the working copy is wrong, which is why nobody notices — and a mixed tree makes an
   end-of-line-anchored pattern behave differently on its two halves. `scripts/package.sh` pipes
   through `tr -d '\r'` for exactly this. Anything else that writes JSON through `jq` needs the same.
2. **gosec reports G304 and G703 on the same line, and suppressing one reveals the other.** A
   `#nosec G304` on an `os.Open` with a variable path silences that rule and the next run reports
   G703 at the same column, which reads like the suppression did not work. `#nosec G304 G703` is the
   form. Cost: one wasted lint cycle.
3. **`doctor` masks the plugin cache path, so the `update --from` command it prints is not one you
   can paste.** That is M-6's trade working as designed — a real cache path carries the user name —
   and `--full` is the answer. It looked like a defect on this machine only because the directory
   used to verify it was under `D:\AI_DEV\` and had no user name in it to mask.
4. **`reportService` returns early when the service is not answering, so all three version lines are
   skipped on a machine whose service is down.** Pre-existing and not touched here. If the version
   section is ever wanted independently of the pipe, it has to move out of that section.
5. **The archive's hash is a function of the whole commit**, not of the version string. `README.md`
   and `LICENSE` are in it, so a documentation commit between packaging and tagging changes the hash
   and the workflow will refuse the release. Package last, commit, tag, push.
6. **`claude plugin validate` warns that no version is declared, and that warning is expected.** It
   is the cost of §2's third reproducibility condition. A future session that "fixes" it by adding
   the field to the manifest breaks the release workflow, and the failure will look like a
   determinism bug rather than like this.
7. **Neither workflow has ever run.** `[unverified]`: the runner image lists `gcc 15.2.0` under its
   tools, read from actions/runner-images' own Windows readme, but whether it is on `PATH` where
   `scripts/race.sh` looks has not been observed. If it is not, the race step fails loudly with the
   script's own message and the fix is one `ENGRAMUX_CC` line. `actionlint` v1.7.7 is clean over both
   files at exit 0, which is syntax and expressions and not behaviour.
8. **A wall-clock figure from one run is not comparable to one from another run on this machine**,
   and `scripts/race.sh`'s 90 minutes is a per-test-binary hang guard rather than a budget for the
   run. Both inherited from session 26 and both still true.
9. **`.capture/`-dependent gates skip silently without it.** A green run on a machine with no corpus
   has measured nothing, and the skip is the only thing that says so.
10. **`go test` without `-p 1` is refused by a guard**, on the command line and not in `GOFLAGS`. So
    is a recursive `rm`, which is why `scripts/package.sh` stages into a fresh `mktemp -d` under
    `dist/` and cleans up nothing.

---

## 6. Done when

Whatever is taken has a test watched failing under a mutation that **changes the answer** rather than
removing a reference; each behaviour change is a branch named for its step, merged `--no-ff`; the
suite, the pinned linter and the race script are green in that order, not concurrently, **and on the
tree that is being handed over**; and a session 28 brief exists.

**Five things stay one owner action away and none is an agent's to take.** The unpushed batch needs
the owner's own review and authorisation. M7's 150 labels and M8's 281 P5 labels are theirs by
decision. Condition 3 needs the Defender exclusion walked through the Security UI once and recorded.
Backlog 52 needs one Codex turn. And the first release — the tag, the page, the archive — is a
publication act, which is the thing this session built everything else up to and deliberately did
not do.
