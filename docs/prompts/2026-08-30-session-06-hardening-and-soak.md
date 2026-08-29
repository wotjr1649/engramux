# Session 06 — Engramux Phase 6: hardening and soak

Session 05 built the MCP server. **Phase 5 is done and every one of §8's seven gate clauses is
green**, so what is left in 1.0 is Phase 6: the redaction audit, and a soak past 72 hours. The
soak's precondition is that the service binary stops changing, which is the constraint this whole
session has to be planned around.

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
| Branch | `phase-5-mcp-server`, **6 commits, local only**. It was deliberately not merged and not pushed: the decision was to hold it until the gate had been seen working on the live service, which it now has. Merging and pushing it is this session's first act |
| `main` | `cac4d52`, pushed, and **behind the branch**. The live service is running the *branch*, not `main` |
| Last full verification, at `f9e09d1` | `go test -p 1 -count=1 ./...` **16 packages ok** · pinned linter `0 issues.`, **exit 0** (the exit code, not the summary line) · `./scripts/race.sh` **exit 0, 16 packages, no data races** · both `CGO_ENABLED=0` builds ok, the service with `-H=windowsgui` |
| `dist/` | Rebuilt at `f9e09d1` and **installed**. `engramux.exe` 4,285,440 B, `engramux-service.exe` 13,917,184 B. `dist-rollback/` still holds the Phase 4 binaries and is gitignored by `*.exe`; it is now two versions behind and is a rollback to Phase 4, not to Phase 5's prerequisites |
| The live service | Running the branch's binaries, restarted 2026-08-30 02:47. It drained the 6 events spooled during the stop, which is I-04 end to end for the second install running |
| Both hosts | Registered against the live endpoint and **smoke-tested**. Claude Code and Codex each reach it with the static bearer header |
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

The three transport clauses run against `mcpserver.Listen` and `Server.Serve` — the production
wiring, the same two calls `internal/service` makes — and each was confirmed by a deliberate break.

---

## 3. What session 05 built, so you do not rebuild it

- **`internal/mcpserver`.** The Streamable HTTP handler, the bearer middleware, `net/http`'s
  cross-origin middleware wrapped around it, and the four tools. The tools call the `pipe.Handler`
  `internal/service` already builds rather than dialing the pipe, so a tool call takes the same read
  gate a CLI read takes and the replies are `internal/ipc`'s documents.
- **`internal/mcpconf`.** `mcp.json`, written by the service and read by `doctor` and the installer.
  It is a package of its own because `doctor` lives in the relay binary, which must not link the
  SQLite driver. **Nothing in it can hand a caller a token** — the read side decodes the URL and has
  no field for one.
- **`doctor` gained an `mcp` section**: the endpoint, whether anything is listening, and one line
  per host saying whether that file points here, at another URL, or at nothing. The host lines do
  not decide the exit code; §7 below says why that matters to you.
- **The installer registers both hosts.** Codex by splicing a TOML table into `config.toml`; Claude
  Code by running its own `claude mcp add --scope user`, because that host's file is a live state
  file and a read-modify-write from outside is a lost update.
- **Backlog 28 is new** — `mcp.json`'s DACL — and **27 is narrowed**: the tool surface carries its
  refusal reason because it does not cross the wire; the wire still does not.

---

## 4. Measured last session — do not rediscover it

All of it is in §7.1 with its reproduction.

- **The SDK costs 5.4 MB of service binary and nothing of the relay.** 8,497,152 → 13,917,184 B and
  3,862,528 B unchanged. §5.9's "eight direct and three indirect modules" was the SDK's own
  `go.mod`; what this repository gained is six indirect modules.
- **`net/http` in the relay costs +93.7%**, which is why `doctor` probes the endpoint with a dial.
  The relay is 4,285,440 B, +10.9% over Phase 5's prerequisites, for `net/url` and a dialer. **Watch
  this number.** It is spawned once per hook event.
- **`mcp.json` carries no permissions of its own.** Every ACE inherited, Go's `0o600` writes nothing
  to the DACL, and the mode reads back `0666`. So the bearer token is the whole of the control, and
  on the machine measured one more local principal can read it. That is backlog 28.
- **Neither host drops a static `Authorization` header.** Claude Code `2.1.251` and Codex `0.150.1`,
  measured against the installed build. It is a fact about two of somebody else's releases; re-run
  it on a host upgrade.
- **`jsonschema-go` infers a `json.RawMessage` as the `[]byte` it is**, so `get_event`'s tool output
  type is `any` rather than its reply document — a typed one refuses every call whose payload is a
  JSON object. The reason is in `internal/mcpserver/tools.go` with the exact error.

---

## 5. It is installed, and here is what it answered

