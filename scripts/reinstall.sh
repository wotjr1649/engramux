#!/usr/bin/env bash
# Replaces the installed Engramux with what is in dist/, and checks it answered.
#
# It used to be six commands and two traps. Both traps are now inside the binary,
# in `engramux update`, which is what Step 6 was for:
#
#   * `schtasks /end` returns before the process is gone, so a `/run` straight
#     after it leaves nothing running - the new instance loses the pipe race
#     (I-09) and exits, then the old one finishes dying. `update` waits for the
#     service to stop *answering* before it touches a file, which is the only
#     evidence that the image is free.
#   * MSYS path conversion turns `/end` into a path. Nothing here runs schtasks
#     any more, so nothing here needs MSYS_NO_PATHCONV.
#
# What is left is three commands, and M-7 predicted that: "nearly empty" is the
# answer rather than "entirely", because `update` alone does not tell you the
# thing came back healthy.
#
# `$cli` stays dist's copy for the update call and that is not incidental.
# Windows will not let a running image overwrite itself, so `update` refuses
# when it is the installed copy - running dist's is what makes it work at all.
#
# What it deliberately does not do is verify the build. The suite, the pinned
# linter and the race script are AGENTS.md's, in that order, and the race script
# alone is about nine minutes - putting it here would make a reinstall something
# nobody runs.
#
# It also does not check whether an agent is allowed to run it. That rule is in
# AGENTS.md and it is about the agent, not about installing: baking it in here
# would refuse the owner's own first install on a machine where the hosts are
# not registered yet, which is the one time it is certainly right to run.
#
# One thing `update` does not do that `install --apply` did: re-register the
# logon task. That is the division rather than a gap - `update` refuses outright
# when the task points somewhere other than the installed service, and says to
# run `install --apply`. A machine that has never installed is the same answer.
#
# Usage: scripts/reinstall.sh [--no-verify]
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cli="$repo/dist/engramux.exe"
# `if` and not `[ ... ] && x`: under `set -e` an AND-list whose left side is
# false returns non-zero and ends the script, so the terse form would exit here
# on every run that did *not* pass the flag - which is every ordinary run.
verify=1
if [ "${1:-}" = "--no-verify" ]; then
    verify=0
fi

if [ ! -x "$cli" ]; then
    echo "reinstall.sh: no $cli - build it first, with the commands in AGENTS.md" >&2
    exit 1
fi
if [ ! -x "$repo/dist/engramux-service.exe" ]; then
    echo "reinstall.sh: no $repo/dist/engramux-service.exe - build it first" >&2
    exit 1
fi

echo "reinstall.sh: update"
"$cli" update --from "$repo/dist"

if [ "$verify" -eq 0 ]; then
    exit 0
fi

# The installed copy and not dist's, because what is being checked is what a
# user runs. The path is spec 5.6's layout rather than something parsed out of
# doctor: that output is masked by default since M-6, so the path it prints is
# not one anything can execute.
#
# `doctor` is the whole verification - it reads the task, both binaries, both
# hosts' hook entries, the spool, the log and the service, and exits non-zero
# when any of that is wrong. status is printed beside it because a green doctor
# still says nothing about whether the thing has any data in it.
# cygpath, because %LocalAppData% is a Windows path with backslashes and this
# shell cannot open the mixed form - it answers "No such file or directory" and
# the fallback below would then quietly test dist's copy while this comment said
# otherwise. Measured: that is exactly what happened the first time.
local_appdata="${LOCALAPPDATA:-}"
if command -v cygpath >/dev/null 2>&1 && [ -n "$local_appdata" ]; then
    local_appdata="$(cygpath -u "$local_appdata")"
fi
installed="$local_appdata/engramux/bin/engramux.exe"
if [ -x "$installed" ]; then
    cli="$installed"
fi

echo "reinstall.sh: doctor"
"$cli" doctor
echo "reinstall.sh: status"
"$cli" status
