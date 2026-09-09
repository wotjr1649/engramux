package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/schedule"
)

// update replaces the two installed binaries and takes the service through a
// stop, a wait, a copy and a start. It never touches host configuration.
//
// # Why this is not `install --apply` minus the host writes
//
// Because that division was a guess and reading the code contradicted it.
// [host.Install] is a plan, a copy, a task registration and a start - there is
// no stop anywhere in it, and `internal/schedule` exported no way to make one
// until this step added [schedule.End]. The only stop-and-wait that existed was
// eight lines of bash in scripts/reinstall.sh. So this is a new lifecycle
// command that happens to reuse the copy planner, rather than a subtraction.
//
// What the division does buy is the thing M-7 wrote it for: an agent may run
// `update` full stop, where `install --apply` needs `doctor` to have confirmed
// both hosts are already registered (AGENTS.md). Safety here is the definition
// of the command rather than a condition on the caller.
//
// # What it deliberately does not do
//
// It does not restore the previous binaries when a copy fails. Nothing copies
// them aside, and saying "restarts what was there" would describe a rollback
// this product does not have - after a partial copy, what was there is a mixed
// pair. What it does instead is start the service again and say exactly which
// destinations were replaced, so the state is legible rather than silently
// half-done.
func update(args []string) int {
	ctx := context.Background()

	from, rest, ok := updateSource(args)
	if !ok {
		return 1
	}

	opt, err := currentPaths(rest)
	if err != nil {
		warn("update: %v", err)
		return 1
	}
	// Resolved after the paths and not with the arguments, because the
	// fallback source is one of them: the plugin cache moves with
	// CLAUDE_CONFIG_DIR and only [resolvePaths] knows where it went.
	from, note, found := updateFrom(from, opt.PluginCache)
	if !found {
		for _, line := range updateNoSourceHelp {
			warn("%s", line)
		}
		return 1
	}
	if note != "" {
		fmt.Println(note)
	}
	if code := updateRefusals(ctx, from, opt); code != 0 {
		return code
	}

	bins := []host.Binary{
		{Name: host.RelayName, Role: host.Relay},
		{Name: host.ServiceName, Role: host.Service},
	}
	// probe false: the service is still up, so its image is locked by
	// definition and a probe here would refuse every installed machine. The
	// probe that matters happens after the stop, in [updateWaitForUnlock].
	plan, unchanged, err := host.PlanCopies(from, opt.BinDir, bins, false)
	if err != nil {
		warn("update: %v", err)
		return 1
	}
	for _, u := range unchanged {
		fmt.Printf("already up to date: %s\n", u)
	}
	if len(plan) == 0 {
		fmt.Println("nothing to replace.")
		return 0
	}

	fmt.Printf("stopping %s\n", opt.TaskName)
	if err := schedule.End(ctx, opt.TaskName); err != nil {
		// Not fatal on its own: a task with no running instance answers
		// with an error, and the wait below is what establishes whether
		// anything is still up.
		fmt.Printf("stop reported: %v\n", err)
	}
	if !updateWaitForStop() {
		warn("update: a service is still answering after %v; not replacing a binary under it", updateStopWait)
		return 1
	}
	fmt.Println("stopped.")

	if err := updateWaitForUnlock(from, opt.BinDir, bins); err != nil {
		warn("update: %v", err)
		updateStart(ctx, opt.TaskName)
		return 1
	}

	copied, copyErr := host.Copy(plan)
	for _, c := range copied {
		fmt.Printf("replaced %s\n", c)
	}
	if copyErr != nil {
		warn("update: %v", copyErr)
		warn("the destinations listed above were replaced and the rest were not; there is no " +
			"rollback, so re-run this once the cause is fixed")
	}

	updateStart(ctx, opt.TaskName)
	if !updateWaitForStart() {
		warn("update: the service did not answer within %v - run `engramux doctor`", updateStartWait)
		return 1
	}
	fmt.Println("started.")
	if copyErr != nil {
		return 1
	}
	return 0
}

// The three bounds. Stop and start match scripts/reinstall.sh's own 60 s, which
// is the only figure anyone has measured this against. The unlock bound is
// shorter because it is waiting on a handle Windows releases with the process,
// not on a process that has to finish exiting.
const (
	updateStopWait   = 60 * time.Second
	updateStartWait  = 60 * time.Second
	updateUnlockWait = 30 * time.Second
	updatePoll       = time.Second
)

