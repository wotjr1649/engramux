// Command engramux-service is the single persistent process per Windows user
// (I-01). It must be linked with -H=windowsgui: Task Scheduler controls this
// process's own creation flags, so a console-subsystem service would show a
// window every time it started (spec 5.1).
//
// # One entry point, no subcommand
//
// Running this binary starts the service. There is no `serve` verb, and the
// reason is measured rather than assumed: a -H=windowsgui binary launched from
// a terminal *does* inherit that terminal's stdio. -H=windowsgui means Windows
// allocates no console of its own, not that the handles are lost. So developing
// and dogfooding against engramux-service.exe directly works, and a subcommand
// would be a second way to spell the only thing it does.
//
// The log still goes to a file (spec 5.6), because under Task Scheduler there
// is no console at all and stderr goes nowhere. The one line below is for the
// terminal case, where a startup failure would otherwise look like a process
// that exited for no reason.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/wotjr1649/engramux/internal/service"
)

func main() {
	// Ctrl+C, for the terminal case. Task Scheduler stops a task with
	// TerminateProcess and this never fires there - which costs nothing,
	// because the exclusive lock does not survive process death either way
	// (docs/evidence/crash). It is what makes a dogfooding session stoppable
	// without killing it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dir, err := service.Dir()
	if err == nil {
		err = service.Run(ctx, dir)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "engramux-service:", err)
		os.Exit(1)
	}
}
