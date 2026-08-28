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
//     Re-run this script after a rebuild to push a new relay to the hooks.
//
//   - MERGES into the existing configuration. Both files already carry other
//     tools' hooks; nothing that is not Engramux's is added, reordered or
//     removed. An Engramux entry is recognised by its command path, so running
//     this twice refreshes rather than duplicates.
//
//   - Backs up each file it touches to <file>.engramux-backup-<timestamp>.
//
// The two hosts take different shapes (spec 4.2), and this is not a detail:
// Claude Code takes `command` plus an `args` array, exec form, no shell. Codex
// takes `commandWindows` as one string. Claude Code has no `commandWindows`
// key, so a hook that only sets it is never invoked.

import { readFileSync, writeFileSync, existsSync, mkdirSync, copyFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join, dirname } from 'node:path'

// The 11-event intersection (spec 4.1). Both hosts expose all of these; 1.0
// handles the intersection and nothing else.
// The value is the `matcher`, or null to omit the key.
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
  SessionStart: '*',
  SessionEnd: null,
  UserPromptSubmit: null,
  PreToolUse: '*',
  PostToolUse: '*',
  Stop: null,
  SubagentStart: null,
  SubagentStop: null,
  PreCompact: '*',
  PostCompact: '*',
  PermissionRequest: '*',
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

const REPO = join(dirname(new URL(import.meta.url).pathname.slice(1)), '..')
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
      const matcher = EVENTS[event]
      kept.push(matcher === null ? { hooks: [makeHook()] } : { matcher, hooks: [makeHook()] })
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

if (!remove) {
  for (const [src, dest] of [[DIST_RELAY, RELAY], [DIST_SERVICE, SERVICE]]) {
    if (!existsSync(src)) {
      console.error(`missing ${src} - build first:`)
      console.error(`  CGO_ENABLED=0 go build -ldflags "-s -w"               -o dist/engramux.exe         ./cmd/engramux`)
      console.error(`  CGO_ENABLED=0 go build -ldflags "-s -w -H=windowsgui" -o dist/engramux-service.exe ./cmd/engramux-service`)
      process.exit(1)
    }
    if (apply) {
      mkdirSync(BIN, { recursive: true })
      copyFileSync(src, dest)
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
install(CODEX, 'codex', () => {
  const quoted = `"${RELAY.replaceAll('\\', '/')}"`
  return {
    type: 'command',
    command: quoted,
    commandWindows: quoted,
    timeout: TIMEOUT_SECONDS,
    statusMessage: 'engramux capture',
  }
}, (doc) => (doc.hooks ??= {}))

console.log(changes.join('\n'))
console.log(
  apply
    ? `\ndone. start the service before it matters:\n  ${SERVICE}\nthen check:\n  ${RELAY} status`
    : '\nnothing was written. re-run with --apply.',
)
