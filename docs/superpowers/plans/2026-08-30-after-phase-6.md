# Execution order after Phase 6

**rev.2** · 2026-09-02 — rev.1 was 2026-08-30. This revision records that the soak closed, admits
backlog **34** into Step 1, replaces the merge order the Node installer's deletion had set, and names
where the publication conditions live.

Order only. Every decision, value and measurement below belongs to
`docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` or to
`2026-08-27-engramux-1.0-design.md` rev.4, and is cited rather than repeated. Where this file and
either spec disagree, the spec wins.

## The blocker, and what it is not

One thing blocked, until 2026-09-02: anything that changes a **shipped** `.go` file waited for the
Phase 6 soak to close, and it closed (1.0 spec §8's Phase 6 row and §7.1's soak row). Membership was
decided by `go list -deps` against the two commands, not by a file name, and that is still the test
for what a build ships.

It was not a general freeze. Documentation, tests, scripts and any package no shipped package imports
were free while the soak ran, and the soak-window work took them as they came. The freeze was on what
got built and installed.

It is also not a reboot budget. The reboot that started the soak was for the clock; an ordinary
upgrade stops and restarts the service (1.0 spec §5.5), so the steps below cost a restart each and
not a logon each. Batching is therefore about migrations, not about reboots.

## Steps

**Step 1 — the clearing build.** Everything already decided, already blocked only by the freeze, and
touching no schema but the one index below: backlog **30** (the protocol revision the server does not
offer), **31** and **32** (the two withdrawals decided on 2026-08-30, which delete more than they
add), **33** (a search reply with no match count), **9**'s three remaining shipped-source sites, and
**6**. **16**, **17** and **27** are wire-contract changes and belong here too, because one build is
one compatibility event and three separate ones are three.

**34** joins on 2026-09-02, the covering index behind the status reply's per-cell breakdown, as its
own migration — the one schema change in this step. Admitted because the soak measured it as the shipped binary's only defect
and the 1.0 spec §7.1's read-deadline row decides it comes before any change to the deadline; and
because an index-only migration carries none of the rebuild and backfill cost that the no-schema rule
exists to batch (the 1.0 spec's `00002` row is what that rule is about).

**35** joins the same day, from the first real run of `engramux install`: a re-install must not report
Claude Code's registration as failed when the host already points at the endpoint (backlog 35, memory
spec §8). Admitted because Step 1's own reinstall is the next time the installer runs on this machine.

Blocked by: nothing since the soak closed; it branches from the `main` that carries Step 2. Unblocks:
nothing — every later step is independent of it. It goes first because it is the only step whose
content is already settled, and because a smaller queue makes the next build's failures easier to
attribute.

Done when: the suite, the pinned linter and the race script are green, and the binaries reinstalled
through `engramux install` answer Step 2's `doctor`.

**Done 2026-09-02**, on `step-1-clearing-build`: eleven rows, one commit each, every one with a test
that fails when its fix is undone and a break-it pass that watched it fail. Suite 17 packages, the
pinned linter `0 issues.` at exit 0, the race script 16 packages with no report. Installed through
`engramux install --apply` on the owner's machine: migration `00003` applied at the first start,
`doctor` green with its two new lines, the search reply's total and a refusal's reason both seen at
the terminal, and Claude Code's own `mcp list` answering `Connected` against the now-stateless server.
The installer's first run after backlog 35 said "already points at this endpoint" where the run
before it had reported a failure.

**Step 2 — installation and diagnosis.** Memory spec **M-5** and **M-6**. Independent of every other
step and of each other, but ordered together because `doctor`'s stage judgement and `install`'s
notion of a complete installation are the same definition written twice if they are split.

It is second rather than last on purpose: every subsequent build is then installed through the new
path, which is the only way that path gets exercised before a stranger runs it.

Done: session 07 took it during the soak window, because the freeze was on rebuilding rather than
on writing, and it merged into `main` with `--no-ff` on 2026-09-02, after the soak closed and before
Step 1's branch. What remains of it is its first real run: the rebuilt pair installed through
`engramux install` on the owner's machine, which happens before Step 1 is cut so that Step 1's build
is the second install through the new path and not the first.

Unblocks: publication, which the owner's conditions gate on a decided and verified setup.

Done when: a capture-only installation is green, a fresh machine is told to install rather than
shown four red sections, and the eleven hook entries are checked against the installed relay. The
clean-machine verification is **not** part of this: it moved to being a condition of publication on
2026-08-30, because binding it here would have left the branch unmergeable while main kept moving,
and on 2026-09-02 the memory spec's §8 redefined it as a clean *profile* and says why.

