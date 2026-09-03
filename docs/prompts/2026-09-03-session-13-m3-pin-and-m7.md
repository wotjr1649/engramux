# Session 13 — Engramux: the two gates a person has to write, and Step 6

Session 12 did its brief's T1, T2, T4 and T7. It changed gate **M3**'s shape on `main`, built
**Step 5** on `step-5-injection` and merged it, and verified the tree. What it did **not** do is T3
and T5 — the fifty queries and the pin that turns M3 from a skip into a gate — because those wait on
a human and the human was not in the session. T6, M7's harness, was cut, which is what its own brief
said to cut first.

So this session opens with the same one blocked item the last two did, plus a feature that is built
and switched off and a gate that would license switching it on.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context — including the
two carve-outs about the service and about `install --apply`.

Read, in this order: `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` **rev.9**,
and in it the new **What building it settled (M-4)** section — it is what Step 5 decided that rev.8
did not, and its last two subsections are the findings this session exists to act on; then **§5**'s
**M3** and **M7** rows; then `docs/superpowers/plans/2026-08-30-after-phase-6.md` rev.3's **Step 5**
Done paragraph and **Step 6**. Backlog rows **28**, **36**, **37**, **38** and **40**.

**Written 2026-09-03, by session 12.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Carries Step 5 merged `--no-ff`, plus M3's shape change, spec rev.9, the plan's Done paragraph and backlog 41's closure. Several commits ahead of `origin/main`. **The owner was not asked to push.** `git status -sb` is the answer |
| Installed | **Step 4**, on migration `00005`. Step 5 changes both binaries and **has not been installed** — the tree and the installation are one build apart, which they have not been since Step 1 |
| Injection | Built, and **off**. There is no `inject.json` on this machine and the installer writes none. `doctor` reports the state either way and prints the path on the off answer |
| Gates | **M1**, **M2** pass in the normal suite. **M4** passes over the corpus. **M5**, **M6**, **M9**, **M10** pass, first numbers in rev.9. **M3** still skips. **M7** is un-run and un-built |
| M3's fixture | `.capture/m3/candidates.tsv`, **120 candidate lines**, every answer filled and verified, every query still `TODO-WRITE-THE-QUERY`. Unchanged since session 11 |
| Backlog | **Five rows.** **28** a publication condition, **36** a memory item's title, **37** the Defender quarantine, **38** the contention gate's margin under `-race`, **40** the duplicated-key divergence. **41** closed by Step 5 |
| Branches | `origin/step-2-engramux-install` is still on the remote; deleting it is the owner's remote change |

---

## 2. What to do, in order

**T1 — Read §1, then `git status -sb`.** Nothing below needs a branch until T4: T2 and T3 are
test-and-documentation work on `main`, and T5 is a step of its own.

**T2 — Hand the owner `candidates.tsv` first, then work while it is out.** Same ask as the last three
briefs and the same reason it has not moved: it is the one thing in this project only the owner can
do. Everything else here is independent of it.

**T3 — Pin M3's number the moment `queries.tsv` lands.** Run `TestGateM3CrossHostRecall`, read the
recall it reports per host, write those into `m3PinnedRecall`, and record the corpus and the date in
the spec's M3 row. The gate is already shaped for this: with a fixture present and the pin unset it
**fails** and prints the number to pin, so the run that measures is also the run that tells you what
to write. Getting it backwards — pinning a number nobody measured — is the one thing the shape was
changed to prevent.

**T4 — Build M7's harness and run the gate.** It is what licenses turning injection on for anybody,
and it is now the only thing between a built feature and a usable one. The shape is
`TestWriteM3Candidates`'s and for the same reason: the prompt side is already captured as
`UserPromptSubmit` events, the injector's output for each is mechanical, and the only column that
has to be human is the verdict. Two things rev.9 says before you start. The **corpus's own prompts**
are the only honest source, because a synthesised prompt measures the synthesiser. And the harness
must not print an excerpt or a prompt into a terminal — `internal/inject`'s gates are measured clean
and this one has to be too.

**T5 — Step 6, the update path.** Memory spec **M-7** and the plan's Step 6. Independent of
everything above. It is the step that makes the sentence in §1 above — *the tree and the installation
are one build apart* — stop being a thing a person fixes by hand.

**T6 — Verify, install, close.** Suite, the pinned linter (check its exit code, never its summary
line), `./scripts/race.sh`, in that order and not concurrently. The plan gets a dated Done paragraph
for whatever closed, and a session 14 brief lands in this directory. **Push only after asking.**

---

## 3. What only the owner can do

