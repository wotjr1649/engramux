# Backlog

Deferred findings that no test owns yet. Each was triaged as non-blocking by the review that
raised it; none blocks a merge. This file owns **only** the carry list — it decides nothing. The
spec owns decisions, invariants, budgets and measurements; a plan owns execution order.

Extracted from `.superpowers/sdd/2026-08-28-session-02-search/progress.md` before that gitignored
ledger was deleted, which is the whole reason this file exists: the triage lived on one machine.

**When a test starts catching an item, delete the row.** The test is the better owner. Same rule
AGENTS.md applies to its own "What will bite you" table.

## Carried from Phase 1-4

**Every row of this section is closed**, the last four — 6, 9, 16 and 17 — in Step 1's build on
2026-09-02, each with a test that fails when its fix is undone. The paragraphs below are what the
closures taught and stay for that; the rows themselves are gone by this file's own rule.

**13, 14, 21, 10, 15, 1, 2, 12 and 19 closed in the second soak-window pass.** 13's mutation is the
one worth carrying: the depth guard's `return ""` was changed to return what the walk had already
collected, and **exactly one test in the suite went red** — the new one. Nothing else could see a
partial walk, which is what the row said. 14 is closed by measurement rather than by a fix: 21 shapes
where `encoding/json` and `json_valid` could plausibly disagree — lone surrogates, invalid UTF-8, a
raw NUL, numbers past float64, a byte-order mark — and they agree on all 21, 15 valid and 6 invalid.
`TestTheTwoWalksAgreeOnWhatIsValid` holds it and would go red on a driver that introduced one. 21 was
closed by the product searching its own corpus: the Codex clamp warning's text was recovered from a
2026-08-29 capture and is now in §7.1, path-stripped. **10 and 15 needed nothing** — both reasonings
were already written at their narrowest scope, in `TestTheTokenizerReadsBothIllFormedShapesTheSameWay`
and on `goJSONDepthLimit`, so the rows were duplicating the code rather than deferring anything. 1, 2,
12 and 19 were done in the first pass and their rows outlived them by one commit.

**3, 4, 7, 20, 22 and 23 closed during the Phase 6 soak**, none of them touching a shipped `.go`
file. 3, the pipe-name assertion now names `ENGRAMUX_TEST_PIPE_SID` and reports whether it is set,
never either name — both are derived from a real SID. 20, the `EVENTS` table is validated once at
module load rather than at each read, because `matcher` and `codexTimeout` had the same hole; a
string row now throws instead of installing a hook with the matcher silently deleted. 23, the Claude
Code side sweeps all eleven events, which a lowered `TIMEOUT_SECONDS` fails on all eleven. 4 and 7
are `AGENTS.md`; 22 is a stale parenthetical.

**8 is withdrawn, not fixed.** Its number was a misreading: 158 documents carry a Hangul-stem
particle token somewhere and 136 carry one before any Latin-stem token, and the 22 between those is
`deriveParticle` returning the first match rather than the trim dropping anything. Measured over the
901 captures, the ASCII-only trim changes not one token and the class holds 162 candidates either
way. The trim was widened anyway, for consistency with `atTokenStart` and not for a number;
`deriveParticle` and `particleStemShapes` carry the measurement.

## Pre-existing defects confirmed by the 2026-08-29 adversarial review

Raised while reviewing the Phase 5 design; each is a property of code that already shipped, not
of that design. Rows 25 and 26 were closed by Phase 5 and are gone; the numbering is not
renumbered, because a row's number is how the sessions that discussed it refer to it.

Four more closed in the Phase 6 pre-soak build and are gone the same way. **29**, `events.id`
reaching a reader unmasked: `getEvent` and `searchEvents` now mask it, and
`TestPhase6AnEventIdThatCarriesAUserPathIsMasked` holds both halves — a secret-shaped id is
rewritten and a real UUIDv7 is not, so a hit's id still round-trips to `get_event`. **24**, the
unimplemented 512 KiB field cap: withdrawn from spec §6 rather than implemented, on §7.4's own
measurement, and `ipc.MaxFrameLen`'s justification rewritten from the same numbers. **5**, the
unwrapped error in `ListenCurrent`. **11**, the depth check that also ran after a pop.

A third section stood here, "Phase 5 prerequisites this review surfaced" — the masked status and
list-sessions replies, `get_event`'s measured bound, the `(id, project_id)` pair, the trust boundary
on a caller-supplied path, and the bound on the single connection. Every one is now a test, so by
this file's own rule the list is gone. Spec 8's Phase 5 row names the tests that own them.

