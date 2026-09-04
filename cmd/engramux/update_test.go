package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/host"
	"github.com/wotjr1649/engramux/internal/schedule"
)

// The two refusals below are the whole of what stops `update` from doing
// something worse than nothing, and both were found by reading rather than by
// running: the self-image one produces an error message that blames a lock the
// process printing it is holding, and the task one lets new bytes land beside a
// task that starts an old binary from somewhere else and calls it a success.

func TestUpdateRefusesToOverwriteItself(t *testing.T) {
	const bin = `C:\Users\x\AppData\Local\engramux\bin`

	// The installed copy: same directory as the destination.
	if msg := updatePathRefusal(filepath.Join(bin, host.RelayName), `D:\build`, bin); msg == "" {
		t.Error("running the installed copy was allowed; Windows will refuse the copy several " +
			"steps later with a message that blames the service")
	} else if !strings.Contains(msg, bin) {
		t.Errorf("the refusal does not name the directory: %q", msg)
	}

	// A build tree: different directory, and allowed.
	if msg := updatePathRefusal(`D:\build\engramux.exe`, `D:\build`, bin); msg != "" {
		t.Errorf("a build tree was refused: %q", msg)
	}

	// --from pointing at the installed directory copies a file over itself.
	if msg := updatePathRefusal(`D:\build\engramux.exe`, bin, bin); msg == "" {
		t.Error("--from pointing at the install directory was allowed")
	}

	// An uncleaned destination. This case exists because a break-it pass
	// killed the first two attempts at it, and what it killed is worth more
	// than the case itself: filepath.Join cleans its own result, and
	// filepath.Dir cleans too, so neither an uncleaned self nor a joined one
	// can tell filepath.Clean(filepath.Dir(self)) from filepath.Dir(self).
	// The only argument that distinguishes them is the destination, which
	// arrives from host.Options rather than from a filepath call and is
	// therefore the one that is not already clean.
	dirty := `C:\Users\x\AppData\Local\engramux\.\bin`
	if dirty == bin {
		t.Fatal("this case is meant to need cleaning and does not")
	}
	if msg := updatePathRefusal(filepath.Join(bin, host.RelayName), `D:\build`, dirty); msg == "" {
		t.Error("an uncleaned install directory got past the self-image guard")
	}
}

func TestUpdateRefusesATaskThatIsNotThisInstallation(t *testing.T) {
	opt := host.Options{
		BinDir:   `C:\Users\x\AppData\Local\engramux\bin`,
		TaskName: "Engramux",
	}
	installed := filepath.Join(opt.BinDir, host.ServiceName)

	if msg := updateTaskRefusal(schedule.Task{Command: installed}, nil, opt); msg != "" {
		t.Errorf("the installation's own task was refused: %q", msg)
	}

	// No task: two binaries would land with no task and no hook entries, and
	// doctor would report a broken installation rather than an absent one.
	msg := updateTaskRefusal(schedule.Task{}, schedule.ErrNotRegistered, opt)
	if msg == "" {
		t.Fatal("an unregistered task was allowed")
	}
	if !strings.Contains(msg, "install --apply") {
		t.Errorf("the refusal does not say what to run instead: %q", msg)
	}

	// A task pointing somewhere else: the copy would succeed and the start
	// would run the wrong binary.
	if msg := updateTaskRefusal(schedule.Task{Command: `D:\elsewhere\engramux-service.exe`}, nil, opt); msg == "" {
		t.Error("a task running a different binary was allowed")
	}

	// A query that failed for any other reason is not a green light.
	if msg := updateTaskRefusal(schedule.Task{Command: installed}, errors.New("schtasks exploded"), opt); msg == "" {
		t.Error("an unreadable task registry was treated as a match")
	}
}

// TestUpdateSaysWhereToGetAnArtefact holds the one thing a bare `update` can
// usefully do while there is no delivery channel: name the door that exists.
func TestUpdateSaysWhereToGetAnArtefact(t *testing.T) {
	if _, _, ok := updateSource(nil); ok {
		t.Error("a bare update claimed a source")
	}
	for _, args := range [][]string{
		{"--from", `D:\build`},
		{`--from=D:\build`},
	} {
		got, rest, ok := updateSource(args)
		if !ok {
			t.Errorf("%v was not read as a source", args)
			continue
		}
		if got != `D:\build` {
			t.Errorf("%v gave %q, want %q", args, got, `D:\build`)
		}
		// The value must not survive as a positional. Every command
		// here takes an optional task name and taskName reads the first
		// argument that does not begin with a dash - so a directory
		// left in that position becomes the task name. The first real
		// run of this command did exactly that and asked Windows for a
		// task called D:/AI_DEV/engramux/dist.
		if len(rest) != 0 {
			t.Errorf("%v left %v behind, which taskName would read as a task name", args, rest)
		}
	}
	// A genuine positional still survives, because a caller may name a task.
	if _, rest, _ := updateSource([]string{"--from", `D:\build`, "OtherTask"}); len(rest) != 1 || rest[0] != "OtherTask" {
		t.Errorf("the task name did not survive: %v", rest)
	}
	// A --from with nothing after it is not a source, and must not be read
	// as the empty string - that would plan a copy from the process's own
	// working directory.
	if got, _, ok := updateSource([]string{"--from"}); ok {
		t.Errorf("a dangling --from gave %q", got)
	}
}

// TestUpdateDoesNotSendTheReaderAfterAnArtefactThatDoesNotExist holds the one
// thing a bare `update` must not do.
//
// The message contradicted itself: its first line said there is no delivery
// channel and its second told the reader to download the release archive. There
// is no release - no tag, no archive, no marketplace entry - so the second line
// sent whoever read it looking for a file nobody has ever built, and it is the
// first thing a reader meets after the README tells them `update` is not a first
// step.
//
// What is asserted is the pair, because either half alone passes for the wrong
// reason: a message that names no source at all is honest and useless, and a
// message naming --from could still carry the download line beside it.
//
// This test is deleted, not relaxed, on the day a release exists. Telling a
// reader to download an archive is then correct, and a test forbidding it would
// be pinning a fact that has moved.
func TestUpdateDoesNotSendTheReaderAfterAnArtefactThatDoesNotExist(t *testing.T) {
	help := strings.Join(updateNoSourceHelp, "\n")
	for _, banned := range []string{"release", "archive", "download", "unpack"} {
		if strings.Contains(strings.ToLower(help), banned) {
			t.Errorf("the bare-update message says %q, and there is no release to get:\n%s", banned, help)
		}
	}
	if !strings.Contains(help, "--from <directory>") {
		t.Errorf("the bare-update message does not name the only door there is:\n%s", help)
	}
}
