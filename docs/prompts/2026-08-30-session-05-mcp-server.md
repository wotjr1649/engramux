# Session 05 — Engramux Phase 5: the MCP server

Session 04 built everything Phase 5 needs that is not MCP. This session adds the server: the
`github.com/modelcontextprotocol/go-sdk` dependency, the Streamable HTTP handler, the four tools,
`mcp.json`, and the installer entries that point the two hosts at it. **Nothing else in Phase 5 is
left**, so what is not on the list below is Phase 6.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file
carries what they cannot: the state of the work when the session opened, and which of §8's Phase 5
gate clauses have something to run against.

Read `docs/superpowers/specs/2026-08-27-engramux-1.0-design.md` **§5.9 first** — it was rewritten
last session and it is the whole design of what you are building. Then §5.2 the request set, §5.6
the filesystem layout, §8's Phase 5 row, §10's closed questions 1 and 3.

---

## 1. Where the work stands

| | |
|---|---|
| Branch | `phase-5-prerequisites`, pushed. It has not been merged: §2 below says what merging it waits on |
| `main` | `60008a2`, pushed. Untouched last session |
| Last full verification, at `d8a580d` | `go test -p 1 -count=1 ./...` **14 packages ok** · pinned linter `0 issues.`, **exit 0** (exit code checked, not the summary line) · both `CGO_ENABLED=0` builds ok, the service with `-H=windowsgui` · **`./scripts/race.sh` exit 0, no data races** — it had not been run since `643db43` and now has been |
| `dist/` | Rebuilt at `d8a580d` and **installed**. `dist-rollback/` holds the Phase 4 binaries it replaced; that directory is the rollback and is gitignored by `*.exe` |
| The live service | Running the binaries this branch built, restarted 2026-08-30 00:52. It drained the 6 events the relay spooled during the stop, which is I-04 end to end |
| Phases | 1, 2, 3, 4 done and gated. **5 is half done**, 6 waits on it |

---

## 2. §8's Phase 5 gate: what is green and what has nothing to run against

The gate row is written in full in the spec. Session 04 satisfied four of its seven clauses; the
three transport clauses had no server to run against and are this session's.

| Clause | State |
|---|---|
| Bearer required — no token and a wrong token both refused | **Not written.** No server exists |
| Cross-origin rejected | **Not written.** Against the middleware, *not* `CrossOriginProtection`, which is nil by default and applies nothing |
| Bound to `127.0.0.1` only | **Not written** |
| No MCP-facing field carries a user path | **Green.** `TestPhase5GateNoReplyFieldCarriesAUserPath` in `internal/service` sweeps the marshalled reply with `secret.Detect` rather than naming fields, so a field the MCP layer adds is covered the moment it is in a reply |
| `get_event` bounded by a measured number | **Green.** 1 MiB, measured; `TestMaskingExpandsAndTheGetEventBoundHoldsOverTheCorpus` in `internal/secret` is the reproduction |
| Cross-project isolation | **Green**, at both surfaces: `TestPhase5GateGetEventChecksTheProjectWithTheId` and `TestPhase5GateASearchScopedToOneProjectNeverReturnsAnother` |
| A reader does not push ingest past 800 ms | **Green.** `TestPhase5GateAReaderDoesNotPushIngestPastItsBudget`, measured with and without the gate |

**The branch merges when the three transport clauses are green**, not before.

---

## 3. What session 04 built, so you do not rebuild it

- **Two new pipe request types**, `GetEvent` and `ListSessions`, plus `Doctor` — which was already
  in §5.2's set and had never been implemented. All three are routed, tested, and reachable from
  the CLI (`engramux event <id> [project]`, `engramux sessions [project]`, `engramux doctor`).
- **`ipc.SearchRequest.Project`.** Empty means every project. The CLI has `--project <path>`, read
  in first position only.
- **`project.FromArgument`** — the trust boundary for a caller-supplied path: absolute only, UNC
  refused, the walk bounded at 32 ancestors. `project.Identify` is deliberately **not** bounded;
  `internal/project/argument.go` says why at length, and the short version is that the ingest stall
  is a slow stat rather than many stats.
- **The masked egress.** `StatusReply.DatabasePath`, `SearchHit.EventName` and `Cell.EventName` are
  masked. There is one status shape and the CLI sees the masked path too.
- **The read gate**: a 4 s query deadline, read concurrency of one, ingest priority. It is created
  in `run` and passed to `handlers`, which is the production wiring a test can hold.