**27, 30, 31, 32 and 33 closed in Step 1's build on 2026-09-02**, and with them **34**, the soak's
covering index, and **35**, the installer's re-registration, both filed after this section was
written. Each closed with a test that fails when its fix is undone; spec §5.2, §5.6, §5.9 and §7.1
carry what they changed. **28 closed on 2026-09-04** and this section is now empty: `mcp.json` is
written with a protected DACL of its own, and `doctor` reports the two host files' permissions as a
finding rather than changing them. Memory spec §8's second publication condition carries what the
build settled, including the two things it reversed - Administrators is on the DACL where
`internal/pipe`'s pattern has only SYSTEM and the owner, and the GENERIC_ALL/FILE_ALL_ACCESS
distinction turned out not to bite through `ACLFromEntries`. Four tests own it:
`TestWriteNarrowsTheFileItPublishes`, `TestRestrictWritesTheExactDACL`,
`TestRestrictLeavesTheFileReadable` and `TestPermissionsNamesNoPrincipal`, each watched failing
under a mutation that changes the answer rather than removing a reference.

## Raised by Step 3's first live install, 2026-09-02

**36 closed on 2026-09-05**, on a branch with backlog 48's display half - they are the title and
the body of the same hit, and row 48 said so. A title is now the first line that is not a label this
reader recognises. The set is closed and measured rather than listed, on spec 6.1's precedent:
`Outcome`, `Rollout context` and `Reusable knowledge` open a line of 55 of the 55 rollout summaries
on this machine, `Preference signals`, `References` and `Key steps` of 54, and nothing else
capitalised reaches more than one file. Claude Code's 21 notes carry one candidate, `Related` on 5,
which is below anything that reads as a format and is left out.

**The Codex call site moved with it, and that was not in the row.** `codexBlock` replaces each
recognised field label with its bare value before the body is indexed, so a title taken from the body
was the thread id - measured, a UUID on all 55 header blocks. The title is chosen from the block as
it was written instead, which makes the rule literally true of the file rather than of what survived
the parser. `TestFirstLineSkipsARecognisedLabel`, `TestASummarysTitleIsNeitherItsLabelNorItsThreadID`
and `TestNoItemOnThisMachineIsTitledWithALabel` own it - the last over the corpus the set came from,
with a vacuity guard that asks the files and not the function under test.

| # | Where | What |
|---|---|---|
| 37 | The two shipped binaries, and memory spec §8 | **Windows Defender quarantines the CLI on behaviour, and this is a publication condition rather than one machine's problem.** Measured 2026-09-03 00:16:03 on the owner's machine: `engramux.exe` was removed from the build output directory and from the install directory as `Behavior:Win32/Execution.A!ml`, severity 5, `DidThreatExecute` False - blocked before it ran, so nothing was compromised. `Behavior:` and `!ml` are the whole finding: it is a behavioural machine-learning detection on executing a freshly built, rare, unsigned binary, not a signature on the bytes. The service binary was untouched and kept running. It is **not** the first: `Trojan:Win32/Commando.A!ml` fired on 2026-08-30 against the soak sampler's `schtasks /create`, so **two of the four detections this machine has ever recorded are Engramux doing exactly what it is designed to do** - run a new unsigned executable, and register a scheduled task. A stranger's first install is those same two shapes. What does *not* explain it is the `-s -w` strip in the build line: this is a behaviour detection and the flags are not implicated, so do not change them hoping it helps. `Add-MpPreference -ExclusionPath` was refused with HRESULT 0xc0000142 - unelevated, or Tamper Protection, which is what that feature is for - so an exclusion has to go through the Windows Security UI, by a human, and that is not a route an agent takes. The real fixes are code signing or a documented exclusion step in the install instructions. **Memory spec §8 answered that on 2026-09-03**, as its fourth publication condition and deliberately as an outcome rather than a mechanism: Microsoft's March 2024 SmartScreen change removed EV's instant bypass, so signing accumulates reputation across releases rather than switching the detection off, and a first release by a new publisher still has none. This row stays as the measurement that condition rests on. No test can own it: it is a property of an external control on the machine the binary lands on |

## Raised by Step 4's race run, 2026-09-03

