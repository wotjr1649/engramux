# Session 02 — Engramux Phase 4: search

Engramux captures. It cannot yet hand anything back. This session builds the half of the
product that §1 names in the same breath as the other: *"captures … and serves them back
through FTS5 and MCP."*

`CLAUDE.md` imports `AGENTS.md`, so the standing rules — commands, gotchas, boundaries, document
ownership, output language — are already in your context. This file does not repeat them.

**This brief was rewritten after an adversarial review found its first version unsafe to
implement.** Three reviews — repository forensics, SQLite's own documentation, and a build of
the specified index over the real corpus — found the phase pointing at a number the repository
had already withdrawn, and four of the plan's mechanics wrong. The reports are at
`.superpowers/sdd/2026-08-28-session-01-phase-1-capture-core/review-brief-forensics.md`,
`review-fts5-docs.md` and `review-fts5-measured.md` (that directory is gitignored, local only).
Everything below already accounts for them. §5.7 and §8 were amended in `743e364` to match.

---

## 1. Where the work stands

| | |
|---|---|
| HEAD | `743e364`, branch `main`, working tree clean |
| Unpushed | **41 commits ahead of `origin/main`. Do not push.** `origin` is public |
| Last full verification, at that commit | `go build ./...` ok · `go test -p 1 -count=1 ./...` 13 packages ok · pinned linter `0 issues`, **exit 0** · `./scripts/race.sh -count=1` exit 0 |
| Phase 1 | complete — all four §8 clauses pass together from an empty directory |
| Phase 3 | complete — 30 concurrent starts leave one service; `doctor` reports the execution time limit and the restart policy |
| Phase 2 | not started, **and not well defined**. Phase 1's ingest is already generic and I-14 says a non-enabled cell is "stored but not parsed", which is what happens today. The allowlist has nothing to govern until something parses per cell |
| Phase 5 | not started. §10 question 3 asks what the four MCP tools are |
| Phase 6 | four `ponytail:` debts in source, no soak |

**It is installed and running on this machine.** Hooks are live in both hosts, the service
starts at logon through the registered task `\Engramux`, and it has captured over 1,400 events
of real work. `engramux status`, `engramux cells` and `engramux doctor` all answer. That is not
a demo — it is a second corpus, alongside the 902 captures in `.capture/fixtures-raw/`.

**The running service will make the test suite fail**, and it did so four times in one session.
It holds the pipe and the database; the pipe name comes from the user SID, not the data
directory, so redirecting `LOCALAPPDATA` isolates nothing. Stop it before running the suite and
start it again afterwards:

    schtasks /end /tn "\Engramux"      MSYS_NO_PATHCONV=1, or //end - never both
    schtasks /run /tn "\Engramux"

The first task below removes this friction permanently.

---

## 2. What counts as done

§8's Phase 4 gate was rewritten in `743e364`. It is **known-item retrieval, gated per class**:
the query is derived from the document it must find, so relevance judgements are free and
anyone can re-run it.

Five classes, each gated separately, because an average is exactly what hid the failure last
time: **two-character Korean**, **a content word carrying a particle**, **two tokens**,
**camelCase**, **a path basename**. Plus one precision assertion, because a recall-only gate
cannot see what indexing structure destroys.

Runs over the fixtures always, and over `.capture/` when present — the skip pattern
`internal/fixtures` established.

**Do not resurrect the old baseline.** §5.7's "93.3% micro-averaged recall on 12,664 documents"
is withdrawn, and the reasons are traps rather than history: the harness was deliberately
removed in `34b36cc` as having no artifact; a "document" was a `memory_items` row and §5.8
leaves that table empty; micro-averaging was banned by name in two earlier revisions for hiding
Korean failure behind bulk English matching; and the metric was a `COUNT(*)` match, which rev.2
itself called "not a search-quality metric".

---

## 3. Measured already — do not rediscover it

All `[verified]`, against the real corpus and SQLite's documentation.

**The configuration works.** §5.7's exact `CREATE VIRTUAL TABLE` is accepted; external content
works against a `TEXT PRIMARY KEY` table under `STRICT`; the index costs about 1.26× the
content. Korean content words are reachable by a two-character prefix query.

**`unicode61` does not split a Latin stem from an attached Korean particle.** `Codex는` is one
token, so an exact-word search for `Codex` misses it. That makes per-token expansion
load-bearing rather than convenient — an implementation that drops it for "exact" searches
breaks Korean-adjacent text silently.

**Bare `token*` is a syntax error, not a miss.** A hyphenated identifier answers `no such
column: time`; a Windows path answers `no such column: C`. Both are everywhere in this corpus.
Quote it: `"token"*`, star **outside** the quotes, embedded `"` doubled.

**`prefix='2 3 4'` buys latency, not recall.** With the two-character prefix index in place a
bare two-character `MATCH` still returns nothing and the same query with a trailing star
returns the row. Only the star matches. The index costs about 3.2×. **Decide whether that trade
is worth keeping and say why** — nobody has measured the latency it buys.

**A trigram tokenizer cannot match fewer than three characters in any script.** Documented, not
Korean-specific, and it disqualifies trigram here regardless of index size.

---

## 4. The four mechanics the first version got wrong

**Never `snippet()` or `highlight()`.** With external content, `highlight()` takes its text from
the **content table** and its markers from the **index** — desync puts markers in the wrong
place silently. `snippet()` against a missing base row returns some rows and then fails with
`database disk image is malformed` **in `rows.Err()`**, which a loop that ignores `Err()` never
sees. Instead: **match → collect rowids → `secret.Mask` the whole payload → build the excerpt
in Go.**

