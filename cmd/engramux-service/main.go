// Command engramux-service is the single persistent process per Windows user
// (I-01). It must be linked with -H=windowsgui: Task Scheduler controls this
// process's own creation flags, so a console-subsystem service would show a
// window every time it started (spec 5.1).
package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "engramux-service: not implemented")
	os.Exit(1)
}
