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
