# Engramux

`github.com/wotjr1649/engramux`

Engramux captures Claude Code and Codex session hook events into SQLite and serves them back
through FTS5 and MCP. One service per Windows user multiplexes every concurrent session, across
every project. The hook side is a relay that is spawned once per event and exits, so it never
blocks the host and never fails one — it exits 0 on every path, panics included.

What it is for is capture you do not have to ask for. There is no `remember this` tool call: the
eleven events Claude Code and Codex have in common are recorded as they happen.

Reading is pull-only **by default** — a CLI, and five tools on an authenticated loopback MCP
endpoint. There is one push path and it ships off: hook-time context injection can write into
`UserPromptSubmit`, and it turns on only when a file named `inject.json` exists in the data
directory. Nothing in the installer writes that file, and its gate has not been run.

It exists because [`thedotmack/claude-mem`](https://github.com/thedotmack/claude-mem) does this job
but breaks constantly on Windows. Engramux is a reference reimplementation in Go — **not a fork**,
and **not a migration path**: there is no importer, and a new installation starts with an empty
database.

**Windows only.** Nothing here has been measured on any other platform, and no build tag enforces
it — the constraint comes from the dependencies. `golang.org/x/sys/windows` has no Go files at all
off Windows, and `github.com/Microsoft/go-winio`'s named-pipe symbols are behind a `windows`
constraint, so a build for another `GOOS` fails. `[unverified]`: nobody has run that build, and the
exact message is unrecorded. One artefact, `windows/amd64`; `arm64` is unmeasured.

## There is no release

No tag, no release page, no marketplace entry, no archive, and no CI — this repository has never
built an artefact anywhere but on a developer's machine. The only path anyone can walk today is
**building from source, and that is the developer path**, labelled as one deliberately: the spec
rejects source as a primary path for end users, and the Defender section below is why it is not
merely inconvenient.

The version is `0.x` and there is no compatibility promise. Nothing outside `internal/` and `cmd/`
is exported — `pkg/` included — because a public API surface is a promise 1.0 has not earned. A
build that is not a release calls itself `0.0.0-dev`, with the first twelve characters of the commit
appended when the binary carries build information, and `.dirty` on top of that when the tree was
modified.

Four publication conditions are recorded in the memory spec. **One is closed** — the bearer token's
file permissions, on 2026-09-04. Three are open:

- **A first install on a clean profile.** Read this one plainly: these two binaries have never been
  installed onto a Windows profile that has not already run every earlier build of them. If you
  install this, you are the first.
- **This README.** It is condition 3, and it does not close itself: the condition asks for the
  Defender exclusion steps, and those are `[unverified]` below rather than given.
- **A first run that survives antivirus.** See below.

## What it stores, and who can read it

Read this before installing anything. Engramux records **raw prompts, file contents, tool output,
and the paths you work in**, for both agents, continuously. It also indexes each host's own native
memory files. That is the product, not a side effect.

**The database has no permissions of its own.** `engramux.db`, its write-ahead log, the spool and
the logs all inherit whatever `%LOCALAPPDATA%\engramux` grants, and the one measurement taken there
found a machine-local group with read access. The file is not encrypted, and it grows: a soak run
recorded it at 182,829,056 B. Anyone who can read that file reads every prompt you have typed into
either agent.

**The bearer token sits in three files, and only one of them is this product's to narrow.**
`mcp.json` now carries a protected DACL of its own — SYSTEM, Administrators, and the user the
service runs as — set before the token reaches disk. The other two belong to the hosts:
`~/.codex/config.toml` and Claude Code's user configuration keep whatever their parent directory
grants, and re-permissioning another product's configuration is not this one's to do. `doctor`
reports those two as a finding rather than changing them. Rotation is the file: delete `mcp.json`,
restart the service, and re-run the installer.

**The trust boundary is the machine, not your user account.** The MCP endpoint is loopback and
authenticated, and the 1.0 spec explicitly withdrew the sentence claiming more: any process of any
locally logged-on user can reach the endpoint, and the bearer token is the only control.

**Secrets are tagged rather than destroyed, and masked on four egress surfaces** — the service log,
the reply documents, the MCP tool results, and MCP tool errors. Five surfaces are deliberately out
of scope, each with a stated reason in the spec, and one of them matters to a reader here: **the
spool**. An event the service was not up to receive sits on disk as the host sent it, unmasked,
until the drain replays it.

**It makes no outbound network call.** Measured 2026-09-03: `net/http` is imported by exactly one
file in this repository, and it uses it to listen on loopback. Nothing is uploaded and there is no
telemetry. What has *not* been measured is the artefact at the socket level — the MCP SDK links an
HTTP client, TLS and an OAuth2 package into the service binary, none of which anything in this
repository calls.

## Windows Defender will quarantine the CLI

It has happened here, and the memory spec's fourth publication condition says a stranger's first
install presents the same two shapes. Treat that as the condition's own claim rather than as
something anyone has watched on another machine — nobody has, which is what condition 1 above is.

**`Behavior:Win32/Execution.A!ml`** removed `engramux.exe` — the CLI — from both the build output
directory and the install directory, severity 5, blocked before it ran. The service binary was not
touched and kept running. `Behavior:` and `!ml` are the whole finding: a behavioural
machine-learning detection, not a signature on the bytes.

**`Trojan:Win32/Commando.A!ml`** fired earlier on the soak sampler's `schtasks /create`. Installing
registers a logon task, so this is a shape an install presents. The `!ml` suffix says it is also a
model verdict; nothing here records it being classified further.

Two things follow, and both are counter-intuitive.

**Do not change the build flags hoping it helps.** The `-s -w` strip is not implicated. This is a
behaviour detection on executing a freshly built, rare, unsigned binary.

**Building from source is worse for this, not better.** A release artefact is the same bytes for
everyone who downloads it, so its prevalence accumulates; a binary you built is unique to your
machine and stays rare forever, and rarity is what this detection keys on. Nothing here is
code-signed; signing is sequenced behind a release process rather than bought.

**The exclusion procedure is `[unverified]`, and that is not a formality.**
`Add-MpPreference -ExclusionPath` was attempted and refused with HRESULT `0xc0000142` — unelevated,
or Tamper Protection, which is what that feature is for. An exclusion therefore has to go through
the Windows Security UI by hand, for **both** the build output directory and the install directory.
Nobody has walked that route and recorded the result, so no steps are given here rather than guessed
at. The two directories are `dist\` under your checkout and `%LOCALAPPDATA%\engramux\bin`.

## Building it

Go 1.27 or later. No C toolchain is needed and none may be required — every shipped binary is built
`CGO_ENABLED=0`, written out rather than inherited, because the default happens to be 0 on the
machine this was written on: a command line that had simply omitted it would look correct here and
break elsewhere.

```bash
CGO_ENABLED=0 go build -ldflags "-s -w"               -o dist/engramux.exe         ./cmd/engramux
CGO_ENABLED=0 go build -ldflags "-s -w -H=windowsgui" -o dist/engramux-service.exe ./cmd/engramux-service
```

The service binary needs `-H=windowsgui`. Without it, a console window appears every time the
service spawns a child.

Run the copy in `dist\`, not an installed one. `install` and `update` both refuse to let an
installed binary overwrite itself, with two separate refusals saying so.

## Checking it

There is no CI. These three run locally, in this order:

```bash
go test -p 1 ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run
bash scripts/race.sh
```

Four things about them are worth knowing before you read their output.

`-p 1` is what the suite is run with. Nothing in this repository enforces it, and whether it can be
dropped is untested.

**The linter's exit code is the answer, never its summary line.** It is invoked through `go run` at
a pinned version on purpose: a `golangci-lint` built with an older Go cannot typecheck this tree, and
when it fails to it still prints `0 issues.` — and exits 7. Building it with the local toolchain is
what keeps the linter and the standard library it reads in agreement. The first run downloads and
builds it.

`scripts/race.sh` needs `CGO_ENABLED=1` and a C compiler, which this repository does not carry. It
looks for one and prints what to do when it cannot find one. There is no CGO-free route to the race
detector on `windows/amd64`.

**Several gates skip on a fresh clone, and a skip is not a pass.** Three different things they want
are absent there: a captured corpus under `.capture/`, which is never committed; this machine's own
native memory files; and a snapshot of a live database, which only exists once the service has run.
Read the output rather than the exit code for those.

## Installing it

**Run `engramux install` with no flags first.** That is a dry run: it reports the same plan and
writes nothing. `--apply` is what writes, and it is not a small action:

- **Two binaries**, copied into `%LOCALAPPDATA%\engramux\bin`.
- **Four files that belong to the hosts, not to this product** — `~/.claude/settings.json`,
  `~/.codex/hooks.json`, `~/.codex/config.toml`, and Claude Code's user configuration.
- **Eleven hook entries per host**, one per captured event.
- **A logon task**, which is what starts the service when you sign in — and it starts the service
  now, as well.
- **A data directory** under `%LOCALAPPDATA%\engramux` — the database and its write-ahead log, a
  spool, `mcp.json`, and logs.

It also **finds the `claude` CLI on your `PATH` and runs it**, because registering the MCP endpoint
with Claude Code goes through that binary rather than through a file this product writes.

Undoing it is partial, and the part that is not undone is the part that matters.
`engramux install --remove` takes the hook entries and the MCP registration out of both hosts and
removes the logon task. `engramux unregister` removes **only** the logon task. Neither removes the
binaries, and **neither removes the data directory** — your captured prompts stay on disk until you
delete `%LOCALAPPDATA%\engramux` yourself.

`engramux doctor` reports what is wired, what is stale, and what is missing;
`engramux doctor --full` stops masking its own output.

**`engramux update --from <dir>` cannot be your first step.** It refuses outright when no logon task
is registered, before it touches a file or stops the service, and tells you to run `install --apply`
instead. It is how an *existing* installation is replaced: it stops the service, waits for it,
copies the binaries, and starts it again. `scripts/reinstall.sh` wraps it and has the same
precondition. Running `update` with no `--from` at all says there is no delivery channel and points
at a directory you already have — it used to tell you to download a release archive, one line after
saying there is nothing to read from.

## Delivery, in the future tense

Today there is no channel for either host, so `update --from <directory>` is the only door, and it
is the same door for both.

When a channel exists it will be a GitHub Release as the substrate and a Claude Code plugin as the
channel, and the release archive will be the same artefact both ways — a Codex user who unpacks it
by hand gets bytes identical to what a plugin user receives. **What will be unequal is the
noticing.** Codex has a plugin system of its own, but as of 2026-09-03 it documents neither an
archive source nor an update command, so a Codex plugin could carry the binaries and could not fetch
a new release or say that one exists. That is a difference in convenience, not in capability, and it
is written here rather than left for a Codex user to discover.

## Where the decisions live

- `AGENTS.md` — how the work is done, the commands, and a long table of things that will bite you.
- `docs/superpowers/specs/` — decisions, invariants, budgets, and measurements.
- `docs/superpowers/backlog.md` — deferred findings no test owns yet.
- `docs/prompts/` — one work order per session, dated. A record, never updated.

This is one developer's project with no CI and no release process. Nothing here promises that an
issue will be answered.

## Licence

Apache-2.0. See `LICENSE`.
