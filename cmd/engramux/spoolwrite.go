//go:build !engramux_settlepanic

package main

import "github.com/wotjr1649/engramux/internal/spool"

// spoolWrite saves the undelivered event. This is the file the production
// build compiles; spoolwrite_panicinject.go is the other implementation of the
// same symbol, and it exists only so a test can build a relay whose settle
// panics. Neither the tag nor any trace of the panicking version reaches a
// shipped binary, and there is no flag or environment variable that could.
//
// The nil logger is deliberate, not an omission. spool.Write's age sweep logs
// the records it drops, and this process installs no slog handler at all, so
// passing anything other than nil would hand it the unfiltered package default
// - see internal/spool's logger. The service, which does install a filtered
// one, is what reports those drops.
func spoolWrite(dir, id string, payload []byte) error {
	return spool.Write(dir, id, payload, nil)
}