| # | Where | What |
|---|---|---|
| 38 | `internal/service`'s `TestPhase5GateAReaderDoesNotPushIngestPastItsBudget` | **The margin this row was filed about is gone, and the row's headline outlived it.** Filed 2026-09-03 at `bd53297`, which carries none of Step 4: five runs of the gate alone under `-race` gave a slowest ingest of 692, 784, 852, 777 and 751 ms against spec 5.3's 800 ms - one of five over. Step 4's own `instr(lower(col), ?)` regression took it to 832-928 ms and the `LIKE ... ESCAPE` fix took it to 375-502 ms; **that fix is committed, and this row went on quoting the pre-fix figure as current for a day.** A session 14 brief propagated it, and correcting that is what produced the re-measurement. **Measured 2026-09-04 at HEAD**, the gate alone under `-race`, five runs with `-count=1`: worst **481, 422, 523, 509 and 463 ms**, median 6-7 ms, **none over**. So a red contention gate today is worth investigating rather than shrugging at, and the standing advice to re-run one before believing it is withdrawn. **Two questions the row raised survive the margin's disappearance and are what is still unowned**: whether the load constants were sized against a machine rather than against the clause, and whether asserting the relay's whole post-dial budget against the handler alone is the right reading - the test's own doc comment calls that "a deliberately stricter reading than the clause needs". Neither is answered by re-running the gate unchanged, which is why the 20-runs-per-arm sweep this row seemed to ask for was designed and then dropped: it can only address the third reading, `-race` multiplying a cost, and a 50x ratio at 5+5 runs already settles that one. **Do not fix either by moving the number.** Two traps for whoever does take it up: `go test` without `-count=1` replays a cached run and prints its `-v` log verbatim, so a loop collects one measurement and nineteen copies and reports zero variance; and the figure is a `t.Logf`, absent without `-v`, rendered with `%s` so it changes unit above one second - a parser matching only `ms` silently drops exactly the runs that went over |

## Raised by the pre-push review of Step 4, 2026-09-03

| # | Where | What |
|---|---|---|
| 40 | `internal/store`'s `Derive` and migration `00005`'s backfill | **A duplicated JSON key is resolved differently by the two walks.** Measured 2026-09-03: for `{"command":"first","command":"second"}`, Go's decoder takes the **last** and SQLite's `json_extract` takes the **first**. JSON itself does not say which is right, so neither side is wrong and there is nothing to repair - only a choice about which to adopt, and adopting SQLite's means giving `Derive` a token-stream walk like `Leaves` has rather than a `json.Unmarshal`. The cost is one pathological document's boost differing between the insert path and the backfill path; the derived columns are a ranking input that nothing selects. `Leaves` is unaffected and the reason is structural - a walk that emits every string leaf visits both values on both sides, where a walk that extracts one member has to choose. Not reachable from either host's encoder, which marshals from maps and structs. `TestTheTwoJSONParsersDivergeOnADuplicatedKey` pins it. **A row 39 stood beside this one for a few hours on 2026-09-03 and was withdrawn rather than fixed**: it reported the ill-formed-Unicode divergence between `Leaves` and `json_tree` as a new, unowned defect that decided what `events_fts` holds. Every part of that was already false when it was written - the 1.0 spec §7.1 has recorded the divergence since Phase 4, `TestLeavesCoercesWhatIsNotWellFormed` has pinned it over three shapes rather than two, and `TestTheTokenizerReadsBothIllFormedShapesTheSameWay` measured that all three spellings index the same two tokens, so on that side it **cannot change a search result**. What was genuinely new is one clause of this row's own subject: the same coercion now also lands in the derived columns, where `LIKE` rather than the tokenizer is the comparison, so the tokenizer measurement does not cover it. `TestDeriveCoercesIllFormedUnicodeWhereJSONExtractDoesNot` owns that, and by this file's own rule it is a test rather than a row |

## Raised by Step 4's first live search, 2026-09-03

**41 is closed by Step 5's build on 2026-09-03**, and by this file's own rule the row is gone: three
tests own it. `TestInvokesEngramux` is the decision itself, sixteen rows of which the last five are
the ones that matter - a command line that *mentions* this product is kept and one that *invokes* it
is dropped. `TestBuildExcludesThisProductsOwnCommands` holds the same pair through the selector, with
the kept row carrying the same words as the dropped one. And `TestTheSelfExclusionOverTheCorpus`
measures it over the captures: **0 of 216** command lines mentioning this product invoke it, which is
correct for a corpus taken before this product had a binary to run, and which a string match would
have answered 216.

What the row left open and the build did not close is the causal half it marked `[unverified]`:
whether the boost promoted that document or bm25 had it first. Step 5 makes it moot for injection -
the exclusion is in the selector and fires either way - and it stays unmeasured for search, where the
row already said it was not worth a harness of its own.