- **`doctor`** reports the registration, the local state (both binary paths, the data directory, the
  spool depth, the last log line) and the service, marks the unreachable half, and exits 1. Its
  service half carries the **real** database path — the one place an unmasked path is right — and
  the tokenizer comparison that closes backlog 18.
- **The installer** writes through a temporary file, an fsync and a rename, and plans both host
  configurations before writing either.

Two backlog rows were closed by tests and deleted: 18 (nothing verified the live tokenizer) and 25
(direct writes, split state). Row 26 was closed by a **fix**: the reply-write timeout observed once
on the live service was `serveConn` setting one deadline before the request frame that covered the
read, the handler and the write. Reproduced with a slow handler, fixed with a second deadline.

---

## 4. Measured last session — do not rediscover it

Everything below is in §7.1 with its reproduction. It is repeated here only because it is what would
otherwise be measured again.

- **Masking expands.** Over 901 corpus payloads: 881 grew, 19 shrank, 1 unchanged; largest masked
  **173,609 B**; worst expansion **1.1220×**; **901 of 901** still valid JSON, which is what lets a
  reply splice one in verbatim instead of escaping it into a string. `get_event`'s 1 MiB bound is
  6.0× that largest and a quarter of the reply frame.
- **The project filter is not the problem it looked like.** 19,503 events, 40 projects, `LIMIT 20`.
  A term in every document: unscoped 24.2 ms, scoped to a 500-event project 26.4 ms, scoped to a
  3-event project 39.0 ms. A term in one document in a hundred: 1.08 / 0.64 / 0.47 ms. **`ORDER BY
  rank` makes FTS5 score every matching document before the first row**, so `LIMIT` stops nothing
  early for the unscoped query either — the match set decides the cost. At a realistic selectivity a
  scoped search is *faster*, because phase two builds an excerpt per hit.
- **The read gate, measured through the production wiring.** 20 ingests against 96 concurrent
  searches over 4,000 events: median **1.5 ms**, worst **6.6 ms**. Without the gate: **722 ms** and
  **2.66 s**.
- **Two things the break-it pass found**, both now in §5.9. Ingest priority is worth less than it
  looks once read concurrency is one, because `database/sql` already serves waiting acquisitions in
  arrival order; and the contention gate passes on read concurrency **alone**, so the ingest
  handler's own wiring needed a fourth test after a deliberate break left every other test green.
- **A deadline is sized against the client, not against the transport.** `readDeadline` was set to
  2 s to match the pipe's connection deadline, and that refused a real `engramux status` within half
  an hour of the install — a cold read of a 108 MB database, where the same command warm takes
  164–499 ms. It is 4 s now, with 1 s of the CLI's 5 s left for the reply. **The bug was found by
  running the thing, not by a test**, and the test that exists now only holds the two ends apart.
- **The relay binary must not link the SQLite driver.** `cmd/engramux` is the hook relay as well as
  the CLI. Importing `internal/service` for one function took it from 3,828,736 to 8,002,048 bytes.
  Watch the binary size when you add the SDK — it goes in `cmd/engramux-service` only.

---

## 5. It is installed, and here is what it answered

The user ran the apply step at the end of session 04 — stop, copy, start — and the whole surface was
exercised against the real database. What it said, because these are the numbers nobody had:

- **`doctor`, exit 0.** `index tokenizer  agrees with the migration ("unicode61 remove_diacritics 2")`.
  That is backlog 18's question, asked of a real installation for the first time, and the answer is
  agreement. The local half printed both binary paths, the data directory, a spool of 0, and the log
  line showing the drain replaying the 6 events captured while the service was down.
- **`status` shows the masked path and `doctor` the real one**, which is the split §5.9 specifies,
  seen working rather than reasoned about.
- **`sessions` and `search --project .`** resolved this repository's working directory to its project
  and answered from it. `event <id>` returned a 12,301-byte masked payload tagged `user-path`; the
  same id under another project answered "no such event in this project".
- **The UNC guard fires on both spellings.** Verified from a script file rather than an inline
  command, because the first attempt appeared to show a UNC path being accepted and that was an
  artifact: the backslashes had been collapsed before any shell saw them, so what reached the service
  was a rooted-but-driveless path, which `filepath.Abs` correctly resolves against the current drive
  and which is correctly accepted. Git Bash preserves a single-quoted `\\host\share\dev` exactly. The
  guard was never wrong, and neither was the shell — the command that tested it was. §8's first item
  is the same failure in a third costume.

