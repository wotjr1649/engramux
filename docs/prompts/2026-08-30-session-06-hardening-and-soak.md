# Session 06 — Engramux Phase 6: hardening and soak

Session 05 built the MCP server, then reviewed it and found three defects in its own work, one of
them severe. All three are fixed. **Phase 5 is done and every one of §8's seven gate clauses is
green**, so what is left in 1.0 is Phase 6: the redaction audit, and a soak past 72 hours. The
soak's precondition is that the service binary stops changing, and it now has.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file
carries what they cannot: the state of the work when the session opened, and how that session was
scoped.

Read `docs/superpowers/specs/2026-08-27-engramux-1.0-design.md` **§8's Phase 6 row first** — it is
two sentences and it is the whole of what this phase owes. Then §6.1 for what a secret is, §7.5
for corpus hygiene, and §5.9 only if you touch the MCP surface, which you should not need to.

---

## 1. Where the work stands

| | |
|---|---|
| `main` | `5a8de8a`, **pushed**. `phase-5-mcp-server` was merged fast-forward and is what the live service runs. Start a branch of your own |
| Last full verification, at `5a8de8a` | `go test -p 1 -count=1 ./...` **16 packages ok** · pinned linter `0 issues.`, **exit 0** (the exit code, not the summary line) · `./scripts/race.sh` **exit 0, no data races** · both `CGO_ENABLED=0` builds ok, the service with `-H=windowsgui` |
| `dist/` | Built at `5a8de8a` and **installed**. `engramux.exe` 4,285,440 B, `engramux-service.exe` 13,919,232 B |
| `dist-rollback/` | **Refreshed.** It now holds `cac4d52` rebuilt — the last known-good build before the MCP endpoint existed — rather than the Phase 4 binaries it used to hold. Not the same bytes that ever ran, because the worktree path differs, but the same commit |
| The live service | Running the merged build, restarted 2026-08-30 04:00. Zero `ERROR` lines since that start |
| Both hosts | Registered and smoke-tested, and the registration now **survives a restart** — measured over two stop/start cycles with no installer run in between |
| Phases | 1, 2, 3, 4, 5 done and gated. **6 is all that is left** |

---

## 2. §8's Phase 5 gate: all seven, and where each lives

Nothing here is yours to redo. It is written down so you do not go looking.

| Clause | Test |
|---|---|
| Bearer required — no token and a wrong token both refused | `TestPhase5GateNoTokenAndAWrongTokenAreBothRefused`, `internal/mcpserver` |
| Cross-origin rejected | `TestPhase5GateACrossOriginRequestIsRejected`, same package |
| Bound to `127.0.0.1` only | `TestPhase5GateTheListenerIsOnLoopbackAndNoOtherInterface`, same package |
| No MCP-facing field carries a user path | `TestPhase5GateNoReplyFieldCarriesAUserPath`, `internal/service` |
| `get_event` bounded by a measured number | `TestMaskingExpandsAndTheGetEventBoundHoldsOverTheCorpus`, `internal/secret` |
| Cross-project isolation | `TestPhase5GateGetEventChecksTheProjectWithTheId` and `TestPhase5GateASearchScopedToOneProjectNeverReturnsAnother` |
| A reader does not push ingest past 800 ms | `TestPhase5GateAReaderDoesNotPushIngestPastItsBudget` |

Three more tests guard things the gate does not name and the review found missing:
`TestTheTokenAndThePortBothSurviveARestart`, `TestATokenThisNeverWroteIsReplaced`, and
`TestBothSurfacesShareOneReadGate`. Every one of the ten was confirmed by a deliberate break.

---

## 3. What session 05 built, so you do not rebuild it

- **`internal/mcpserver`.** The Streamable HTTP handler, the bearer middleware, `net/http`'s
  cross-origin middleware wrapped around it, and the four tools. The tools call the `pipe.Handler`
  `internal/service` already builds rather than dialing the pipe, so a tool call takes the same read
  gate a CLI read takes and the replies are `internal/ipc`'s documents.
- **`internal/mcpconf`.** `mcp.json`, written by the service and read by `doctor` and the installer.
  Its read side has no field for a token; the service's own token reader is in `internal/mcpserver`,
  which the relay binary does not link. That is what makes "the CLI cannot print the token"
  structural rather than a convention.
- **`doctor` gained an `mcp` section**: the endpoint, whether anything is listening, and one line
  per host. The host lines do not decide the exit code; §7 says why that matters to you.
- **The installer registers both hosts.** Codex by splicing a TOML table into `config.toml`; Claude
  Code by running its own `claude mcp add --scope user`, because that host's file is a live state
  file and a read-modify-write from outside is a lost update.
- **`readHold`** is a test seam in `internal/service/gate.go`, the way `readDeadline` and
  `drainInterval` are. It exists for one test and is zero in every shipped path.

---

## 4. Measured last session — do not rediscover it

All of it is in §7.1 with its reproduction.

- **The token is sticky, and it had to become so.** It rotated per start at first, which broke both
  hosts at every logon while `doctor` reported them healthy. Now measured: two restarts, no
  installer run in between, same token, both hosts still connected.