## Raised by session 15's adversarial review and by closing backlog 28, 2026-09-04

**43 and 44 closed on 2026-09-04**, each with a test watched failing under a mutation that changes
the answer rather than removing a reference. **43**: `mcpconf.Write` sweeps before it writes, and
its temporary file is renamed `mcp.json.engramux-tmp-*` so that the sweep has something unambiguous
to glob — `mcp.json.*` would also reach a copy the user made by hand, and a sweep that removes a
credential must not be able to remove anything else. `TestWriteSweepsWhatAKilledRunLeft` owns it;
the `.bak` neighbour in that test is what says the sweep is bounded by the infix, and it goes red on
the widened glob.

**44 is a bound and not a sweep**, which is the decision the row left open. A backup here is meant
to be recoverable — `Plan`'s comment describes the failure it exists for, and `install.go` prints
every path `Commit` returns so that a person can go and use one — so removing all of them would take
the remedy away with the exposure, which is the same trade that put Administrators on `mcp.json`'s
DACL. Three survive; the prune runs **before** the copy and keeps one fewer, so the copy a run is
about to take is not a candidate for its own prune and no ordering mistake can reach it. Ordering is
by modification time, because the RFC3339Nano stamp trims trailing zeros and a name sort is not
reliably chronological — `...-55-1Z` sorts after `...-55-12Z` — with the name as tie-break, because
several copies written inside one ~15.6 ms Windows file-time tick share a modification time exactly.
`host.Backups` answers a count and a time and cannot be asked for a name, and `doctor` prints it
beside the permissions line on the two branches where the file carries the token.
`TestBackupsAreBoundedAndTheNewestSurvive` is the one that matters: reversing the sort leaves three
copies and the *wrong* three, `[v0 v1 v5]`, which a count-only assertion passes.
`TestBackupsCountsNoneWithoutFailing`, `TestBackupsReportsTheCountItWasGiven`,
`TestBackupsIsSilentWhenThereAreNone` and `TestBackupsNamesNoFile` own the rest.

**The count on the owner's machine stayed `[unverified]`.** The same credential-directory guard
refused this session's listing and was not worked around, which is the reason the `doctor` line
exists rather than a gap in this note: the guard stops an *agent's shell* from expanding a glob in a
credential directory, because the expansion lands in an agent's context, and a product counting its
own files exposes nothing to an agent.

**45 closed on 2026-09-04**, on its own branch and merged `--no-ff` because it is a product string.
The message a bare `update` prints contradicted itself in two consecutive lines: the first said
there is no delivery channel to read instead, the second said to download the release archive and
unpack it. The row left open which way to resolve it - name the developer path, or say there is no
channel - and the owner chose to say only what is true, so the message now points at a directory the
reader already has, holding both binaries, and anticipates no channel at all. The three lines moved
into a named value because `warn` writes straight to `os.Stderr` with no seam and naming the string
is a smaller change than adding one.
`TestUpdateDoesNotSendTheReaderAfterAnArtefactThatDoesNotExist` owns it and asserts the pair - no
release-fetching verb, and the flag form named - because either half alone passes for the wrong
reason: a message naming no source is honest and useless, and one naming `--from` could still carry
the download line beside it. Both arms were watched failing under a mutation that changes the
answer. **The test is deleted rather than relaxed on the day a release exists**, since telling a
reader to download an archive is correct then and a test forbidding it would pin a fact that moved.

**42 closed on 2026-09-04**, on its own branch and merged `--no-ff`, and **ahead of gate M7 under
the row's own escape clause rather than in spite of it**. `internal/injectconf` is the switch file
and the two spec constants as a leaf - `encoding/json`, `fmt`, `os`, `path/filepath`, `time` and
nothing else - and `cmd/engramux` reaches it instead of `internal/inject`. The four switch symbols
are gone from `internal/inject` entirely, since nothing outside the relay used them; `MaxBytes` and
`Budget` are named again there, because that is where the spec's reader looks and where the gates
read them, and a name in two places is safe here only because the new test fails the moment the
second name is used to reach the leaf the long way round. Measured: fourteen banned packages to
**zero**, and the relay from **8,703,488 B to 4,817,408 B**. That is still not the 3,862,528 B §7.1
records - the remaining 954,880 B is growth this row was never about, and **the spec's figure stays
stale for a reason that is now separable from the driver**.

