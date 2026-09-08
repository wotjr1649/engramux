"""Freeze metadata-only cross-project splits before opening prompt text."""
import contextlib
import hashlib
import json
import pathlib
import sqlite3

root = pathlib.Path.cwd()
old = root / '.capture/m7/snapshot/engramux.db'
source = root / '.capture/session-resume-2026-09-08/engramux.db'
prior = root / '.capture/selection-quality/followup-2026-09-08/manifest.json'
dest = root / '.capture/selection-quality/transfer-2026-09-08'
if dest.exists():
    raise SystemExit('transfer split already exists; refusing regeneration')

def digest(path):
    with path.open('rb') as data:
        return hashlib.file_digest(data, 'sha256').hexdigest()

binding = {'old_db': digest(old), 'db': digest(source), 'wal': digest(pathlib.Path(str(source)+'-wal')), 'prior_manifest': digest(prior)}
excluded_project = json.loads(prior.read_text(encoding='utf-8'))['project_id']
with contextlib.closing(sqlite3.connect(old.as_uri()+'?mode=ro',uri=True)) as before, contextlib.closing(sqlite3.connect(source.as_uri()+'?mode=ro',uri=True)) as after:
    excluded_sessions = {r[0] for r in before.execute('SELECT id FROM sessions')}
    rows = after.execute("SELECT id,session_id,project_id,received_at FROM events WHERE event_name='UserPromptSubmit' ORDER BY received_at,id").fetchall()
    # Exclude whole sessions, including any that also touched the project
    # whose development and holdout prompts have already been inspected.
    excluded_sessions.update(r[1] for r in rows if r[2] == excluded_project)
    remaining = [r for r in rows if r[1] not in excluded_sessions and r[1] and r[2] and r[3] > 0]

sessions = sorted({r[1] for r in remaining}, key=lambda s: (hashlib.sha256(s.encode('utf-8')).hexdigest(), s))
if len(sessions) < 2:
    raise SystemExit('not enough unexposed sessions')
development = set(sessions[:len(sessions)//2])
arms = {name: [] for name in ('development', 'holdout')}
for event, session, project, stamp in remaining:
    arm = 'development' if session in development else 'holdout'
    arms[arm].append({'event_id': event, 'session_id': session, 'project_id': project, 'received_at': stamp})
if digest(source) != binding['db'] or digest(pathlib.Path(str(source)+'-wal')) != binding['wal']:
    raise SystemExit('snapshot changed during metadata read')
manifest = {
    'source': 'metadata-only split, no prompt or selector output read',
    'digests': binding,
    'split_rule': 'SHA-256 of stored session UTF-8 bytes, ascending with string tie-break; first floor(n/2) development',
    'limitations': 'Same user, different projects; aggregate hook-field metadata was previously audited. Not new-user or fully independent evidence.',
    'arms': arms,
}
dest.mkdir()
with (dest/'manifest.json').open('x', encoding='utf-8', newline='') as output:
    json.dump(manifest, output, indent=2)
print(json.dumps({name: {'sessions': len({r['session_id'] for r in rows}), 'prompts': len(rows), 'projects': len({r['project_id'] for r in rows})} for name, rows in arms.items()}))
