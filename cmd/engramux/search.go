package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// search asks the service for events matching the words a person typed and
// prints them (I-08).
//
// There is no fallback that reads the database directly, and there cannot be
// one: the service holds it exclusively (I-07), so this pipe is the only way to
// reach the index. A service that is down therefore fails here, exactly as
// [status] does, rather than producing a second, lesser answer.
//
// The words are joined by single spaces and sent as one query. Nothing is
// interpreted here - not a leading dash, not a quote, not an operator - because
// internal/search quotes every token before it reaches MATCH, and 1.0
// deliberately offers no query language.
func search(args []string) int {
	if len(args) == 0 {
		warn("usage: engramux search <words…>")
		return 2
	}

	reply, err := askSearch(strings.Join(args, " "))
	if err != nil {
		warn("search: %v", err)
		return 1
	}
	// Reached only after Verify, which is what makes "no hits" mean nothing
	// matched. A rejected ACK decodes into a reply with no hits too, and
	// printing this for one would report an empty index as an empty result.
	if len(reply.Hits) == 0 {
		// Stdout, because this is the CLI path and a person asked for
		// it. The relay's silence on stdout (spec 4.5) is a rule about
		// hook events.
		_, _ = fmt.Fprintln(os.Stdout, "no results")
		return 0
	}

	for _, h := range reply.Hits {
		// The event name and the excerpt are both quoted, and for the
		// same reason [cells] quotes the event name: they are payload
		// text, so they carry untrusted bytes - control characters and
		// terminal escapes among them - and the excerpt spans leaves,
		// so it carries the newline internal/store joins them with.
		// Quoting is what keeps one hit to one block of three lines.
		//
		// The host is not quoted: the events.host CHECK constrains it
		// to three values this program knows.
		_, _ = fmt.Fprintf(os.Stdout, "%s  %-11s  %.64q\n%s\n%q\n\n",
			stamp(h.ReceivedAtMS), h.Host, h.EventName, h.ID, h.Excerpt)
	}
	return 0
}

// askSearch sends one Search request and returns the reply it can accept.
//
// The reply is checked with [ipc.SearchReply.Verify] before a single hit is
// read, and that check is what tells a search reply from the rejected ACK the
// service answers a request it will not serve. Without it an Ack would decode
// into a reply with an empty hit list and this command would print "no results"
// for a service that refused the request.
//
// No limit is sent. The command has no flag for one, so the service applies
// [ipc.DefaultSearchLimit]; a limit is a wire field rather than a CLI feature
// until something asks for it.
func askSearch(query string) (ipc.SearchReply, error) {
	var zero ipc.SearchReply

	payload, err := json.Marshal(ipc.SearchRequest{Query: query})
	if err != nil {
		return zero, fmt.Errorf("encode the query: %w", err)
	}
	raw, err := roundTrip(ipc.Search, payload)
	if err != nil {
		return zero, err
	}

	var reply ipc.SearchReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return zero, fmt.Errorf("decode the reply: %w", err)
	}
	if err := reply.Verify(); err != nil {
		// The reply is bounded on its way into the message: it is bytes
		// off the wire and capped only by ipc.MaxFrameLen. It is also
		// the one case where those bytes are not payload text - a
		// refusal is an ACK - so 200 of them are safe to show.
		return zero, fmt.Errorf("%w: the service replied %.200q", err, raw)
	}
	return reply, nil
}
