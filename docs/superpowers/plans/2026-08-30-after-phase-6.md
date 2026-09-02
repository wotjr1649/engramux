# Execution order after Phase 6

**rev.1** · 2026-08-30

Order only. Every decision, value and measurement below belongs to
`docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` or to
`2026-08-27-engramux-1.0-design.md` rev.4, and is cited rather than repeated. Where this file and
either spec disagree, the spec wins.

## The blocker, and what it is not

One thing blocks: anything that changes a **shipped** `.go` file waits for the Phase 6 soak to close
(1.0 spec §8's Phase 6 row). Membership is decided by `go list -deps` against the two commands, not
by a file name.

It is not a general freeze. Documentation, tests, scripts and any package no shipped package imports
are free while the soak runs, and the soak-window work has been taking them as they came. The freeze
is on what gets built and installed.

It is also not a reboot budget. The reboot that started the soak was for the clock; an ordinary
upgrade stops and restarts the service (1.0 spec §5.5), so the steps below cost a restart each and
not a logon each. Batching is therefore about migrations, not about reboots.

## Steps

**Step 1 — the clearing build.** Everything already decided, already blocked only by the freeze, and
touching no schema: backlog **30** (the protocol revision the server does not offer), **31** and
**32** (the two withdrawals decided on 2026-08-30, which delete more than they add), **33** (a search
reply with no match count), **9**'s three remaining shipped-source sites, and **6**. **16**, **17**
and **27** are wire-contract changes and belong here too, because one build is one compatibility
event and three separate ones are three.

Blocked by: the soak. Unblocks: nothing — every later step is independent of it. It goes first
because it is the only step whose content is already settled, and because a smaller queue makes the
next build's failures easier to attribute.

Done when: the suite, the pinned linter and the race script are green, and the reinstalled binaries
answer `doctor`.

**Step 2 — installation and diagnosis.** Memory spec **M-5** and **M-6**. Independent of every other
step and of each other, but ordered together because `doctor`'s stage judgement and `install`'s
notion of a complete installation are the same definition written twice if they are split.

It is second rather than last on purpose: every subsequent build is then installed through the new
path, which is the only way that path gets exercised before a stranger runs it.

Blocked by: nothing, and **it is being done first** - session 07 took it during the soak window,
because the freeze is on rebuilding rather than on writing. The installer half is on
`step-2-engramux-install`; `doctor` is the rest of it.

Unblocks: publication, which the owner's conditions gate on a decided and verified setup.

Done when: a capture-only installation is green, a fresh machine is told to install rather than
shown four red sections, and the eleven hook entries are checked against the installed relay. The
clean-VM verification is **not** part of this - it moved to being a condition of publication on
2026-08-30, because binding it here would leave the branch unmergeable until a VM exists while main
kept moving.

**Ordering that the deletion of the Node installer created.** `main` still has
`scripts/install-hooks.mjs`; this branch does not. So after the soak: close the series, delete the
sampler task, run `race.sh`, then **build and install Step 1 from `main`**, where the script is still
a fallback if anything goes wrong - and only then merge this branch and verify the new installer by
reinstalling with it. Merging first would make the new installer's first real run and a new binary's
first install the same event, with two candidate causes for anything that broke.

**Step 3 — native memory indexed.** Memory spec **M-2**. First of the memory work because it is the
smallest, because it is the only capability neither host can have, and because **P4** cannot be
measured until it exists.

Blocked by: nothing but the freeze. Unblocks: gate **M3**, and the coverage half of **M8**.

Done when: gates **M1**, **M2** and **M3** pass. **M2** is the one to watch — a parser that skips
quietly passes a review and fails a format change silently, which is the failure this gate is shaped
against.

**Step 4 — derived fields.** Memory spec **M-3**. After Step 3 rather than before it because the
retrieval evidence puts the points in the selector, and a selector is easier to judge once the corpus
it selects over is complete.

Before writing the migration, settle whether Step 3 and Step 4 share an FTS rebuild. If they do,
they are one migration and one backfill; if the derived fields are filters on the base table and
never enter the index, they are two and the batching argument disappears. The 1.0 spec's `00002` row
is what makes the question worth asking rather than assuming.

Blocked by: Step 3, for judgement rather than for compilation. Unblocks: gate **M7**, which is what
decides whether Step 5 ships enabled.

Done when: gate **M4** passes — and **M4** is the step's own delete condition, not a formality.

**Step 5 — injection, built and disabled.** Memory spec **M-4**. Last of the memory work, and it
ships off.

Blocked by: Step 4, because **M7** is unlikely to clear without it. Unblocks: nothing; it is the end
of this plan.

Done when: **M5**, **M6** and **M9** pass and **M8** is reported. Enabling it for a user is a
separate act from shipping it, and **M7** is what licenses that act.

## Not ordered here

The remaining 1.0 backlog rows that need no build are being taken during the soak and are not steps.
Backlog **28**, the bearer token's inherited permissions, is not scheduled: the 1.0 spec §5.9 accepts
the exposure deliberately, so moving it is a decision nobody has taken rather than work nobody has
done.
