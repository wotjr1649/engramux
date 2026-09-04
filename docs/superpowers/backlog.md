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

| # | Where | What |
|---|---|---|
| 36 | `internal/memory`'s `firstLine` | **A memory item's title is often the wrong line.** A Codex rollout summary's sections all begin with the same prose label, so eight of the fourteen hits in the first live `engramux search` printed the title `Outcome: success` - which tells a reader nothing and is the field they scan. The excerpt carries the substance, so this is a display defect and not a retrieval one. What a better title would be is the decision: the file's own first-level heading, the entry key, or the first line that is not a label the parser recognises. Not blocking, and no test owns it because no test can say which of those is right |
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

| # | Where | What |
|---|---|---|
| 42 | `cmd/engramux`'s `doctor.go` and `inject.go`, and 1.0 spec §7.1 | **The relay binary links the SQLite driver, against an invariant this repository declared and still repeats.** Measured 2026-09-04: `go list -deps ./cmd/engramux` reports `modernc.org/sqlite` and `github.com/pressly/goose/v3`, reached through `internal/inject` → `internal/search` → `internal/store`, which both `doctor.go` and `inject.go` import. `dist/engramux.exe` is **8,703,488 B** against the **3,862,528 B** §7.1 records twice. The session-05 brief states the rule outright - *"The relay binary must not link the SQLite driver"* - and `doctor.go`'s own comment explains that it uses `spool.Dir()` rather than importing `internal/store` *because* that would link the driver, in the file that imports it anyway two hundred lines above. What this costs is not only bytes: §5.9's rejection of an stdio proxy and `doctor`'s rejection of a `net/http` probe (+93.7%) are both argued against the 3,862,528 B baseline, so both rejections now rest on a figure that has moved. The relay is spawned once per hook event, which is what made the size an invariant rather than a preference. **Measured 2026-09-04, after this row was first written and against it: it is a package split, not a design decision.** The relay uses exactly four symbols from `internal/inject` - `Enabled()`, `ConfigName`, `Budget` and `MaxBytes` - and not one of them touches the database: `config.go` imports `encoding/json`, `fmt`, `os` and `path/filepath` and nothing else. The relay never calls `Build` at all, because injection happens service-side and the relay only gates on the switch file and forwards over the pipe. The driver arrives because those four symbols share a *package* with `Build`, which reaches `internal/search` and `internal/store`. And that package is the only route: `cmd/engramux`'s non-test imports contain no `internal/store`, and none of the other eight internal packages it imports reaches the driver. So the fix is to move the switch and the two budget constants into a leaf package both sides import. The first version of this row called it a design decision about how `doctor` reports injection; that was written from a reviewer's account without checking the coupling surface, and it is wrong. **A test could own it and none does**: a dependency assertion over `go list -deps ./cmd/engramux` fails the moment it happens again, and is the part of this row worth more than the split. **Sequencing**: it edits `internal/inject` and `m7_test.go`'s imports, so it waits for gate M7 unless the split is proved behaviour-neutral by reproducing pass 2's recorded figures exactly |
| 45 | `cmd/engramux`'s `update.go` | **The product tells the user to download a release archive that does not exist.** Running `update` with no `--from` fails in `updateSource` with a message instructing the reader to download the release archive, unpack it and point `--from` at the directory. There is no release: no tag, no `.github/`, no marketplace entry, no archive, confirmed 2026-09-04. The message is right about the *shape* of the eventual channel and wrong about today, and it is the first thing a reader meets when they try the command the README says is not a first step. Trivial to change and not changed here: it is a product string, so it wants its own branch, and whether it should name the developer path or simply say there is no channel yet is a small decision rather than none |
| 46 | `internal/inject`'s abstention reasons | **`ReasonNoHits` covers at least three different situations and `ReasonTooBroad` absorbs a fourth, so the service log cannot tell recall from silence - which is the thing §6's fifth mitigation asks it to do.** Read 2026-09-04. One return point produces `ReasonNoHits`, reached when the event side and the memory side are both empty after filtering, and the event side empties three ways: nothing matched at all; the only match was the prompt's own event, removed by the exclusion; or everything that survived the exclusion was one of this product's own command lines, removed by `keepable`'s `InvokesEngramux` filter. The third is not in the spec's account of it. Worse, `broad` is set when **either** index exceeds the 200-document ceiling, so a run where the memory side was suppressed and the event side was emptied by the exclusion returns `ReasonTooBroad` - an exclusion reported as a selectivity ceiling. Session 14 read the reason one way, wrote it into the spec and corrected it the same day; the spec's corrected sentence still says the one surviving match *is* the prompt's own event, which was measured over 115 of 120 and not 120. **What a fix is**: split the constant, and return what `keepable` removed. That also makes gate M7's `byReason` count the causes for free and with no leak surface, where measuring it from outside the injector cannot - `candidates`, `maxMatches` and `keepable` are all unexported, so an out-of-package replication measures a different population. Product behaviour changes (a log line), so it wants its own branch. **Deliberately not done in session 15**, because the M7 fixture was being labelled and the gate re-injects: changing the injector mid-labelling makes pass 2 and the gate measure different treatments |