**The order after the soak, decided 2026-09-02, replacing the one the Node installer's deletion had
set.** Step 2 merges first, the rebuilt pair is installed through `engramux install`, and Step 1
branches from that `main`. The earlier order — build Step 1 from a `main` that still had
`scripts/install-hooks.mjs` as a fallback, and merge afterwards — rested on two things that did not
hold: the fallback is one checkout from history rather than gone, and a rebuild from the branch
changes both binaries (the `GODEBUG` pins reach the service), so neither order isolates one cause.
What decides it is that Step 1's done-condition names `doctor`, and the `doctor` it should answer is
the one Step 2 wrote.

**Step 3 — native memory indexed.** Memory spec **M-2**. First of the memory work because it is the
smallest, because it is the only capability neither host can have, and because **P4** cannot be
measured until it exists.

Blocked by: nothing. Unblocks: gate **M3**, and the coverage half of **M8**.

Its design questions were open when this revision was written and are not any more: the memory spec
**rev.2** reads both hosts' memory on the owner's machine and answers all of them under M-2, adding
a fifth MCP tool and naming the 1.0 rows that moves. This step is executed against that revision, and
nothing here restates a value from it.

Done when: gates **M1**, **M2** and **M3** pass. **M2** is the one to watch — a parser that skips
quietly passes a review and fails a format change silently, which is the failure this gate is shaped
against.

**Done 2026-09-02**, on `step-3-native-memory`: nine commits, the design settled before any of them.
The reading came first and corrected the section it was written into — no setting names Claude Code's
memory path, and Codex's line-level index schema stopped being `[unverified]` — then nine decisions,
then the build. Memory spec **rev.3** carries all of it; nothing here repeats a value from it.

**The gates.** **M1** over this machine's own memory: 81 files, 303 items, no crash and no text
silently dropped, asserted as a partition rather than a byte comparison because the parsers
deliberately drop a field's key name. **M2** over synthesised drift, all three shapes, each asserted
on both halves — something warned, and nothing was lost. **M3** verified once against a fixture
generated from the corpus and then left to skip, because a generated fixture is not the
human-labelled one it asks for: claude-code 1 of 1 over 38 items, codex 11 of 11 over 265, and
replacing `ORDER BY rank` with an id order took codex to 10 of 11 and failed the gate.

**What the real corpus found that no synthesised fixture did**, four defects, each now owned by a
test: a heading is not unique within a file; the parser was stripping the very word §6.1's credential
rule matches on, so a credential reached a reply bare and *detected clean*; a URL scheme parsed as a
field label; and a Codex rollout summary's sections did not inherit the `cwd` their own file names,
which filed 92 items under no project. The middle two came out of widening the Phase 6 redaction
audit to the five surfaces it now has, through the literal-needle half that clause exists for.

**Verified**: the suite 17 packages ok, the pinned linter `0 issues.` at exit 0 with both values read,
`./scripts/race.sh` 17 packages with no report at exit 0. **Installed** through `engramux install
--apply` twice — the second time to carry the `cwd` fix — with `doctor` green at exit 0, migration
`00004` applied, and the service's own log reading `indexed native memory files=81 written=303` on its
first pass. **P4 seen at the terminal**: one `engramux search` returned `14 of 14 native memory
matches` across `codex-rollout`, `codex-index`, `codex-raw` and `claude-note:project`, and a hit's id
round-tripped through `engramux memory` to the whole item. The live MCP surface was called from a real
Claude Code session and answered; what that call also showed is decision 9's one cost, which the spec
now prices — a client that cached the reply schema before the upgrade rejects a reply carrying memory
hits until it reconnects. The service's own log carries M2 firing in production on the three shapes
the reading predicted.

**One row raised and none closed**: backlog **36**, a memory item's title. **28** is still the only
other one, and it is still a publication condition.

**Step 4 — derived fields.** Memory spec **M-3**. After Step 3 rather than before it because the
retrieval evidence puts the points in the selector, and a selector is easier to judge once the corpus
it selects over is complete.

Before writing the migration, settle whether Step 3 and Step 4 share an FTS rebuild. If they do,
they are one migration and one backfill; if the derived fields are filters on the base table and
never enter the index, they are two and the batching argument disappears. The 1.0 spec's `00002` row
is what makes the question worth asking rather than assuming. **Settled 2026-09-02** in the memory
spec rev.2's M-2: they are two, and the reason is there rather than here.

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

The 1.0 backlog rows that needed no build were taken during the soak and are not steps. The
publication conditions — a first install on a clean profile, backlog **28**, a `README` — are the
memory spec §8's, and they are not ordered here because none of Steps 1–5 waits on them. **28** is
there rather than in a step since 2026-09-02: the 1.0 spec §5.9 accepts the exposure on the owner's
machine, and a stranger's machine is where it stops being the owner's to accept.
