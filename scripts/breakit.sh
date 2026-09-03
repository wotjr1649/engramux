#!/usr/bin/env bash
# breakit.sh - one deliberate mutation, asserted present, then the suite.
#
# AGENTS.md's "TDD, then break it" steps 3-5, with the three failure modes that
# section names made loud rather than skipped:
#
#   - a mutation that never applied reads exactly like a passing suite, so the
#     replacement is asserted present before anything runs (NOOP);
#   - a mutation that does not compile reads exactly like a test that does not
#     care, so a build failure is its own verdict (BUILD);
#   - `git checkout -- <file>` restores HEAD and not the working tree, so this
#     refuses a dirty tree rather than deleting uncommitted work.
#
# Usage: scripts/breakit.sh <file> <old> <new> <package> [run-regexp]
set -u

file=$1; old=$2; new=$3; pkg=$4; run=${5:-}

if [ -n "$(git status --porcelain -- "$file")" ]; then
  echo "DIRTY  $file has uncommitted changes; commit before a break-it pass"
  exit 2
fi

python - "$file" "$old" "$new" <<'PY'
import io, sys
path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
s = io.open(path, encoding="utf-8", newline="").read()
if s.count(old) != 1:
    sys.exit("NOOP   %d occurrences of the old text in %s" % (s.count(old), path))
io.open(path, "w", encoding="utf-8", newline="").write(s.replace(old, new, 1))
PY
if [ $? -ne 0 ]; then exit 3; fi

# Asserted present: the file on disk now differs from HEAD, and by this exact
# replacement. A sed address that missed or a literal that did not match the
# file's bytes is caught here rather than reported as `ok`.
if ! git diff --quiet -- "$file"; then
  args=(-p 1 -count=1)
  [ -n "$run" ] && args+=(-run "$run")
  out=$(go test "${args[@]}" "$pkg" 2>&1)
  code=$?
  if printf '%s' "$out" | grep -qE '(build failed|\[build failed\]|cannot use|undefined:|declared and not used|imported and not used)'; then
    verdict="BUILD  the mutation does not compile; discarded, not killed"
  elif [ $code -ne 0 ]; then
    verdict="KILLED $(printf '%s' "$out" | grep -E '^\s+--- FAIL|^--- FAIL' | head -3 | tr -s ' ')"
  else
    verdict="SURVIVED  the suite is green with the mutation in place"
  fi
else
  verdict="NOOP   the file is unchanged after the replacement"
fi

git checkout -- "$file"
echo "$verdict"
