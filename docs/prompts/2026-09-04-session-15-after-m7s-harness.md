# Session 15 — Engramux: publication blockers, and M7 measured against an unchanged injector

Session 14 opened on the first brief in five with nothing red and nothing waiting on the owner. It
closes with one thing waiting on the owner, four corrections to its own committed text, and a session
plan that an adversarial review dismantled before any of it was built.

**That review is the most useful thing session 14 produced, and how it went is worth more than its
conclusions.** Session 14 built gate M7's harness, drafted a pre-registered rule for reopening the
injector's `MatchAll`, and planned a README and a flaky-gate sweep. Two subagents and Codex, each on
a different axis, then found: the harness's gate would have been a gate in name only; two of its
three non-vacuity arms were vacuous by construction; the decision rule would have decided *yes* on an
artefact; its yield clause rested on a misread abstention reason; the first human label was asking a
question nobody can answer from a prompt; the sweep would have measured a margin that no longer
exists; and the README would have printed install steps whose only walkable path the spec rejects.
Everything below is what survived.

`CLAUDE.md` imports `AGENTS.md`, so the standing rules are already in your context — including the
two carve-outs about the service and about `install --apply`.

Read, in this order: `docs/superpowers/specs/2026-08-30-engramux-memory-architecture.md` **rev.15**,
and in it *What M7 will measure* — the whole section, because rev.15 rewrote three of its
subsections — and *What the first reading over a real corpus said*. Then **§8**'s publication
conditions, **§5**'s M6 and M7 rows, and `docs/superpowers/backlog.md` rows **28**, **37** and **38**.

**Written 2026-09-04, by session 14.**

---

## 1. Where the work stands

| | |
|---|---|
| `main` | Everything below, merged `--no-ff`, **not pushed**. Session 14 did not ask |
| Installed | Step 6's build, verified by `update` itself. `doctor` exit 0, both hosts on the endpoint |
| Gates | M1–M6, M9, M10 pass. **M4's delete condition got its second measurement and does not fire** — improved in all three classes, regressed in none. **M7 is built and un-run**, waiting on 150 labels |
| Injection | Built, **off**, `MatchAll` unchanged and staying that way this session |
| Suite | Green over every package. Pinned linter `0 issues.` at exit 0. `scripts/race.sh` green, no data race |

---

## 2. The one thing waiting on the owner

**Label `.capture/m7/prompts.tsv`.** 150 rows, four columns, and only the third is yours: replace
each `TODO` with `yes` or `no` for **`wanted_context`** — *would you have wanted earlier context
here?*

**That is not the same question as "does relevant history exist", and the file says so.** The first
draft asked the second, which reads like gate M6's condition and is not answerable from a prompt.
What this label supports is the false-positive half — bytes spent where none were wanted — and
coverage. The existence half needs a fixture that does not exist, and rev.15 names it.

**Answer from the prompt alone**, and **do not read any diagnosis of the abstention rate before you
finish**. Pass 2 is what shows you the injector's output and it comes after, deliberately; the
review's finding is that seeing retrieval per prompt contaminates a blind label.

**Do not delete `.capture/m7/snapshot/`.** It is the frozen corpus the fixture is keyed to. The gate
fails loudly rather than quietly when an emitted block carries no label, which is what happens when
the corpus moves — so re-taking it means re-labelling.

---

## 3. What to do, in order

**T1 — seal pass 1, then run M7 against an unchanged injector.** Pass 2 writes
`.capture/m7/blocks.tsv`; label each block; run the gate. **Change nothing about the injector while
this runs** — the gate re-injects and scores what comes back *now*, so a selector change mid-session
makes pass 2 and the gate measure different treatments. If M7 goes green, **do not turn injection
on**: the spec says the switch file's existence is the record of the user's consent, and an agent
writing it makes that sentence false.

**T2 — the README.** Publication condition 3, and there is no file. Write it for the **current state
and for a developer**, not for publication. The review's findings are its specification:

- **Say in the first paragraph that there is no release.** No tag, no `.github/`, no marketplace
  entry, no zip. The only path a reader can walk today is build-from-source, which M-7 explicitly
  rejects as a primary path — so present it as the developer path and label it one.
- **`update --from` cannot be a first step.** It refuses outright when no logon task is registered.
- **Name both Defender detections.** `Behavior:Win32/Execution.A!ml` fired on the CLI in both
  directories; `Trojan:Win32/Commando.A!ml` fired on the soak sampler's `schtasks /create`. §8 says a
  stranger's first install is those same two shapes, and installing registers a task.
- **Mark the exclusion procedure `[unverified]` and attribute it.** `Add-MpPreference -ExclusionPath`
  was refused with HRESULT 0xc0000142, and nothing here records the Windows Security UI route being
  walked. M-7 asks for the steps **for the two directories**. Say the `-s -w` strip does not explain
  the detection, because it is the first thing a reader will try to change.
- **Disclose backlog 28 before any `install --apply` line.** The MCP bearer token lands in three
  files with inherited DACLs and §8 condition 2 is open; recommending the install without saying so
  ships an unremediated finding as advice.
- **The Codex inequality is future tense.** There is no channel for either host today, so write what
  will be unequal when one exists rather than describing it as current.
- **Scope call, stated rather than assumed**: `AGENTS.md`'s "no code blocks in documents" is
  practised over `docs/` — measured, both specs and the plan carry **0** fenced blocks and
  `AGENTS.md` itself carries **2** — so a root README follows `AGENTS.md`'s precedent. But do not
  present an install command as a reproduction: nobody has seen its output in a fresh reader's state.

