# The first install on a machine that has never had one

This is a work order for a person, not for a session. It is the run that memory spec §8's
conditions 1, 3 and 4 have been waiting on since 2026-08-30, and it is written down because those
conditions each say what a satisfying outcome is and none of them says what somebody has to bring
back. A run that produces no record leaves all three exactly where they were.

**Written 2026-09-04, by session 15, against `bf0fc69`. A brief is a record and is never updated.**

---

## 1. Why this is one visit and not several

Condition 1 asks for an install by the two shipped binaries alone onto a profile that has never run
them. **The moment you install, that profile is spent for that condition.** You get one attempt per
profile, and the machine has one profile you care about.

Two things follow. The binaries you carry are the ones the work is finished on, not a build you are
about to replace — which is why backlog 42 landed first and why backlog 46 did not, since injection
ships off and its abstention reasons are never written to a log on a machine that has not turned it
on. And the observations that cannot be repeated come before the ones that can.

**The run is on a new machine rather than on a second local account.** That exceeds condition 1
rather than substituting for it: a profile is the unit, and a new machine carries a new profile plus
the one thing the condition explicitly accepted losing, which is another Windows build.

## 2. Before you start

**Install Claude Code and Codex first, both of them.** This is not a preference. `PlanMerge` returns
no plan and no error when a host's configuration file is absent, so `install --apply` on a machine
with neither host present copies the binaries, registers the logon task, creates the data directory
and `mcp.json`, reports success, and hooks nothing at all. There is no message that says so. And the
two hosts are not interchangeable for the parser: in the captured corpus `tool_response` is an object
from Claude Code and a string or an array from Codex, so one host exercises half of it.

**Carry the two binaries in a folder of their own.** `install` copies from the directory the running
binary sits in, so any folder works — but not `%LOCALAPPDATA%\engramux\bin`, because a binary
refuses to overwrite itself and that refusal is what you would hit.

**Clone the repository as well.** Not to build from — no Go toolchain is needed — but so that the
README, this file and the issue form are on the machine where you need them, and so that `git blame`
is available when you want to know which commit introduced something.

**Do not double-click `engramux.exe`.** With no arguments it is the relay: it reads standard input
under a one-second budget and exits. Nothing happens, nothing is printed, and it looks broken. Every
command in this file passes a verb.

## 3. The order, and why it is an order

1. **Antivirus, first and untouched.** Adding an exclusion before the first run destroys condition
   4's evidence permanently: "nothing was quarantined" on a machine with an exclusion proves nothing
   and the profile does not become clean again.
2. **The install**, once.
3. **The logon**, which needs a real sign-out and sign-in.
4. **Capture, search and MCP**, which are repeatable and can be re-run as often as you like.

`install --remove` is deliberately not in this run. It is the same code path the owner's machine
already exercises, and its value on a fresh profile is low against the cost of taking the
installation apart at the end of a visit that is about it working.

## 4. Step by step, with what to record

### 4.1 First run, before anything is excluded

Put the two binaries in their folder and run `engramux doctor` from it. This is the first execution
of an unsigned, freshly copied binary on that machine, which is the exact shape
`Behavior:Win32/Execution.A!ml` fired on.

**If antivirus fires, record all of it before doing anything else**: the detection string exactly as
shown, which file it named, the severity, whether the file had executed before it was removed, and
the time. That is condition 4's evidence and it is not recoverable once you act on it.

**Then walk the Windows Security UI and add the exclusion**, for both the folder holding the
binaries and `%LOCALAPPDATA%\engramux\bin`. Write down the path you took through the interface and
what it asked for. `Add-MpPreference -ExclusionPath` was refused with HRESULT `0xc0000142` on the
owner's machine — unelevated, or Tamper Protection — so the UI is the route and nobody has recorded
it. **That record is condition 3**, and it is the only thing that closes it.

**If antivirus stays quiet, condition 4 closes and condition 3 does not.** Record the negative
deliberately: which binaries ran, that the antivirus was at its defaults, and that nothing had been
excluded. Condition 3 is satisfied by a written procedure, not by an absence.

### 4.2 The install

Run `engramux install` with no flags first. It is a dry run: it prints the same plan and writes
nothing. Read the plan — it names four files belonging to the hosts, eleven hook entries per host, a
logon task and a data directory — and then run `engramux install --apply`.

It **passes** when it completes, and `engramux doctor` afterwards reports both hosts registered,
both binaries in place, the logon task registered, and the endpoint answering.

Record the whole `doctor` output. Not `--full`: the plain form masks paths and account names, and
this output is going into an issue.

### 4.3 The logon

Sign out and sign back in. Do not start the service by hand and do not run the installer again.

It **passes** when `engramux status` answers immediately after sign-in, with nobody having started
anything. This is condition 1's fourth observation and it is the one a sandbox or a container could
never have given: the first three are an install working, and this is the logon task working, which
is the claim the condition was written to test.

### 4.4 Capture

Open a **new** session in each host. A session that was already running will not have the hook
configuration, and it will not have the MCP endpoint either — both hosts read those at start.

Do ordinary work in each: type a prompt, let it run a tool, let it finish.

It **passes** when `engramux doctor` shows a non-zero event count and `engramux sessions` lists a
session from each host. Five of the eleven events are forced by simply using an agent —
`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse` and `Stop` — and those five are the
criterion. `PreCompact`, `PostCompact`, `SubagentStart`, `SubagentStop`, `PermissionRequest` and
`SessionEnd` are situational, and their absence in a short visit is not a finding.

This is also the first time the migration chain runs to completion against a real machine from an
empty database rather than incrementally over months. A first start that pauses is expected and
documented; a first start that errors is a finding.

### 4.5 Search and MCP

Run `engramux search` for a word you know you typed. It **passes** when the result is the text you
typed, with an excerpt that reads as an excerpt rather than as a fragment with markers in the wrong
place.

Then ask each host to use the Engramux MCP tools. There are five: `search`, `get_event`,
`get_memory`, `list_sessions` and `status`. It **passes** when the host can call at least `search`
and `status` and gets an answer rather than a schema error.

## 5. What is a finding

A finding is anything that did not match this file, plus anything that was true and confusing.
Documentation that is wrong is a finding of the same weight as code that is wrong — condition 3 is
about documentation, and this run is the only time the README will be read by somebody on a machine
that is not the author's.

**A skip is not a pass.** Several gates skip on a fresh clone because three things they want are
absent there: a captured corpus under `.capture/`, the machine's own native memory files, and a
snapshot of a live database. If you run the test suite on the new machine, read the output and not
the exit code.

## 6. Where a finding goes

**If an outsider could have filed it, it is a GitHub issue.** Install, documentation, antivirus,
error messages, anything the product said or failed to say. The issue form asks for what these
conditions need, so filling it in is the recording step rather than an extra one.

**Everything else goes to `docs/superpowers/backlog.md`.** That is the carry list for findings no
test owns yet, and it is for the things only somebody reading this code would know to look at.

One boundary, one direction, and nothing is in both lists.

## 7. What not to bring back

`doctor` masks its own output and `--full` stops masking. Anything leaving that machine is the
masked form.

The database, the write-ahead log, the spool and the service log are raw capture by design: prompts,
file contents, tool output and the paths you work in. None of them belongs in an issue, and
`.capture/` is never committed from any machine.

Backlog 37 carries the antivirus measurement these conditions rest on. A second machine adds a data
point to it. It does not replace it, and the owner's machine's record stays as it is.
