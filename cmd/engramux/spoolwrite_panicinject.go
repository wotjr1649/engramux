//go:build engramux_settlepanic

package main

// spoolWrite panics. This file is compiled only under the engramux_settlepanic
// build tag, which nothing but the relay's settle-panic test sets; the
// production build compiles spoolwrite.go instead and contains none of this.
//
// A separate tag from engramux_panicinject, because the two tests need
// different programs: that one panics in the dial, so settle recovers and still
// spools, and this one panics *inside settle's own body*, after the recover
// that catches what main was doing has already run. One tag setting both would
// make the first test unwritable.
//
// That inner path is the one with nothing behind it. settle's deferred handler
// is the last thing in the process, and a panic in the code that saves events
// takes the event with it - the assertion is only that the process still exits
// 0, which is what I-03 promises and what a host sees.
func spoolWrite(string, string, []byte) error {
	panic("engramux_settlepanic: spoolWrite")
}
