# Session 02 — Engramux Phase 4: search

Engramux captures. It cannot yet hand anything back. This session builds the half of the
product that §1 names in the same breath as the other: *"captures … and serves them back
through FTS5 and MCP."*

`CLAUDE.md` imports `AGENTS.md`, so the standing rules — commands, gotchas, boundaries,
document ownership, output language — are already in your context. This file does not repeat
them. It carries the state of the work, the decisions already made and what measured them, and
what will bite you that the standing rules do not say.

---

## 1. What counts as done

§8's Phase 4 row is one line: `[auto]` **Recall over the corpus meets §5.7's measured
baseline.**

That baseline is 93.3% micro-averaged recall on 12,664 real documents, and **the first thing
to establish is whether that number is reproducible at all.** The measurement exists; the query
set that produced it may not. Session 01's brief flagged this in as many words — "do not treat
§8 Phase 4 as settled" — and it is still unsettled. If the query set is gone, the gate needs a
new target measured the same way, written down with its harness, and §8 amended. Do that
first, before writing a line of FTS5, because a gate nobody can re-run is not a gate.

---

## 2. State right now

**Phases 1 and 3 are complete.** Phase 3 was done before Phase 2 deliberately: nothing executed
the Phase 1 code, so §8's order was overridden with the user's agreement.

| | |
|---|---|
| Branch | `main`, **39 commits ahead of `origin/main`, unpushed** |
| `origin` | public GitHub. Do not push without asking |
| Working tree | clean |
| Size | 78 files, about 16,700 lines added this session, roughly 2:1 test to production |
| Packages | `fixtures host ipc pipe project schedule secret service spool store` |

**It is installed and running on this machine.** Hooks are live in both hosts, the service
starts at logon through the registered Task Scheduler entry `\Engramux`, and it has captured
over 1,400 events of real work. `engramux status`, `engramux cells` and `engramux doctor` all
answer. That is not a demo — it is the corpus this phase will be searching.

### Phase status against §8

| Phase | State |
|---|---|
| 1 — capture, journal, ACK, replay | **complete**, all four gate clauses pass together from an empty directory |
| 2 — remaining events behind the 22-cell allowlist | **not started, and not yet well defined.** Phase 1's ingest is already generic and I-14 says a non-enabled cell is "stored but not parsed" — which is what happens today. So the allowlist has no parsing to govern until something parses per cell. Decide what Phase 2 means before building it |
| 3 — service, singleton, provisioning | **complete**, both gates verified: 30 concurrent starts leave one service, and `doctor` reports the execution time limit and the restart policy |
| 4 — FTS5 and search | **this session** |
| 5 — MCP, four tools | not started. §10 question 3 asks what the four tools are, now that search targets raw events |
| 6 — hardening and soak | four `ponytail:` debts recorded in source, no soak |

---

## 3. Decisions already made, and what measured them

§5.7 owns this phase and most of it is already settled. Do not reopen these without new
evidence.

| Decision | Evidence |
|---|---|
| Search targets **raw event text**, not extracted titles | Without an LLM the deterministic extractor produces `<tool>: <basename>`. Korean appears in 228 of 902 captures across 10 host×event cells, and arrives through `tool_input.command` (86), `tool_response.stdout` (68), `tool_input.content` (32), `last_assistant_message` (28) — every field a title throws away |
| Tokenizer `porter unicode61 remove_diacritics 2`, `prefix='2 3 4'` | 93.3% micro-averaged recall on 12,664 documents. A trigram tokenizer reaches 98.0% overall but **0% on two-character Korean queries** and costs 5.16× the index |
| Queries expand per token with a trailing `*` | rev.1's rule — wrap the whole input, append one `*` — measured **6.2%** |
| External-content tables, not independent ones | Independent tables make `rebuild` silently empty the index, leave `'delete'` unsupported, and lose base rows to rowid reuse |
| `AFTER UPDATE` triggers are safe with old values passed explicitly | Measured under update load on an external-content table |

---

## 4. The two things most likely to go wrong

**The index holds secrets and the search result must not.** I-10 names a search result as an
egress, alongside the log. §5.7 says "FTS5 over the redacted event payload", and read naively
that means indexing the masked form — but the database stores the **original** by design
(tagged, never erased), and `secret.Mask` re-encodes, so a masked index would not match the
stored bytes and would break every round-trip guarantee Phase 1 built. The resolution the
existing code already implies: **index the original, mask on the way out**, exactly as
`secret.NewLogHandler` does for the log. The index is internal to the machine and is not an
egress; the result is. Get this the right way round before writing the migration, and write the
egress test the same way clause 4 is written — assert the secret is absent from the result
**and** still present in the row.

**goose splits statements on `;`.** Phase 4 is the first phase with triggers, so the gotcha
that has been sitting in `AGENTS.md` all along finally becomes live. Wrap every trigger body in
`-- +goose StatementBegin` / `-- +goose StatementEnd` or it is silently truncated.

---

## 5. Carried over, and none of it blocks this phase

- **A running development service makes the test suite fail.** It bit four times in one session
  — twice me, twice a subagent — and now that the service auto-starts at logon it will bite
  more. The decision taken: **add a pipe-name override for tests**, so the tests that launch
  real binaries stop contending with the live service, leaving only the singleton test using
  the real derivation. `AGENTS.md`'s `-p 1` note already anticipates this. Do it early; it will
  save the session time.
- **The push axis is deferred**: `README.md` and `LICENSE` do not exist, there is no CI, and CI
  has an unverified prerequisite — whether GitHub's Windows runners can supply a C toolchain
  for `-race`. The LICENSE choice is the user's.
- `_journal_mode` arrived in `modernc.org/sqlite` v1.55.0, five weeks before it was adopted
  here. On any driver upgrade, re-read that CHANGELOG's shorthand keys: three tests would
  notice a regression and only two of them can fail.
- Four `ponytail:` debts in source, each naming its ceiling and upgrade path. None is causing
  trouble live.

---

## 6. How this session runs

Same as the last one, because it worked: `superpowers:subagent-driven-development`, a fresh
subagent per task, the task list in this file **is** the plan input, and no task brief contains
code. The full argument is in `docs/prompts/2026-08-28-session-01-phase-1-capture-core.md` §3;
it has not changed.

Two things that earned their place last session and should carry:

**Measure before dispatching.** Every task that went smoothly had its load-bearing facts
measured first — the pragma readback values, the `_txlock` technique, when exclusive locking
actually takes the lock. Every task that needed a fix round had a fact nobody had checked.

**The break-it step is where the real findings came from.** Not one of the session's serious
bugs was found by review. `json.Marshal` silently rewriting every payload, a DSN opening the
wrong file, a killed child dying of its own deadlock detector, a `[verified]` claim false for
four revisions — all of them surfaced when somebody deliberately broke the thing and watched
what did *not* go red.

The recurring lesson, stated once so the next session does not have to relearn it: **a check
that cannot fail is not a check.** The `-shm` bug survived four revisions because every test
measured a brand-new database, which is the one case where it cannot happen.

---

## 7. First action

Establish whether §5.7's 93.3% is reproducible. Everything else in this phase is downstream of
whether there is a gate to build toward.

When Phase 4's gate passes, stop and report. Do not begin Phase 5, and do not push.
