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
go test -p 1 -count=1 -run TestPhase4GateM4 -v ./internal/search/   # memory spec §5's M4, and its own delete condition
go test -p 1 -count=1 -run TestPhase6RedactionAudit -v ./internal/service/   # spec §8's Phase 6 gate,
go test -p 1 -count=1 -run TestPhase6TheMasked -v ./internal/secret/         # both halves of it
bash scripts/soak-sample.sh                                                  # spec §8's Phase 6 soak
bash scripts/soak-summary.sh docs/evidence/soak/soak.tsv                     # reduces a soak series to the figures spec §7.1 records
bash scripts/reinstall.sh                                                    # dist/ over the installed service, then doctor and status
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

`TestPhase4GateM4` is a different gate that happens to sit in the same package, and it is the one
that can delete code. It measures the derived-field boost over the corpus with the boost on and off,
three classes, recall@10 and MRR — and by its own terms, no improvement in any class means migration
`00005`, `store.Derive` and the ORDER BY term come out rather than the threshold coming down. It
skips when `.capture/` is absent, like `TestPhase4Gate`'s corpus mode. Unlike that mode its output
is safe to paste: it logs counts and figures and never a derived query, which matters more here than
there, because every query it derives is a command line or a touched path.

The Phase 6 gate is two commands because it is two modes and neither alone is the audit: the
`internal/service` half loads one event with a generated sample of every shape and sweeps every
reply document, MCP tool result and MCP tool error; the `internal/secret` half masks every real
capture and rescans it, and skips itself when `.capture/` is absent. What each surface is and
which five are deliberately out of scope is spec §8's Phase 6 row — do not re-derive it. Neither
command prints a document or a file name — measured, 0 of the 24 and 0 of the 5 `-v` lines carry a
drive-letter path — so unlike `TestPhase4Gate` their output is safe to paste.

`scripts/soak-sample.sh` appends one TSV line to `.capture/soak/soak.tsv` and reads everything from
outside the service, because the soak's precondition is that the binary stops changing. The command
above is the one-shot form on purpose: every other line in that block exits, and `--every 1800` —
which is how the soak is actually run — does not. It dies with the shell that started it anyway;
surviving a logoff needs a scheduled task, which is the user's to create. A missing prerequisite,
a bad `--every`, or a log it cannot append to are all exits rather than a loop that writes nothing.

`CGO_ENABLED=0` is written out rather than inherited: the environment default happens to be 0 on the
machine this was written on, which means a build that violates the boundary would look fine here.

The linter is invoked through `go run` at a pinned version rather than as whatever
`golangci-lint` is on `PATH`, and that is not fussiness. A golangci-lint **built with Go 1.26**
cannot typecheck Go 1.27's `math/rand/v2` — it uses generic methods — so every package that
imports `crypto/rand`, directly or transitively, makes it print `0 issues.` **and exit 7**.
`go run` builds it with the local toolchain, so the linter and the standard library it is
reading always agree. The first run downloads and builds; after that it is cached.

