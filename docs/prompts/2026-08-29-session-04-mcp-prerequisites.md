# Session 04 — Engramux Phase 5: the prerequisites, not the server

Phase 4 is merged, installed and live. This session builds everything Phase 5 needs that is not
MCP: the egress that a model reading a tool result makes necessary, the project scoping the search
tool was ruled to have, the bound on the one database connection that a non-human caller makes
reachable, and the two new pipe request types the tool surface will sit on. **No MCP server is
built this session, and `github.com/modelcontextprotocol/go-sdk` is not added.**

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file
carries what they cannot: the state of the work when the session opened, the decisions the
grilling that preceded it took, and what measured them.

Two documents you must read before touching anything. `docs/superpowers/specs/2026-08-27-engramux-1.0-design.md`
is the spec — §3's invariants, §5.2 the pipe, §5.4 the single connection, §5.6 the filesystem
layout, §5.7 search, §5.9 MCP, §6.1 secrets, §8 the phase gates, §10 the open questions.
`docs/superpowers/backlog.md` is new this session and holds 26 deferred rows plus a **Phase 5
prerequisites** section that is the checklist behind §4 below.

---

## 1. Where the work stands

| | |
|---|---|
| Branch | Create `phase-5-prerequisites` from `main`. **Push the branch to `origin` and keep pushing it.** The previous phase spent three sessions with 74 commits on one machine; that does not repeat. `main` is merged into only when the gate clauses this session owns are green |
| `main` | `f52efc4`, **pushed** — `origin/main` is the same commit. Working tree clean, 85 commits |
| Last full verification, at `f52efc4` | `go test -p 1 -count=1 ./...` **14 packages ok, none failing** · pinned linter `0 issues.`, **exit 0** (exit code checked, not the summary line) · both `CGO_ENABLED=0` builds ok, the service with `-H=windowsgui` |
| Not run at `f52efc4` | `./scripts/race.sh`. It last passed at `643db43` with zero data races and exit 0. Everything since is documents and a history rewrite that changed no tree, so it is very likely still green — **run it before you claim it** |
| Phases | 1, 2, 3, 4 done and gated. **5 (MCP) and 6 (hardening plus 72-hour soak) remain.** Phase 6 waits on Phase 5 because the service binary is still changing |
| History | `main` was rewritten this session to take an OS username out of one blob and one commit message, then pushed. The tip tree hash was unchanged by the rewrite, which is what proved no code moved. `origin/main` was and is an ancestor throughout — nothing was force-pushed |
| The live service | Running, PID 11292, task `\Engramux`, about 10,300 events, spool 0, database about 108 MB. It runs the Phase 4 binaries. The suite runs beside it — **do not stop it** |
| SDD workspaces | `.superpowers/sdd/` holds three session directories, gitignored, 78 files. The 28 `review-*.diff` files were deleted this session because every one is regenerable from git; the task reports, research documents and ledgers were kept because they are not |

---

## 2. What counts as done

**Spec §8's Phase 5 gate row is written in full this session, and this session satisfies only part
of it.** That is deliberate: the gate leads the implementation rather than trailing it. Write all
of it — bearer required, cross-origin rejected, bound to `127.0.0.1` only, cross-project isolation,
and the egress clauses below — and mark plainly, in the brief you write for session 05 rather than
in the spec, which clauses have no server to run against yet.

What this session must make green:

- **Cross-project isolation.** Two projects' events in one database; a search scoped to A never
  returns B. Name it what it is — a filtering-correctness gate, not a security boundary. §2 already
  puts the whole Windows SID inside the trust boundary and the caller picks the project, so calling
  it a leakage gate would invite someone to believe an authorization check exists where none does.
- **No MCP-facing field carries a user path.** `status`, `list_sessions`, `get_event` and
  `SearchHit.EventName`.
- **A `get_event` result is bounded**, with the number written down and measured.
- **Ingest is not starved by a reader.** A reader holding the connection does not push a relay past
  its 800 ms post-dial budget when ingest is waiting.

Every one of these gets the break-it step from `AGENTS.md`: write the test, watch it fail,
implement, watch it pass, break that invariant alone, confirm the test reddens, revert. Steps 3–5
are not committed. The previous phase's findings came from that step and from nowhere else.

---

## 3. Measured this session — do not rediscover it

Four adversarial reviews ran against the Phase 5 design before this brief was written: two web
research passes against primary sources, one repository-reading reviewer, and one Codex pass. What
they established is below. **Everything here is verified unless it says otherwise.**

### The transport decision, and why it is HTTP

The design was reviewed hard enough that two independent reviewers recommended rejecting HTTP for
a stdio MCP proxy over the existing named pipe. That recommendation was **not taken**, and the
reason is a measurement neither reviewer had:

