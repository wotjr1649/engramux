# Explicit hook linkage audit, 2026-09-08

The frozen snapshot already contains candidate turn linkage in raw event payloads. This
changes the next step after the receive-order adjacency rejection: inspect explicit keys,
not neighboring receipt timestamps. No product schema or selection behavior changed here.

`python -X utf8 docs/evidence/hook-linkage.py` read the frozen database with `mode=ro` and
`query_only=ON`. It examines only UserPromptSubmit and Stop for the two known hosts, bounds
the scan at 100,000 events and each payload at 1 MiB, and reports a closed field allowlist.
It prints no payload, identifier value or user path. Invalid and oversized counts were zero.

| Host | UserPromptSubmit | Stop | Nonempty key on every examined event |
| --- | ---: | ---: | --- |
| Claude Code | 675 | 676 | prompt_id |
| Codex | 46 | 41 | turn_id |

Grouping by host, stored session, project and key value gives the following cardinalities.
Counts are groups, not event counts, and do not establish semantic answer relevance.

| Host | Exactly one prompt and one Stop | Both kinds, multiple rows | Prompt only | Stop only |
| --- | ---: | ---: | ---: | ---: |
| Claude Code | 604 | 13 | 35 | 59 |
| Codex | 39 | 0 | 7 | 2 |

The [official Claude Code hooks reference](https://code.claude.com/docs/en/hooks#common-input-fields),
checked on 2026-09-08, describes `prompt_id` as the identifier of the user prompt being
processed, available from v2.1.196. The [official Codex Stop input schema](https://github.com/openai/codex/blob/main/codex-rs/hooks/schema/generated/stop.command.input.schema.json)
describes `turn_id` as the active turn identifier. These sources support interpreting the
fields as linkage candidates; they do not guarantee that every installation, old capture,
repeated Stop or adversarial payload yields exactly one correct answer.

A strict paired-answer experiment is now supportable for unambiguous keys. It must scope
by host, session and project, keep only records available before the trigger, distinguish
missing/null/nonstring/oversized keys, and refuse multiplicity rather than choosing by receipt
order. A same-key association still does not prove that a selected answer helps the current
request. The existing source events and IDs remain authoritative references; no private key
values should be added to public evidence or logs. The previously used holdouts remain exposed.
