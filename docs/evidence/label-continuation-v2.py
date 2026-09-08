"""Write separate root-agent excerpt judgements for the reviewed development outputs."""
import collections
import hashlib
import json
import pathlib

root = pathlib.Path('.capture/selection-quality')
cases = [
    ('continuation-v2-temporal-wanted-review.json',
     '1b8419118fa189d721fdc9921f7bcb1d2074396f83a9b10590650a546a602647',
     ['no', 'yes', 'yes', 'yes', 'yes', 'unknown', 'yes', 'unknown', 'yes', 'yes', 'yes', 'yes', 'unknown', 'yes']),
    ('transfer-2026-09-08/development-continuation-v2-replay.json',
     '93df02bbe6488b2968b00dbf02c9fe10101dffb8178ea56fc0e51b5ce8f59c72',
     ['unknown', 'yes']),
]
for ordinal, (name, digest, labels) in enumerate(cases, 1):
    raw = (root / name).read_bytes()
    if hashlib.sha256(raw).hexdigest() != digest:
        raise SystemExit('reviewed output hash mismatch')
    rows = json.loads(raw)
    if ordinal == 2:
        rows = [dict(PromptID=r['PromptID'], EventID=b['ID'], Bytes=b['Bytes'])
                for r in rows if r['Arm'] == 'continuation' for b in (r['Blocks'] or [])]
    if len(rows) != len(labels):
        raise SystemExit('reviewed block count mismatch')
    counts = collections.defaultdict(lambda: [0, 0])
    judgements = []
    for row, label in zip(rows, labels, strict=True):
        counts[label][0] += 1
        counts[label][1] += row['Bytes']
        judgements.append({'prompt_id': row['PromptID'], 'event_id': row['EventID'],
                           'bytes': row['Bytes'], 'label': label})
    destination = root / ('continuation-v2-old-judgements.json' if ordinal == 1 else 'continuation-v2-transfer-judgements.json')
    if destination.exists():
        raise SystemExit('judgements already exist; refusing overwrite')
    with destination.open('x', encoding='utf-8', newline='') as handle:
        json.dump({'source': 'agent', 'output_sha256': digest, 'labels': judgements,
                   'limits': 'Excerpt relevance only; not owner intent, current fact verification or task success.'}, handle, indent=2)
        handle.write('\n')
    print(json.dumps({'population': ordinal, 'counts': dict(counts)}, sort_keys=True))