**Masking a fragment leaks, tested on the real rules.** Replaying `internal/secret`'s patterns
against excerpt-sized windows, all of these come back **unchanged**: a value cut from its key,
an `sk-` prefix cut mid-token, a cut `Bearer`, a path split across the window, and Codex's
string-carried JSON where an escaped quote defeats the credential rule. With `user-path`
matching 900 of 902 rows this is the hot path, not an edge case.

**Indexing raw JSON is a precision problem the old gate could not see.** `session`, `id`,
`hook`, `event`, `name` and `cwd` appear in 901 of 902 documents as tokens, against 76–277 for
real text. Structure tokens only ever **raise** recall. The cheap alternative: `internal/secret`
already walks every string leaf at ingest, so collect them into one column while it is there.
The JSON1 route does **not** work — `json_tree` fails at `rebuild` with `no such table:
main.json_tree`, and a hard-coded path list walks into §4.4's host-shape trap.

**§5.7's "over the redacted payload" is an amendment, not a reading.** Record it as one. The
database stores the original by design (I-10: tagged, never erased), and external content's
`rebuild` reads the content table — so an index of masked text cannot be kept in sync with it.

---

## 5. Tasks

Adjust the boundaries when the code says they are wrong, and say so when you do.

**T1 — Test-only pipe-name override.** So the tests that launch real binaries stop contending
with the live service, leaving only the singleton test on the real derivation. `AGENTS.md`'s
`-p 1` note already anticipates this. *Gate:* the full suite passes with the service running.
Do this first; it pays for itself within the session.

**T2 — The gate, before any FTS5 exists.** All five classes, over the fixtures, **failing**.
*Gate:* each class fails for the right reason, and the failure names the class. A gate written
after the thing it gates is a gate shaped to pass.

**T3 — Schema and triggers.** The FTS5 table, external content, and the `AFTER UPDATE` /
`INSERT` / `DELETE` triggers with old values passed explicitly. *Guards:* I-10. *Gate:*
`INSERT INTO t(t) VALUES('integrity-check')` — **with `rank=1`**, because the no-argument form
and `rank=0` both pass silently against a desynced index. *Break it:* drop one trigger and
confirm `rank=1` catches it.

**T4 — What gets indexed.** Raw payload or collected string leaves; decide with T2's precision
assertion in hand, not before. *Gate:* the precision class moves in the direction you predicted.

**T5 — Query construction.** Per-token expansion with the quoting rule. *Gate:* a hyphenated
identifier, a Windows path, a bare `AND`, and a lone `"` all produce results or clean errors —
never a SQL syntax error reaching the caller.

**T6 — Search over the pipe, and the egress.** I-08: every read travels the pipe. `Search` is
one of §5.2's five request types and is currently answered `Rejected`. *Guards:* I-10. *Gate:*
a runtime-generated secret is findable by a term that is **not** the secret, and absent from
the returned excerpt, while the row still holds it — clause 4's shape, at the second egress.

**T7 — Phase 4's gate, green, run together from clean.**

---

## 6. Open, and what to do about each

| | |
|---|---|
| Is `prefix='2 3 4'` worth 3.2× the index? | **Measure the latency it buys** during T3, then decide and record it in §5.7 |
| Raw payload vs collected leaves | **Decide in T4 with T2's precision numbers**, not before |
| A search filtered by project needs an `UNINDEXED` column | **Decide in T3.** There is no index on `events.project_id` or `received_at` either; migrating later costs a second migration |
| Corpus payloads are not production bytes | 656 of 902 carry Go-re-marshalled escapes. Korean is stored literally so §5.7's argument survives — **do not measure escaping behaviour against `.capture/`** |
| Phase 2's meaning | **Not this session.** Do not build an allowlist that governs nothing |
| README, LICENSE, CI, push | **Not this session.** CI has an unverified prerequisite: whether GitHub's Windows runners can supply a C toolchain for `-race` |
| `_journal_mode` is five weeks old in the driver | On any driver upgrade re-read its CHANGELOG's shorthand keys. Three tests would notice a regression and only two can fail |

---

## 7. How this session runs

`superpowers:subagent-driven-development`. A fresh subagent per task, review between tasks, and
**the task list in §5 is the plan input** — each entry is a brief: deliverable, gate, invariant
guarded. **No task brief contains code.** The full argument is in
`docs/prompts/2026-08-28-session-01-phase-1-capture-core.md` §3 and has not changed.

Two habits that earned their place last session:

**Measure before dispatching.** Every task that went smoothly had its load-bearing facts
measured first. Every task that needed a fix round had a fact nobody had checked.

**The break-it step is where the findings came from.** Not one of the previous session's
serious bugs was found by review: `json.Marshal` silently rewriting every payload, a DSN opening
the wrong file, a killed child dying of its own deadlock detector, a `[verified]` claim false
for four revisions. All surfaced when somebody deliberately broke the thing and watched what
did *not* go red.

Stated once so it is not relearned: **a check that cannot fail is not a check.** The `-shm` bug
survived four revisions because every test measured a brand-new database, the one case where it
cannot happen. The recall claim survived because nobody could re-run it.

---

## 8. First action

**T1, then T2.** Write the gate failing before any FTS5 exists — it is the only artifact this
phase has that its previous version did not.

When Phase 4's gate passes from clean, stop and report. Do not begin Phase 5, and do not push;
41 commits are already waiting on a decision that is not yours.
