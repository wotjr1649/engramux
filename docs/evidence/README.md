# Evidence harnesses

The design document marks every factual claim `[verified]` or `[unverified]`. These are the programs
behind the `[verified]` ones that needed more than a one-line command. Each but `soak` is its own Go
module, so the root module's `./...` does not pick them up; `soak` is data, not a program.

Run one with:

```bash
cd docs/evidence/<name> && go mod tidy && go build -o probe.exe . && ./probe.exe
```

| Harness | Answers | Result recorded in |
|---|---|---|
| `pidreuse` | How often Windows reuses a PID, and how quickly. Decides whether pipe peer verification needs process creation time | §7.2, §7.4-1 |
| `pipe` | Whether N processes racing `ListenPipe` on one fixed name produce exactly one winner. This is I-09 | §5.2, §7.2 |
| `crash` | Whether a row committed with the production DSN survives `TerminateProcess`, and whether a new process can then take the exclusive lock | §7.2 |
| `wal` | WAL growth per event under the corpus payload distribution, and checkpoint cost | §7.2 |
| `ckpt` | Cold `TRUNCATE` checkpoint cost at several WAL sizes. `wal` measures TRUNCATE *after* a PASSIVE has already copied every page, which is not the number the checkpoint policy needs | §5.4, §7.4-2 |
| `nodespawn` | What spawning Node costs versus a Go binary, as the comparison against upstream claude-mem | §7.2 |
| `console` | Whether a console-less parent spawning a console child creates a visible window, under three creation-flag variants. Build `child` as a console binary and `parent` with `-H=windowsgui`, then run `parent <dir>` | §5.1, §7.1 |
| `exclusive` | Whether `locking_mode=exclusive` refuses a second connection and survives a writer + reader + checkpoint load. It also reports no `-shm` file, and **that half of its answer was measured on the one case that cannot fail** — a database it creates itself. Under the DSN it ran against, every reopen made a 32,768-byte one. The three states that decide it are `TestOpenCreatesNoSharedMemoryFile` and `TestReopenCreatesNoSharedMemoryFile` in `internal/store`, and `TestReopeningAHotWALCreatesNoSharedMemoryFile` in `internal/spool` | §5.4, §7.1 |
| `soak` | Not a program. The Phase 6 soak's sampler output, three TSV files: `soak.tsv` is the 72-hour series from the reboot that started the clock, its first fourteen rows being the pre-reboot tail; `soak-shakedown.tsv` is the instrument check that preceded it; `soak-pre-reboot.tsv` an earlier fragment. Reduce a file with `bash scripts/soak-summary.sh docs/evidence/soak/soak.tsv` from the repository root; it reads from the last uptime reset and prints no path | §7.1, §8 Phase 6 |

`nodespawn` needs a trivial Go binary to measure bare process creation cost. `noop-main.go.txt`
holds it; put it in `noop/main.go` and `go build -o noop.exe ./noop` before running.

Two cautions, both of which bit during the original run:

- **Order matters in `wal`.** Measuring `TRUNCATE` right after a full `PASSIVE` reports the cost of
  truncating an already-checkpointed WAL, not the cost of a checkpoint. That is why `ckpt` exists
  separately.
- **`pidreuse` runs at roughly 197 spawns/s**, about a hundred times real load. That is deliberate:
  a higher spawn rate cycles the PID space faster, so the reuse intervals it reports are a lower
  bound. At real load the gaps are longer, which only makes the conclusion safer.