**Windows image locks, measured live on the installed binaries.** Opening the installed
`engramux.exe` for write succeeds; opening `engramux-service.exe` for write fails with `EBUSY`,
errno `-4082`. The relay is free precisely because it is spawned per event and exits. A stdio MCP
proxy would be long-lived, so it would lock the relay binary for the life of every host session —
and the installer's refusal path, built last session, would then fire on the relay with advice
(`schtasks /end`) that cannot apply to a host-spawned process. Every update would require closing
every Claude Code and Codex window. Today an update needs only the service stopped.

What the reviewers were actually right about is that most of the damage came from a design error,
not from HTTP: the port and token had been given to the installer, when **spec §5.6 already assigns
`mcp.json` — bound port and bearer token — to the service.** Reverting to the spec removes the
rotation drift, the probe/bind race, and the installer's inability to make its own writes take
effect. That reversion is a Phase 5 decision, recorded in T1, and it is why session 05 is smaller
than it looked.

**What survives as an accepted cost, and must be written that way rather than denied:** binding
`127.0.0.1` is a machine boundary, not a SID boundary. Any process of any locally logged-on user
can reach the endpoint, and the bearer token is the only control. Windows has no Winsock equivalent
of the named pipe's DACL; `SO_EXCLUSIVEADDRUSE` prevents a second process from stealing the port
and is not access control over who connects. **Spec §5.9 currently contains the sentence "I-13 is
not weakened by the same-SID trust boundary." That sentence is false and T1 replaces it.**

### The two hosts

- **Codex CLI, `rust-v0.151.0`.** An HTTP MCP server entry uses `url` where a stdio one uses
  `command`; there is no separate transport key. `http_headers` is a documented map of static
  headers, and OpenAI's own config reference names authorization headers as a credential source
  checked **before** the OAuth fallback engages. **A static `Authorization` bearer written straight
  into `config.toml` works, and no environment variable is needed.**
- The inline `bearer_token` field **is** rejected — that is a different field, reported in
  `openai/codex` issue 19275 against 0.124.0 and **closed 2026-04-24**. Do not confuse the two.
  `experimental_use_rmcp_client` does not exist anywhere in the current source; it was retired in
  January 2026. Streamable HTTP needs no feature gate.
- **Claude Code** supports `--transport http` with `--header`, and the file form `"type": "http"`
  with a `headers` map. **[unverified]** Several open issues report static headers being dropped, or
  an unwanted OAuth registration attempt in preference to the configured header, on some released
  versions. Nobody has reproduced this against the installed build. **Session 05 smoke-tests it
  before the installer depends on it** — this session does not need it.
- **MCP specification revision `2026-07-28`.** Authorization is OPTIONAL and binds only
  implementations that opt into its OAuth 2.1 framework, so a static bearer is spec-legal provided
  the server does not half-claim OAuth conformance. stdio and Streamable HTTP are the only standard
  bindings; the old HTTP+SSE transport is no longer one.

### The SDK, for session 05 rather than this one

`github.com/modelcontextprotocol/go-sdk` **v1.7.0**, released 2026-07-28, is current and v1.x
stable. Verified at that tag: `mcp.NewStreamableHTTPHandler` is the entry point;
`StreamableHTTPOptions.CrossOriginProtection` is **deprecated** in favour of wrapping the handler in
cross-origin middleware, and the compatibility shim that restored the old default-on behavior is
telegraphed for removal in v1.8.0; **that field is nil by default and nil means no protection is
applied**, so §5.9's "cross-origin protection enabled" does not describe a default.
`DisableLocalhostProtection` is a separate, live field that defaults to false, so DNS-rebinding
protection **is** on by default. `MaxRequestBodyBytes` defaults to 4 MiB. `Stateless` defaults to
false, which keeps a session map alive for the life of a process that Phase 6 requires to soak past
72 hours.

Cost: it adds **eight direct and three indirect modules**, `golang.org/x/oauth2` among them, to a
project with four direct dependencies. Its `go` directive is 1.25.0, under this repository's 1.26.0
ceiling. It marshals with `segmentio/encoding/json`, which was compared against `encoding/json` on
the two behaviours §7.1 pins — `RawMessage` compaction and HTML escaping — and found byte-identical.
None of it requires cgo.

### Ports, on this machine specifically

`netsh int ipv4 show dynamicport tcp` reports **start port 1024, 13977 ports** — an ephemeral range
of **1024 to 15000**, not the Windows default of 49152 to 65535. Excluded ranges are empty right
now, but Hyper-V, WSL2 and Docker reserve blocks the moment they start. **The general advice to put
a fixed server port in the registered range 1024–49151 is backwards on this machine**, because that
is where the ephemeral allocator is living. Any port derivation must state its band and be measured
here rather than reasoned from the documented default. `netsh int ipv4 add excludedportrange` can
reserve a block, but that is a host-global network configuration change and is not something the
service does on its own.