- **The SDK costs 5.4 MB of service binary and nothing of the relay**, which is the check §5.9's
  rejection of a stdio proxy rests on.
- **`net/http` in the relay costs +93.7%**, which is why `doctor` probes with a dial. The relay is
  4,285,440 B. **Watch this number**: it is spawned once per hook event.
- **`mcp.json` carries no permissions of its own.** Every ACE inherited, `0o600` writes nothing to
  the DACL. The token is the whole of the control, and it now lives in three files (backlog 28).
- **Neither host drops a static `Authorization` header.** Claude Code `2.1.251`, Codex `0.150.1`.
  A fact about somebody else's releases; re-run it on a host upgrade.
- **`jsonschema-go` infers a `json.RawMessage` as the `[]byte` it is**, so `get_event`'s tool output
  type is `any`. The exact error is in `internal/mcpserver/tools.go`.

---

## 5. What the review found, and what it says about reviewing

Three defects, in code that had already passed a suite, a linter, a race detector and a live smoke
test. They are worth reading before you decide how much reviewing this session needs.

- **The token rotated per start.** Severe: the product broke at every logon. Nothing caught it
  because the smoke test ran inside one service lifetime — the failure needs a *restart*, which no
  test and no observation in that session performed.
- **Three comments named a ceiling that only one of two surfaces has.** `ipc.MaxFrameLen` bounds a
  pipe reply; an MCP reply never reaches `WriteFrame`.
- **Nothing held the design's central claim** — that both surfaces share one read gate. A deliberate
  break left three packages green.

The pattern in all three: the new surface was assumed to inherit the old one's properties. If you
add a surface, that is the question to ask about every property the old one had.

---

## 6. The tasks

**T1 — The redaction audit.** §8's Phase 6 `[auto]` clause is *"redaction audit finds nothing"*, and
that sentence does not say what the audit is over. Decide it and write it down before running it —
the surfaces that exist now are the log file, every reply document, every MCP tool result, every
tool *error*, `doctor`'s output, and the installer's output. `TestTheLogFileNeverCarriesASecret` and
`TestPhase5GateNoReplyFieldCarriesAUserPath` are the two that already exist; what is missing is
whether anything sweeps them all with one detector, and whether the audit runs over the real corpus
or over fixtures. `doctor` deliberately prints real paths and is not an egress — an audit that flags
it has the wrong scope.

**T2 — The soak.** `[manual]`, 72 hours. It can start now: the binary is merged, installed and
verified. What to record while it runs is a decision — the WAL's size, the checkpoint log lines,
RSS, the MCP session map's growth across host restarts, and the spool depth are the candidates. The
session map is the one nothing bounds: `SessionTimeout` is left at zero on purpose, and a soak is
exactly the instrument that would say whether that was right.

**T3 — Whatever the audit and the soak turn up.** Nothing else.

---

## 7. Open, and what to do about each

| Open | What to do |
|---|---|
| Backlog 28, the token in three files with inherited ACLs | The one backlog row that plausibly belongs to a hardening phase. `internal/pipe`'s listener already builds a DACL, so there is a pattern; the cost is a different atomic-write path from `os.CreateTemp` plus rename, and it only reaches `mcp.json` — the two host files are not this product's to narrow. §5.9 already accepts the exposure, so this is a design change, not a bug fix |
| `doctor` cannot see a token mismatch | Accepted, deliberately. After the sticky fix it needs `mcp.json` to be lost **and** the same port re-bound by luck; losing the file normally changes the port, which `doctor` does catch. Closing it would cost either the relay binary reading a token — the property just made structural — or the service reading host configuration files, which it does not do |
| Backlog 27, a refusal with no reason | Narrowed, not closed. The tool surface carries its reason because it does not cross the wire; the CLI still reads a bare rejected `Ack` |
| Backlog 24, the unimplemented 512 KiB cap | Still a row |
| The 23 remaining backlog rows | Untouched unless a task is already standing in that file |
| An MCP reply has no size ceiling | Stated in three comments and in §5.9's tool table, and left that way: a cap needs a number nobody has measured |
| The MCP session map | Unbounded by design, and the soak is its instrument |
| Codex `SessionEnd` past the clamp, §7.3 | Still unmeasured; needs a deliberately slow hook in a user's own configuration. Not agent work |

---

## 8. Five things that will bite

1. **`schtasks /end` then `/run` back to back leaves nothing running.** There is a row in
   `AGENTS.md` now. It cost this session a confusing `Access is denied` and a host reporting
   `ConnectionRefused` against a service that had simply never started.
2. **`doctor`'s `mcp` host lines do not decide its exit code**, on purpose. If you change that,
   `TestDoctorReadsTheTaskWhetherOrNotTheServiceIsUp` fails on a developer machine and passes in a
   clean one, which is the worst way for a test to fail.
3. **A test that runs the installer with `--apply` and a real `PATH` edits your own Claude Code
   configuration.** `runInstallerWithPath` with an empty `PATH` is the seam; there is a row in
   `AGENTS.md`.
4. **Check the linter's exit code, never its summary line.**
5. **Write any file containing backslashes with a file-write tool and verify the bytes afterwards.**
   `LC_ALL=C grep -nP '[^\x00-\x7F]'` over what you changed is the check.