The rollback is `dist-rollback/`. There was no migration, so a database snapshot is not the rollback
and is not needed.

---

## 6. The tasks

Six, in order, each one implementer and one fresh reviewer.

**T1 — The dependency, alone.** Add `github.com/modelcontextprotocol/go-sdk` v1.7.0 and nothing
else, in its own commit, and record what `go.mod` actually gained against §5.9's claim of eight
direct and three indirect modules. Check the two `CGO_ENABLED=0` builds and both binary sizes before
and after; §4's last point is why.

**T2 — The handler and the three transport clauses.** `mcp.NewStreamableHTTPHandler`, bound to
`127.0.0.1` on port 0, with the bearer check and cross-origin middleware. Write the three gate tests
first — they are §2's three missing clauses and they are what the branch merges on.
`CrossOriginProtection` is **deprecated and nil by default**; wrap the handler instead. Do not
half-claim OAuth conformance: no `WWW-Authenticate` challenge naming metadata the server does not
serve.

**T3 — `mcp.json`.** The service binds, mints the token and writes the file with §5.6's temporary
file and atomic rename. The port is sticky: reuse the one the previous start recorded, fall back to
0 when that bind fails. `doctor` reports the disagreement when a host configuration's URL is stale.
The token is a §6.1 secret: never logged, never printed by a CLI command, never in an error message.
§5.9 leaves one thing `[unverified]` — what file permissions `mcp.json` actually carries once
written, since Go's mode is advisory on Windows. Measure it or leave it marked.

**T4 — The four tools.** `search`, `get_event`, `list_sessions`, `status`, over the pipe request
types session 04 built. The project argument is **required in the tool schema**, where the SDK
enforces it structurally; it stays optional on the wire. Nothing here reimplements a query.

**T5 — The hosts, smoke-tested.** Claude Code's static-header support is the one `[unverified]`
thing the installer would depend on: several open issues report headers being dropped or an OAuth
attempt preferred, and nobody has reproduced any of it against the installed build. **Smoke-test it
before the installer writes anything.** Codex's `http_headers` is documented and corroborated by a
closed issue and is also not smoke-tested.

**T6 — The installer entries.** Now there is a shape to write against, so the TOML writer session 04
deferred is in scope. Same rules as the hook configuration: temporary file, atomic rename, plan
before write.

---

## 7. Open, and what to do about each

| Open | What to do |
|---|---|
| Claude Code dropping static headers | **T5, before the installer depends on it.** It is the one thing that could still change the transport |
| `mcp.json`'s file permissions on Windows | **T3.** Measure the ACL that results, or leave §5.9's `[unverified]` standing with what rests on it |
| Backlog 27, a refusal with no reason | **Read it before T4.** A refused request answers a bare rejected `Ack`, so a caller that sent a bad project learns only that it was refused. A person guesses; a model cannot. The tool surface is the first caller that has to, so decide there whether the tools carry their own error text or whether the wire gains a reason - and if it is the wire, that is a design change to a Phase 1 contract the relay depends on |
| Backlog 24, the unimplemented 512 KiB cap | Still a row. Implementing it and correcting §6 are different decisions and neither is Phase 5's |
| The 23 remaining backlog rows | Untouched unless a task is already standing in that file |
| Phase 6 soak | After the merge. The service binary stops changing when this session ends |
| Codex `SessionEnd` past the clamp, §7.3 | Still unmeasured; needs a deliberately slow hook in a user's own configuration. Not agent work |

---

## 8. Four things that will bite

All observed last session.

1. **Write any file containing backslashes with a file-write tool**, and **verify the bytes
   afterwards**. A `Write` call turned a six-character escape for U+001B into a raw ESC byte inside
   a Go comment; an `Edit` turned a pair of apostrophes into U+201D; and the sentence you are
   reading did it a third time. All three compiled or rendered.
   `LC_ALL=C grep -nP '[^\x00-\x7F]'` over the files you changed is the check.
2. **A `python - <<'PY'` heredoc that asserts its replacement found nothing writes nothing** — and
   the earlier replacements in the same script are lost with it. Two edits were silently skipped
   that way, one of them the CLI switch, which the linter caught only as `func is unused`.
3. **Check the linter's exit code, never its summary line.** `0 issues.` and exit 7 is a linter that
   typechecked nothing.
4. **A test that hangs is a test that detected something**, and it will still cost you the package's
   whole `go test` timeout. A deadline test needs a backstop longer than the deadline it asserts,
   with the elapsed time asserted alongside the error.
