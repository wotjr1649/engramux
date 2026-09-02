#!/usr/bin/env bash
# Replaces the installed Engramux with what is in dist/, and checks it answered.
#
# Six commands become one, and the two that are easy to get wrong are the reason
# this exists rather than a note in a document.
#
#   * `schtasks /end` returns before the process is gone. Running
#     `install --apply` straight after it starts a second instance while the
#     first is still exiting, that instance loses the pipe race (I-09) and
#     exits, and then the first finishes dying - leaving no service at all, with
#     a log line that reads like a singleton conflict rather than like an empty
#     machine. The wait below is what avoids it, and polling for a `status` that
#     *succeeds* would prove nothing: during the gap it is answered by the
#     instance on its way out.
#   * MSYS path conversion turns `/end` into a path. MSYS_NO_PATHCONV=1 is set
#     for exactly the two schtasks calls and nothing else.
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

echo "reinstall.sh: stopping the logon task"
MSYS_NO_PATHCONV=1 schtasks /end /tn Engramux >/dev/null 2>&1 || true

echo "reinstall.sh: waiting for the old service to stop answering"
for _ in $(seq 1 60); do
    "$cli" status >/dev/null 2>&1 || break
    sleep 1
done
if "$cli" status >/dev/null 2>&1; then
    echo "reinstall.sh: a service is still answering after 60s; not installing over it" >&2
    exit 1
fi

echo "reinstall.sh: installing"
"$cli" install --apply

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