// updateSource reads --from, and returns the arguments with it removed.
//
// `--from` was the only door while there was no delivery channel. Since 0.1.0
// the plugin is one, so a missing `--from` is no longer a refusal here: it means
// "resolve a source", and [updateFrom] does that. What this still refuses is a
// `--from` that names nothing, because that is a typo rather than an omission
// and quietly reading the plugin cache instead would be the wrong repair.
//
// # Why it returns the rest, and what happened when it did not
//
// Every command here takes an optional positional task name, and `taskName`
// reads the first argument that does not begin with a dash (`withoutFlags`).
// A separated `--from <dir>` leaves that directory sitting in that position, so
// the first real run of this command asked Windows for a task called
// `D:/AI_DEV/engramux/dist` and refused it as not being a task path. The
// message was accurate and about the wrong thing entirely.
//
// Removing the flag *and its value* is the fix rather than teaching taskName
// about this one flag: the coupling is that a flag with a value consumes two
// arguments, and it belongs to whoever knows the flag.
func updateSource(args []string) (from string, rest []string, ok bool) {
	rest = make([]string, 0, len(args))
	dangling := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--from" && i+1 < len(args):
			// An empty value is the same mistake as no value: it
			// must not read as the empty string, which would plan a
			// copy out of this process's working directory.
			if args[i+1] == "" {
				dangling = true
			}
			from = args[i+1]
			i++ // the value is the flag's, not a positional
		case a == "--from":
			dangling = true
		default:
			if v, found := strings.CutPrefix(a, "--from="); found {
				if v == "" {
					dangling = true
				}
				from = v
				continue
			}
			rest = append(rest, a)
		}
	}
	if dangling {
		warn("update: --from names no directory. Give it one, or leave it off " +
			"and the plugin cache is read instead.")
		return "", rest, false
	}
	// No --from at all is not an error any more; it means "resolve one".
	// The caller does that, because the plugin cache is a resolved path.
	return from, rest, true
}

// updateFrom answers the directory to copy from, and the line to print about
// how it was chosen.
//
// # Why a bare `update` now finds something
//
// The plugin is the delivery channel: Claude Code fetches the release archive,
// checks it against the SHA-256 the marketplace entry names, and unpacks it
// into its cache. Until this, `update` still had to be handed that directory by
// hand - so `/plugin update` moved bytes into a place nothing read and the
// service kept running the build it already had, which is a channel delivering
// where nobody looks.
//
// # An explicit --from wins, and that is not a tie-break
//
// Building into `dist/` and updating from it is the development loop, and that
// build is routinely newer than the released plugin. A resolver preferring the
// cache would undo it every time.
//
// # No downgrade guard, deliberately
//
// [host.PlanCopies] compares bytes, so an unchanged version is already a no-op
// that prints "nothing to replace". An older cache version is visible in the
// note this returns, printed before anything is stopped or copied, and it is
// undone by one explicit `--from`. Guarding it properly means asking the
// running service its version over IPC before stopping it.
// ponytail: no version comparison here, add the IPC one if a real downgrade bites.
func updateFrom(explicit, cacheRoot string) (dir, note string, ok bool) {
	if explicit != "" {
		return explicit, "", true
	}
	found, version := newestPlugin(cacheRoot)
	if found == "" {
		return "", "", false
	}
	return found, fmt.Sprintf("no --from given; taking %s %s from Claude Code's plugin cache", pluginName, version), true
}

// updateNoSourceHelp is what a bare `update` says.
//
// It is a value rather than three warn calls so that a test can read it. warn
// writes straight to os.Stderr with no seam, and adding one for a string is a
// larger change than naming the string.
//
// # It has been wrong in both directions, and that is why it is tested
//
// It first said "download the release archive, unpack it" one line after saying
// there was no delivery channel - sending a reader after a file nobody had
// built. It was then narrowed to name only a directory the reader was supposed
// to already have, which was true while there was no release. 0.1.0 was
// published on 2026-09-09 and the plugin became a real channel, so this is now
// reached only when that channel is *absent* - and what it has to name is how
// to get it.
var updateNoSourceHelp = []string{
	"update: no --from, and Claude Code's plugin cache holds no copy to read.",
	"the plugin is the delivery channel: it fetches the release archive, checks its",
	"SHA-256 and unpacks it, and a bare `update` then reads that directory.",
	"    /plugin marketplace add wotjr1649/engramux",
	"    /plugin install engramux@engramux",
	"or name a directory holding both binaries yourself:",
	"    engramux update --from <directory>",
}

