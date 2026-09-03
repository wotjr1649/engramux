#!/usr/bin/env python3
"""Exact-text replacement in one file, newline-preserving.

Written because `pathlib.write_text` opens in text mode and turns every \\n into
CRLF on Windows (AGENTS.md's own row on it), and because a heredoc'd sed is how
a Windows path literal loses a backslash. Reads the old and new text from files
so that nothing passes through a shell quoting layer.

Usage: patch.py <target> <old-file> <new-file>
Exits non-zero unless the old text occurred exactly once.
"""

import io
import sys


def main() -> int:
    target, oldpath, newpath = sys.argv[1], sys.argv[2], sys.argv[3]
    s = io.open(target, encoding="utf-8", newline="").read()
    old = io.open(oldpath, encoding="utf-8", newline="").read()
    new = io.open(newpath, encoding="utf-8", newline="").read()
    n = s.count(old)
    if n != 1:
        print("patch.py: %d occurrences in %s, want 1" % (n, target), file=sys.stderr)
        return 1
    io.open(target, "w", encoding="utf-8", newline="").write(s.replace(old, new, 1))
    return 0


if __name__ == "__main__":
    sys.exit(main())
