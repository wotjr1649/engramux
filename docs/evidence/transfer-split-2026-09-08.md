# Cross-project session split, 2026-09-08

The frozen snapshot contains additional prompt populations not opened in the earlier
selection experiments. A metadata-only scan excluded every session in the original M7
snapshot and every session with any prompt in the already-exposed Engramux project.
It left **20 sessions / 131 prompts across three other projects**. No prompt text or selector
output was read to construct this split.

The rule was fixed before execution: sort stored session IDs by SHA-256 of their UTF-8 bytes,
use the original string to break ties, and assign the first floor(n/2) sessions to development.
The remainder is holdout. `python -X utf8 docs/evidence/freeze-transfer-sessions.py` produced:

| Arm | Sessions | Prompts | Projects | Prompts per session, descending |
| --- | ---: | ---: | ---: | --- |
| Development | 10 | 23 | 3 | 6, 4, 3, 2, 2, 2, 1, 1, 1, 1 |
| Holdout | 10 | 108 | 1 | 40, 35, 14, 7, 5, 2, 2, 1, 1, 1 |

The uneven distribution was accepted without rerolling. A direct set-intersection assertion
confirmed no session appears in both arms. A second generator invocation exited 1 with
`transfer split already exists; refusing regeneration`. The private manifest binds the old
database, current frozen database, WAL and prior split manifest by SHA-256, and contains only
selected event/session/project identities and receipt timestamps. Source database and WAL
hashes were checked again before writing it. All generated data remains under `.capture/`.

This is same-user, session-disjoint, cross-project followup evidence, not a new-user study.
Aggregate hook-field metadata from this snapshot was previously inspected; that exposure is
disclosed in the manifest. The holdout is a single project and two sessions supply 75/108
prompts. Request-level proportions must not be presented as independent trials or evidence
for all three projects. Report aggregate byte accounting and per-session paired outcomes
separately, with the session distribution and project scope visible. No new quality threshold
is introduced by this split.

The selection-review subagent reviewed these aggregate counts and reporting limits. A direct
project-ID set intersection found that the holdout project also occurs in development, so
independence is at session level, not project level. Exclusions apply to evaluated prompts;
historical events remain available as production-like retrieval context, including older
development sessions, subject to the strictly-before-trigger cutoff. This is a fresh-prompt
evaluation against existing history, not a wholly disjoint training/retrieval corpus. No
future event may enter the retrieval index, regardless of which split its session belongs to.

No candidate is selected by this document. Development prompts may inform a later candidate;
holdout prompts and outputs remain unopened until that candidate and its labelling rules are
fixed. After holdout use, it becomes exposed evaluation data and must not be reused as an
untouched sample for further tuning. Original M7 labels, prior failures and used holdouts are
preserved rather than replaced by this population.
