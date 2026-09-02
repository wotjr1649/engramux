package main

import (
	"testing"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// TestStatusPrintsTheHealthLines is backlog 31 at the terminal, pinned by
// value: the two new lines sit between spool and database, a checkpoint that
// has not happened says so, and a failed one carries its reason.
func TestStatusPrintsTheHealthLines(t *testing.T) {
	const ms = 1700000000123
	fresh := ipc.StatusReply{UptimeMS: 1500, Events: 3, SpoolDepth: 0, DatabasePath: "<db>"}
	want := "uptime      1.5s\nevents      3\nspool       0\nerrors      0\ncheckpoint  none yet\ndatabase    <db>\n"
	if got := formatStatus(fresh); got != want {
		t.Errorf("fresh status =\n%q\nwant\n%q", got, want)
	}

	failed := ipc.StatusReply{UptimeMS: 1500, Events: 3, Errors: 2, DatabasePath: "<db>",
		LastCheckpoint: &ipc.CheckpointResult{AtMS: ms, Error: "busy"}}
	want = "uptime      1.5s\nevents      3\nspool       0\nerrors      2\ncheckpoint  " + stamp(ms) + " failed: busy\ndatabase    <db>\n"
	if got := formatStatus(failed); got != want {
		t.Errorf("failed-checkpoint status =\n%q\nwant\n%q", got, want)
	}

	ok := ipc.StatusReply{LastCheckpoint: &ipc.CheckpointResult{AtMS: ms}}
	if got, want := checkpointLine(ok.LastCheckpoint), stamp(ms)+" ok"; got != want {
		t.Errorf("checkpointLine(ok) = %q, want %q", got, want)
	}
}