**Gate M3's fifty queries.** Third brief running. The file exists, the answers are verified, and the
one rule is to write the query **from memory rather than from the answer beside it** — a query cut
from the answer's own words measures the tokenizer, which another gate already measures.

**Turning injection on, if they want to.** It is one file, and `doctor` prints its path. Nothing in
this session should write it: M7 has not run, and the switch is the user's consent rather than a
setting an agent adjusts.

**Backlog 28 and the clean profile.** Both publication conditions and neither this session's.

---

## 4. Decided 2026-09-03 by Step 5 and not to be reopened

Every one is in the memory spec **rev.9**'s M-4 section with its reasoning and, where there is one,
its measurement. Nothing here repeats a figure.

- **A prompt is reduced to three terms**, identifiers before length, with a selectivity ceiling that
  measures the corpus's answer rather than guessing at the prompt. This is what the feature *is*.
- **The fence is a nonce minted after the body and checked against it**, and a body that would
  collide is refused rather than escaped.
- **The switch is a file whose absence is off**, read by the relay, reported by `doctor`.
- **`Inject` is a request type of its own**, which is what makes an old service fail closed.
- **The relay writes stdout for one event only**, and the 1.0 spec §4.5 moves exactly that far.
- **The deadline is a wall clock and not `ctx.Err()`.** A Go timer is not a clock, and M10 is what
  found it.

---

## 5. Open

| Open | What it decides |
|---|---|
| **Why native memory contributes nothing to injection** | Measured: **0 of 16** injections carried a memory item, while the pull path ranks them fine. The three-term AND is too narrow for a 303-item index. Whether that is the reduction's fault or the corpus's is what **M7** would say, and it is the first question to ask the numbers rather than the design |
| **Whether abstention ever happens on a real corpus** | **16 of 16** prompts injected in the gate's run. The corpus is 902 documents and the queries are already narrow, so the selectivity ceiling never fired. **P2** is this product's sharpest claim and it is currently measured only against inputs built to have no history |
| **What a cold read costs at 227 MB** | Unchanged from session 12's brief and still the honest gap. Every M10 reading is warm. The one over-budget reading anyone has is the race run's, and rev.9 records what it shows |
| **Backlog 36, a memory item's title** | Still nobody's, and injection would surface it the moment a memory item ever reaches an excerpt — which, per the row above, it currently does not |
| **Whether the boost's three columns are worth 18.4 MB** | Unchanged. M4's own delete condition wants a second measurement |

---

## 6. Things that will bite

1. **`TestGateM3CrossHostRecall` fails rather than skips once a fixture exists and the pin does not.**
   That is deliberate and it is the whole point of the shape. Do not "fix" it by setting the pin to
   zero; `m3Unpinned` is `-1` for exactly that reason.
2. **A red `scripts/race.sh` is not evidence of a regression until you have a baseline.** Backlog
   **38**. Session 12 also learned the second half of this: the race detector is slow enough to make
   a *correct* deadline abstention look like a violation, and the gate that asserted on duration
   rather than on injection had to be corrected. Read what an assertion actually says before
   believing a red one.
3. **`git checkout -- <file>` restores HEAD, not the working tree.** `scripts/breakit.sh` refuses a
   dirty tree for this reason, asserts a mutation is present before running the suite, and tells a
   build failure from a passing one. Use it rather than a fresh `sed`.
4. **The suite is slow and `internal/search` under `-race` is around 757 s.** Start the race script
   in the background and read its output file; do not pipe it through `tail`.
5. **`TestPhase4Gate`'s corpus mode logs one line carrying a real path.** Redact everything after
   `candidate documents: ` before any of its output goes anywhere. `internal/inject`'s four gates are
   measured clean and so are M3's and M4's.
6. **The installed service holds its database exclusively (I-07).** No test may open it, which is why
   every injection figure is over `.capture/fixtures-raw` and not over the 18,000-event corpus the
   machine actually has. Anything wanting the real numbers has to stop the service first — which the
   carve-out permits, in an interactive turn, restarting it in the same one.
7. **A live MCP client caches the reply schema**, so a session open across an upgrade rejects a reply
   whose shape changed until it reconnects. Step 5 adds a request type but no new tool, so this one
   is quiet for now.

`AGENTS.md`'s own table carries the rest.

---

## 7. Done when

M3 has a pinned number measured over the owner's own fixture; M7's harness exists and its gate has
either run or been reported as un-runnable with the reason; Step 6 is done or explicitly deferred
with what is left; suite, pinned linter and race script are green; the plan says what closed with the
evidence; and a session 14 brief exists.
