# Backlog

Deferred findings that no test owns yet. Each was triaged as non-blocking by the review that
raised it; none blocks a merge. This file owns **only** the carry list — it decides nothing. The
spec owns decisions, invariants, budgets and measurements; a plan owns execution order.

Extracted from `.superpowers/sdd/2026-08-28-session-02-search/progress.md` before that gitignored
ledger was deleted, which is the whole reason this file exists: the triage lived on one machine.

**When a test starts catching an item, delete the row.** The test is the better owner. Same rule
AGENTS.md applies to its own "What will bite you" table.

## Carried from Phase 1-4

| # | Where | What |
|---|---|---|
| 6 | `engramux doctor` | Say when `ENGRAMUX_TEST_PIPE_SID` is set in doctor's own environment, so a leftover export is diagnosable. Raised by Codex during T1 |
| 9 | `internal/service/service.go:90,98`, `internal/store/checkpoint.go:90` | "about 4.1 MiB" is 4.1 MB — 1,000 pages × 4 KiB is 4,096,000 B, which is 3.9 MiB. Corrected in `checkpoint_test.go` and spec §5.4/§7.1; **these three sites are shipped source, so they wait for the post-soak build** |
| 16 | `maxEventNameRunes = 64` | Couples the wire to what one client prints. An MCP client wanting the whole name is what would move it. Recorded in the constant |
| 17 | EventName truncation | Carries no marker, so a shortened name is indistinguishable from a real 64-rune one. A client that needs to know needs a flag on the hit, not a suffix |

**Every row left needs a build.** The soak window closed all fifteen that did not, so what remains
was exactly the Step 1 and Step 2 queue of `plans/2026-08-30-after-phase-6.md` until the soak filed
**34**, which needed a build and belonged to no step for two days; on 2026-09-02 the plan put it in
Step 1 as its own migration, and its row says so. A row appearing here that does *not* need a build
is now a sign that something was filed rather than done.

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

| # | Where | What |
|---|---|---|
| 27 | the refusal path, all reply types | **A refused request carries no reason.** Observed live on 2026-08-30: `engramux sessions //host/share/dev` is correctly refused - the UNC guard fires - and the caller sees only *"the service replied rejected"*, because `ipc.Ack` has no field for a reason and every refusal is an Ack. A person can guess; a model cannot correct itself, and Phase 5's tool surface is the first caller that has to. Fixing it is a design change to a Phase 1 contract the relay depends on - either a reason field on `Ack` or an error field per reply document - so it is a row and not a patch. **Narrowed by Phase 5, not closed.** The tool surface no longer has the problem: `internal/mcpserver` calls the same `pipe.Handler` closures rather than dialing the pipe, so the handler's own error is in hand and is what the tool returns, masked. `TestARefusedCallCarriesAMaskedReason` holds it. The row stays because the wire still answers a bare rejected `Ack`, and the CLI is still the caller that reads it |
| 30 | `internal/mcpserver/serve.go:278` | **The server does not offer MCP revision `2026-07-28`, which §5.9 reasons from.** Measured over `server/discover`: `supportedVersions` is `["2025-11-25","2025-06-18","2025-03-26","2024-11-05"]`. `mcp.NewStreamableHTTPHandler` is given nil options, so `StreamableHTTPOptions.Stateless` is false; against a bare SDK server with only that field flipped the list gains `"2026-07-28"` at its head and changes in no other way. `initialize` cannot show this — SEP-2575 made it the legacy handshake and the SDK caps that path at `2025-11-25` regardless. `TestTheServerOffersTheRevisionsItActuallyOffers` pins the list as it is and fails when the field is set, which is the signal to update §5.9 with it. Needs a build |
| 31 | `docs/.../design.md` §5.6 vs the tree | **`health.json` is specified and does not exist.** §5.6 assigns it to the service — panics, errors, spool depth, checkpoint results — and gives the reason: `doctor` is a separate process and cannot read the service's memory. `grep -rn health --include=*.go` finds nothing outside `healthy`. So `doctor` reports none of those four, and the artefact that exists to let it is absent. **Decided 2026-08-30: withdraw `health.json` from §5.6 and put an error counter and the last checkpoint result on `StatusReply` instead.** No new file means no new failure mode where a stale file explains a live service; the cost accepted is that a dead service reports nothing, which the file would have covered. Needs a build |
| 32 | `internal/ipc/envelope.go:17`, `internal/pipe/serve.go:395` | **`ipc.Drain` is a declared request type with no handler**, and `pipe.Handler` has no field for one — `serve.go`'s `default:` branch names it as the read this build does not implement. §5.5's upgrade procedure is "drain, stop, replace, start" and **step 1 has no wire path**; the only drain is the service's own 30-second timer. **Decided 2026-08-30: withdraw the drain step from §5.5 and remove the `ipc.Drain` constant with it.** The spool is durable and `service.go:242` drains at every start - the log's `replayed spooled events` line on each start is the evidence - so a stop without a drain loses nothing and the promise was never needed. Needs a build |
| 33 | `internal/ipc/search.go:101` | **A search reply carries hits and no total, so the product cannot count its own corpus.** Found while trying to measure how often a model calls the engramux MCP tools unprompted: `.capture/` holds 714 tool calls and 0 MCP calls of any server, but that corpus predates the MCP server and therefore measures nothing about the question; the live database is the right population and there is no way to ask it "how many". `search` returns at most 100 hits with no match count, and `status`/`cells` count by `(host, event_name)` rather than by tool. One `count(*)` beside the existing MATCH closes it, and it closes a second thing with it: `no results` today means an empty corpus, a wrong project, an intersection that emptied, and a genuinely absent term, all in the same two words. Needs a build |
| 28 | `mcp.json` **and both host configuration files** | **The bearer token sits in three files whose DACLs are all inherited.** Measured (spec 7.1) on `mcp.json`: every ACE is `(I)`, Go's `f.Chmod(0o600)` writes nothing to it, and on the machine measured a machine-local group holds `(RX)`. The installer then copies the same token into `~/.codex/config.toml` and Claude Code's user configuration, whose permissions this product does not set at all - and the token is sticky, so all three copies are long-lived rather than per-start. Spec 5.9 accepts the exposure, because the token is the whole of the control at that transport either way. Narrowing `mcp.json` is a change of its own: a security descriptor built with `golang.org/x/sys/windows` and passed to `CreateFile`, which is a different atomic-write path from `os.CreateTemp` plus rename. `internal/pipe`'s listener already builds a DACL, so there is a pattern to reuse. Narrowing the other two is not this product's to do. **Scheduled 2026-09-02 as a publication condition** rather than a step — memory spec §8 — with `mcp.json` narrowed and the two host files reported by `doctor` as a finding |

