## What changed since 0.1.0

**`engramux` is now a bare command inside Claude Code.** The archive carries `bin/engramux`, and
Claude Code puts every enabled plugin's `bin/` on the Bash tool's `PATH`. It is a **shim onto the
installed CLI**, not a second copy of it: a plugin pinned at one version answering for a service
running another is exactly what `doctor`'s *"installed and running agree"* line exists to prevent.
Run it before `install --apply` and it tells you what to run instead of failing at you.

**The README now says that installing does not put anything on your `PATH`.** It never did, it never
claimed to, and the first person that caught was the author — one command after installing 0.1.0.
In your own terminal the CLI is `%LOCALAPPDATA%\engramux\bin\engramux.exe` until you add that
directory yourself, and the README now also warns that `setx` is the wrong tool for adding it: it
truncates `PATH` at 1024 characters and writes the truncation without failing.

Nothing else changed. No Go source outside a new test, no schema, no migration, no host
configuration, and the service, the hooks and the MCP endpoint behave exactly as they did in 0.1.0.

## Still true from 0.1.0

Hook-time context injection is built and ships disabled. `CLAUDE_CONFIG_DIR` is still ignored by
every path derived under `~/.claude`, and that is still a known limitation rather than a fix —
backlog 54 carries it. The Windows Defender exclusion procedure is in the README and has been
walked.
