package main

import (
	"encoding/json"
	"fmt"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// replied is what every CLI read reports when the reply it decoded fails its
// own Verify: verifyErr is that failure, raw is the frame.
//
// A request the service would not serve is answered with a rejected
// [ipc.Ack] whatever it asked for, and since backlog 27 that Ack says why. So
// the frame is read again as an Ack, and when it is one with a reason, the
// reason is the message - it is the service's own error, already masked on its
// way out (I-10), and it is what a person can act on. Only when there is no
// reason to show is the frame itself printed, bounded on its way into the
// message: it is bytes off the wire and capped only by ipc.MaxFrameLen, and an
// Ack is three short fields while anything else is payload text.
func replied(verifyErr error, raw []byte) error {
	var ack ipc.Ack
	if err := json.Unmarshal(raw, &ack); err == nil && ack.Status == ipc.Rejected && ack.Reason != "" {
		return fmt.Errorf("%w: the service refused it: %s", verifyErr, ack.Reason)
	}
	return fmt.Errorf("%w: the service replied %.200q", verifyErr, raw)
}
