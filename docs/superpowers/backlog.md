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
carry what they changed. **28** is the one row left, and it is a publication condition rather than
a build.

| # | Where | What |
|---|---|---|
| 28 | `mcp.json` **and both host configuration files** | **The bearer token sits in three files whose DACLs are all inherited.** Measured (spec 7.1) on `mcp.json`: every ACE is `(I)`, Go's `f.Chmod(0o600)` writes nothing to it, and on the machine measured a machine-local group holds `(RX)`. The installer then copies the same token into `~/.codex/config.toml` and Claude Code's user configuration, whose permissions this product does not set at all - and the token is sticky, so all three copies are long-lived rather than per-start. Spec 5.9 accepts the exposure, because the token is the whole of the control at that transport either way. Narrowing `mcp.json` is a change of its own: a security descriptor built with `golang.org/x/sys/windows` and passed to `CreateFile`, which is a different atomic-write path from `os.CreateTemp` plus rename. `internal/pipe`'s listener already builds a DACL, so there is a pattern to reuse. Narrowing the other two is not this product's to do. **Scheduled 2026-09-02 as a publication condition** rather than a step — memory spec §8 — with `mcp.json` narrowed and the two host files reported by `doctor` as a finding |


## Raised by Step 3's first live install, 2026-09-02

| # | Where | What |
|---|---|---|
| 36 | `internal/memory`'s `firstLine` | **A memory item's title is often the wrong line.** A Codex rollout summary's sections all begin with the same prose label, so eight of the fourteen hits in the first live `engramux search` printed the title `Outcome: success` - which tells a reader nothing and is the field they scan. The excerpt carries the substance, so this is a display defect and not a retrieval one. What a better title would be is the decision: the file's own first-level heading, the entry key, or the first line that is not a label the parser recognises. Not blocking, and no test owns it because no test can say which of those is right |
| 37 | The two shipped binaries, and memory spec §8 | **Windows Defender quarantines the CLI on behaviour, and this is a publication condition rather than one machine's problem.** Measured 2026-09-03 00:16:03 on the owner's machine: `engramux.exe` was removed from the build output directory and from the install directory as `Behavior:Win32/Execution.A!ml`, severity 5, `DidThreatExecute` False - blocked before it ran, so nothing was compromised. `Behavior:` and `!ml` are the whole finding: it is a behavioural machine-learning detection on executing a freshly built, rare, unsigned binary, not a signature on the bytes. The service binary was untouched and kept running. It is **not** the first: `Trojan:Win32/Commando.A!ml` fired on 2026-08-30 against the soak sampler's `schtasks /create`, so **two of the four detections this machine has ever recorded are Engramux doing exactly what it is designed to do** - run a new unsigned executable, and register a scheduled task. A stranger's first install is those same two shapes. What does *not* explain it is the `-s -w` strip in the build line: this is a behaviour detection and the flags are not implicated, so do not change them hoping it helps. `Add-MpPreference -ExclusionPath` was refused with HRESULT 0xc0000142 - unelevated, or Tamper Protection, which is what that feature is for - so an exclusion has to go through the Windows Security UI, by a human, and that is not a route an agent takes. The real fixes are code signing or a documented exclusion step in the install instructions. **Memory spec §8 answered that on 2026-09-03**, as its fourth publication condition and deliberately as an outcome rather than a mechanism: Microsoft's March 2024 SmartScreen change removed EV's instant bypass, so signing accumulates reputation across releases rather than switching the detection off, and a first release by a new publisher still has none. This row stays as the measurement that condition rests on. No test can own it: it is a property of an external control on the machine the binary lands on |
