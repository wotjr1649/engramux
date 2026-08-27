# Engramux

Engramux captures Claude Code and Codex session hook events into SQLite and serves them back
through FTS5 and MCP. One service per Windows user multiplexes every concurrent session.

It exists because `thedotmack/claude-mem` does this job but breaks constantly on Windows.
Engramux is a reference reimplementation in Go — not a fork.

**Behavior parity with claude-mem is not a goal.** Read the reference to learn *what* the tool
does. Do not read it to learn *how*. Its architecture is what fails on Windows; mirroring it
reproduces the failure. When a design question comes up, answer it from the Windows constraint
and from measurement — not from what the reference did.

## Commands

```bash
go build -ldflags "-s -w"               -o dist/engramux.exe          ./cmd/engramux
go build -ldflags "-s -w -H=windowsgui" -o dist/engramux-service.exe  ./cmd/engramux-service
go test -p 1 ./...
golangci-lint run
```

`-H=windowsgui` is not optional for the service binary. Without it the service pops a console
window every time it spawns a child process.

`-p 1` is not optional either. Parallel test binaries collide on the single SQLite file and on
pipe names. A repository hook rejects `go test` without it.

## How we work

**Code first.** Working code corrects the design document, never the other way round. Do not put
code into a document unless you have run it. Two earlier revisions of the design doc were
discarded for exactly this.

**TDD, then break it.** For every test that guards an invariant:

```
1. write the test, watch it fail
2. implement, watch it pass
3. deliberately break the implementation — that invariant only
4. confirm the test now fails      <- if it still passes, the test is fake
5. revert, confirm it passes again
```

Steps 3-5 do not get committed. This exists because an earlier test suite survived 15 of 20
deliberate mutations: transaction control could be deleted wholesale and everything stayed green.
The assertions checked presence (`!= ""`) instead of value. Assert exact values — round-tripped
bytes, error identity via `errors.Is`, observable state change.

**Evidence.** A claim that something was verified requires an observed check. Never write that a
check passed unless you ran it and saw it pass. Self-review is not evidence.

## What will bite you

None of these are derivable from the code or from general Go knowledge.

| Symptom | Cause and fix |
|---|---|
| `database is locked` during tests, doctor, or migrations | A development `engramux-service.exe` is running and holds the DB exclusively. This is almost never a DSN or pragma bug. Stop the service first |
| `Access is denied` from `go build -o` | You are overwriting a running `.exe`. Stop it first |
| A DSN pragma silently has no effect | Only `_pragma` values skip validation — `syncronous(3)` returns `err=nil` and SQLite ignores it. Read every pragma back at startup and fail if it does not match |
| Writer permanently returns `cannot start a transaction within a transaction` | Raw `BEGIN IMMEDIATE` SQL. One missed `ROLLBACK` wedges the connection forever. Use `_txlock=immediate` plus `db.BeginTx` / `tx.Commit` |
| A `CREATE TRIGGER` migration applies but the trigger body is truncated | goose splits statements on `;`. Wrap triggers in `-- +goose StatementBegin` / `-- +goose StatementEnd` |
| Flaky named-pipe tests | `winio.DialPipeContext` fails immediately when the pipe does not exist yet — it only retries on `ERROR_PIPE_BUSY`. Own the listener/dial race with an explicit retry loop. Also: `winio.DialPipe(path, nil)` silently uses a 2s timeout; always pass a context |
| A console window flashes | A console-less parent spawning a console child with default flags creates a real, visible console. Every child the service spawns needs `SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}` |
| `go build` passes but `go test` fails to build | The `go` directive in `go.mod` is below the symbol you used. `go vet`'s `stdversion` catches it; `go build` does not. Reproduce: use `slog.NewMultiHandler` under `go 1.25.7` |
| A concurrency test passes and proves nothing | `testing/synctest` does not report data races — two goroutines doing `x++` inside a bubble pass silently. It also cannot see syscalls or real I/O, so it is useless for pipe tests. Use it for timeouts, backoff, and drain logic only |
| `-race` will not run | It requires `CGO_ENABLED=1` *and* a C compiler. There is no CGO-free route on windows/amd64 (golang/go#6508, open since 2013, no Windows plan). Race verification lives in `scripts/`, outside the shipped build path |
| A tool-output parser works for one host and silently misreads the other | `tool_response` has different shapes per host: an object from Claude Code, a string or an array from Codex. An object-assuming parser is wrong for every Codex event |
| `t.TempDir()` cleanup fails on Windows | An open handle. Close the `*sql.DB` and every listener before the test ends, and make sure `-wal` / `-shm` are released |
| Goroutine-leak checks always report zero | `Profile.Count()` returns 0 before the detecting GC cycle. Trigger detection with `WriteTo` and parse its output |
| A log redactor runs, and secrets are still in the log | `slog.Record.Attrs` hands the callback an `Attr` **by value** — assigning to `a.Value` is a no-op. Rebuild the record with `slog.NewRecord` and `AddAttrs` |
| Redaction produces JSON that no longer parses | A `\S+` token pattern swallows the closing quote and brace. Bound it to `[^\s"',}\]]+`, and keep the redacted payload valid JSON — an unmarshalable payload gets dropped without being sent or spooled |

## Boundaries

- **`CGO_ENABLED=0` for every shipped binary.** The test-only `-race` path is the single carve-out.
- **No `pkg/`.** A public API surface is out of scope for 1.0.
- **Never commit `.capture/`.** It holds raw prompts, file contents, tool output, and user paths.
  Promoting a capture to a fixture requires redaction *and* substituting the username out of paths.
- **`origin` is a public repository.** Pushing publishes to the world. Check for usernames, machine
  names, email addresses, and real SIDs before committing.
- **MCP binds `127.0.0.1` and keeps bearer authentication.** The design treats a single Windows SID
  as inside the trust boundary; that is not permission to drop local defenses. `127.0.0.1` limits
  the machine, not the process.
- **Redaction happens before the insert**, not after. Purge is in-place replacement.
- **Do not edit the user's host configuration** (`~/.claude`, `~/.codex`). Ask them to run the
  command themselves.

### For coding agents

- Write Go source and any file containing backslashes with a file-write tool, not a shell heredoc.
  Heredocs collapse `\\` to `\`, which corrupts Windows path literals and Go rune literals.
- The design document is `docs/superpowers/specs/`. It owns the values — versions, schema, package
  layout, invariants. This file does not restate them; open the spec when you need one.

## Output language

Write commit messages, pull request descriptions, code comments, and documentation in English.
Reply to the user in whatever language they are using.