### Facts about this repository that contradict things people assume

- **`events_fts` has no `project_id` column.** It was added in Phase 4's T3 and then removed after
  measurement: `MATCH ... AND project_id = ?` planned identically to the unfiltered MATCH, so the
  constraint never reached the virtual-table index, and the column bought one byte. Project scoping
  is therefore a **post-MATCH, pre-LIMIT** join filter on `events`. **Its cost is unmeasured** —
  with many projects, a small project's `limit` forces SQLite to walk a long ranked list to fill it.
  T4 measures this; do not assume it is free and do not assume it is fatal.
- **Spec §6's 512 KiB field cap is documented and not implemented.** The non-test tree has comments
  and `internal/search`'s unrelated `maxTokenBytes`, and nothing that enforces a field cap;
  `readStdin` is explicitly unbounded and says so. A stored payload is bounded only by
  `ipc.MaxFrameLen`. This is backlog row 24 and stays there — deciding between implementing the cap
  and correcting the spec is its own decision, and it is not this session's.
- **`secret.Mask` expands rather than shrinks.** It re-marshals whenever anything matched and
  `encoding/json` HTML-escapes, so a source byte can become six. `ClassUserPath` matched 1,714 times
  across 900 of 902 captures, so essentially every real payload takes the re-encode path. This is
  why `get_event` needs a number before it is written.
- **`internal/ipc/status.go` justifies leaving `DatabasePath` unmasked on three grounds** — one SID
  inside the boundary, the pipe DACL admitting only that SID, and the CLI printing it on the same
  machine. Every one is void when the reader is a model. The same shape sits at
  `internal/service`'s `EventName` handling, which the spec records as a known I-10 perimeter.
- **The installer writes each host configuration with a direct `writeFileSync`** where §5.6 requires
  a temporary file and an atomic rename, and it fully rewrites the Claude file before it parses the
  Codex one. Backlog row 25; T6 closes it.
- **A reply-write timeout was observed live**, once, on 2026-08-29: one `engramux status` failed
  with `read the reply: ipc: read frame length: EOF` while the service logged
  `pipe: write reply` / `ipc: write frame length: i/o timeout` at the same instant. Immediate retry
  succeeded, service up throughout. One human, one CLI invocation, no MCP. Backlog row 26. T5 looks
  at it; if it cannot be reproduced it stays a row rather than becoming a fix.
- **`busy_timeout` does not bound the contention T5 is about.** It governs SQLite lock contention,
  and §5.4 leaves no second connection to contend with. Reader-versus-ingest is `database/sql` pool
  waiting, which `busy_timeout` does not touch, and the pipe handler receives the service root
  context rather than the client's deadline.

---

## 4. Tasks

Six, in order. Each is one implementer and one fresh reviewer.

**T1 — The spec.** Rewrite §5.9 around the decisions below. Replace the false sentence named in §3.
Write §8's Phase 5 gate row in full. Confirm §5.6's assignment of `mcp.json` to the service and say
in §5.9 that the service — not the installer — binds the port, mints the token, and writes that file
with the atomic rename §5.6 already requires. Close §10's open question 3 with the four tools, and
open question 1 with the `doctor` behavior in T6. Record the contention decision from T5 and the
port-band problem from §3. Everything unmeasured is marked `[unverified]` with what rests on it.
No code blocks.

**T2 — Egress on what already exists.** One `status` reply shape, masked, used by both the CLI and
MCP: the CLI sees the masked path too, because `doctor` is where a real path belongs and T6 is
putting it there. Apply masking to `SearchHit.EventName`, closing the perimeter the spec currently
only records. The gate clause is that no reply field carries a user path.

**T3 — Two new pipe request types.** `get_event` and `list_sessions`, at the pipe layer only; no
MCP, and CLI exposure only if it costs nothing. `get_event` takes an id and a project and checks
them **together** — `events.id` is a global relay-minted UUID, so an id alone reads across
projects. It returns the whole payload masked, under an explicit bound that is measured, not
guessed; note that the reply frame is 4 MiB and masking expands. `list_sessions` masks
`projects.root`, which is exactly the shape `ClassUserPath` matches.

