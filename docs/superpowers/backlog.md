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
| 1 | `main_test.go:284-292`, `service_test.go:166-171` | `requirePipeFree` mutates the environment as a side effect of a predicate-named helper |
| 2 | five sites | Five copies of the pipe-name override key format. Harmless, but five |
| 3 | `ipc/pipename_test.go:56-62` | The first assertion prints neither value nor names the variable, so a leftover `ENGRAMUX_TEST_PIPE_SID` in a shell trips it with no way to see why |
| 4 | `AGENTS.md` | The `-p 1` row is five sentences. Shorten |
| 5 | `internal/pipe/listen.go:134-136` | Returns `ipc`'s error unwrapped. Unreachable today |
| 6 | `engramux doctor` | Say when `ENGRAMUX_TEST_PIPE_SID` is set in doctor's own environment, so a leftover export is diagnosable. Raised by Codex during T1 |
| 7 | `AGENTS.md:76-82` | The fenced "TDD, then break it" block sits outside the Commands exception to the no-code-blocks rule. Predates the session that found it |
| 8 | particle rule | Trims only trailing ASCII punctuation, so a token ending in Hangul followed by non-ASCII punctuation (ellipsis, full-width period) stops being a candidate. Corpus Hangul particle candidates 158 to 136. Faithful to the ruling, recorded because the ruling did not consider it |
| 9 | `checkpoint_test.go:291` | "4.1 MiB" is 3.95 MiB / 4.1 MB. Unit shorthand echoing the pre-existing comment at :244 |
| 10 | fts5vocab test | Pins the reference spelling's token COUNT and compares the other two spellings to it, rather than three literal token lists. Exact and proven to fail two ways; noted only |
| 11 | `internal/store/leaves.go:162` | The depth guard is evaluated on the closing-delimiter path too, where it can never fire. One dead comparison, traded for one check covering both openers |
| 12 | `fts_test.go:170-178` | Duplicates `leaves_test.go:204-212`'s raw `INSERT INTO events` block; only the id prefix and `received_at` differ. A five-line helper removes it and the `internal/secret` import with it |
| 13 | leaves walk | No test covers the ORDER in which the depth guard fires — shallow leaves already collected, then an over-deep subtree. `nestedJSON` always puts the leaf at the bottom. The code is correct; nothing would catch a partial-walk regression |
| 14 | leaves walk | The guard closes only the DEPTH mode of `json_valid`'s refusal. Any other shape Go accepts and SQLite refuses still diverges silently, caught only if such a payload happens to be in `TestTheTwoWalksAgree`'s set |
| 15 | `leaves_test.go` | `goJSONDepthLimit = 10000` pins an undocumented `encoding/json` implementation detail. Accepted and self-flagged: it is the measurement behind the constant's "ten times" claim, and an unmeasured number is what AGENTS.md forbids |
| 16 | `maxEventNameRunes = 64` | Couples the wire to what one client prints. An MCP client wanting the whole name is what would move it. Recorded in the constant |
| 17 | EventName truncation | Carries no marker, so a shortened name is indistinguishable from a real 64-rune one. A client that needs to know needs a flag on the hit, not a suffix |
| 19 | `%q` rune precision | True and verified by throwaway, but nothing in the suite holds `fmt`'s rule — only `truncateRunes` does. Four lines would hold it. If Go changed `%q` precision to bytes the CLI would display fewer characters than the wire carries: cosmetic |
| 20 | `scripts/install-hooks.mjs:137` | A half-migrated `EVENTS` row silently loses its matcher. Destructuring `matcher` from an old-style STRING value yields `undefined`, which is not `null`, so the entry is pushed with `matcher: undefined` and `JSON.stringify` deletes it; `codexTimeout` goes the same way. Measured. One line to close: throw when a table row is not an object |
| 21 | spec 7.1 / 7.3 | The Codex `SessionEnd` clamp row labels the observed half honestly but does not record the WARNING TEXT itself, which is that observation's only evidence. A home-path-stripped copy would let the next Codex version be compared against it |
| 22 | `scripts/install-hooks.mjs` header | Lists `SessionEnd` and the Subagent pair among the events using `*`, but the `EVENTS` table gives all three `matcher: null`. The table matches the live file; the comment is stale |
| 23 | `internal/host/install_hooks_test.go` | Checks only `SessionEnd` on the Claude Code side, so a change lowering the other ten Claude timeouts would pass |

## Pre-existing defects confirmed by the 2026-08-29 adversarial review

Raised while reviewing the Phase 5 design; each is a property of code that already shipped, not
of that design. Rows 25 and 26 were closed by Phase 5 and are gone; the numbering is not
renumbered, because a row's number is how the sessions that discussed it refer to it.

A third section stood here, "Phase 5 prerequisites this review surfaced" — the masked status and
list-sessions replies, `get_event`'s measured bound, the `(id, project_id)` pair, the trust boundary
on a caller-supplied path, and the bound on the single connection. Every one is now a test, so by
this file's own rule the list is gone. Spec 8's Phase 5 row names the tests that own them.

| # | Where | What |
|---|---|---|
| 27 | the refusal path, all reply types | **A refused request carries no reason.** Observed live on 2026-08-30: `engramux sessions //host/share/dev` is correctly refused - the UNC guard fires - and the caller sees only *"the service replied rejected"*, because `ipc.Ack` has no field for a reason and every refusal is an Ack. A person can guess; a model cannot correct itself, and Phase 5's tool surface is the first caller that has to. Fixing it is a design change to a Phase 1 contract the relay depends on - either a reason field on `Ack` or an error field per reply document - so it is a row and not a patch. **Narrowed by Phase 5, not closed.** The tool surface no longer has the problem: `internal/mcpserver` calls the same `pipe.Handler` closures rather than dialing the pipe, so the handler's own error is in hand and is what the tool returns, masked. `TestARefusedCallCarriesAMaskedReason` holds it. The row stays because the wire still answers a bare rejected `Ack`, and the CLI is still the caller that reads it |
| 28 | `mcp.json` **and both host configuration files** | **The bearer token sits in three files whose DACLs are all inherited.** Measured (spec 7.1) on `mcp.json`: every ACE is `(I)`, Go's `f.Chmod(0o600)` writes nothing to it, and on the machine measured a machine-local group holds `(RX)`. The installer then copies the same token into `~/.codex/config.toml` and Claude Code's user configuration, whose permissions this product does not set at all - and the token is sticky, so all three copies are long-lived rather than per-start. Spec 5.9 accepts the exposure, because the token is the whole of the control at that transport either way. Narrowing `mcp.json` is a change of its own: a security descriptor built with `golang.org/x/sys/windows` and passed to `CreateFile`, which is a different atomic-write path from `os.CreateTemp` plus rename. `internal/pipe`'s listener already builds a DACL, so there is a pattern to reuse. Narrowing the other two is not this product's to do |
| 24 | spec 6 vs. the tree | **The 512 KiB field cap is documented but not implemented.** Spec 6 says field values are capped by a limiter that preserves JSON validity. Searching the non-test tree for it finds comments and the query builder's `maxTokenBytes`, and nothing that enforces it; `readStdin` is explicitly unbounded and says so. A stored payload is therefore bounded only by `ipc.MaxFrameLen`. Either implement the cap or correct spec 6 |