`TestTheRelayDoesNotLinkTheSQLiteDriver` is the part worth more than the split, as the row said. It
runs `go list -deps` rather than asserting a size, because a size assertion goes red on an
`ldflags` change and green on a driver arriving in a build that stripped more; and it names the
suspect import in its failure, so the answer is "this import did" instead of "something linked
SQLite". Watched failing at fourteen packages before the split, and again under a mutation that
changes the answer: **one blank import of `internal/inject` in `doctor.go` brings all fourteen
back**.

**Behaviour-neutrality was proved, not asserted.** Pass 2 ran over the frozen snapshot through
`ENGRAMUX_M7_DIR` before and after: 28 injections, largest 4,944 B, median 1,918 B, 122 abstained,
172 blocks, the three abstention reasons and all nine per-stratum figures identical line for line
with only the timing normalised out. The M7 fixture was not touched and is still 150 of 150
unlabelled. **Row 46 was deliberately left**: it changes a log line, and the same tripwire would
have to be re-run against it.

| # | Where | What |
|---|---|---|
| 46 | `internal/inject`'s abstention reasons | **`ReasonNoHits` covers at least three different situations and `ReasonTooBroad` absorbs a fourth, so the service log cannot tell recall from silence - which is the thing §6's fifth mitigation asks it to do.** Read 2026-09-04. One return point produces `ReasonNoHits`, reached when the event side and the memory side are both empty after filtering, and the event side empties three ways: nothing matched at all; the only match was the prompt's own event, removed by the exclusion; or everything that survived the exclusion was one of this product's own command lines, removed by `keepable`'s `InvokesEngramux` filter. The third is not in the spec's account of it. Worse, `broad` is set when **either** index exceeds the 200-document ceiling, so a run where the memory side was suppressed and the event side was emptied by the exclusion returns `ReasonTooBroad` - an exclusion reported as a selectivity ceiling. Session 14 read the reason one way, wrote it into the spec and corrected it the same day; the spec's corrected sentence still says the one surviving match *is* the prompt's own event, which was measured over 115 of 120 and not 120. **What a fix is**: split the constant, and return what `keepable` removed. That also makes gate M7's `byReason` count the causes for free and with no leak surface, where measuring it from outside the injector cannot - `candidates`, `maxMatches` and `keepable` are all unexported, so an out-of-package replication measures a different population. Product behaviour changes (a log line), so it wants its own branch. **Deliberately not done in session 15**, because the M7 fixture was being labelled and the gate re-injects: changing the injector mid-labelling makes pass 2 and the gate measure different treatments |

## Raised by the first install on a machine that had never run these binaries, 2026-09-04

**49 closed on 2026-09-05**, on its own branch and merged `--no-ff`. Spec 4.3 now reads
`transcript_path` first and the key rules only where it does not answer, and migration `00006`
re-judges every stored event against that order, moves the ones whose transcript names a different
host, and deletes the sessions it empties. **The fix is written as the rule and not as the symptom**
- `PreCompact` and `PostCompact` are two more cells the corpus has no Claude Code capture of and may
carry the same keys, so repairing only `SessionStart` would have left them. The payload is stored
verbatim and never rewritten (I-10), which is what makes a re-scan a computation rather than a
guess, and `00001`'s own CHECK comment says the schema was shaped for exactly this.

**Reordering was not a preference between two defensible rules.** Claude Code's `SessionStart` key
set is a strict subset of Codex's, so no ordering of key rules could have separated that cell; only
the value of `transcript_path` can. And the count could not have told anyone which order was right:
the path resolves for 900 of 902 captures and agrees with the host in all 900, so 4.3 scores
900/900 under either.

**Three counts written down as literals broke when the fifth fixture arrived, and that is the part
worth keeping.** `internal/spool`'s Phase 1 gate reserved ingest id 5 for its secret clause, which
became the new fixture's own id; `Ingest` ACKs a duplicate as committed (I-05), so the clause read
the fixture's row instead of its own and asserted the wrong `privacy_class` for an event it had
never written. `internal/store`'s desync test collided at the same number and was therefore
asserting nothing at all. Both now derive from the fixture count. A literal that happens to equal a
collection's size is a coupling nothing declares.

`TestTheCorpusCoverageIsWhatIsRecorded` is what outlives the defect: the corpus holds **13 of the 22
host x event cells**, it names the nine that are empty, and it reads the host out of
`transcript_path` rather than out of each capture's recorded label - because the first attempt to
find this defect compared detection against that label, came back clean, and proved nothing.
`TestDetectCorpusMeasurement` now counts the captures where the two halves disagree and asserts
zero, which is the assertion that would have caught this had the corpus held one Claude Code
`SessionStart`.

