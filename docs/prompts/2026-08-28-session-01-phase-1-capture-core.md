# Session 01 — Engramux Phase 1: the capture core

Implement Phase 1 of Engramux. Nothing before this session wrote any Go; this session writes the
first line of it.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules — commands, gotchas, boundaries, document
ownership, output language — are already in your context. This file does not repeat them. It
carries what they cannot: the state of the work, the decisions already made and what measured
them, and how this session runs.

---

## 1. What counts as done

Phase 1 is defined in §8 of `docs/superpowers/specs/2026-08-27-engramux-1.0-design.md`. Its gate has
four clauses, and the session is finished when all four pass with a real command you ran and watched:

1. The four fixtures round-trip **byte for byte**.
2. `PRAGMA foreign_key_check` returns empty.
3. A kill between `COMMIT` and the ACK replays **exactly once**.
4. A runtime-generated secret is **tagged on ingest and absent from the log**.

The four fixtures are named in §8. They were chosen so that all three host-detection paths and all
three `tool_response` shapes are exercised now rather than discovered in a later phase.

Partial credit is not a thing here. A clause that "should" pass has not passed.

---

## 2. State right now

**No Go code exists.** No `go.mod`, no `cmd/`, no `internal/`, no tests, no `.golangci.yml`, no
`.github/`, no `LICENSE`, no `README.md`. Everything tracked is documentation, evidence harnesses,
and one script — run `git ls-files` and you will see the whole repository in one screen.

| | |
|---|---|
| Branch | `main`, 22 commits, **11 unpushed** |
| `origin` | a public GitHub repository. Do not push without asking |
| Corpus | `.capture/fixtures-raw/`, 902 raw captures, gitignored, local only, never committed |
| Go | 1.27.0 windows/amd64, `CGO_ENABLED=0` is the environment default |
| Linter | golangci-lint 2.12.2 — the **v2** config schema (`version: "2"`, `linters.settings.*`). v1 examples will not load |
| Race | mingw-w64 GCC 16.2.0 extracted beside the repo; `scripts/race.sh` finds it. Verified in both directions: it catches a deliberate `x++` race and stays quiet on the mutex-guarded version |
| Editor tooling | gopls v0.23.0, active via the `gopls-lsp` plugin |

### Documents, and who owns what

- `docs/superpowers/specs/2026-08-27-engramux-1.0-design.md` (rev.4) — **the design**. Owns
  decisions, invariants, budgets, measurements. It deliberately holds no Go signatures, no DDL, and
  no package tree, because code owns those and three earlier revisions were discarded for trying to
  own them.
- `docs/evidence/` — nine harnesses, each its own module, behind the spec's `[verified]` claims.
  `docs/evidence/crash/main.go` in particular is the technique for gate clause 3: the child commits,
  reports on stdout, then blocks, and the parent calls `TerminateProcess`. Reuse the shape.
- `docs/chatgpt/` — archive, superseded in full. Do not read it for current design.
- `docs/superpowers/plans/` does not exist, and this session does not create it. See §3.

---

## 3. How this session runs

**Use `superpowers:subagent-driven-development`.** A fresh subagent per task, review between tasks.
This is the project standard, not a suggestion, and it exists because a single long context is how
the earlier attempts drifted.

**One adaptation, and getting it right matters more than anything else here.** That skill normally
executes a plan document. This project will not have one. `AGENTS.md` forbids code blocks in
documents, and a 5,748-line plan was deleted from this repository for being exactly that — code
nobody had run, dressed as instructions. So:

> The task list in §4 **is** the plan input. Each entry is a brief: deliverable, gate, invariant
> guarded, files touched. **No task brief contains code.** The subagent writes the code; the brief
> says what the code must do and how you will know it did.

If you find yourself writing a document with Go in it, you have reproduced the failure this project
already had three times. Stop and write the code instead.

### Per task

1. Dispatch a fresh subagent with the brief from §4, plus the two or three spec sections it needs.
   Do not hand it this whole file.
2. It works TDD, then **breaks it**: after the test goes green, deliberately break the
   implementation — that invariant only — confirm the test goes red, revert, confirm green. Steps
   3–5 never get committed. This exists because an earlier suite survived 15 of 20 deliberate
   mutations; its assertions checked presence rather than value.
3. Review the diff yourself before the task closes.
4. **Cross-check with Codex** (below).
5. Commit. One task, one commit, English message.

### Cross-checking with Codex

Use the `codex:codex-rescue` agent. It is a forwarder — one call, and it returns Codex's output
unchanged — so the whole review has to fit in the prompt you hand it.

- **After each task**, ask Codex for a read-only review of that task's diff. Say "read-only, no
  edits" explicitly; the agent otherwise defaults to write-capable. Give it the commit range and
  the invariant the task guards, and ask what would make the test pass while the behavior is wrong.
