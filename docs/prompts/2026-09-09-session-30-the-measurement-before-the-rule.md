# Session 30 — Engramux: the measurement before the rule

You are continuing in Claude Code at D:/AI_DEV/engramux. CLAUDE.md contains @AGENTS.md and both
load for you already, so this brief restates none of their rules, commands or hazard table. Reply
in Korean; repository documents stay English.

## What this session is

**Run the M15 and M16 measurement.** Both were registered in the memory spec on 2026-09-09, before
a harness existed and before one byte was counted, and neither has been run. Read
`docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` §5's rows for **M15** and
**M16** and the section *What M15 measures, and the sentence it must not lose*. **That section is
the design; this brief deliberately does not restate it**, because the spec owns decisions and
measurements and a brief is never updated.

You have no authority to publish. Not a tag, not a release, not a push of a tag. **If M16 passes,
the migration and the release that would carry it are a separate decision and you stop and report
rather than taking them.** Everything else the contract already allows — stopping and starting the
installed service is a named carve-out, and copying a file is a read.

## Verified repository state, 2026-09-09

`git rev-parse HEAD` was `7851901` before this brief's own commit; `git branch --show-current` was
`main`; `git status --porcelain` was empty; `git log --merges --oneline | wc -l` was 39. Recheck
before acting on any of it.

Three releases went out today, in this order: **0.1.0** the first one, **0.1.1** adding
`bin/engramux` and the README paragraph saying installing puts nothing on your `PATH`, and **0.1.2**
closing backlog 54 and making a bare `engramux update` read Claude Code's plugin cache. Each was
verified the same way and each held: the archive hash reproduced locally, the release workflow's
rebuild produced the same hash on a different machine, and the published asset downloaded back
byte-identical to the local build. The installed service is running 0.1.2 and `doctor` reports both
hosts at eleven of eleven events.

**Checks that last passed.** The full suite green at exit 0 over every package, `internal/search`
at 213 s. The race suite green over every package, `internal/search` at 3,612 s — that one ran
before the last two merges and has not been repeated. The pinned linter at `0 issues.` and exit 0,
checked by exit code rather than by the summary line.

## The order

1. **Write the harness and run M16 first.** It is the smaller question and if it passes it removes
   about a third of the file, which changes nothing about M15's fractions but is worth knowing
   before you spend the long run.
2. **Then M15**, whose expensive half is the reachability sweep over the live corpus.
3. **Report both**, and stop.

**The sweep's cost is the one number nobody has.** `TestEveryCandidateDocumentIsReachable` logs
2,262 queries per arm over the 901-document fixture corpus; the live database holds 46,598 events,
so the candidate count scales with it and the run may be long. Measure it and say what it was rather
than predicting it.

## How to get a corpus without breaking anything

Stop the installed service, copy `engramux.db` **and** `engramux.db-wal` together, start the service
again in the same turn. The pair is the snapshot — `schtasks /end` is a hard kill, so the WAL holds
committed frames the `.db` does not have, and the `.db` alone is a database missing them. Events
that arrive while it is down are spooled by the relay and replayed by the drain, so nothing is lost;
that was measured during a reinstall in an earlier session and four events came back that way.

**Two copies from that one snapshot.** M16 migrates one of them. M15 and the value-length
distribution read the other, unmigrated, so that M15's byte fractions have today's schema as their
denominator.

## What is already built that you should not rebuild

`m4Classes` is three classes and `m11Classes` is two — **five, not eight**, and an earlier draft of
this work said eight twice before the code was read. `m4CandidatesFor` and `m11CandidatesFor` take
the documents as an argument, so both point at any corpus you hand them. `TestEveryCandidateDocumentIsReachable`
already computes M15's criterion exactly: whether a query cut from a document returns that document,
as membership rather than as a rank. `store.Leaves` is the walk M16 needs an SQL twin of, and
`00002`'s backfill already is that twin.

## What is unverified and what to do about it

**Whether FTS5 external content accepts a VIRTUAL generated column at all.** Nothing here has tried
it. The reasoning that it should is that `content='events'` reads the content table only on rebuild
and integrity-check, and this repository never calls `snippet()` — but that is an argument, not a
result. **Try it on the copy; if it is refused, say so and stop rather than reaching for the
fallback**, which is indexing the payload directly and is a different measurement the spec names.

**Whether the session-size distribution lets M15's bar be met.** Roughly 140–160 sessions hold
46,598 events, so the average session is near 300 events and a session with 300 events is unlikely
to contain no reachable one. That reasoning says the bar will fail. It is an average and the
distribution is unmeasured, which is exactly why the measurement is worth running — **do not let the
expectation shape the harness.**

## Out of scope, named so it is not drifted into

Writing the eviction code, whatever M15 answers — the bar decides that and a later session builds it.
Compaction, which the spec defers to a two-gigabyte data directory and which cannot be automatic for
the reason recorded there. Backlog 58 and 59, both deferred deliberately today. Phase 2, whose
execution design is the next thing to be grilled and whose 150 blind judgements are the owner's
time. Backlog 55 and 56, which are inside the injector and are pre-registered as Phase 2's one
retry rather than as work to do before it.

## Where the rest of today's decisions live

The direction was settled by four rounds of interview with the owner and the results are in the
spec, not here. The priority order the owner set for what gets grilled next is **A** this
measurement, **B** Phase 2's execution design, **C** the deletion and inventory CLI, **D** the
long-term selection rules, **E** the retrieval-side question, **F** a viewer, **G** the exit
condition for all of it. A is what you are doing.
