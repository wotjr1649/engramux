# Configuration-home resolution diagnosis, 2026-09-08

Backlog 54 is reproduced without reading or modifying host configuration.
`go test -p 1 -count=1 -timeout 2m -run '^TestMeasureClaudeConfigHomeResolution$' -v ./cmd/engramux`
used three temporary directories, set CLAUDE_CONFIG_DIR and cleared the explicit settings/cache
test overrides. Both reported comparisons were false: settings and plugin cache still resolve
under the supplied default home. The diagnostic process passed while reporting that defect;
it is not a correctness gate or an implementation fix.

The current [official environment reference](https://code.claude.com/docs/en/env-vars) states
that CLAUDE_CONFIG_DIR moves settings, session history and plugins. The
[settings documentation](https://code.claude.com/docs/en/settings) repeats that rule and treats
the global app-state/MCP file separately. These sources were inspected on 2026-09-08. They
confirm the two observed path failures but do not explicitly settle the relocated global
app-state filename used by the installed binary.

The local code has a consistency risk beyond displaying a wrong path. internal/memory's
ClaudeHome honors the override, while cmd/engramux's resolvePaths hardcodes the default home.
In internal/host/install.go, the ClaudeMCP path feeds PointsAtEndpoint before deciding whether
to invoke registration. A wrong MCP-state path can therefore defeat the already-registered
check. Fixing settings/cache alone would leave that installation decision unresolved.

An independent reviewer received only a conceptual description and confirmed a minimal pure
resolver matrix: default home, overridden home, explicit ENGRAMUX overrides, and Windows paths.
It also identified the global MCP-state location as a separate fact to verify. No private
configuration or captured session was provided to that reviewer.

An attempted static inspection of the installed Claude executable was rejected by the
PreToolUse credential-path guard because the command contained the global state filename.
The command did not execute. No alternate shell, encoding, file-write script or delegate was
used to repeat that inspection. Public official-document inspection continued independently;
no assertion about the installed binary's exact relocation behavior is made.

Implementation remains pending the exact global-state rule and supported environment-path
semantics. No host files, installer registration, dependency graph or product behavior changed.
The diagnostic only calls resolvePaths and uses t.Setenv/t.TempDir; it never calls install,
doctor, register or unregister. Do not close backlog 54 based on this test's exit code.

## Closed 2026-09-09, and the open fact was settled by the host rather than by a document

**The global application-state file moves with the variable, and it moves *inside* the directory
the variable names.** That was the one thing this diagnosis could not settle, and neither
documentation page answered it — both fetches came back truncated before the entry. The installed
host answered it in two commands. With `CLAUDE_CONFIG_DIR` pointed at an empty temporary directory,
`claude mcp list` printed *"No MCP servers configured. Use `claude mcp add` to add a server."*; the
same command without the override printed this product's endpoint and `✔ Connected`. The host then
wrote its own copy of that file, and a `backups/` directory beside it, into the directory the
variable named.

**So the rule has two shapes rather than one, which is why a single join would have been wrong.**
Unset, the file is a *sibling* of the configuration home. Set, it is a *child* of it. Settings and
the plugin cache are children in both arms.

**What was implemented.** `internal/claudehome`, a leaf importing `os`, `path/filepath` and
`strings` and nothing else, so that `cmd/engramux` can reach it without reaching `database/sql` —
`TestTheRelayDoesNotLinkTheSQLiteDriver` ran and still passes. `resolvePaths` and
`internal/memory.ClaudeHome` are now both callers of it rather than two spellings, which is the
consistency risk this document identified: `ClaudeMCP` feeds `PointsAtEndpoint`, so the wrong path
there defeats the already-registered check rather than only printing a wrong line.

**What holds it.** `cmd/engramux/config_home_test.go` replaces the diagnostic this document
describes, and asserts both arms of all three paths plus the precedence of the `ENGRAMUX_*`
overrides, and that the Codex paths do not follow a Claude Code variable. Two mutations were
applied and both were caught: joining the application-state file onto the configuration home in the
default arm fails the defaults test, and inverting the override test in `Dir` fails both. The pinned
linter reports 0 issues at exit 0.

**Still unverified, deliberately.** Nobody has run an install on a machine that actually sets the
variable; what is measured is the resolver and the host's own behaviour, not an end-to-end install
under the override. And the credential-path guard refused the command that would have confirmed the
default location by stat — the default is taken from the shape this repository already shipped and
from `doctor` reporting both hosts correctly wired on a machine with the variable unset. No guard
was worked around in either session.
