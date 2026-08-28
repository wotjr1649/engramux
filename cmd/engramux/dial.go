//go:build !engramux_panicinject

package main

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

// dial connects to the service's pipe. This is the file the production build
// compiles; dial_panicinject.go is the other implementation of the same
// symbol, and it exists only so a test can build a relay whose dial panics.
// Neither the tag nor any trace of the panicking version reaches a shipped
// binary, and there is no flag or environment variable that could.
//
// winio.DialPipeContext retries only ERROR_PIPE_BUSY: a pipe that does not
// exist fails immediately rather than after the dial budget, which is what the
// relay wants when no service is running.
func dial(ctx context.Context, name string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, name)
}