- **When stuck for three attempts without new evidence**, hand Codex the diagnosis rather than a
  fourth attempt. Two independent readings of the same failure beat one reading four times.
- **Treat its findings as claims, not verdicts.** Every review in this project's history has been
  partly wrong, including the ones that were mostly right — one confidently reported that a hook
  did not exist while it was actively firing. Verify before you act on a finding, and say so when
  you do.

### Where the reviews go

Findings that change the design go into the spec. Findings that would bite the next agent go into
`AGENTS.md`'s gotcha table. Findings that are just this task's bug go into the code and nowhere
else — the documents are not a changelog.

---

## 4. Task list

Thirteen tasks. This decomposition is mine, not validated by execution; adjust the boundaries when
the code tells you they are wrong, and say so when you do.

**T1 — Module and scaffolding.** `go mod init github.com/wotjr1649/engramux`. `.golangci.yml` on the
v2 schema, enabling at least `errcheck`, `sqlclosecheck`, `rowserrcheck`, `noctx`, `errorlint`,
`gosec`. Do **not** enable the `std-error-handling` exclusion preset — it excludes every unchecked
`.Close()`, which is most of what you want caught here. *Gate:* `go build ./...`, `go test -p 1
./...`, and `golangci-lint run` all succeed on an empty tree.

**T2 — The four fixtures, synthesised.** Per §6.2: shape copied from real captures, every value
invented, nothing raw committed. Plus the shape test that reads `.capture/fixtures-raw/` and asserts
each fixture's JSON key paths and types still occur in the real corpus — skipped when the corpus is
absent, so a contributor can run everything else. *Gate:* the shape test passes locally and skips
cleanly when `.capture/` is renamed away.

**T3 — Host detection.** The three-step rule from §4.3. *Guards:* I-12. *Gate:* all four fixtures
classify correctly. *Break it:* delete step 3 and confirm the `codex SessionEnd` fixture fails —
that is the cell a two-step rule loses entirely, 13 of 13.

**T4 — Secret detection and tagging.** The classes in §6.1. Tag, never erase. *Guards:* I-10.
*Gate:* a secret generated **at test runtime** is tagged with the right class; the generator has its
own test asserting the detector matches what it produces, so a generator that quietly emits
something harmless cannot make this vacuous. Nothing secret-shaped is written to the repository.
*Break it:* neuter one class and confirm only that class's test fails.

**T5 — Envelope and frame codec.** Length-prefixed UTF-8 JSON with a request type, per §5.2. Length
validated before allocation. *Gate:* a golden file, not a struct round-trip — marshalling and
unmarshalling with the same struct round-trips happily even when the JSON tags are wrong.

**T6 — Storage: migrations, DSN, pragma readback.** goose with `embed.FS`; migrations live under the
package that embeds them. Triggers wrapped in `-- +goose StatementBegin`/`End`. The DSN from §5.4,
single connection. *Guards:* I-07, I-11. *Gate:* every pragma is read back and compared; a
deliberately misspelled pragma fails startup rather than being silently ignored. *Break it:* remove
one readback and confirm the misspelling test goes green — that is the bug, and the test must catch
its own absence.

**T7 — Schema and `foreign_key_check`.** The first migration. `events.id` is the relay-minted
UUIDv7; there is no `idempotency_key` column (§5 below explains why); `tool_use_id` is nullable and
non-unique; `privacy_class` carries T4's tag. FKs to `projects` carry `ON DELETE CASCADE`. *Gate:*
gate clause 2.

**T8 — Project identity and session upsert.** Identity per §6: same repository, same worktree,
surviving drive-letter case and trailing separators. Sessions are created lazily on first event —
in the corpus, only 9 of 19 sessions have a `SessionStart` at all, and three claude-code sessions
have none. Any git subprocess gets the no-window creation flag (I-02). *Gate:* two paths differing
only in case and trailing separator produce one project row.

**T9 — Ingest transaction.** Idempotent on `events.id`. *Guards:* I-04, I-05. *Gate:* the same event
ingested twice leaves one row and ACKs `committed` both times — a duplicate is not an error, and
`rejected` would mean permanent loss.

**T10 — Named pipe server.** `ListenPipe` on the fixed name from §5.2, DACL, accept loop. Verify the
peer PID on the accepted connection and **never cache it** (§7.4-1). *Guards:* I-09. *Gate:* a
second listener on the same name is refused. Give every test a unique pipe name and close it in
`t.Cleanup`.

**T11 — Relay.** stdin → detect → envelope → dial → ACK verification → spool on any failure → **exit
0 on every path, including panic**. ACK counts as success only when version, status `committed`, and
the returned ingest ID all match. Budget per §5.3: 1 s total, 200 ms dial, 800 ms after.
*Guards:* I-03. *Gate:* every failure mode exits 0, including a panic injected in the adapter.

