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
```

`TestPhase1Gate` runs Phase 1's four gate clauses in one pass over one database it builds from
an empty directory. It is in `internal/spool` because clause 3 kills a child that has to be a copy
of the running test binary, and that package's `TestMain` is what turns a re-executed copy into
one.

`CGO_ENABLED=0` is written out rather than inherited: the environment default happens to be 0 on the
machine this was written on, which means a build that violates the boundary would look fine here.

The linter is invoked through `go run` at a pinned version rather than as whatever
`golangci-lint` is on `PATH`, and that is not fussiness. A golangci-lint **built with Go 1.26**
cannot typecheck Go 1.27's `math/rand/v2` — it uses generic methods — so every package that
imports `crypto/rand`, directly or transitively, makes it print `0 issues.` **and exit 7**.
`go run` builds it with the local toolchain, so the linter and the standard library it is
reading always agree. The first run downloads and builds; after that it is cached.

Run `go test` with `-p 1`. Parallel test binaries collide on the single database file and on fixed
pipe names — both are consequences of 1.0 decisions, not laws. If the tests ever move to per-test
databases and per-test pipe names, re-measure and drop this. Nothing in the repository enforces it;
a contributor's machine may or may not have a guard.

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
| `database is locked` during tests, doctor, or migrations | A development service is running and holds the database exclusively. This is almost never a DSN or pragma bug |
| Tests fail with *"something is already listening on `\\.\pipe\engramux.v1-…`"*, or a relay delivers when the test expected it to spool | The same cause as the row above, through the other door: a development service is running. The pipe name is derived from the **user SID, not the data directory**, so pointing `LOCALAPPDATA` somewhere else does not isolate anything — a relay reaches whichever service currently owns the pipe. One machine, one instance, by design (I-01, I-09). Stop the service before running the suite; the tests say so themselves rather than failing obscurely |
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
- `docs/chatgpt/` — archive, superseded in full. Do not read it for current design.

## Output language

Write commit messages, pull request descriptions, code comments, and documentation in English.
Reply to the user in whatever language they are using.
