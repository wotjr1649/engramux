# Engramux

Engramux captures Claude Code and Codex session hook events into SQLite and serves them back
through FTS5 and MCP. One service per Windows user multiplexes every concurrent session.

It exists because `thedotmack/claude-mem` does this job but breaks constantly on Windows.
Engramux is a reference reimplementation in Go — not a fork.

**Full behavior parity with claude-mem is not a 1.0 goal**, and its architecture is never a model.
Adopting an individual compatible behavior is fine when an active spec asks for it — migration and
interoperability are legitimate later goals. Do not copy its process, I/O, or installation
architecture: that is the part that fails on Windows, and mirroring it reproduces the failure. Its
data model, tool surface, and search UX are Windows-neutral — read those freely, but treat what you
find as a candidate, never as a justification. Decisions are justified by the Windows constraint and
by measurement.

## Commands

Build targets are the directories under `cmd/`. The service binary must be linked as a GUI
subsystem binary; without `-H=windowsgui` it pops a console window every time it spawns a child.

```bash
CGO_ENABLED=0 go build -ldflags "-s -w"               -o dist/engramux.exe         ./cmd/engramux
CGO_ENABLED=0 go build -ldflags "-s -w -H=windowsgui" -o dist/engramux-service.exe ./cmd/engramux-service
go test -p 1 ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run
./scripts/race.sh            # test suite under the race detector
go test -p 1 -count=1 -run TestPhase1Gate -v ./internal/spool/   # spec §8's Phase 1 gate
go test -p 1 -count=1 -run TestPhase4Gate -v ./internal/search/  # spec §8's Phase 4 gate
go test -p 1 -count=1 -run TestEveryCandidateDocumentIsReachable -v ./internal/search/
```

`TestPhase1Gate` runs Phase 1's four gate clauses in one pass over one database it builds from
an empty directory. It is in `internal/spool` because clause 3 kills a child that has to be a copy
of the running test binary, and that package's `TestMain` is what turns a re-executed copy into
one.

`TestPhase4Gate` runs spec §8's five known-item classes and the precision assertion twice over:
once over the fixtures, and once over `.capture/`'s captures, which skips itself when that
directory is absent. Before pasting its `-v` output anywhere, read the table row about it below.
The second command is not covered by the first — `TestEveryCandidateDocumentIsReachable` sweeps
every candidate document of every class rather than the gate's 25-per-class sample, under two
tokenizer arms, and it is what priced the stemmer out of `00002`.

`CGO_ENABLED=0` is written out rather than inherited: the environment default happens to be 0 on the
machine this was written on, which means a build that violates the boundary would look fine here.

The linter is invoked through `go run` at a pinned version rather than as whatever
`golangci-lint` is on `PATH`, and that is not fussiness. A golangci-lint **built with Go 1.26**
cannot typecheck Go 1.27's `math/rand/v2` — it uses generic methods — so every package that
imports `crypto/rand`, directly or transitively, makes it print `0 issues.` **and exit 7**.
`go run` builds it with the local toolchain, so the linter and the standard library it is
reading always agree. The first run downloads and builds; after that it is cached.