**T4 — Project scoping.** Add the project field to `ipc.SearchRequest`. **Empty means all
projects** — that keeps the wire to one meaning and does not break a single existing CLI
invocation; the "cannot be omitted" constraint belongs in the MCP tool schema, where the SDK
enforces it structurally, and session 05 puts it there. Service-side filter, with the cost measured
per §3. Validate the project argument: **absolute paths only, UNC rejected, walk depth bounded** —
`project.Identify` stats each ancestor and takes no context, and `internal/store`'s ingest path
already records that a dead UNC host or a vanished mapped drive can stall the service. The CLI keeps
its **global default** and gains `--project`, which resolves a relative argument to an absolute path
before sending. Do **not** add `--all`: `cmd/engramux/search.go`'s own doc comment says nothing is
interpreted there, not a leading dash, and a flag that un-scopes would also silently change what
every existing invocation returns.

**T5 — The one connection.** A query deadline on reads, a read concurrency of one, and **ingest
priority** — a waiting ingest goes before a queued read. I-04 is why the product exists; a reader
must not push a writer out. Three mechanisms, each verified separately. Look at backlog row 26 here.

**T6 — Operations.** `doctor` reports everything knowable without the service when the service is
down — scheduled task registration, binary paths, spool depth, the last log line — marks the
service section unreachable, and exits 1; the moment it is most needed is the moment it is broken.
`doctor` also has the service read the live index's tokenizer and **compare it against what the
migration expects**, reporting agreement or disagreement rather than printing a string for a human
to compare; I-07 leaves the service as the only thing that can look, and backlog row 18 has gone
unchecked because nobody was going to do the comparison by eye. Installer: temporary file plus
atomic rename for both host configurations. **No TOML writer this session** — there is no MCP entry
to write yet, so it would be unused code designed against an unknown shape.

---

## 5. Open, and what to do about each

| Open | What to do |
|---|---|
| The project-filter cost with many projects | **Measure it in T4.** Nobody has. If it is bad, the answer is a decision recorded in §5.7, not a silent index |
| `get_event`'s bound | **A measured number in T3**, in the spec, with what it was measured against. Every other egress here is bounded — the excerpt to 240 runes, the event name to 64 |
| Claude Code dropping static headers | **[unverified]**, and not this session's problem. Session 05 smoke-tests the installed build before the installer relies on it |
| Backlog 24, the unimplemented 512 KiB cap | Stays a backlog row. Implementing it and correcting §6 are different decisions and neither is in scope |
| The reply-write timeout, backlog 26 | T5 looks; one observation is not a reproduction. If it cannot be reproduced, leave the row |
| Phase 6 soak | Waits. The service binary changes again in session 05 |
| Codex `SessionEnd` behavior past the clamp, §7.3 | Still unmeasured; needs a deliberately slow hook in a user's own configuration. Not agent work |
| The remaining 23 backlog rows | Untouched unless a task is already standing in that file |

---

## 6. How this session runs

Exactly as sessions 02 and 03: `superpowers:subagent-driven-development`, a fresh implementer per
task, a fresh reviewer per task, fix rounds resumed on the same implementer, scoped re-reviews, the
ledger as the recovery map. **Measure before dispatching.** Every task in the previous phase that
went smoothly had its load-bearing facts measured first; every task that needed a fix round had a
fact nobody had checked.

The session ends with a live install, the way session 03 did: rebuild `dist/`, **you ask the user to
run the installer's apply step** — an agent does not edit host configuration — and then verify
against the real database. There is no migration this session, so the rollback is the **previous
`dist/` binaries**, not a database snapshot; keep a copy before overwriting.

Four things that will bite, all of them observed:

1. **Write any file containing backslashes with a file-write tool.** A shell heredoc collapsed `\\`
   in a Go test literal last session, and an inline `node -e` with escaped backslashes failed to
   parse this session. `AGENTS.md` says this and is right both times.
2. **Grep every commit for a username before calling it done.** One got into a Phase 4 commit and
   survived a review that was checking escapes rather than names. This session had to rewrite 55
   commits to take it out of history, which was cheap only because nothing had been pushed. It is
   pushed now.
3. **Check the linter's exit code, never its summary line.** It prints `0 issues.` and exits 7 when
   it typechecked nothing.
4. **A review can be right about the standard and wrong about the fact.** This session's reviewer
   claimed Codex has no way to send a static bearer, which would have killed the transport; a
   follow-up against primary sources showed it had confused two different config fields. Verify a
   finding before you act on it, and verify it against the source the finding is about.

---

## 7. First action

Read `docs/superpowers/backlog.md`, the **Phase 5 prerequisites** section first — it is §4's
checklist in the words of the reviews that produced it. Then read spec §5.6, §5.9 and §8's Phase 5
row, which are what T1 rewrites. Then create the branch and push it before the first commit, so the
work exists somewhere other than this machine from the start.

Then T1. The spec leads; the code follows it.
