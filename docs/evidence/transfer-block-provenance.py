"""Audit selected development block event types; print only fixed enums and counts."""
import collections
import json
import pathlib
import sqlite3

root = pathlib.Path('.capture/selection-quality/transfer-2026-09-08')
path = root / 'development-anchor-replay.json'
if path.stat().st_size > 2097152:
    raise SystemExit('replay exceeds audit bound')
records = json.loads(path.read_bytes())
if len(records) != 46:
    raise SystemExit('unexpected replay size')
counts = collections.defaultdict(lambda: [0, 0])
snapshot = pathlib.Path('.capture/session-resume-2026-09-08/engramux.db').resolve()
known = {'UserPromptSubmit', 'Stop', 'SubagentStop', 'PostToolUse', 'SessionStart', 'SessionEnd', 'PreToolUse'}
with sqlite3.connect(snapshot.as_uri() + '?mode=ro', uri=True) as db:
    db.execute('PRAGMA query_only=ON')
    for record in records:
        arm, wanted = record['Arm'], record['Wanted']
        if arm not in ('baseline', 'anchor') or wanted not in ('yes', 'no'):
            raise SystemExit('unexpected label')
        blocks = record['Blocks'] or []
        if len(blocks) > 200:
            raise SystemExit('block bound exceeded')
        for block in blocks:
            row = db.execute('SELECT event_name FROM events WHERE id=?', (block['ID'],)).fetchone()
            if row is None:
                raise SystemExit('selected event missing')
            kind = row[0] if row[0] in known else 'other'
            size = block['Bytes']
            if not isinstance(size, int) or not 0 <= size <= 5000:
                raise SystemExit('invalid block size')
            counts[(arm, wanted, kind)][0] += 1
            counts[(arm, wanted, kind)][1] += size
print(json.dumps([
    {'arm': arm, 'wanted': wanted, 'event': kind, 'blocks': values[0], 'bytes': values[1]}
    for (arm, wanted, kind), values in sorted(counts.items())
], indent=2))
