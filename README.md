# Engramux

`github.com/wotjr1649/engramux`

Engramux captures Claude Code and Codex session hook events into SQLite and serves them back
through FTS5 and MCP. One service per Windows user multiplexes every concurrent session, across
every project. The hook side is a relay that is spawned once per event and exits, so it never
blocks the host and never fails one — it exits 0 on every path, panics included.

What it is for is capture you do not have to ask for. There is no `remember this` tool call: the
eleven events Claude Code and Codex have in common are recorded as they happen.

Reading is pull-only **by default** — a CLI, and six tools on an authenticated loopback MCP
endpoint. There is one push path and it ships off: hook-time context injection can write into
`UserPromptSubmit`, and it turns on only when a file named `inject.json` exists in the data
directory. Nothing in the installer writes that file, and its official owner-assessed gate has
not passed. Agent-estimated evaluations are recorded separately and do not authorize activation.

To resume a particular session, use `get_session_resume` with the project, host and
`host_session_id` from `list_sessions` or the session the user named. It returns the most recently
captured user prompt and main assistant reply, with event references for reading the full masked
records through `get_event`. These are historical records to verify, not a generated summary or
a claim about current workspace state. Delayed capture can differ from conversation order.

It exists because [`thedotmack/claude-mem`](https://github.com/thedotmack/claude-mem) does this job
but breaks constantly on Windows. Engramux is a reference reimplementation in Go — **not a fork**,
and **not a migration path**: there is no importer, and a new installation starts with an empty
database.

**Windows only.** Nothing here has been measured on any other platform, and no build tag enforces
it — the constraint comes from the dependencies. `golang.org/x/sys/windows` has no Go files at all
off Windows, and `github.com/Microsoft/go-winio`'s named-pipe symbols are behind a `windows`
constraint, so a build for another `GOOS` fails. `[unverified]`: nobody has run that build, and the
exact message is unrecorded. One artefact, `windows/amd64`; `arm64` is unmeasured.

## The release

**0.1.0 is the first one.** Until 2026-09-09 there was none at all: checked on 2026-09-08 with `gh
release list --repo wotjr1649/engramux --limit 5` and `git tag --list`, nothing was listed and the
repository had no tags, and the marketplace named the placeholder `0.0.0`. What held it was the four
publication conditions below, the last of which closed the day this shipped.

The archive is the whole of the release — one zip, `windows/amd64` — and the marketplace entry in
`.claude-plugin/` names its SHA-256, so the tag's own workflow rebuilds it and refuses the release
when the two disagree. Building from source stays supported and stays **the developer path**: the
spec rejects it as the primary route for end users, and the Defender section below is most of why.

Hosted checks run on every push: the [main checks run for 3e4b44c](https://github.com/wotjr1649/engramux/actions/runs/34190680850)
completed successfully, verified with `gh run view 34190680850 --repo wotjr1649/engramux --json
headSha,status,conclusion,url`. That covers a revision's hosted checks and neither the release
archive nor the private-corpus gates, which skip on a runner that has no captured corpus.

The version is `0.x` and there is no compatibility promise. Nothing outside `internal/` and `cmd/`
is exported — `pkg/` included — because a public API surface is a promise 1.0 has not earned. A
build that is not a release calls itself `0.0.0-dev`, with the first twelve characters of the commit
appended when the binary carries build information, and `.dirty` on top of that when the tree was
modified.

Four publication conditions are recorded in the memory spec, and **all four are now closed**. Three
closed on 2026-09-04: the bearer token's file permissions, a first install on a profile that had
never run these binaries, and a first run that antivirus did not touch. What that install measured is
in the spec; the short version is that it worked, including the part no disposable environment can
test — after a sign-out and a sign-in the service was already running, started by nothing but the
logon task.

**The last to close was this file, on 2026-09-09.** Condition 3 asks for the Defender
exclusion steps, and it is satisfied by a recorded procedure rather than by an absence — so it stayed
open through a machine on which Defender never fired. The steps are below, walked rather than
guessed.

Two things about the closed pair are worth knowing before they read as more than they are. That
install was onto a second machine belonging to the same person, not a stranger's. And the binary
antivirus left alone is **not** the binary that was quarantined here on 2026-09-03: it is a
different, smaller build from a different commit. Two machines, two builds, two verdicts.

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
install presents the same two shapes.

**On a second machine, on 2026-09-04, it did not happen at all.** Default settings, no exclusions in
place, first execution of freshly copied unsigned binaries, and nothing was touched. That is not the
all-clear it looks like, for a reason worth stating plainly: the build that was left alone is not
the build that was quarantined — it is smaller and from a later commit — so what the two runs
together show is that a `Behavior:` `!ml` verdict is a machine's opinion of a rare executable rather
than a property of these bytes. **Plan for it firing on yours.**

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

**1.0 will ship unsigned, and 1.0.1 onwards will carry a signature.** That order is forced rather
than preferred. No certificate is being bought; what replaces buying one is SignPath Foundation,
which signs qualifying open-source projects at OV level for free — and its first requirement is
that the project **already be released in the form to be signed**. So whatever the first release
is, it cannot be signed, and a pre-release existing only to clear that bar would buy nothing the
bar does not already force. If you meet an unsigned binary from this project, you were told here
first, which is the standard the fourth publication condition sets.

**The exclusion procedure, and the elevation is the whole of it. [verified] 2026-09-09.**
`Add-MpPreference -ExclusionPath` had been attempted once from an ordinary session and refused with
HRESULT `0xc0000142`, which says the call never started and does not say why: the session was not
elevated, or Tamper Protection was on, which is what that feature is for. **It was elevation.** The
same cmdlet run from an elevated PowerShell added the paths and `(Get-MpPreference).ExclusionPath`
read them back. An earlier revision of this paragraph told you the Windows Security UI was the only
route, which was an inference from that refusal rather than something anyone had tried.

**Exclude both directories**, because this detection has taken the CLI from each of them: `dist\`
under your checkout, and `%LOCALAPPDATA%\engramux\bin`. From an elevated PowerShell,
`Add-MpPreference -ExclusionPath` accepts them as one comma-separated list, and
`(Get-MpPreference).ExclusionPath` is how you confirm it rather than trusting a silent success.

**If it is still refused after you elevate, Tamper Protection is what is left**, and then the route
is the Windows Security UI by hand — Virus & threat protection, Manage settings, Exclusions, Add an
exclusion, Folder — once per directory. That branch is `[unverified]`: nobody here has reached it,
because elevation was the answer here.

Two things the exclusion does not do. It is a **path**, so moving your checkout leaves it pointing
at nothing and the next build is unprotected. And it is not retroactive: a binary already quarantined
comes back through Protection History, not through this.

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

These three run locally, in this order. A push and a pull request run the same three in the same
order on GitHub Actions — but a runner has no captured corpus and no native memory, so several
gates skip there that do not skip here, and a green run on a runner is the weaker statement of the
two. The last paragraph of this section is why.

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

**What it does not do is put itself on your `PATH`**, and that surprises people — the first person
it caught was the author, one command after installing. `install --apply` writes no environment
variable and touches no shell profile, so in your own terminal `engramux` is *not* a command until
you make it one. Until you do, spell the path: `%LOCALAPPDATA%\engramux\bin\engramux.exe`. Adding
that directory to your user `PATH` is yours to do, and **`setx` is the wrong tool for it** — it
truncates the value at 1024 characters and writes the truncation without failing.

**Inside Claude Code it is already a bare command.** The plugin ships `bin/engramux`, and Claude
Code puts every enabled plugin's `bin/` on the Bash tool's `PATH`. That file is a shim onto the
binary `install --apply` put in place rather than a second copy of it, so there is one build
answering the name however it is reached; run it before installing and it prints what to run
instead. None of that reaches your own shell, which is why the paragraph above exists.

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
precondition. **Since 0.1.2, `engramux update` with no arguments reads Claude Code's plugin cache.** That is
what makes updating the plugin mean something: the host fetches the release archive, checks its
SHA-256 and unpacks it, and this command then takes the newest complete version it finds there. It
names the version before it stops anything, an explicit `--from` still outranks it — which is what
keeps a `dist/` build from being undone by the released one — and with no plugin installed it says
how to install one instead of failing at you.

## Delivery, in the future tense

Today there is no release for either host, so `update --from <directory>` is the only door, and it
is the same door for both.

The channel it is waiting for is built and is a GitHub Release as the substrate with a Claude Code
plugin on top: one archive per version, and the marketplace entry in `.claude-plugin/` carries that
archive's SHA-256, so the host fetches it, checks it, and unpacks it into its plugin cache.
`engramux update` reads that directory, and `engramux doctor` says whether what is there is newer
than what is installed. **Nothing in this product fetches anything** — the party that downloads is
the host you already trust to download, and this codebase has still never made an outbound network
call.

**The release archive is the same artefact both ways.** A Codex user who downloads it and unpacks it
by hand gets bytes identical to what a plugin user receives. What is unequal is the noticing: Codex
has a plugin system of its own, but as of 2026-09-03 it documents neither an archive source nor an
update command, so a Codex plugin could carry the binaries and could not fetch a new release or say
that one exists. That is a difference in convenience, not in capability, and it is written here
rather than left for a Codex user to discover.

## Where the decisions live

- `AGENTS.md` — how the work is done, the commands, and a long table of things that will bite you.
- `docs/superpowers/specs/` — decisions, invariants, budgets, and measurements.
- `docs/superpowers/backlog.md` — deferred findings no test owns yet.
- `docs/prompts/` — one work order per session, dated. A record, never updated.

This is one developer's project. It has a release process and two check workflows now, and neither
has ever run on anything but that developer's machine. Nothing here promises that an issue will be
answered.

## Licence

Apache-2.0. See `LICENSE`.
