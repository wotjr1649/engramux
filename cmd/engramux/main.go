// Command engramux is the hook relay and the CLI. It is a console-subsystem
// binary so that it inherits the host's stdio when a hook invokes it, and one
// process runs per hook event (spec 5.1).
package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "engramux: not implemented")
	os.Exit(1)
}
