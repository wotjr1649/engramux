#!/usr/bin/env bash
# Runs the test suite under the race detector.
#
# -race needs CGO_ENABLED=1 and a real C toolchain; there is no CGO-free route
# on windows/amd64 (golang/go#6508). Shipped binaries stay CGO_ENABLED=0 — this
# script is the carve-out, and it only ever builds test binaries.
#
# The compiler is discovered, not hardcoded, so this file carries no machine
# path. Search order:
#   1. $ENGRAMUX_CC
#   2. $CC
#   3. ../_tools/mingw64/bin/gcc.exe, beside the repository
#   4. gcc on PATH
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

for candidate in \
    "${ENGRAMUX_CC:-}" \
    "${CC:-}" \
    "$repo/../_tools/mingw64/bin/gcc.exe" \
    "$(command -v gcc 2>/dev/null || true)"
do
    # -f as well as -x: a directory is "executable" and would slip through,
    # then fail much later inside --print-file-name with a confusing message.
    [ -n "$candidate" ] && [ -f "$candidate" ] && [ -x "$candidate" ] && { cc="$candidate"; break; }
done

if [ -z "${cc:-}" ]; then
    cat >&2 <<'MSG'
race.sh: no C compiler found.

The race detector cannot run without one. Get a portable mingw-w64 — this is an
extraction, not an install: no administrator rights, no PATH change, no
registry, and deleting the folder removes it completely.

  1. Download the x86_64 posix-seh-ucrt archive from
     https://github.com/niXman/mingw-builds-binaries/releases/latest
  2. Extract it next to this repository, so that ../_tools/mingw64/bin/gcc.exe
     exists. On Windows the built-in tar handles .7z:
       tar -xf <archive>.7z
  3. Or point ENGRAMUX_CC at a compiler you already have.
MSG
    exit 1
fi

# The official adequacy check. A toolchain too old for -race echoes the argument
# back instead of resolving it. Go's linker silently omits -lsynchronization in
# that case and the link fails on WaitOnAddress, far from the real cause.
resolved="$("$cc" --print-file-name libsynchronization.a)"
if [ "$resolved" = "libsynchronization.a" ]; then
    echo "race.sh: $cc cannot resolve libsynchronization.a — too old for -race." >&2
    echo "         Needs mingw-w64 runtime v8 or later." >&2
    exit 1
fi

echo "race.sh: using $cc"
"$cc" --version | head -1

# -p 1 for the same reason as an ordinary run: tests share one database file and
# fixed pipe names.
#
# The timeout is per test binary and not for the run, and 30m was not generous
# enough. Measured 2026-09-06: internal/search alone is **2521 s, or 42 minutes**
# under -race - it is the corpus gates, which are one package's share of the tree
# and most of its wall clock - and at 30m the package died with
# `panic: test timed out after 30m0s` naming whichever test happened to be
# running, which reads like that test hanging rather than like the budget. The
# figure it replaces came from an estimate ("-race adds 5-15x") rather than from
# a stopwatch; 90m is that measurement with room for a loaded machine, and it is
# still a hang guard rather than a performance budget. Re-measure it when a gate
# is added, and raise it here rather than skipping the gate.
#
# Re-measured 2026-09-07 with gate M14 added: internal/search is **3473.0 s, or
# 57.9 minutes**, so 32 minutes of this guard are left rather than 48. The four
# readings before it spread 2110.8 s to 2563 s, which is the machine's own
# spread of about 450 s - this one is 910 s above the highest of them, so the
# size is the gate and not the day. 90m is not raised here because it still
# holds, but one more corpus gate of M14's size does not fit under it.
CGO_ENABLED=1 CC="$cc" exec go test -race -p 1 -timeout 90m "$@" ./...
