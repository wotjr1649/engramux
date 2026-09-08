"""Exact acceptance for the fixed public four-arm history experiment."""
import hashlib
import json
from pathlib import Path

root = Path(__file__).parent
design_bytes = (root / 'incremental-tasks-2026-09-08.json').read_bytes()
design = json.loads(design_bytes)
output = json.loads((root / 'incremental-output-2026-09-08.json').read_bytes())
if output['design_sha256'] != hashlib.sha256(design_bytes).hexdigest():
    raise SystemExit('task design changed after selector run')
for item in output['results']:
    if item['sha256'] != hashlib.sha256(item['text'].encode()).hexdigest():
        raise SystemExit('selector output changed')
answers = json.loads((root / 'incremental-answers-2026-09-08.json').read_bytes())
arms = ('absent_off', 'absent_on', 'visible_off', 'visible_on')
if set(answers) != set(arms):
    raise SystemExit('incomplete arms')
ids = {case['id'] for case in design['cases']}
for arm in arms:
    if set(answers[arm]) != ids:
        raise SystemExit('incomplete cases')
    passed = 0
    for case in design['cases']:
        actual = answers[arm][case['id']]
        expected = case['expected']
        success = (isinstance(actual, dict) and actual == expected and
                   all(type(actual[k]) is type(v) for k, v in expected.items()))
        passed += success
        print(json.dumps({'arm': arm, 'case': case['id'], 'accepted': success}))
    print(json.dumps({'arm': arm, 'accepted': passed, 'total': len(ids)}))