// updateRefusals is every check that has to happen before anything stops.
//
// The two decisions are split out below and take everything they read as
// arguments, which is what makes them exercisable at all - the alternative is a
// guard about this process's own location and this machine's own task registry
// that no test can ever reach. resolveOptions is written the same way for the
// same reason.
func updateRefusals(ctx context.Context, from string, opt host.Options) int {
	self, err := os.Executable()
	if err != nil {
		warn("update: locate this binary: %v", err)
		return 1
	}
	if msg := updatePathRefusal(self, from, opt.BinDir); msg != "" {
		warn("update: %s", msg)
		return 1
	}
	task, err := schedule.Query(ctx, opt.TaskName)
	if msg := updateTaskRefusal(task, err, opt); msg != "" {
		warn("update: %s", msg)
		return 1
	}
	return 0
}

// updatePathRefusal refuses a copy that cannot work, and says why in the
// sentence the cause deserves.
//
// Windows will not overwrite a running image, and the process that would be
// overwritten by an update run out of the install directory is the one running
// it. Without this the failure arrives several steps in, as a sharing violation
// whose own message says to try again in a moment - advice about a lock held by
// the process printing it. `install` has the same guard for the same reason
// (resolveOptions) and it is not reachable from here, because that one is a
// property of installOptions rather than of a command.
func updatePathRefusal(self, from, binDir string) string {
	if filepath.Clean(filepath.Dir(self)) == filepath.Clean(binDir) {
		return fmt.Sprintf("this is the installed copy in %s, and Windows will not let it "+
			"overwrite itself. Run the engramux.exe you unpacked or built", binDir)
	}
	if filepath.Clean(from) == filepath.Clean(binDir) {
		return "--from is the installed directory; there is nothing to copy from it"
	}
	return ""
}

// updateTaskRefusal refuses when the logon task is not the thing this would be
// updating.
//
// The task is what actually starts a service, and this command does not
// re-register it - `install` does. So a task pointing somewhere else would have
// update copy new bytes into BinDir, start an old binary from elsewhere, and
// report success. A machine with no task at all is worse: two binaries would
// land with no task and no hook entries, and `doctor` would switch from "run
// install --apply" to a broken-installation report, which is exactly the
// confusion M-6 removed.
func updateTaskRefusal(task schedule.Task, err error, opt host.Options) string {
	switch {
	case errors.Is(err, schedule.ErrNotRegistered):
		return fmt.Sprintf("%s is not registered, so there is no installation to update. "+
			"Run `engramux install --apply` first", opt.TaskName)
	case err != nil:
		return fmt.Sprintf("read the logon task: %v", err)
	}
	want := filepath.Join(opt.BinDir, host.ServiceName)
	if filepath.Clean(task.Command) != filepath.Clean(want) {
		return fmt.Sprintf("%s runs a different binary than the one this would replace. "+
			"Run `engramux install --apply` to re-register it", opt.TaskName)
	}
	return ""
}

// updateWaitForStop waits until nothing answers the pipe.
//
// It polls `status` rather than the task's state, and the reason is in
// [schedule.End]: `schtasks /end` returns before the process is gone, and a task
// that has only just been asked to stop still reads as running. A service that
// has stopped answering is the only evidence that the image is free.
func updateWaitForStop() bool {
	deadline := time.Now().Add(updateStopWait)
	for {
		if _, err := askStatus(); err != nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(updatePoll)
	}
}

// updateWaitForStart waits until something answers again.
func updateWaitForStart() bool {
	deadline := time.Now().Add(updateStartWait)
	for {
		if _, err := askStatus(); err == nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(updatePoll)
	}
}

// updateWaitForUnlock waits for the destinations to be writable.
//
// A stopped service has stopped answering before Windows has necessarily
// released its image, and a scanner can hold a file it has just seen appear.
// [host.PlanCopies] with the probe on is the same handle a copy would ask for,
// so this reuses it rather than opening a second definition of "writable".
func updateWaitForUnlock(from, dest string, bins []host.Binary) error {
	deadline := time.Now().Add(updateUnlockWait)
	for {
		_, _, err := host.PlanCopies(from, dest, bins, true)
		if err == nil {
			return nil
		}
		var locked *host.LockedError
		if !errors.As(err, &locked) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("a destination was still locked after %v: %w", updateUnlockWait, err)
		}
		time.Sleep(updatePoll)
	}
}

// updateStart asks the task to run and says so, without deciding anything: the
// caller's own wait is what establishes whether it came up.
func updateStart(ctx context.Context, name string) {
	fmt.Printf("starting %s\n", name)
	if err := schedule.Run(ctx, name); err != nil {
		warn("update: start %s: %v", name, err)
	}
}