**T12 — Spool and drain.** Bounded on count, bytes and age; a repeatedly failing record is
quarantined, not retried forever. Drain is a context-aware bounded batch. *Gate:* gate clause 3, and
the kill happens after `COMMIT` returns and before the ACK is written — see
`docs/evidence/crash/main.go`.

**T13 — Egress filter and the Phase 1 gate.** The log filters on `privacy_class`. Rebuild the record
with `slog.NewRecord` and `AddAttrs`; assigning to `a.Value` inside a `Record.Attrs` callback is a
no-op and leaves the secret in the log. *Gate:* all four clauses of §8 Phase 1, run together, from
clean.

---

## 5. Decisions already made, and what measured them

Do not reopen these without new evidence. Each cost a measurement.

| Decision | Evidence |
|---|---|
| `events.id` (relay UUIDv7) **is** the idempotency key; there is no `idempotency_key` column | The best host-derived key collapses 902 captures into 762 groups — `UNIQUE` would reject 15.5% of real traffic, with 114 `SubagentStop` rows sharing one key because that event carries no identifier at all. Combined with "duplicate → ACK committed" it would have ACKed distinct events and dropped them |
| Secrets are tagged, not erased | Erasing destroys the memory the product exists to keep. Tagging makes false positives free, so detection is deliberately generous |
| Fixtures are synthesised | Real captures carry the user's work: Korean alone appears in 228 of 902 through `tool_input.command`, `tool_response.stdout`, `tool_input.content`. `origin` is public |
| Break-it secrets are generated at runtime, never committed | A committed `known-bad` file puts credential-shaped strings in a public repository, trips push protection, and is indistinguishable from a real leak |
| The pipe name is fixed, no nonce, no discovery file | 20 rounds × 30 racing processes gave exactly one winner every round; all 580 losers got `Access is denied`. A per-start nonce would have let all 30 succeed |
| Peer PID verified at accept, never cached | Zero PID reuse inside 1 s at 197 spawns/s — a hundred times real load — but 48.93% overall. A PID held past a second means nothing |
| Checkpoint is a straight `TRUNCATE` | Cold `TRUNCATE` is ~0.54 ms/MiB, 32.5 ms at the 64 MiB threshold, `busy=0` every time. The `PASSIVE`-probe policy was inherited from a multi-connection design |
| `locking_mode=exclusive`, one connection | No `-shm` file, second connection refused, 92% throughput, zero errors under load. And the lock does **not** survive process death — 20/20 reopened after `TerminateProcess`, so the service can restart after a crash |
| 1.0 is pull-only; `SessionStart` emits nothing | Without an LLM there is nothing worth pushing, and pushing it costs tokens every session |
| Search targets raw event text | Extracted titles are `<tool>: <basename>`; the content lives in the raw fields |

---

## 6. Not verified — and what to do about each

| | What to do |
|---|---|
| Codex clamps `SessionEnd` at 3 s | **Nothing.** Demoted from load-bearing: the relay's own ceiling is 1 s and measured p95 is 14.2 ms. Measuring it needs a deliberately slow hook in the user's Codex config, which an agent does not install |
| The relay round-trip budget in §5.3 | The old p50/p95 numbers were removed because no harness survived. **T11 re-measures them.** Record what you get in §7.1 with the harness committed beside it |
| Phase 4's recall target | The justification for the old target was measured false. **Not this session** — but do not treat §8 Phase 4 as settled |
| CI runner toolchain adequacy for `-race` | `scripts/race.sh` runs the adequacy check itself and fails loudly. It will reveal itself the first time CI exists. CI is deliberately deferred until after this vertical slice |
| The task decomposition in §4 | Mine, not validated by execution. Adjust and say so |

---

## 7. Things that will bite you that `AGENTS.md` does not say

- **`.superpowers/sdd/` may reappear.** The subagent-driven skill writes task briefs there. It is
  gitignored, which means it survives sessions invisibly. A brief from a previous, deleted plan was
  found there instructing `go mod edit -go=1.24` — below the goose floor, straight into a
  `stdversion` failure. If you find briefs there you did not write, delete them.
- **Only 13 of the 22 host×event cells exist in the corpus.** The other 9 need captures only the
  user can produce, since an agent does not edit host configuration. That is a Phase 2 dependency,
  not a Phase 1 one, but do not plan around fixtures that cannot be made.
- **The `-p 1` guard is a hook in the user's own Claude Code settings, not in this repository.** It
  will stop you here and will not stop a contributor. Do not read its absence elsewhere as
  permission.
- **`docs/evidence/` modules are nested.** They stay out of the root module's `./...`. Do not "fix"
  that.

---

## 8. First action

Start with T1. Before dispatching anything, read §8 of the spec — the phase table and the fixture
table beneath it — so the gate you are building toward is the one actually written down.

When Phase 1's four clauses pass together from clean, stop and report. Do not begin Phase 2, and do
not push; the repository is public and 11 commits are already waiting on a decision that is not
yours.