**47 closed on 2026-09-05**, on its own branch and merged `--no-ff`. `sessions` with no argument is
corpus-wide and `[project]` narrows it, matching `search`; spec 5.9 carries the decision and why the
"an existing CLI invocation must not change what it returns" clause is withdrawn for this command
and kept for its sibling. `ListSessionsReply.ProjectRoot` was singular and could not survive the
change, so **the root travels per session**: a listing spanning projects has no single one, and the
per-row field still does the job the reply-level one had. `TestListSessionsWithNoProjectIsCorpusWide`
in `internal/service` and `TestSessionsScopeDefaultsToEveryProject` in `cmd/engramux` are what own
it now — the first over the handler, the second over the argument, because a handler that answers
corpus-wide is not the same claim as a command that asks corpus-wide.

**48 closed on 2026-09-06**, on two branches merged `--no-ff`, and the ranking half closed on a
measurement rather than on a change. The display half closed the day before, beside backlog 36 -
*Raised by Step 3's first live install* carries it, because the title and the excerpt were one piece
of work.

**The complaint is real and larger than the row knew.** Memory spec 5's M11, over the 901-document
corpus: 82.2% of documents carry neither a prompt nor an assistant message, and a human-text
document is outside the top ten for **64.7% of prompts and 45.3% of replies** searched by their own
most distinctive word - with the top ten *entirely* plumbing in 11 of those 11 prompt cases and 58
of those 63 reply cases. The row was filed on one observation of six hits; the corpus says the shape
holds. A perfect down-weight would move 41.2% and 42.4% of queries from invisible to visible, which
is the exact ceiling and was computed without choosing a weight.

**And no weight ships.** The gate swept 0, 1, 2, 3, 4, 5, 20 and 100 over five classes - two gain,
and M4's three unchanged as the harm arm, whose targets are exactly what a weight buries. The
condition, registered before the gate existed, was: improves recall@10 in a gain class, regresses it
in none of the five. **`a touched path` loses one document of 25 at weight 1 and never recovers**, so
nothing in the sweep qualifies - and weight 5 would have bought 31 replies of 139 for that one
document. A supplementary run over every candidate rather than 25 confirmed the single document was
representative: the harm is monotone and reaches all three harm classes by weight 5.

**What that means, and it is the useful part.** The machinery is not in the way by mistake. The
document that ran the command genuinely contains the path, so lifting the prompt above it costs the
command line its own place. A uniform event-class weight is not the instrument for this. What might
be is a signal separating "this document is *about* the query" from "this document *contains* it",
which is a retrieval question and not a re-weighting one, and which this corpus cannot answer today.
That is a new question rather than this row, so this row closes rather than being reworded into it.

`TestGateM11PlumbingRarelyBuriesTheAnswer` and `TestGateM11TheWeightEarnsItsPlace` in
`internal/search` own both halves, each with every figure pinned rather than only its verdict.

## Raised by closing 50, 2026-09-05

**50 closed on 2026-09-05**, on two branches merged `--no-ff`, and neither of the two things this row
said before it was the cause. Not the document shape: the official reference shows `hooks.json` with
a top-level `hooks` member, and Codex's own `[hooks.state]` table in `~/.codex/config.toml` keys all
eleven events to that exact file with a trust hash each. Not the trust gate: the owner opened
`/hooks` and all eleven were trusted and enabled, with no startup warning.

**`CodexEntry` wrapped the relay path in quotes inside the value**, so the command line Codex
received was one quoted token from its first character - and Codex echoed it instead of running it.
The echo is what found it. `hook context: <the relay path>` appeared in the Codex TUI beside two
other plugins' real hook output, and a `hook context` line is a hook's own **stdout**; this relay
writes nothing on stdout on any event (spec 4.5), so something other than the relay produced that
line. Every hook reported `completed` while nothing ran, the spool stayed empty because the relay
was never there to spool, and `doctor` answered `11 of 11 events point at the installed relay`
throughout.

**Two measurements closed it, and the second is the one that matters.** A wrapper that spawned the
same binary with the path unquoted delivered a `codex UserPromptSubmit` at 19:46:25 - the first
Codex event this product has ever captured. That wrapper changed two things at once, so the entry
itself was then rewritten with only the quotes removed, and the second prompt landed at 19:51:45.
The first proves the relay, the pipe, the service and host detection were never the problem; only
the second attributes it to the quotes.

