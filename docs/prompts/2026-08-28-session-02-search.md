# Session 02 — Engramux Phase 4: search

Engramux captures. It cannot yet hand anything back. This session builds the half of the
product that §1 names in the same breath as the other: *"captures … and serves them back
through FTS5 and MCP."*

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context. This file
does not repeat them.

**This brief was rewritten after an adversarial review found the first version unsafe to
implement.** Three reviews — repository forensics, SQLite's own documentation, and a build of
the specified index over the real corpus — found that the phase's gate pointed at a number the
repository had already withdrawn, and that four of the plan's mechanics were wrong. The
findings are in `.superpowers/sdd/2026-08-28-session-01-phase-1-capture-core/review-*.md`.
Everything below already accounts for them.

---

## 1. What counts as done

§8's Phase 4 gate was rewritten in the same pass. It is now **known-item retrieval, gated per
class**: the query is derived from the document it must find, so relevance judgements are free
and anyone can re-run it.

Five classes, each gated separately, because an average is exactly what hid the failure last
time: **two-character Korean**, **a content word carrying a particle**, **two tokens**,
**camelCase**, **a path basename**. Plus one precision assertion, because a recall-only gate
cannot see what indexing structure destroys.

Runs over the fixtures always, and over `.capture/` when it is present — the skip pattern
`internal/fixtures` established.

**Do not resurrect the old baseline.** §5.7's "93.3% micro-averaged recall on 12,664 documents"
is withdrawn, and the reasons matter because they are traps to avoid rather than history: the
harness was deliberately removed in `34b36cc` as having no artifact; a "document" was a
`memory_items` row and §5.8 leaves that table empty; micro-averaging was banned by name in two
earlier revisions for hiding Korean failure behind bulk English matching; and the metric was a
`COUNT(*)` match, which rev.2 itself called "not a search-quality metric".

---

## 2. State right now

Phases 1 and 3 are complete. Phase 3 was done before Phase 2 deliberately: nothing executed the
Phase 1 code, so §8's order was overridden with the user's agreement.

| | |
|---|---|
| Branch | `main`, **41 commits ahead of `origin/main`, unpushed** |
| `origin` | public GitHub. Do not push without asking |
| Live | hooks in both hosts, service auto-starts at logon via the task `\Engramux`, 1,400+ real events captured |
| Phase 2 | not started, **and not well defined** — Phase 1's ingest is already generic and I-14 says a non-enabled cell is "stored but not parsed", which is what happens today. The allowlist has nothing to govern until something parses per cell |
| Phase 5 | not started. §10 question 3 asks what the four MCP tools are |
| Phase 6 | four `ponytail:` debts in source, no soak |

---

## 3. What was measured, so you do not rediscover it

Built against the real corpus and against SQLite's documentation. All of it is `[verified]`.

**The configuration works.** §5.7's exact `CREATE VIRTUAL TABLE` is accepted; external content
works against a `TEXT PRIMARY KEY` table under `STRICT`; the index costs about 1.26× the
content on the corpus. Korean content words are reachable by a two-character prefix query.

**`unicode61` does not split a Latin stem from an attached Korean particle.** `Codex는` is one
token, so an exact-word search for `Codex` misses it. That is what makes per-token expansion
load-bearing rather than a convenience — an implementation that drops it for "exact" searches
breaks Korean-adjacent text silently.

**Bare `token*` is a syntax error, not a miss.** A hyphenated identifier answers `no such
column: time`; a Windows path answers `no such column: C`. Both shapes are everywhere in this
corpus. Quote it: `"token"*`, star **outside** the quotes, embedded `"` doubled.

**`prefix='2 3 4'` buys latency, not recall.** With the two-character prefix index in place,
`MATCH '한국'` still returns nothing and `'한국*'` returns the row. Only the star matches. The
index costs about 3.2×. **Decide whether that trade is worth it and say why** — nobody has ever
measured the latency it buys.

**A trigram tokenizer cannot match fewer than three characters in any script.** Documented, not
Korean-specific. That disqualifies it here regardless of index size, so the cost comparison in
the old text is beside the point.

---

## 4. The four mechanics the first version of this brief got wrong

