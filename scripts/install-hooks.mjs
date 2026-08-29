// Installs (or refreshes) the Engramux capture hooks for Claude Code and Codex.
//
// Run it yourself. An agent does not edit host configuration.
//
//   node scripts/install-hooks.mjs            # show the plan, change nothing
//   node scripts/install-hooks.mjs --apply    # back up, then write
//   node scripts/install-hooks.mjs --remove --apply
//
// What it does, and why each part is the way it is:
//
//   - Copies the two binaries out of dist/ into %LOCALAPPDATA%\engramux\bin\.
//     Hooks point at the copy, not at dist/, so a `go build` during development
//     cannot collide with a hook firing from a live session - the build would
//     fail with "Access is denied" and the hook would run a half-written file.
//     Re-run this script after a rebuild to push a new relay to the hooks. A
//     destination that already holds these exact bytes is left alone, and one
//     the running service has locked stops the run before anything is written.
//
//   - MERGES into the existing configuration. Both files already carry other
//     tools' hooks; nothing that is not Engramux's is added, reordered or
//     removed. An Engramux entry is recognised by its command path, so running
//     this twice refreshes rather than duplicates.
//
//   - Backs up each file it touches to <file>.engramux-backup-<timestamp>. The
//     two host files are written in sequence, not together: if Claude Code's
//     settings.json is rewritten and then Codex's hooks.json fails to parse,
//     the first stays installed with its backup beside it.
//
// The two hosts take different shapes (spec 4.2), and this is not a detail:
// Claude Code takes `command` plus an `args` array, exec form, no shell. Codex
// takes `commandWindows` as one string. Claude Code has no `commandWindows`
// key, so a hook that only sets it is never invoked.