The user ran the apply step: stop, copy, start, apply again, and then both hosts.

- **`doctor`, exit 0, every section green.** The `mcp` section reported the endpoint, `listening
  yes`, and both hosts `points at this endpoint`. The bound port was inside the 1024–15000 ephemeral
  range §5.9 measured on this machine, which is the range that made a derived port wrong.
- **The two-run install is real and is the design.** The first `--apply` copies the binaries and
  says the endpoint is not published yet, because the service has not started on the new build. The
  second, after the start, is what registers both hosts. Anything that wants one run would have to
  give the installer the port or the token, which §5.9 refuses.
- **Zero errors in the service log since the restart.** Every `ERROR` and `WARN` in the file predates
  it.
- **Codex's own `hook: Stop Failed` line is not ours.** Several Stop hooks run in that
  configuration; the service log has nothing at that moment.

---

## 6. The tasks

**T1 — Merge and push.** `phase-5-mcp-server` into `main`, fast-forward, then push. `origin` is
public: scan the merged diff for user names, machine names, email addresses and real SIDs before
pushing, the way session 04 did. This is first because the soak below cannot start until the
binaries are the ones that will be soaked.

**T2 — The redaction audit.** §8's Phase 6 `[auto]` clause is *"redaction audit finds nothing"*, and
that sentence does not say what the audit is over. Decide it and write it down before running it —
the surfaces that exist now are the log file, every reply document, every MCP tool result, every
tool *error*, `doctor`'s output, and the installer's output. `TestTheLogFileNeverCarriesASecret` and
`TestPhase5GateNoReplyFieldCarriesAUserPath` are the two that already exist; what is missing is
whether anything sweeps them all with one detector, and whether the audit runs over the real corpus
or over fixtures. Note that `doctor` deliberately prints real paths and is not an egress — an audit
that flags it has the wrong scope.

**T3 — The soak.** `[manual]`, 72 hours. It needs the service to stop changing, so it starts after
T1 and after any T2 fix lands. What to record while it runs is a decision: the WAL's size, the
checkpoint log lines, RSS, the MCP session map's growth across host restarts, and the spool depth
are the candidates. The session map is the one nothing bounds — `SessionTimeout` is left at zero on
purpose (see `internal/mcpserver`), and a soak is exactly the instrument that would say whether that
was right.

**T4 — Whatever the audit and the soak turn up.** Nothing else.

---

## 7. Open, and what to do about each

| Open | What to do |
|---|---|
| Backlog 28, `mcp.json`'s DACL | It is a security-relevant row and Phase 6 is the hardening phase, so this is the one backlog row that plausibly belongs to this session. `internal/pipe`'s listener already builds a DACL, so there is a pattern; the cost is a different atomic-write path from `os.CreateTemp` plus rename. Decide it deliberately — §5.9 already accepts the exposure, so implementing it is a design change and not a bug fix |
| Backlog 27, a refusal with no reason | Still a row, and still about the wire. The CLI is the caller that reads a bare rejected `Ack`. A reason field on `Ack` is a Phase 1 contract change |
| Backlog 24, the unimplemented 512 KiB cap | Still a row. Implementing it and correcting §6 are different decisions |
| The 23 remaining backlog rows | Untouched unless a task is already standing in that file |
| `dist-rollback/` is two versions old | It rolls back to Phase 4, not to Phase 5's prerequisites. If you want a one-step rollback from the soak build, take it before you replace anything |
| The MCP session map | Unbounded by design, and Phase 6 is when that stops being a guess |
| Codex `SessionEnd` past the clamp, §7.3 | Still unmeasured; needs a deliberately slow hook in a user's own configuration. Not agent work |

---

## 8. Four things that will bite

1. **`doctor`'s `mcp` host lines do not decide its exit code**, on purpose: a host configuration is
   another product's file, and folding it in would make the exit code depend on whose machine ran
   it. If you change that, `TestDoctorReadsTheTaskWhetherOrNotTheServiceIsUp` fails on a developer
   machine and passes in a clean one, which is the worst way for a test to fail.
2. **A test that runs the installer with `--apply` and a real `PATH` edits your own Claude Code
   configuration.** The `HOME` redirection does not protect it, because that binary resolves its own
   file. `runInstallerWithPath` with an empty `PATH` is the seam, and there is a row in `AGENTS.md`.
3. **Check the linter's exit code, never its summary line.** `0 issues.` and exit 7 is a linter that
   typechecked nothing.
4. **Write any file containing backslashes with a file-write tool and verify the bytes afterwards.**
   `LC_ALL=C grep -nP '[^\x00-\x7F]'` over what you changed is the check. A `python - <<'PY'` heredoc
   whose replacement finds nothing writes nothing, and takes the earlier replacements in the same
   script with it.
