"""Persist root-agent judgements of already inspected development excerpts only."""
import collections
import hashlib
import json
import pathlib

root = pathlib.Path('.capture/selection-quality/transfer-2026-09-08')
raw = (root / 'development-anchor-replay.json').read_bytes()
digest = hashlib.sha256(raw).hexdigest()
if digest != 'f51e378691629d772367406ab3eef1bfb42b1155d5b0bdf9029ca0487f8c0dc2':
    raise SystemExit('reviewed replay hash mismatch')
records = [r for r in json.loads(raw) if r['Arm'] == 'anchor' and r['Wanted'] == 'yes']
# Order is the reviewed, strictly chronological wanted-prompt order, not event IDs.
# The final four excerpts lack enough context to bind them to the accepted recommendation.
labels = [[], ['no'], ['no'], ['no'], ['yes'], ['no'] * 7 + ['unknown'] * 4]
if len(records) != len(labels):
    raise SystemExit('wanted prompt count mismatch')
output = []
counts = collections.defaultdict(lambda: [0, 0])
for record, decisions in zip(records, labels, strict=True):
    blocks = record['Blocks'] or []
    if len(blocks) != len(decisions):
        raise SystemExit('reviewed block count mismatch')
    for block, label in zip(blocks, decisions, strict=True):
        output.append({'prompt_id': record['PromptID'], 'event_id': block['ID'],
                       'label': label, 'bytes': block['Bytes']})
        counts[label][0] += 1
        counts[label][1] += block['Bytes']
result = {'source': 'agent', 'scope': 'wanted development excerpts only',
          'replay_sha256': digest, 'labels': output,
          'limits': 'Relevance of the shown excerpt, not current factual accuracy or task success. Unknown is not no.'}
try:
    handle = (root / 'anchor-wanted-judgements.json').open('x', encoding='utf-8', newline='')
except FileExistsError:
    raise SystemExit('judgements already exist; refusing overwrite') from None
with handle:
    json.dump(result, handle, ensure_ascii=True, indent=2)
    handle.write('\n')
print(json.dumps(dict(counts), sort_keys=True))
