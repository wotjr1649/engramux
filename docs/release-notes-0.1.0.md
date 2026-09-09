## What this release is

The first one. Engramux captures every Claude Code and Codex session hook event — the eleven the two
hosts have in common — into SQLite as it happens, across every project, and indexes both hosts' own
native memory files beside them. One service per Windows user multiplexes every concurrent session.
The hook side is a relay spawned once per event that exits, so it never blocks the host and never
fails one.

Reading is pull-only: a CLI, six tools on an authenticated loopback MCP endpoint, and a `recall`
skill this release adds so that a model has a reason to call them. Without it the tools reach the
host as names with no description and nothing prompts the call.

`0.x`, so there is no compatibility promise, and nothing outside `internal/` and `cmd/` is exported.

**Read the README before `install --apply`.** It writes two host configuration files per host,
eleven hook entries each, a logon task, two binaries and a data directory — and what it then records
is raw prompts, file contents, tool output and the paths you work in, continuously, for both agents.
That is the product rather than a side effect, and the README says who on your machine can read it.

## What it does not do

**Hook-time context injection is built and ships disabled.** It turns on only when a file named
`inject.json` exists in the data directory, nothing in the installer writes that file, and its
owner-assessed gate has not passed. It belongs to 0.2.0, not to this release.

There is no importer and no migration path from `thedotmack/claude-mem`. A new installation starts
with an empty database.

## Known limitation: `CLAUDE_CONFIG_DIR` is ignored

**If you set `CLAUDE_CONFIG_DIR`, this release will not find your Claude Code configuration.** Every
path it derives under `~/.claude` — the settings file and the plugin cache — is spelled from your
home directory and ignores the variable that moves it, while the native-memory reader honours it. On
such a machine `doctor` reports the eleven Claude Code hook entries missing, `install --apply` writes
them where the host will not read them, and the plugin cache line says there is nothing there. Codex
is unaffected.

This is read from the source rather than observed: nobody has met it, and nothing measures how many
users set that variable. It ships as a known limitation rather than holding the release because the
fix is one resolution shared by both halves — a plugin cache honouring the variable beside a settings
file that does not is worse than neither — and that changes product behaviour. Backlog 54 carries it.

## If Defender takes the CLI

The exclusion procedure is in the README and it has been walked: from an elevated PowerShell,
`Add-MpPreference -ExclusionPath` takes both directories at once, and
`(Get-MpPreference).ExclusionPath` reads them back. The README says which two directories, why
building from source makes this worse rather than better, and what to do if elevation is not what
was refusing you.