**T3 — backlog 28.** `mcp.json` written with a security descriptor of its own, on the pattern
`internal/pipe`'s listener already builds. §8 condition 2 names it, the two host files are explicitly
not this product's to narrow, and `doctor` reports those as a finding. Product change: its own
branch, merged `--no-ff`.

**T4 — the abstention description, and nothing beyond a description.** Report the rate **per script
stratum** — the fixture is 40 hangul, 29 mixed, 81 latin — and per cause. **No decision rule.** §4
says why the one session 14 drafted was withdrawn.

**T5 — push, after asking.** Everything is local.

---

## 4. Decided 2026-09-04 and not to be reopened without new evidence

**The reopening of the injector's `MatchAll` is withdrawn for now.** The owner reopened it, a rule
was drafted, and measurement invalidated the rule before it reached the spec. Three things did it:

- **Its yield clause was satisfied by construction.** "At least one match" holds for 115 of the 120
  abstentions that had a query at all, because the one match is the prompt's own event — counted
  before the exclusion removes it. The rule would have decided *yes* on an artefact.
- **Its budget clause measured a proxy.** The quantity the deadline is enforced against is
  `Result.Elapsed` over the whole call; a hand-summed pair of searches is not it. The exact
  measurement is one parameter on an unexported function away. And the relay clamps the injector's
  500 ms by what is left of its own second, so 500 is not what production grants either.
- **It was aimed at the wrong variable.** 69 of the 150 prompts carry Hangul, against a corpus rev.11
  measured at 74% English and an injector that carries no translator on purpose — so **a Hangul
  prompt receiving zero bytes is capability P2 working**. Comparing AND against OR over that fixture
  attributes to the join what the language wall causes.

**What a real injector session needs first**, so it is not re-derived: an as-of cutoff so a prompt
searches only its own past; the prompt's **stored** `project_id` rather than re-resolving `cwd`
against a filesystem that has moved on; a host split, because every prompt in the corpus is Claude
Code's while the switch is one global boolean; and a counterfactual that changes the join **only** —
the arm that exists today also changes scope, the exclusion, the ceiling and the deadline.

**M4's boost stays.** Improved in all three classes on its second measurement.

---

## 5. Open

| Open | What it decides |
|---|---|
| **Whether the 83% abstention is P2 or a defect** | Not answerable by `wanted_context` alone. It needs the candidate pool relevance-labelled, which is a larger fixture than this one |
| **M8 and P5** | The pull path's own "native-grade or better" evidence, and P5's fixture does not exist. This is about the surface that is actually used |
| **Publication conditions 1 and 4** | A clean-profile install, and a first run that survives antivirus. Neither is an agent's |
| **What a cold read costs at 256 MB** | Unchanged. Every reading is warm |
| **M3's translation flaw** | Unchanged, `[unverified]` |

---

## 6. Things that will bite

1. **A backslash escape through a script literal is not safe.** `AGENTS.md` has the row; session 14
   hit it **three times**, twice losing an edit silently. File-write tool, every time.
2. **`filepath.Dir` and `filepath.Join` both clean their own results.** A test that builds an
   "uncleaned" path with either asserts nothing about a `Clean` around them. A break-it pass caught
   two attempts at one such case before the third worked, and the argument that distinguishes them
   turned out to be the one that did not come from a filepath call.
3. **`-trimpath` removes the `-ldflags` line from `go version -m`.** Measured. The release build is
   the one build where that diagnostic is blind; run the binary and read the version instead.
4. **A flag's value survives as a positional.** `taskName` reads the first argument without a dash,
   so `--from <dir>` made the first real `update` ask Windows for a task named after a directory.
5. **A non-vacuity arm can be vacuous in either direction**, and registering one is not the same as
   it measuring anything. Two of three were dropped or demoted the same day: a shuffle keyed by
   `(prompt, block)` always scores zero, and an arm retrieving different documents scores unjudged —
   which is irrelevant — and passes for free.
6. **An abstention reason is not a diagnosis.** `ReasonNoHits` reads *nothing in the corpus matched*
   and the code returns it both when the search found nothing and when the exclusion emptied what it
   found. Session 14 read it the first way, wrote it into the spec, and corrected it the same day.
   **The product's own log line still carries the same ambiguity.**
7. **`go test` without `-count=1` replays a cached run and prints its `-v` log verbatim.** A loop
   collecting a distribution gets one measurement and nineteen copies, and reports zero variance.
8. **`%s` on a `time.Duration` changes unit above one second.** A parser matching only `ms` silently
   drops exactly the runs that went over budget.
9. **`-race` needs `CGO_ENABLED=1` and a compiler that is not on `PATH`.** `scripts/race.sh` finds
   one at `../_tools/mingw64/bin/gcc.exe`; a bare `go test -race` fails with a cgo error that reads
   like a toolchain problem.
10. **The corpus moves while you measure it.** 20,075 to 21,527 events inside one session, on the
    machine doing the measuring. The snapshot exists for this — and it freezes the database, not the
    clock and not project identity.

`AGENTS.md`'s own table carries the rest.

---

## 7. Done when

M7 has run against a labelled fixture and an unchanged injector, or is reported as still waiting with
the count outstanding; a `README` exists carrying the seven things §3 names; backlog 28 is closed or
explicitly carried with what is left; the abstention rate is reported per stratum; the suite, the
pinned linter and the race script are green; the plan says what closed with the evidence; and a
session 16 brief exists.
