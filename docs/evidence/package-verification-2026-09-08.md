# Local packaging verification, 2026-09-08

The unchanged packaging script produced the same verification archive twice from detached
source revision `653bf3a`. Both runs exited 0 and the newly built CLI reported
`0.1.0-verification` through the script's runtime doctor check. This is local packaging
evidence, not a release, installed update, CI release run or Defender validation.

The archive SHA-256 was identical in both runs and matched an independent `Get-FileHash`
check: `4ec59cd622179a256b577ad1aed89149e0c5a068130909550dea3824dabf8d80`.
The worktree's marketplace version and archive hash also matched the generated artifact.

The task-owned detached worktree is `.capture/package-verification`. It was created with
`git worktree add --detach .capture/package-verification HEAD` while HEAD was `653bf3a`.
That source includes the NUL-query fix. `git diff --name-only 653bf3a HEAD` at verification
time showed only the later transfer evaluation document and test, with no binary input
changes. Future source or packaged README changes require a fresh archive and hash.

The first attempted POSIX environment-prefix command was interpreted by the execution tool
as Windows command syntax and failed before creating `dist`. The successful invocation was
the installed Bash executable directly: `& 'C:\Program Files\Git\bin\bash.exe'
scripts/package.sh 0.1.0-verification`, with the worktree as working directory. The second
run also set process-local `GOPROXY=off`, `GOSUMDB=off` and `GOTOOLCHAIN=local` in PowerShell.
No guard was disabled, script rewritten, replacement packager used or global shell setting
changed. The script's final suggestion to commit and tag was not acted on.

Python's `zipfile` CRC verification passed. The archive contained exactly:

- `.claude-plugin/plugin.json`
- `LICENSE`
- `README.md`
- `engramux.exe`
- `engramux-service.exe`

PE header inspection found machine `0x8664` for both executables, CLI subsystem 3 and service
subsystem 2 (GUI). `go version -m` on both staged executables reported `CGO_ENABLED=0`,
`GOOS=windows`, `GOARCH=amd64` and `-trimpath=true`. These static settings do not replace the
runtime version check, which both script invocations performed.

The generated ZIP and staging directories remain in the isolated worktree's ignored `dist`.
Only that worktree's marketplace was modified. The main worktree's marketplace, installed
binaries and host configuration were unchanged. No tag, push or GitHub Release was created.
The archive is deliberately labelled verification and does not resolve publication conditions,
automatic-selection quality or the deferred Defender discussion.
