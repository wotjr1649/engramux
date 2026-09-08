"""Prepare solver inputs from reviewed public task fields and actual selector output."""
import json
from pathlib import Path

root = Path(__file__).parent
design = json.loads((root / 'incremental-tasks-2026-09-08.json').read_bytes())
output = json.loads((root / 'incremental-output-2026-09-08.json').read_bytes())
selected = {row['id']: row['text'] for row in output['results']}
for arm in ('absent_off', 'absent_on', 'visible_off', 'visible_on'):
    rows = []
    for case in design['cases']:
        rows.append({'id': case['id'], 'current_request': case['prompt'],
                     'visible_prior_reply': case['history'] if arm.startswith('visible') else '',
                     'recalled_data': selected[case['id']] if arm.endswith('on') else ''})
    with (root / ('incremental-input-' + arm + '.json')).open('x', encoding='utf-8', newline='') as f:
        json.dump(rows, f, ensure_ascii=True, indent=2)
        f.write('\n')
