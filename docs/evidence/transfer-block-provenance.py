"""Audit selected development block event types; print only fixed enums and counts."""
import collections
import hashlib
import json
import pathlib
import sqlite3

root = pathlib.Path('.capture/selection-quality/transfer-2026-09-08')
path = root / 'development-anchor-replay.json'
if path.stat().st_size > 2097152:
    raise SystemExit('replay exceeds audit bound')
raw = path.read_bytes()
records = json.loads(raw)
judgements = json.loads((root / 'anchor-wanted-judgements.json').read_bytes())
if judgements['source'] != 'agent' or judgements['replay_sha256'] != hashlib.sha256(raw).hexdigest():
    raise SystemExit('judgement binding mismatch')
labels = {(j['prompt_id'], j['event_id']): j['label'] for j in judgements['labels']}
if len(records) != 46:
    raise SystemExit('unexpected replay size')
counts = collections.defaultdict(lambda: [0, 0])
scopes = collections.defaultdict(lambda: [0, 0])
relevance_scopes = collections.defaultdict(lambda: [0, 0])
snapshot = pathlib.Path('.capture/session-resume-2026-09-08/engramux.db').resolve()
known = {'UserPromptSubmit', 'Stop', 'SubagentStop', 'PostToolUse', 'SessionStart', 'SessionEnd', 'PreToolUse'}
with sqlite3.connect(snapshot.as_uri() + '?mode=ro', uri=True) as db:
    db.execute('PRAGMA query_only=ON')
    for record in records:
        arm, wanted = record['Arm'], record['Wanted']
        if arm not in ('baseline', 'anchor') or wanted not in ('yes', 'no'):
            raise SystemExit('unexpected label')
        blocks = record['Blocks'] or []
        trigger = db.execute('SELECT host,session_id,project_id,received_at FROM events WHERE id=?',
                             (record['PromptID'],)).fetchone()
        if trigger is None:
            raise SystemExit('trigger missing')
        if len(blocks) > 200:
            raise SystemExit('block bound exceeded')
        for block in blocks:
            row = db.execute('SELECT event_name,host,session_id,project_id,received_at FROM events WHERE id=?',
                             (block['ID'],)).fetchone()
            if row is None:
                raise SystemExit('selected event missing')
            kind = row[0] if row[0] in known else 'other'
            size = block['Bytes']
            if not isinstance(size, int) or not 0 <= size <= 5000:
                raise SystemExit('invalid block size')
            counts[(arm, wanted, kind)][0] += 1
            counts[(arm, wanted, kind)][1] += size
            if row[3] != trigger[2] or not 0 < row[4] < trigger[3]:
                raise SystemExit('selected block outside project or prefix')
            scope = 'same_session' if row[1:3] == trigger[0:2] else 'other_session'
            scopes[(arm, wanted, scope)][0] += 1
            scopes[(arm, wanted, scope)][1] += size
            if arm == 'anchor' and wanted == 'yes':
                label = labels.get((record['PromptID'], block['ID']))
                if label not in ('yes', 'no', 'unknown'):
                    raise SystemExit('missing or invalid excerpt judgement')
                relevance_scopes[(label, scope)][0] += 1
                relevance_scopes[(label, scope)][1] += size
print(json.dumps([
    {'arm': arm, 'wanted': wanted, 'event': kind, 'blocks': values[0], 'bytes': values[1]}
    for (arm, wanted, kind), values in sorted(counts.items())
], indent=2))
print(json.dumps([
    {'arm': arm, 'wanted': wanted, 'scope': scope, 'blocks': values[0], 'bytes': values[1]}
    for (arm, wanted, scope), values in sorted(scopes.items())
], indent=2))
print(json.dumps([
    {'relevance': label, 'scope': scope, 'blocks': values[0], 'bytes': values[1]}
    for (label, scope), values in sorted(relevance_scopes.items())
], indent=2))