Run `go test` with `-p 1`. Both reasons it was written for are gone: pipe names, because a test
that listens on the derived name or launches a binary that dials it now moves the derivation with
`ipc.TestPipeSIDEnv` first, keyed on the test name and the process id; and the single database
file, because every database a test opens is under its own `t.TempDir`. Whether it can now be
dropped is `[unverified]` — nothing re-measured it, and the timing-sensitive tests are what would
decide it (the 30-concurrent-starts gate, the relay's dial and total budgets). Keep passing it
until someone does. Nothing in the repository enforces it; what refuses a `go test` without `-p` on
the maintainer's machine is a Claude Code `PreToolUse` hook in that machine's settings — which is
why searching `.git/hooks`, `core.hooksPath`, shell profiles and `GOFLAGS` finds nothing, and why
it is not a guarantee for anyone else.

## How we work

**Code first for facts, and only for facts.** A document may decide things before the code exists —
that is exactly what a spec is for. What it may not do is *assert* things before the code exists. An
unmeasured number, an unrun command, an ungated claim: mark it `[unverified]` and write down what
rests on it.

Once the code runs it settles **facts** — timings, sizes, what a dependency actually does — and the
document gets corrected to match. It settles nothing about **intent**. Code that violates an
invariant is a bug; editing the spec to match it is how a bug becomes permanent. Changing an
invariant is a design change, made deliberately, not a side effect of an implementation.

**No code blocks in documents.** Signatures, DDL, and package layout belong to code. The one
exception is a reproduction command you actually ran and whose output you saw. Three spec revisions
and a 5,748-line plan (`3e5fe8d`) were discarded for ignoring this. Plans reach that state far more
easily than specs do.

**TDD, then break it.** For every test that guards an invariant:

```
1. write the test, watch it fail
2. implement, watch it pass
3. deliberately break the implementation — that invariant only
4. confirm the test now fails      <- if it still passes, the test is fake
5. revert, confirm it passes again
```

Steps 3-5 do not get committed. This exists because an earlier suite survived most of a deliberate
mutation pass: transaction control could be deleted wholesale and everything stayed green, because
the assertions checked presence (`!= ""`) instead of value. Assert exact values — round-tripped
bytes, error identity via `errors.Is`, observable state change.

**Evidence.** Never write that a check passed unless you ran it and saw it pass. Self-review is not
evidence.

## What will bite you

Most of these are observations against pinned dependency and toolchain versions, not laws. On a
dependency or Go upgrade, re-verify the rows that name one. When a test or linter starts catching a
row on its own, delete the row — the test is the better owner.

| Symptom | What is actually happening |
|---|---|
| `database is locked` against the real data directory — `doctor`, a migration, a second service started by hand | A development service is running and holds that file exclusively (I-07). This is almost never a DSN or pragma bug. The suite does not meet it: every test opens a database under its own `t.TempDir` |
| *"something is already listening on `\\.\pipe\engramux.v1-…`"*, or a relay delivers when the test expected it to spool | **Not** the development service any more. The name is still derived from the **user SID, not the data directory** — so redirecting `LOCALAPPDATA` isolates nothing — but tests override the SID that feeds the hash with `ipc.TestPipeSIDEnv`, and their children inherit it. So the suite runs with the service up, and this message now means a listener an earlier test leaked, a second copy of the test binary, or a test that reaches the pipe without going through its package's `useTestPipeName`. No shipped path sets that variable, but the read is in every build: whichever process sees it moves, so a relay and a service given different environments land on different names and stop meeting. Nothing is lost when they do — the relay spools and the drain is by directory, so the events arrive at the next `drainInterval` instead of over the pipe (I-01, I-09 still hold; it is one user's own processes either way) |
| A pipe test panics with *"testing: test using t.Setenv, t.Chdir, or cryptotest.SetGlobalRandom can not use t.Parallel"* | The pipe-name override is set with `t.Setenv`, which is process-wide, so a test that moves the name cannot be parallel and cannot have a parallel ancestor. Go 1.27 raises it from `t.Parallel` when `Setenv` came first, and from `Setenv` when an ancestor was already parallel. The panic is the right answer, not a limitation to work around: a process holds one value at a time. This is *in-process* parallelism and has nothing to do with `-p 1`, which sets how many package binaries run at once |
| `Access is denied` from `go build -o` | You are overwriting a running `.exe` |
| A Windows CLI flag is mangled into a path — `schtasks /query` becomes `C:/Program Files/Git/query` | MSYS path conversion. Set `MSYS_NO_PATHCONV=1`, or use `//query` |
| A DSN pragma silently has no effect | Only `_pragma` values skip validation — a typo returns `err=nil` and SQLite ignores it. The answer is I-11, not a retry |
| Writer permanently returns `cannot start a transaction within a transaction` | Raw `BEGIN IMMEDIATE` SQL, and one missed `ROLLBACK`. It is unrecoverable *because* the service has exactly one connection (spec §5.4). If that ever changes, revisit |
| A `CREATE TRIGGER` migration applies but the trigger body is truncated | goose splits statements on `;`. Wrap triggers in `-- +goose StatementBegin` / `-- +goose StatementEnd` |
| Flaky named-pipe tests | `winio.DialPipeContext` fails immediately when the pipe does not exist yet — it only retries on `ERROR_PIPE_BUSY`. Own the listener/dial race. Also: `winio.DialPipe(path, nil)` silently uses a 2s timeout |
| A console window flashes | A console-less parent spawning a console child with default flags creates a real, visible console. Every child the service spawns needs the no-window creation flag (spec §5.1) |
| `go build` passes but `go test` fails to build | The `go` directive in `go.mod` is below the symbol you used. `go vet`'s `stdversion` catches it; `go build` does not |
| `golangci-lint` refuses every config: *"the Go language version (go1.26) used to build golangci-lint is lower than the targeted Go version"* | The `go` directive is a hard ceiling set by the Go version golangci-lint was *built with*, not by the toolchain you run. `go.mod` says `1.26.0` while the toolchain is 1.27.0 for exactly this reason. Raising the directive requires a golangci-lint built on the newer Go first — check `golangci-lint --version` before you touch `go mod edit -go` |
| `go test ./...` or `golangci-lint run` fails on a tree with no packages | Not a config bug. `go test` exits 1 with *"no packages to test"* and golangci-lint exits 5 with *"no go files to analyze"*. Both need at least one package to be meaningful |
| `golangci-lint` prints `0 issues.` and exits **7** | It typechecked nothing. A linter built with Go 1.26 cannot read Go 1.27's `math/rand/v2` (`method must have no type parameters`), so any package importing `crypto/rand` — `github.com/google/uuid` does, for UUIDv7 — fails to load while still printing a clean summary. **Check the exit code, never the summary line.** The pinned `go run` invocation in Commands avoids it by building the linter with the local toolchain |
| An unchecked `fmt.Fprintln` is not flagged, and you conclude errcheck is off | errcheck's own `DefaultExcludedSymbols` is a **separate** mechanism from golangci-lint's exclusion presets — declining the `std-error-handling` preset does not disable it. Measured boundary: writes to literal `os.Stdout`/`os.Stderr` and to `*bytes.Buffer` are excluded; `fmt.Fprintln` to any other `*os.File` or `io.Writer`, plus bare `w.Write` and `f.Close`, are all caught. The exclusions cover exactly the targets whose error is not actionable — do not "fix" it |
| A concurrency test passes and proves nothing | `testing/synctest` does not report data races — two goroutines doing `x++` inside a bubble pass silently. It also cannot see syscalls or real I/O, so it is useless for pipe tests. Use it for timeouts, backoff, and drain logic only |
| `-race` will not run | It requires `CGO_ENABLED=1` *and* a C compiler, and there is no CGO-free route on windows/amd64. `scripts/race.sh` finds a compiler and checks it is new enough; it prints what to do when it cannot. Verify the claim yourself with `CGO_ENABLED=0 go test -race` — if that ever succeeds, delete this row |
| The race detector is green and you are not sure it is looking | It is not enough that `-race` links. Write a deliberate unsynchronised `x++` across goroutines and confirm it fails, then confirm the mutex-guarded version stays quiet. A detector that reports nothing and a detector that is not running look identical |
| A tool-output parser works for one host and silently misreads the other | In the captured corpus `tool_response` is an object from Claude Code but a string or an array from Codex (spec §4.4). Those are the shapes observed, not a contract the hosts promise — preserve a shape you do not recognise instead of assuming it away |
| `t.TempDir()` cleanup fails on Windows | An open handle. Close the database and every listener before the test ends, including the WAL sidecar files |
| A child that should sit still until you kill it dies on its own instead — and the test passes | `select {}` makes that goroutine the only one, so Go's deadlock detector runs and the child prints `fatal error: all goroutines are asleep - deadlock!` while your `TerminateProcess` is still in flight. It does not fire every run, which is worse than always: the kill is sometimes not what ended the process, and the row count looks identical either way. Park the child in a read syscall on a pipe the parent holds open and never writes to — an M stuck in a syscall keeps the detector from running — and assert the exit code is exactly 1 so you know the kill is what ended it. `docs/evidence/crash` is a standalone binary, not a test binary, and is unaffected |
| Goroutine-leak checks always report zero | `Profile.Count()` returns 0 before the detecting GC cycle. Trigger detection with `WriteTo` and parse its output |
| A log redactor runs, and secrets are still in the log | `slog.Record.Attrs` hands the callback an `Attr` **by value** — assigning to `a.Value` is a no-op. Rebuild the record with `slog.NewRecord` and `AddAttrs` |
| Redaction produces JSON that no longer parses | A `\S+` token pattern swallows the closing quote and brace. The payload must stay valid JSON (spec §6) |
| You paste a `TestPhase4Gate` run into a report, a commit message or a chat, and ship a real path | Its corpus mode logs the query it derived for each class, and those are cut from the captures — 900 of 902 of which carry the user's directory. Measured: exactly **1 line** carries a drive-letter path with the OS user name in it — 1 of the 45 lines of a corpus-mode run, or 1 of the 84 the Commands line above emits when it runs both modes. One line in a wall of counts is exactly what gets skimmed past, and `origin` is public. Redact everything after `candidate documents: ` before the output goes anywhere, or grep out `[A-Za-z]:[\\/]` first. The sweep is not affected — measured, 0 of its 17 lines carry a path — and neither is the fixtures mode |
| A snapshot of the database taken after stopping the service is torn, and nothing says so | `schtasks /end` is a hard kill, not a clean close: the service does not get to checkpoint, so the WAL survives the stop holding committed frames the `.db` does not have yet. A snapshot is therefore the **pair** — `.db` and `.db-wal` copied together — and the `.db` alone is a database missing whatever the WAL still held. The pair measured this way had 181,312 B of WAL beside it. `-shm` is not in the pair and does not need to be: §5.4's exclusive locking means it never exists |
| The database file more than doubles, and the first start on the new binary pauses | Migration `00002` backfills `events.leaves` — a second copy of every payload's string text — and then rebuilds the whole FTS index, both in one transaction, so the WAL reaches the size of the migrated database before anything is checkpointed. One cost per database rather than per start, but it is not instant and the disk needs room for the file and its WAL at once. Spec §7.1 has the figures |
| You add `snippet()` or `highlight()` to a query, and the markers land in the wrong place — or a corruption error never surfaces | `events_fts` is an **external content** table, so both functions take their text from `events` and their markers from the index, and a desync moves the markers silently instead of failing. Worse: `snippet()` against a missing base row returns some rows and *then* fails with `database disk image is malformed`, which appears only in `rows.Err()` — a loop that ignores it sees a short result and no error at all. Both would also cut the **stored** payload, which is the one thing I-10 forbids. Build the excerpt in Go from the masked payload, as `internal/search/excerpt.go` does. Nothing in this repository catches a new `snippet()` call, which is why this is a row and not a test |