## Raised by the Phase 6 soak

The soak is the only instrument that runs the shipped binary for days against a database that keeps
growing, so what it raises is what no test's fixture is large enough to reach.

| # | Where | What |
|---|---|---|
| 34 | `internal/store/migrations/00001_schema.sql:78`, `internal/service/service.go:598` | **`events` carries no index but the primary key's, so a status reply's `GROUP BY host, event_name` is a full scan of a table whose `payload` and `leaves` share its b-tree.** `CREATE INDEX` appears nowhere in the migrations and `events` declares no `UNIQUE`, so the grouping has nothing to use and the cost is the size of the file rather than the ~30 rows it returns. `internal/service/gate.go` already carries the reasoning and already states the missing index as unfixed; what this row added was that **nothing scheduled it**: Step 1 of `plans/2026-08-30-after-phase-6.md` was scoped to touch no schema, Steps 3 and 4 carry migrations for other reasons, and a code comment schedules nothing. The soak produced both of its surfaces — nine refused `status` reads in 147 samples and one reply write to a closed pipe — and §7.1's soak row holds what they were. **Scheduled 2026-09-02**: §7.1's read-deadline row decides that the index comes before any change to the deadline, and the plan's Step 1 carries it as its own migration. Delete this row when that migration and its test land |

## Raised by the first install through `engramux install`

The re-install of 2026-09-02 — the Phase 6 binaries replaced by the merged build on the owner's own
machine — is the first run of the Go installer against a real Claude Code and a real Codex.

| # | Where | What |
|---|---|---|
| 35 | `internal/host/claude.go`, `RegisterClaudeMCP` | **A re-install reports Claude Code's registration as failed when it is already there.** `claude mcp add --scope user` exited 1 against a registration the previous installer had made; the existing one was intact, pointed at the live endpoint with the sticky token, `claude mcp get engramux` answered `Connected`, and `doctor` said the host points at this endpoint — so nothing was broken and the report said something was. The cause is `[unverified]`, because the child's output is discarded on purpose (the command line carries the token, and that decision holds); that Claude Code refuses a duplicate name is the obvious reading and was not confirmed. The Node installer reported the same way, so this is parity with a defect and not a regression. `doctor` already answers whether the host points at this endpoint, and an install that gets that answer has nothing to add — skip the `add`, rather than run `remove` and then `add`, which would put the token on a command line twice. Needs a build; the `--remove` path is unaffected |