**Three candidates were declared dead on the strength of `completed`, and one of them was the
answer.** `completed` is exit 0 of whatever Codex spawned, and an echo exits 0. The line carrying
the truth was in the same paste and was read as a label rather than as output. What generalises:
when a host reports success for a hook that captured nothing, the hook's **stdout** is evidence
about what actually ran, and a relay with a documented empty stdout makes any output at all a
contradiction worth chasing.

**What the two wrong diagnoses left behind is the durable part.** `doctor` now reports the last
event actually received per host, which is the only line in that command not derived from a file
this product wrote, and `AGENTS.md` carries the rule that a host's own state outranks the vendor
documentation when the two disagree.

**51 was measured on 2026-09-05 and is now a much narrower row.** What it asked for - a third
spelling, found and measured rather than reasoned about - exists: the 8.3 short path, which is the
only one of four that ran under every shell Codex can hand a hook to, with and without a space.
Spec 4.2 carries the matrix and `internal/host.spaceFree` writes it, over the shortest prefix that
holds every space and nothing below it, so this product's own directory and binary keep the readable
names the installer has always written.

**What found it was reading the host rather than reasoning about the quotes.** codex-rs's hook
command runner at the installed tag hands the value to a shell, and to which shell depends on the
session: `COMSPEC` with `/C` when none is snapshotted, and the snapshotted one as a plain argument
when there is. This machine's Codex snapshots PowerShell, and PowerShell is the only one of the
three shells that reproduces backlog 50 - a fully quoted value is a string literal, so it evaluates
to itself and is printed. `cmd.exe` runs that spelling perfectly. **The nine days of silence were
never a cmd.exe problem, and the row that closed 50 could not have said which shell it was**,
because nothing had asked the host.

| # | Where | What |
|---|---|---|
| 51 | `internal/host`'s `spaceFree` | **A volume with 8.3 names turned off has no spelling at all, and this machine cannot make one.** 8.3 name generation is per-volume and can be disabled, and a directory created while it was off never gets a short name; `spaceFree` then answers the path it was given, which is the spelling measured to work under `cmd.exe` and measured broken under PowerShell. Neither of the two remaining spellings is right for both shells - the plain path fails PowerShell and `& "path"` is a syntax error in `cmd.exe` - so there is still nothing to choose between them without a machine that can be measured. It is narrower than the row it replaces in two ways: it needs a volume with 8.3 off **and** an account name with a space **and** a Codex session that snapshots a shell, and `doctor`'s `codex received` line says nothing has arrived on such a machine. What is measured and owned by a test is everything above that: `TestCodexTakesAPathWithNoSpaceInIt` and `TestSpaceFreeCoversEverySpaceInThePath` |
| 52 | the relay's stdout, and Codex's `Stop` hook | **The premise was wrong and spec 4.5 does not move.** The row said Codex requires a `Stop` hook's stdout to be JSON and that empty is not JSON. codex-rs's stop handler at tag `rust-v0.150.1` - the installed version - does the opposite: it trims stdout, and an empty result is an explicit do-nothing branch. Both the parse and the `hook returned invalid stop hook JSON output` error live in the `else` of it, so **a hook that writes nothing cannot produce that message**. The relay writes nothing on stdout on `Stop` under every configuration, injection on or off, because injection writes on `UserPromptSubmit` and on nothing else. So the failing hook was something else that ran on the same event, and this machine has a candidate: `claude-mem`, still installed, registers its own Codex `Stop` hook that spawns its worker with stdio inherited, so anything that worker prints becomes that hook's stdout. **Not proved** - nobody has read the attribution off the failing line, and the two `Stop` events reaching the database in the same session is consistent with either story. What settles it is one Codex turn with the failure's own text read for whose hook it names; `engramux`'s eleven entries each carry a `statusMessage` and `claude-mem`'s `Stop` entry carries none, though whether the TUI prints one is itself `[unverified]`. **Until it is read, 4.5 stays as it is**: changing an invariant to quiet a symptom that was never measured to be ours is how a wrong finding becomes permanent **Measured again the same evening, and it is a controlled comparison rather than a second reading.** Nothing was installed between: same relay binary, same `hooks.json` since 20:22. At 20:40 two `Stop` events were delivered and the failure printed twice; at 22:13 and 22:16 four `Stop` events were delivered across two sessions and it printed **not once**, with every one of the twelve events those two sessions owed arriving. A path that writes nothing on stdout and has no branch that could vary cannot produce a message that comes and goes, so the relay is excluded twice over - by the vendor's own handler and by its own behaviour holding still while the symptom moved. What is left is only whose hook it was, which is another product's defect and costs this one nothing but the suspicion a user attaches to a red line |