## Boundaries

- **`CGO_ENABLED=0` for every shipped binary**, because a user must not need a C toolchain to run
  Engramux. Paths where that reason does not apply — test-only, development-only — may carve out;
  the `-race` path does.
- **No exported package outside `internal/` until 1.0 ships**, `pkg/` included. A public API surface
  is a compatibility promise, and 1.0 has not earned one.
- **Never commit `.capture/`.** It holds raw prompts, file contents, tool output, and user paths.
  Promoting a capture to a fixture requires redaction *and* substituting the username out of paths.
- **`origin` is public.** Pushing publishes to the world. Check for usernames, machine names, email
  addresses, and real SIDs before committing.
- **An agent does not edit the user's host configuration** (`~/.claude`, `~/.codex`) during a
  session — ask the user to run the command. This is a rule about agent behavior, not a product
  constraint: Engramux writing hook configuration with the user's consent is in scope.

### For coding agents

Write Go source and any file containing backslashes with a file-write tool, not a shell heredoc.
Heredocs collapse `\\` to `\`, which corrupts Windows path literals and Go rune literals.

## Documents

- `docs/superpowers/specs/` — **owns decisions, invariants, budgets, and measurements**, and the
  intended behavior behind them. It does not own implementation facts: `go.mod` owns resolved
  versions, the migrations own the DDL, and the tree owns package layout. This file restates none of
  it. When more than one spec exists, each declares its scope and what it supersedes; read the ones
  covering your task.
- `docs/superpowers/plans/` — execution order only. Cites the spec, owns no values, restates no
  rules. If a plan and the spec disagree, the spec wins. A plan that proves unexecutable is deleted
  rather than repaired (precedent: `3e5fe8d`).
- `docs/prompts/` — one work order per session, dated. It carries what the spec and this file
  cannot: the state of the work when the session opened, and how that session was scoped. **A
  brief is a record and is never updated** — a stale one is correct, because it says what was
  true then. Read the newest to start; read an old one only to find out why something was done
  the way it was. Same rule as everywhere else: no code blocks.
- `docs/superpowers/backlog.md` — the carry list of deferred findings no test owns yet. Owns
  nothing else: not a decision, not a value, not a rule. When a test starts catching a row, delete
  the row.
- `docs/chatgpt/` — archive, superseded in full. Do not read it for current design.

## Output language

Write commit messages, pull request descriptions, code comments, and documentation in English.
Reply to the user in whatever language they are using.
