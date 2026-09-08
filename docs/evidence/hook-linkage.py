"""Read-only, bounded field-presence audit; never print payloads or identifiers."""
import collections
import json
import pathlib
import sqlite3

snapshot = pathlib.Path('.capture/session-resume-2026-09-08/engramux.db').resolve()
fields = ('prompt_id', 'turn_id', 'parent_uuid', 'parentUuid', 'parent_id', 'message_id', 'uuid', 'tool_use_id')
kinds = ('UserPromptSubmit', 'Stop')
counts = collections.Counter()
presence = collections.Counter()
identities = collections.defaultdict(set)
groups = collections.defaultdict(collections.Counter)
invalid = oversized = 0
with sqlite3.connect(snapshot.as_uri() + '?mode=ro', uri=True) as db:
    db.execute('PRAGMA query_only=ON')
    for host, kind, session, project, size, payload in db.execute(
        "SELECT host,event_name,session_id,project_id,length(CAST(payload AS BLOB)),"
        "CASE WHEN length(CAST(payload AS BLOB))<=1048576 THEN payload ELSE NULL END "
        "FROM events WHERE host IN ('claude-code','codex') AND event_name IN ('UserPromptSubmit','Stop') LIMIT 100001"
    ):
        counts[(host, kind)] += 1
        if sum(counts.values()) > 100000:
            raise SystemExit('event bound exceeded')
        if size > 1048576:
            oversized += 1
            continue
        try:
            obj = json.loads(payload)
        except (ValueError, TypeError):
            invalid += 1
            continue
        if not isinstance(obj, dict):
            invalid += 1
            continue
        for field in fields:
            if field not in obj:
                continue
            value = obj[field]
            shape = 'nonempty_string' if isinstance(value, str) and value else 'other'
            presence[(host, kind, field, shape)] += 1
            if shape == 'nonempty_string' and len(value) <= 256:
                identities[(host, kind, field)].add((session, value))
                groups[(host, session, project, field, value)][kind] += 1
report = {
    'events': [{'host': h, 'event': k, 'count': n} for (h, k), n in sorted(counts.items())],
    'presence': [{'host': h, 'event': k, 'field': f, 'shape': s, 'count': n} for (h, k, f, s), n in sorted(presence.items())],
    'shared_within_session': [{'host': h, 'field': f, 'count': len(identities[(h, kinds[0], f)] & identities[(h, kinds[1], f)])} for h in ('claude-code', 'codex') for f in fields],
    'invalid': invalid,
    'oversized': oversized,
    'pair_multiplicity': [
        {'host': host, 'one_prompt_one_stop': sum(g[kinds[0]] == 1 and g[kinds[1]] == 1 for key, g in groups.items() if key[0] == host),
         'paired_but_multiple': sum(g[kinds[0]] > 0 and g[kinds[1]] > 0 and (g[kinds[0]] != 1 or g[kinds[1]] != 1) for key, g in groups.items() if key[0] == host),
         'prompt_only': sum(g[kinds[0]] > 0 and g[kinds[1]] == 0 for key, g in groups.items() if key[0] == host),
         'stop_only': sum(g[kinds[0]] == 0 and g[kinds[1]] > 0 for key, g in groups.items() if key[0] == host)}
        for host in ('claude-code', 'codex')
    ],
}
print(json.dumps(report, indent=2))