Run `go test` with `-p 1`. Both reasons it was written for are gone — pipe names move per test with
`ipc.TestPipeSIDEnv`, and every database a test opens is under its own `t.TempDir` — but whether it
can be dropped is `[unverified]`: the timing-sensitive tests would decide it (the
30-concurrent-starts gate, the relay's dial and total budgets) and nothing re-measured them. Keep
passing it until someone does. Nothing in the repository enforces it — a Claude Code `PreToolUse`
hook in the maintainer's own settings is what refuses a bare `go test`, which is why `.git/hooks`,
`core.hooksPath`, shell profiles and `GOFLAGS` are all empty of it, and why it guarantees nothing
on anyone else's machine.

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

1. write the test, watch it fail
2. implement, watch it pass
3. deliberately break the implementation — that invariant only
4. confirm the test now fails — **if it still passes, the test is fake**
5. revert, confirm it passes again

Steps 3-5 do not get committed. This exists because an earlier suite survived most of a deliberate
mutation pass: transaction control could be deleted wholesale and everything stayed green, because
the assertions checked presence (`!= ""`) instead of value. Assert exact values — round-tripped
bytes, error identity via `errors.Is`, observable state change.

**Evidence.** Never write that a check passed unless you ran it and saw it pass. Self-review is not
evidence.

**Where work lands.** `main` takes documentation, measurements, spec and backlog changes, and
test-only work — anything self-contained that does not change what the binary does. Anything that
changes product behaviour or removes a component gets **a branch per plan step**, named for the
step, merged with `--no-ff`. The merge commit is the point: this repository has none, which is
exactly why "what did that step change" is not a question its history can answer. Deciding once at
the start of a session is not enough — the same session that opens with a backlog row can turn into
a feature port, and that is when the branch is owed.

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
| A shutdown test asserting `net.ErrClosed` goes red about once in 170 full-suite runs | Closing a listener while an `Accept` is **already blocked** in an overlapped read cancels the I/O rather than refusing it, and winio surfaces `ERROR_OPERATION_ABORTED` — *"The I/O operation has been aborted because of either a thread exit or an application request."* Closing it between accepts gives `net.ErrClosed`. Which one you get is a race with no winner worth preferring, and it shows up under machine load. Measured: **7 failures in 1,200 runs (0.6%)**, all seven carrying that identical text, and 0 in 1,200 after accepting both. Accept the pair and nothing else — `endedByClose` in `internal/pipe` and its own rejection test are the shape; widening to "an error came back" would pass a Serve that returned for the wrong reason |
| Flaky named-pipe tests | `winio.DialPipeContext` fails immediately when the pipe does not exist yet — it only retries on `ERROR_PIPE_BUSY`. Own the listener/dial race. Also: `winio.DialPipe(path, nil)` silently uses a 2s timeout |
| A console window flashes | A console-less parent spawning a console child with default flags creates a real, visible console. Every child the service spawns needs the no-window creation flag (spec §5.1) |
| `go build` passes but `go test` fails to build | The `go` directive in `go.mod` is below the symbol you used. `go vet`'s `stdversion` catches it; `go build` does not |
| `golangci-lint` refuses every config: *"the Go language version (go1.26) used to build golangci-lint is lower than the targeted Go version"* | The `go` directive is a hard ceiling set by the Go version golangci-lint was *built with*, not by the toolchain you run — **but only for a golangci-lint you did not build yourself.** The pinned `go run` invocation in Commands builds it with the local toolchain, so the ceiling moves with the toolchain and this stopped being a constraint here. Measured 2026-08-30: the directive was raised from `1.26.0` to `1.27.0` and the pinned linter answered `0 issues.` at exit 0. A `golangci-lint` off `PATH` is where the row still bites — check `golangci-lint --version` before trusting one |
| `go test ./...` or `golangci-lint run` fails on a tree with no packages | Not a config bug. `go test` exits 1 with *"no packages to test"* and golangci-lint exits 5 with *"no go files to analyze"*. Both need at least one package to be meaningful |
| `golangci-lint` prints `0 issues.` and exits **7** | It typechecked nothing. A linter built with Go 1.26 cannot read Go 1.27's `math/rand/v2` (`method must have no type parameters`), so any package importing `crypto/rand` — `github.com/google/uuid` does, for UUIDv7 — fails to load while still printing a clean summary. **Check the exit code, never the summary line.** The pinned `go run` invocation in Commands avoids it by building the linter with the local toolchain |
| An unchecked `fmt.Fprintln` is not flagged, and you conclude errcheck is off | errcheck's own `DefaultExcludedSymbols` is a **separate** mechanism from golangci-lint's exclusion presets — declining the `std-error-handling` preset does not disable it. Measured boundary: writes to literal `os.Stdout`/`os.Stderr` and to `*bytes.Buffer` are excluded; `fmt.Fprintln` to any other `*os.File` or `io.Writer`, plus bare `w.Write` and `f.Close`, are all caught. The exclusions cover exactly the targets whose error is not actionable — do not "fix" it |
| A concurrency test passes and proves nothing | `testing/synctest` does not report data races — two goroutines doing `x++` inside a bubble pass silently. It also cannot see syscalls or real I/O, so it is useless for pipe tests. Use it for timeouts, backoff, and drain logic only |
| `-race` will not run | It requires `CGO_ENABLED=1` *and* a C compiler, and there is no CGO-free route on windows/amd64. `scripts/race.sh` finds a compiler and checks it is new enough; it prints what to do when it cannot. Verify the claim yourself with `CGO_ENABLED=0 go test -race` — if that ever succeeds, delete this row |
| The race detector is green and you are not sure it is looking | It is not enough that `-race` links. Write a deliberate unsynchronised `x++` across goroutines and confirm it fails, then confirm the mutex-guarded version stays quiet. A detector that reports nothing and a detector that is not running look identical |
| `schtasks /end` then `/run` back to back leaves **nothing** running, and the log blames the pipe | `/end` returns before the process is gone. The `/run` starts a second instance while the first is still exiting, that instance loses the pipe race and exits with `pipe: listen: … Access is denied` — which is I-09 working — and then the first finishes dying. What is left is no service at all, with a log line that reads like a singleton conflict rather than like an empty machine. Observed. Wait for the old one to stop answering before starting the new one: `until ! engramux.exe status >/dev/null 2>&1; do sleep 1; done`. A `status` that succeeds during the gap is being answered by the instance that is on its way out, so polling for success before `/run` proves nothing |
| A test runs the installer and quietly edits your own Claude Code configuration | Registering the MCP endpoint runs `claude mcp add --scope user`, and that binary resolves its **own** configuration file — it does not read the `HOME` and `USERPROFILE` a test hands it, so the redirection that isolates every other file isolates nothing here. Two seams stop it and a test has to use one: `host.RegisterClaudeMCP` takes the executable as an argument, so a test points it at a stub (the test binary re-executed, `internal/host/claude_test.go`); and `host.ClaudeCLI` reads `PATH`, so a test that exercises the lookup overrides `PATH` to a directory it made. Anything calling the wired `realSystem` without doing one of those is writing outside its own temporary directory. The Node installer this replaced had the same hazard and an empty `PATH` was what stopped it |
| You verify the installer against a redirected tree, and the service it starts lands on the real one | The installer starts the service through the logon task it has just registered, and Task Scheduler runs a task with its **principal's** environment, not with the environment of whoever ran `schtasks` — so the `LOCALAPPDATA` and `USERPROFILE` you redirected reach every file the installer writes and nothing the task starts. Observed 2026-08-30: session 07's isolated `install --apply` started a service that resolved the real data directory and the real SID-derived pipe, lost the pipe race to the running service (I-09, so nothing was harmed) and wrote its `stopped` line into the **real** service log. Had the real service been down, that binary would have served the real database. Redirecting the environment is not isolation for anything that goes through the task; the only thing that is, is a different user — the memory spec §8's clean-profile condition. Until one exists, stop the real service before an isolated `--apply`, or keep the run without `--apply` |
| A PowerShell one-liner run through bash loses `$_` | bash expands it before PowerShell ever sees it: `$_` is bash's own "last argument of the previous command", so `$_.Resources` arrives as something like `unsetenv.Resources` and PowerShell answers CommandNotFound for it. Observed. Escape it `\$_`, or write the pipeline with no `$_` at all — `Get-MpThreat \| Format-List Name,ID` rather than a `Where-Object { $_... }`. Every other `$name` and `$( )` in the string goes the same way |
| A `.md` you edited from a Python script comes back CRLF, and nothing complains | `pathlib.Path.write_text` opens in text mode, so on Windows every newline is written as CRLF. `.gitattributes` says `*.md text eol=lf`, so git normalises the blob on `add`: the diff is right, the commit is right, and only the working copy is wrong — which is why nobody notices. Pass `newline=""` to both `read_text` and `write_text`, or repair with `sed -i 's/\r$//'` and confirm with `file`. Two things bite afterwards. A `sed` or `grep` pattern anchored at end-of-line behaves differently on the two halves of a mixed tree. And rewriting a file whose content does not change leaves `git status` printing ` M` from a stale stat cache while `git diff HEAD` exits 0 — the exit code is the answer, `status` is not, and chasing that costs more than the original mistake |
| You grep a tree for carriage returns and every file comes back full of them, including files you never touched | The pattern was not a carriage return. Inside `"$( ... )"` bash does **not** apply ANSI-C quoting, so `grep -c $'\r' "$f"` there searches for the BRE `\r`, which is the letter **r** — and nearly every line of Go or prose contains one, so the count comes back at roughly the line count and reads exactly like a CRLF tree. The same expression outside the double-quoted substitution is a real CR and answers 0. Hit twice: session 09 concluded the documents were CRLF and had to correct it, session 10 concluded four commits carried CRLF and ran a `sed` repair that changed nothing. Two things settle it and neither can be fooled: `od -c` on one line, and `wc -c` against the blob size from `git cat-file -s` — CRLF is exactly one byte per line larger, so equal sizes mean equal endings. `file` is right too and was outvoted by the wrong tool. Also note `git cat-file blob` writes its stdout through a conversion here while `git show HEAD:<path>` does not, so the two disagree about a blob and neither is the byte count |
| A heredoc'd Python script writes `\r\n` into a document as two real line breaks | Same rule as the Go one below, one layer further out: `bash <<'EOF'` protects against the *shell*, not against Python's own string escapes. A row written to explain CRLF was itself split into three rows by the `\r\n` inside it. Anything containing a backslash escape goes through a file-write tool, not through a script literal — and read the result back before believing it |
| A tool-output parser works for one host and silently misreads the other | In the captured corpus `tool_response` is an object from Claude Code but a string or an array from Codex (spec §4.4). Those are the shapes observed, not a contract the hosts promise — preserve a shape you do not recognise instead of assuming it away |
| `t.TempDir()` cleanup fails on Windows | An open handle. Close the database and every listener before the test ends, including the WAL sidecar files |
| A child that should sit still until you kill it dies on its own instead — and the test passes | `select {}` makes that goroutine the only one, so Go's deadlock detector runs and the child prints `fatal error: all goroutines are asleep - deadlock!` while your `TerminateProcess` is still in flight. It does not fire every run, which is worse than always: the kill is sometimes not what ended the process, and the row count looks identical either way. Park the child in a read syscall on a pipe the parent holds open and never writes to — an M stuck in a syscall keeps the detector from running — and assert the exit code is exactly 1 so you know the kill is what ended it. `docs/evidence/crash` is a standalone binary, not a test binary, and is unaffected |
| Goroutine-leak checks always report zero | `Profile.Count()` returns 0 before the detecting GC cycle. Trigger detection with `WriteTo` and parse its output |
| A log redactor runs, and secrets are still in the log | `slog.Record.Attrs` hands the callback an `Attr` **by value** — assigning to `a.Value` is a no-op. Rebuild the record with `slog.NewRecord` and `AddAttrs` |
| Redaction produces JSON that no longer parses | A `\S+` token pattern swallows the closing quote and brace. The payload must stay valid JSON (spec §6) |
| You paste a `TestPhase4Gate` run into a report, a commit message or a chat, and ship a real path | Its corpus mode logs the query it derived for each class, and those are cut from the captures — 900 of 902 of which carry the user's directory. Measured: exactly **1 line** carries a drive-letter path with the OS user name in it — 1 of the 45 lines of a corpus-mode run, or 1 of the 84 the Commands line above emits when it runs both modes. One line in a wall of counts is exactly what gets skimmed past, and `origin` is public. Redact everything after `candidate documents: ` before the output goes anywhere, or grep out `[A-Za-z]:[\\/]` first. The sweep is not affected — measured, 0 of its 17 lines carry a path — and neither is the fixtures mode |
| A break-it pass reports no failing test, and the test is fine | The mutation did not compile. Go makes an unused import and an unused local a **compile error**, so a break that removes the last reference to either produces a program that never runs — and a harness grepping for `--- FAIL` sees nothing, which reads exactly like a test that does not care. Hit twice in one session, once concluding an assertion was weak that was not. Two things follow. The harness has to tell a build failure from a passing suite and say which. And a mutation should **change the answer, not remove the reference**: `MappedImage: !errors.Is(...)` rather than `MappedImage: true`, `bytes.Equal(have, append(want, 'x'))` rather than `case false:`. Where that is impossible — the mutation *is* deleting the only call — the mutant is **discarded, not counted as killed**, which is what every Go mutation-testing tool does with one. A third state reads the same and is not either: the mutation **never applied**. A `sed` address that missed, a `python` replacement whose literal did not match the file's bytes — the edit is a no-op, the suite runs unmutated, and `ok` is the honest answer to a question nobody asked. Observed 2026-08-31, one `ok` reported for a mutation that was never in the file. Assert the mutation is present before running the suite, and make a failed application loud rather than skipped |
| A break-it pass reports `NOOP` for a mutation you know is real, and your uncommitted fix is gone | The pass reverts each mutation with `git checkout -- <file>`, which restores **HEAD**, not the working tree. Run it against a dirty tree and it silently deletes every uncommitted change in the files it touches — observed, three files at once, and the only symptom was two later mutations finding nothing to match. Steps 3-5 are not committed, but what they are testing has to be. Commit first, or make the script refuse a dirty tree |
| A background `--every` loop keeps sampling after you stopped it, and two loops interleave rows into one log | Two separate mechanisms, both observed in one session. Stopping a backgrounded shell command from the agent harness kills the wrapper and leaves the `bash` running the loop — `ps -W` shows it, with a `/usr/bin/sleep` child, and only `kill -9` on both ends it. And a running `bash` keeps executing the *function it already parsed*, so rewriting the script does not update a loop that is mid-flight: it keeps writing the old row format into the new file. Kill the loop before editing the script, then start one. `soak-sample.sh` now writes its own pid in every row and refuses a log whose header is not its own, which makes both failures visible rather than preventing them — nothing inside a script can prevent either |
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
  constraint: Engramux writing hook configuration with the user's consent is in scope. `register`
  and `unregister` stay on the user's side of it always: they write host configuration
  unconditionally and there is no precondition that can make them not.

  **Two carve-outs, narrowed until the condition is what makes them safe rather than the reader
  being this repository's owner.** Both were decided on 2026-09-02, after a session spent nine
  turns waiting on a human for commands that mostly wrote nothing.

  An agent may **stop and start the installed service**. It writes no file, and it loses no event:
  a relay that cannot reach the service spools, and the drain replays — measured, four events came
  back that way during that session's own reinstall. Having stopped it, an agent restarts it in the
  same turn, and says so when it cannot.

  An agent may run **`engramux install --apply`** *only after `doctor` reports that both hosts
  already point at the endpoint*. Under that condition the install writes no host configuration at
  all — it copies two binaries, re-registers the logon task and rewrites `mcp.json`, all of which
  are this product's own files. Measured twice on 2026-09-02: both runs answered `already up to
  date` and `already points at this endpoint` and touched neither host file. When the condition does
  not hold, the agent stops and asks, which is what makes this safe on a machine that has never
  installed rather than only on one that has: there, `doctor` reports the hosts unregistered and the
  condition refuses.

  `scripts/reinstall.sh` is the whole cycle in one command. It deliberately does **not** check the
  condition above — that is a rule about the agent, not about installing, and a script that enforced
  it would refuse the user's own first install, which is the one time it is certainly right to run.

  Both carve-outs are for an interactive turn. Unattended work — a loop, a scheduled task — does not
  take them.

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
