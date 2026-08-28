//go:build engramux_panicinject

package main

import (
	"context"
	"net"
)

// dial panics. This file is compiled only under the engramux_panicinject build
// tag, which nothing but the relay's panic test sets; the production build
// compiles dial.go instead and contains none of this.
//
// A build tag rather than a flag, an environment variable or a function
// variable, because all three would be an injection point present in the
// shipped binary - and gate clause 2 asks for a panic the test controls, not
// for a way to make the product panic.
//
// The panic lands after stdin has been read and after the id has been minted,
// which is the case worth testing: main's deferred handler has to recover it
// and still spool the event under the id the send would have used.
func dial(context.Context, string) (net.Conn, error) {
	panic("engramux_panicinject: dial")
}