**Never use `snippet()` or `highlight()`.** With external content, `highlight()` takes its text
from the **content table** and its markers from the **index** — desync puts markers in the
wrong place silently. Worse, `snippet()` against a missing base row returns some rows and then
fails with `database disk image is malformed` **in `rows.Err()`**, which a loop that ignores
`Err()` will not see. Do this instead: **match → collect rowids → `secret.Mask` the whole
payload → build the excerpt in Go.**

**Masking a fragment leaks, and it was tested on the real rules.** Replaying
`internal/secret`'s patterns against excerpt-sized windows, all of these come back
**unchanged**: a value cut from its key, an `sk-` prefix cut mid-token, a `Bearer` cut, a path
split across the window, and Codex's string-carried JSON where an escaped quote defeats the
credential rule. With `user-path` matching 900 of 902 rows this is the hot path. Masking the
whole payload and excerpting afterwards is the only shape that holds.

**Indexing the raw JSON is a precision problem the gate cannot see.** `session`, `id`, `hook`,
`event`, `name` and `cwd` appear in 901 of 902 documents as tokens, against 76–277 for real
text. Structure tokens only ever **raise** recall, so a recall-only gate is blind to what they
destroy — which is why the new gate carries a precision assertion. The cheap alternative:
`internal/secret`'s walk already visits every string leaf at ingest, so collect them into one
column while it is there. Note that the JSON1 route does **not** work — `json_tree` fails at
`rebuild` with `no such table: main.json_tree`, and a hard-coded path list walks straight into
§4.4's host-shape trap.

**§5.7 says "over the redacted payload" and the design does the opposite. That is an amendment,
not a reading.** Record it as one. The database stores the original by design (I-10: tagged,
never erased), and external content's `rebuild` reads the content table — so an index of masked
text cannot be kept in sync with it at all.

---

## 5. Things that will bite you

- **`integrity-check` silently passes the failure you care about.** Against a desynced index,
  the no-argument form and `rank=0` both pass; only **`rank=1`** reports `fts5: checksum
  mismatch`. A consistency test without it is fake.
- **goose splits statements on `;`.** Phase 4 is the first phase with triggers, so the row that
  has been sitting in `AGENTS.md` all along finally becomes live. Wrap every trigger body in
  `-- +goose StatementBegin` / `-- +goose StatementEnd`.
- **The triggers roughly double the database** (3.6 → 7.6 MB on the corpus). Latency is fine —
  p95 1.0 → 2.15 ms against §5.3's one-second ceiling.
- **A search filtered by project needs an `UNINDEXED` column, and that is decided now or
  migrated later.** There is no index on `events.project_id` or `received_at` either.
- **Corpus payloads are not production bytes.** 656 of 902 carry Go-re-marshalled escapes like
  `>`, so a harness over `.capture/` indexes bytes production will not have. Korean is
  stored literally, so §5.7's argument survives, but do not measure escaping behaviour there.
- **`bm25()` is sign-flipped** — prefer `rank`, which is documented as faster with `LIMIT`.
- **A running development service makes the suite fail.** It bit four times in one session.
  The decision taken: add a **test-only pipe-name override** so tests that launch real binaries
  stop contending with the live service, leaving only the singleton test on the real
  derivation. Do it first; it will save the session time.

---

## 6. How this session runs

`superpowers:subagent-driven-development`, a fresh subagent per task, the task list in this
file **is** the plan input, and no brief contains code. The argument is in
`docs/prompts/2026-08-28-session-01-phase-1-capture-core.md` §3 and has not changed.

Two habits that earned their place and should carry:

**Measure before dispatching.** Every task that went smoothly had its load-bearing facts
measured first. Every task that needed a fix round had a fact nobody had checked.

**The break-it step is where the findings came from.** Not one of the previous session's
serious bugs was found by review: `json.Marshal` silently rewriting every payload, a DSN
opening the wrong file, a killed child dying of its own deadlock detector, a `[verified]` claim
false for four revisions. All surfaced when somebody deliberately broke the thing and watched
what did *not* go red.

Stated once so it does not have to be relearned: **a check that cannot fail is not a check.**
The `-shm` bug survived four revisions because every test measured a brand-new database, the
one case where it cannot happen. The recall claim survived because nobody could re-run it.

---

## 7. First action

Write the gate. All five classes, over the fixtures, failing — before any FTS5 exists. It is
the only artifact this phase has that the last version of this phase did not.

When it passes, stop and report. Do not begin Phase 5, and do not push.