## Raised by closing 48, 2026-09-06

**One of 53's candidates closed on 2026-09-06, and it closed on a measurement rather than an
argument.** The row names a second FTS column over the human-authored leaves — rank on *where* the
match fell rather than on what event the document is. Gate M12 asked whether that is a different
instrument from the one M11 had just rejected, because 82.2% of the corpus has no human half and a
column weight is a uniform multiplier over all of it. **It is different, and it is better, and it is
useless here.** Over 534 command lines it recovers eleven of the sixteen M11 lost, with 310 of 530 of
the documents lifted past them matching outside their own human text. Over 120 touched paths it
recovers none, and **0 of 130** of the documents above them are machine-only: a person typed that
file name into a prompt, or a model wrote it back, so there is nothing spurious there to demote. The
class that vetoed M11 is exactly the class the signal cannot reach.

**What generalises out of it is the sentence the two gates now force together.** A file name is the
one thing a person and a tool both write, and when a prompt contains a path the prompt genuinely is
about that path — so no rule that separates *human text from machine text* can separate *a document
that is about a path from another document that is also about it*. That is a constraint on every
remaining candidate and not only on the one it closed. Memory spec 5's M12 carries both tables and
the subset argument; nothing is repeated here.

**53's last candidate has a number too, 2026-09-07, and it is the first one to clear M11's bar.**
How much of the document the query accounts for — a property of the *pair*, where an event class and
a match location are properties of the document. Gate M13 swept the threshold at M11's own weight
against M11's own condition: three rungs of five clear it, and at 5,000 ppm a reply's own words goes
from 76 of 139 to **92** while a command line **gains** one rather than holding level, which neither
of the others managed. On the gain class alone it is the **weakest** of the three - M11 bought
thirty-one there and M12 thirty-three - and both of those lost a touched path, which is what vetoed
them and what this one does not do. **And the supplementary run over every harm candidate
clears nothing**: `an error message` loses five of 96 at that same threshold and `a touched path`
loses at three rungs. The condition was registered before the gate and is not moved for that, so what
the row now carries is a licensed schema change with a sweep still owed. Memory spec 5's M13 carries
both tables; nothing is repeated here.

**One thing three instruments now agree on.** `a prompt's own words` is 6 of 17 at weight 0, 6 under
M11's rule, 6 under M12's, and 6 at every rung of M13's ladder. The eleven buried prompts of M11's
non-vacuity arm are buried by more than any of these terms can lift, and that is a constraint on
whatever is tried next rather than a result about any one of them.

| # | Where | What |
|---|---|---|
| 53 | `internal/search`'s ranking, and what the corpus cannot tell it | **A search cannot tell a document that is *about* the query from one that merely *contains* it, and gate M11 is what established that this is the actual problem rather than a weighting one.** Measured over the 901-document corpus: a person's prompt is outside the top ten for its own most distinctive word 64.7% of the time, and every one of those sits under a top ten of pure machinery - so backlog 48's first-run complaint is real and larger than the six hits it was filed on. But the machinery is not there by mistake. The document that ran the command genuinely contains the path, which is why a uniform event-class down-weight fails M11 at every weight in the sweep: it buys the prompt its place by taking the command line's. **What is missing is a signal, not a coefficient**, and naming one is the work: how much of a document the query accounts for, where in it the match falls, whether the match is in text a person wrote or in a field a tool filled. None of those is in the index today - `events_fts` holds string leaves and nothing about their provenance - so this is a schema and indexing question before it is a ranking one, and it is a genuinely open one rather than a deferred fix. Memory spec 5's M11 carries the two arms' figures and the delete condition that closed 48; nothing is repeated here. **Not blocking. All three of the row's named candidates now have a number and two tests own them** - `TestGateM12TheSignalIsWhereTheMatchFell` and `TestGateM13TheQueryShareOfTheDocument` in `internal/search`, and the paragraphs above this table say what each one settled. **The row does not close on that.** Nothing ships: `Search` still passes 0 and nil and the ranking is what backlog 48 complained about. What is left is one sweep and one candidate nobody has measured - M13's licensed length column swept over the full harm populations rather than over the sampled 25, which is where the harm its own verdict could not see already is; and reserving places in the visible list rather than reordering it, which demotes nothing and so has no harm arm of M11's shape at all. Read the harm classes' weight-0 recall before taking the second one - 17, 13 and 19 of 25 - because those targets are often outside the top ten already and a reserved slot evicts marginal ones |