import { readFileSync, writeFileSync, existsSync, mkdirSync, copyFileSync, openSync, closeSync } from 'node:fs'
import { homedir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

// The 11-event intersection (spec 4.1). Both hosts expose all of these; 1.0
// handles the intersection and nothing else.
//
// The value is the per-event settings: `matcher`, null to omit the key, and
// `codexTimeout` on the one event where Codex's own limit is below
// TIMEOUT_SECONDS. One table, so a renamed event cannot leave a timeout
// stranded under the old name.
//
// Checked against Claude Code's hook reference rather than inferred: `"*"`,
// `""` and an omitted key are documented as EQUIVALENT - all three match every
// occurrence - so nothing here is load-bearing and no event needs a matcher to
// fire at all. `"*"` is written on the events whose matcher would otherwise
// filter something (tool name on PreToolUse/PostToolUse/PermissionRequest,
// start reason on SessionStart, end reason on SessionEnd, agent type on the
// Subagent pair, manual/auto on the Compact pair) purely so a reader can see
// that capturing everything is the intent and not an oversight. `Stop` accepts
// the field but has no matcher support whatsoever, so it is omitted there.
//
// All eleven names below appear verbatim in the documented lifecycle table.
const EVENTS = {
  SessionStart: { matcher: '*' },
  // Codex documents SessionEnd alone as "1 second by default and supports up
  // to 3 seconds", against 600 s for every other hook, and warns at load that
  // it is clamping anything higher. 3 rather than 1 because the relay's own
  // ceiling is 1 s total (spec 5.3) plus process start, which is already over
  // Codex's default; 3 is the documented maximum and an explicit value records
  // that, where omitting it silently means 1. Not `async`: the same
  // documentation says SessionEnd hooks "always run synchronously, even when
  // `async` is true", so that is not a way out. Claude Code is unaffected - its
  // SessionEnd budget is 1.5 s raised to the longest per-hook timeout - and
  // keeps TIMEOUT_SECONDS.
  SessionEnd: { matcher: null, codexTimeout: 3 },
  UserPromptSubmit: { matcher: null },
  PreToolUse: { matcher: '*' },
  PostToolUse: { matcher: '*' },
  Stop: { matcher: null },
  SubagentStart: { matcher: null },
  SubagentStop: { matcher: null },
  PreCompact: { matcher: '*' },
  PostCompact: { matcher: '*' },
  PermissionRequest: { matcher: '*' },
}
const EVENT_NAMES = Object.keys(EVENTS)

// Generous against the relay's own 1 s ceiling (spec 5.3), which it enforces
// itself: past that it spools and exits 0. This is the host's backstop, not the
// budget. Measured round trip is p95 1.04 ms.
const TIMEOUT_SECONDS = 5

const HOME = homedir()
const LOCAL = process.env.LOCALAPPDATA ?? join(HOME, 'AppData', 'Local')
const BIN = join(LOCAL, 'engramux', 'bin')
const RELAY = join(BIN, 'engramux.exe')
const SERVICE = join(BIN, 'engramux-service.exe')

// fileURLToPath, not pathname.slice(1): a URL percent-encodes, so a repository
// under a path with a space or a non-ASCII character resolves to a directory
// that does not exist and the dist check below reports the binaries missing.
const REPO = join(dirname(fileURLToPath(import.meta.url)), '..')
const DIST_RELAY = join(REPO, 'dist', 'engramux.exe')
const DIST_SERVICE = join(REPO, 'dist', 'engramux-service.exe')

// Overridable so the merge can be exercised against copies before it is
// pointed at the real files. Both hold other tools' hooks, and a merge nobody
// watched work is a merge you find out about later.
const CLAUDE = process.env.ENGRAMUX_CLAUDE_SETTINGS ?? join(HOME, '.claude', 'settings.json')
const CODEX = process.env.ENGRAMUX_CODEX_HOOKS ?? join(HOME, '.codex', 'hooks.json')

const apply = process.argv.includes('--apply')
const remove = process.argv.includes('--remove')
const changes = []

const isEngramux = (h) =>
  typeof h?.command === 'string' && h.command.toLowerCase().includes('engramux')

function readJSON(path) {
  if (!existsSync(path)) return null
  return JSON.parse(readFileSync(path, 'utf8'))
}

function backup(path) {
  const stamp = new Date().toISOString().replace(/[:.]/g, '-')
  const dest = `${path}.engramux-backup-${stamp}`
  copyFileSync(path, dest)
  return dest
}

// mergeEvents rewrites doc.hooks in place: every Engramux entry is dropped
// first, then re-added unless --remove. Dropping first is what makes a re-run
// idempotent instead of additive.
function mergeEvents(hooks, makeHook) {
  for (const event of EVENT_NAMES) {
    const entries = Array.isArray(hooks[event]) ? hooks[event] : []

    const kept = []
    for (const entry of entries) {
      const inner = (entry.hooks ?? []).filter((h) => !isEngramux(h))
      // An entry whose only hook was ours goes away entirely; an entry that
      // held someone else's too keeps them, in their original order.
      if (inner.length > 0) kept.push({ ...entry, hooks: inner })
      else if (!entry.hooks) kept.push(entry)
    }

    if (!remove) {
      // Appended last, so an existing gate or guard still runs first.
      const { matcher } = EVENTS[event]
      kept.push(matcher === null ? { hooks: [makeHook(event)] } : { matcher, hooks: [makeHook(event)] })
    }

    if (kept.length > 0) hooks[event] = kept
    else delete hooks[event]
  }
}

function install(path, label, makeHook, hooksOf) {
  const doc = readJSON(path)
  if (doc === null) {
    changes.push(`${label}: ${path} does not exist - skipped`)
    return
  }
  const before = JSON.stringify(doc)
  mergeEvents(hooksOf(doc), makeHook)
  const after = JSON.stringify(doc)

  if (before === after) {
    changes.push(`${label}: already up to date`)
    return
  }
  if (!apply) {
    changes.push(`${label}: would ${remove ? 'remove' : 'install'} ${EVENT_NAMES.length} events in ${path}`)
    return
  }
  const saved = backup(path)
  writeFileSync(path, JSON.stringify(doc, null, 2) + '\n', 'utf8')
  changes.push(`${label}: ${remove ? 'removed' : 'installed'} ${EVENT_NAMES.length} events`)
  changes.push(`${label}: backup ${saved}`)
}

// ---------------------------------------------------------------------------

// Windows locks the image of a running process against writes, and the service
// is meant to be resident - so on an installed machine the one destination
// here that normally cannot be written is the one this script has to overwrite.
// Both are therefore decided before either is copied:
//
//   - Identical bytes are not copied at all. Re-running with no rebuild in
//     between is the common case, and rewriting a file with the bytes it
//     already holds is still a write, which the lock still refuses.
//   - A destination that has to change is opened for writing first. Copying the
//     relay and only then failing on the service is what leaves a new relay, an
//     old service and no hook configuration at all - the state that makes this
//     confusing rather than merely annoying. Refusing before the first copy
//     leaves the machine exactly as it was.
//
// The second point buys exactly one guarantee and it is worth stating at its
// real size: no half-install from a lock or a permission failure on a
// destination that ALREADY EXISTS. That is the defect and the common case. A
// destination that does not exist yet is never probed, and nothing here can
// stop the service being started, or a scanner grabbing the file, between the
// probe and the copy - so the copy loop reports what it did before it failed
// rather than pretending it cannot.
function planCopies() {
  const plan = []
  for (const [src, dest] of [[DIST_RELAY, RELAY], [DIST_SERVICE, SERVICE]]) {
    if (!existsSync(src)) {
      console.error(`missing ${src} - build first:`)
      console.error(`  CGO_ENABLED=0 go build -ldflags "-s -w"               -o dist/engramux.exe         ./cmd/engramux`)
      console.error(`  CGO_ENABLED=0 go build -ldflags "-s -w -H=windowsgui" -o dist/engramux-service.exe ./cmd/engramux-service`)
      process.exit(1)
    }
    if (existsSync(dest) && readFileSync(dest).equals(readFileSync(src))) {
      changes.push(`unchanged ${dest} - identical bytes, not copied`)
      continue
    }
    if (apply && existsSync(dest)) {
      try {
        // 'r+' rather than a trial copy: it asks for the same write handle
        // copyFileSync would ask for, and asks without truncating anything.
        closeSync(openSync(dest, 'r+'))
      } catch (err) {
        // Nothing here looks for a process, so nothing here may claim one is
        // running. The errno is the only measured fact - EBUSY is a mapped
        // image, EPERM is the read-only attribute or an ACL - and which of the
        // two files failed is what makes one cause likelier than the other.
        // The relay branch matters: it is held by a hook that is firing right
        // now, and telling that user to stop the service is wrong advice.
        console.error(`cannot write ${dest}: ${err.code ?? err.message}`)
        console.error(err.code === 'EBUSY'
          ? `EBUSY means a running process has that image mapped, and Windows locks it against writes.`
          : `${err.code ?? 'this'} is not a lock: the file is read-only, or an ACL denies the write.`)
        if (dest === SERVICE) {
          console.error(`for this file the usual cause is that the engramux service is running.`)
          // cmd or PowerShell rather than a Git Bash spelling: MSYS path
          // conversion rewrites /end and /tn into C:/Program Files/Git/...,
          // and the spellings that survive it - //end, or an
          // MSYS_NO_PATHCONV=1 prefix - are wrong in the two shells a Windows
          // user is likelier to be in. One line that is right everywhere,
          // plus the name of the shell that breaks it.
          console.error(`stop it yourself, then run this again. run the line in cmd or PowerShell:`)
          console.error(`Git Bash rewrites /end and /tn into paths and the command fails there.`)
          console.error(`  schtasks /end /tn "\\Engramux"        # registered to start at logon`)
          console.error(`  taskkill /f /im engramux-service.exe   # started by hand`)
          console.error(`stopping it loses nothing: it is a hard kill, so the WAL keeps whatever was`)
          console.error(`committed but not checkpointed, and the next start recovers from it.`)
        } else {
          console.error(`for this file the usual cause is a hook firing right now - the relay runs`)
          console.error(`only for as long as one event takes. wait a moment and run this again.`)
          console.error(`do not stop the service for this one; the service is not what holds it.`)
        }
        console.error(`nothing was copied, and no hook configuration was written.`)
        process.exit(1)
      }
    }
    plan.push([src, dest])
  }
  return plan
}

if (!remove) {
  const plan = planCopies()
  let copied = 0
  for (const [src, dest] of plan) {
    if (apply) {
      try {
        mkdirSync(BIN, { recursive: true })
        copyFileSync(src, dest)
      } catch (err) {
        // Everything the probe cannot reach lands here: a destination that did
        // not exist to be probed, a disk that filled, a scanner holding the
        // file, the service started since the probe ran. The point is not to
        // prevent it - it is to leave the reader with what is on disk instead
        // of a stack trace over a half-finished install.
        console.error(`copying ${src} -> ${dest} failed: ${err.code ?? err.message}`)
        if (copied > 0) {
          console.error(`${copied} of ${plan.length} binaries were already updated, so this install is`)
          console.error(`half done: stop the service and run this again.`)
        }
        console.error(`no hook configuration was written.`)
        process.exit(1)
      }
      copied++
      changes.push(`copied ${src} -> ${dest}`)
    } else {
      changes.push(`would copy ${src} -> ${dest}`)
    }
  }
}

// Claude Code: command PLUS args, which is what selects exec form - the binary
// is spawned directly, no shell, no tokenization.
//
// `args` is empty and must still be present. `command` on its own is SHELL
// form, and on Windows that shell is Git Bash: it would spawn a shell for every
// hook event, which throws away the entire reason this relay is a Go binary
// (4.66 ms to start, against 33.5 ms for bare node), and it would route the
// command string through the MSYS path conversion this repository already lists
// as a gotcha. The relay takes no arguments by design - any argument at all
// puts cmd/engramux on its CLI path instead of its relay path - so the array is
// empty rather than carrying a subcommand.
install(CLAUDE, 'claude-code', () => ({
  type: 'command',
  command: RELAY.replaceAll('\\', '/'),
  args: [],
  timeout: TIMEOUT_SECONDS,
  statusMessage: 'engramux capture',
}), (doc) => (doc.hooks ??= {}))

// Codex: commandWindows as a single string. `command` is set to the same thing
// so the entry is not Windows-only by accident.
install(CODEX, 'codex', (event) => {
  const quoted = `"${RELAY.replaceAll('\\', '/')}"`
  return {
    type: 'command',
    command: quoted,
    commandWindows: quoted,
    timeout: EVENTS[event].codexTimeout ?? TIMEOUT_SECONDS,
    statusMessage: 'engramux capture',
  }
}, (doc) => (doc.hooks ??= {}))

console.log(changes.join('\n'))
console.log(
  apply
    ? `\ndone. start the service before it matters:\n  ${SERVICE}\nthen check:\n  ${RELAY} status`
    : '\nnothing was written. re-run with --apply.',
)
