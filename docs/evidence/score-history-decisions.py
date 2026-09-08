"""Score the frozen synthetic choices and required supporting event references."""
import json
import pathlib

root = pathlib.Path(__file__).parent
design = json.loads((root / 'history-decision-tasks-2026-09-08.json').read_bytes())
answers = json.loads((root / 'history-decision-answers-2026-09-08.json').read_bytes())
for arm in ('latest', 'history'):
    action_count = supported_count = 0
    for case in design['cases']:
        answer = answers[arm][case['id']]
        if set(answer) != {'next_action', 'evidence', 'certainty'}:
            raise SystemExit('unexpected answer fields')
        valid_ids = {r['id'] for r in case['records']}
        supplied_ids = valid_ids if arm == 'history' else {case['records'][-1]['id']}
        evidence = answer['evidence']
        if not isinstance(evidence, list) or any(not isinstance(e, str) for e in evidence):
            raise SystemExit('invalid evidence shape')
        action = (answer['next_action'] == case['expected_action'] and
                  answer['certainty'] == case['expected_certainty'])
        support = (action and len(set(evidence)) == len(evidence) and
                   set(evidence) <= supplied_ids and
                   set(case['required_evidence']) <= set(evidence))
        action_count += action
        supported_count += support
        print(json.dumps({'arm': arm, 'case': case['id'], 'action_and_certainty': action,
                          'decision_with_required_sources': support}, sort_keys=True))
    print(json.dumps({'arm': arm, 'action_and_certainty': action_count,
                      'decision_with_required_sources': supported_count, 'cases': len(design['cases'])}, sort_keys=True))
