# Engramux

Engramux captures Claude Code and Codex session hook events into SQLite and serves them back
through FTS5 and MCP. One service per Windows user multiplexes every concurrent session.

It exists because `thedotmack/claude-mem` does this job but breaks constantly on Windows.
Engramux is a reference reimplementation in Go — not a fork.

**Behavior parity with claude-mem is not a goal.** Do not copy its process, I/O, or installation
architecture: that is the part that fails on Windows, and mirroring it reproduces the failure. Its
data model, tool surface, and search UX are Windows-neutral — read those freely, but treat what you
find as a candidate, never as a justification. Decisions are justified by the Windows constraint and
by measurement.

## Commands

Build targets are the directories under `cmd/`. The service binary must be linked as a GUI
subsystem binary; without `-H=windowsgui` it pops a console window every time it spawns a child.

```bash
go build -ldflags "-s -w"               -o dist/engramux.exe          ./cmd/engramux
go build -ldflags "-s -w -H=windowsgui" -o dist/engramux-service.exe  ./cmd/engramux-service
go test -p 1 ./...
golangci-lint run
```

Run `go test` with `-p 1`. Parallel test binaries collide on the single database file and on fixed
pipe names — both are consequences of 1.0 decisions, not laws. If the tests ever move to per-test
databases and per-test pipe names, re-measure and drop this. Nothing in the repository enforces it;
a contributor's machine may or may not have a guard.

## How we work

**Code first for facts.** A document may decide things before the code exists — that is exactly what
a spec is for. What it may not do is *assert* things before the code exists. An unmeasured number,
an unrun command, an ungated claim: mark it `[unverified]` and write down what rests on it. Once the
code runs, the code is right and the document gets corrected — never the other way round.

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

| Symptom | What is actually happening |
|---|---|
| `database is locked` during tests, doctor, or migrations | A development service is running and holds the database exclusively. This is almost never a DSN or pragma bug |
| `Access is denied` from `go build -o` | You are overwriting a running `.exe` |
| A Windows CLI flag is mangled into a path — `schtasks /query` becomes `C:/Program Files/Git/query` | MSYS path conversion. Set `MSYS_NO_PATHCONV=1`, or use `//query` |
| A DSN pragma silently has no effect | Only `_pragma` values skip validation — a typo returns `err=nil` and SQLite ignores it. The answer is I-11, not a retry |
| Writer permanently returns `cannot start a transaction within a transaction` | Raw `BEGIN IMMEDIATE` SQL, and one missed `ROLLBACK`. It is unrecoverable *because* the service has exactly one connection (spec §5.4). If that ever changes, revisit |
| A `CREATE TRIGGER` migration applies but the trigger body is truncated | goose splits statements on `;`. Wrap triggers in `-- +goose StatementBegin` / `-- +goose StatementEnd` |
| Flaky named-pipe tests | `winio.DialPipeContext` fails immediately when the pipe does not exist yet — it only retries on `ERROR_PIPE_BUSY`. Own the listener/dial race. Also: `winio.DialPipe(path, nil)` silently uses a 2s timeout |
| A console window flashes | A console-less parent spawning a console child with default flags creates a real, visible console. Every child the service spawns needs the no-window creation flag (spec §5.1) |
| `go build` passes but `go test` fails to build | The `go` directive in `go.mod` is below the symbol you used. `go vet`'s `stdversion` catches it; `go build` does not |
| A concurrency test passes and proves nothing | `testing/synctest` does not report data races — two goroutines doing `x++` inside a bubble pass silently. It also cannot see syscalls or real I/O, so it is useless for pipe tests. Use it for timeouts, backoff, and drain logic only |
| `-race` will not run | It requires `CGO_ENABLED=1` *and* a C compiler, and there is no CGO-free route on windows/amd64. Verify for yourself with `CGO_ENABLED=0 go test -race`; if that ever succeeds, delete this row |
| A tool-output parser works for one host and silently misreads the other | `tool_response` is an object from Claude Code but a string or an array from Codex (spec §4.4) |
| `t.TempDir()` cleanup fails on Windows | An open handle. Close the database and every listener before the test ends, including the WAL sidecar files |
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

- `docs/superpowers/specs/` — decisions and measurements. **Owns the values**: versions, schema,
  repository layout, invariants, budgets. This file does not restate them. When more than one spec
  exists, the newest date is current and superseded ones say so on their first line.
- `docs/superpowers/plans/` — execution order only. Cites the spec, owns no values, restates no
  rules. If a plan and the spec disagree, the spec wins. A plan that proves unexecutable is deleted
  rather than repaired (precedent: `3e5fe8d`).
- `docs/chatgpt/` — archive, superseded in full. Do not read it for current design.

## Output language

Write commit messages, pull request descriptions, code comments, and documentation in English.
Reply to the user in whatever language they are using.
