package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
// The words are joined by single spaces and sent as one query. Nothing inside
// the query is interpreted - not a dash, not a quote, not an operator - because
// internal/search quotes every token before it reaches MATCH, and 1.0
// deliberately offers no query language.
//
// # One exception, and it is only in first position
//
// `--project <path>` scopes the search, and it is read only as the first two
// arguments (spec 5.9). The cost is stated rather than hidden: a search for the
// literal words "--project something" can no longer be typed. Reading the flag
// anywhere in the list would cost more - every query carrying those bytes
// anywhere in it - and reading it nowhere would leave this command unable to
// reach the scoping the tool surface has.
//
// # This command's default stays global, and there is no `--all`
//
// Omitting the flag searches every project, which is what this command has
// always done and what an existing invocation must keep doing. A flag to
// *un*-scope would therefore be a flag for the default, and it would only be
// worth having if the default flipped - which would silently change what every
// existing invocation returns. The MCP tool schema is where a project is
// required, because there the caller is a model with no working directory.
func search(args []string) int {
	scope, words, err := searchScope(args)
	if err != nil {
		warn("search: %v", err)
		return 2
	}
	if len(words) == 0 {
		warn("usage: engramux search [--project <path>] <words…>")
		return 2
	}

	reply, err := askSearch(ipc.SearchRequest{Query: strings.Join(words, " "), Project: scope})
	if err != nil {
		warn("search: %v", err)
		return 1
	}
	// Reached only after Verify, which is what makes "no hits" mean nothing
	// matched. A rejected ACK decodes into a reply with no hits too, and
	// printing this for one would report an empty index as an empty result.
	if len(reply.Hits) == 0 && len(reply.MemoryHits) == 0 {
		// Stdout, because this is the CLI path and a person asked for
		// it. The relay's silence on stdout (spec 4.5) is a rule about
		// hook events.
		_, _ = fmt.Fprintln(os.Stdout, "no results")
		return 0
	}

	// How many were shown against how many matched (backlog 33), so a list
	// that is everything reads differently from the first twenty of a
	// thousand. Always printed rather than only when they differ: a line
	// that appears sometimes is a line a reader has to know to look for.
	_, _ = fmt.Fprintf(os.Stdout, "%d of %d matches\n\n", len(reply.Hits), reply.Total)

	for _, h := range reply.Hits {
		// Three of the four are quoted, and the rule is the schema
		// rather than the field's name. events.host has a CHECK
		// constraining it to three values this program knows, so it is
		// printed bare. events.event_name has none - it is whatever a
		// payload's hook_event_name said - and events.id has none
		// either: it is TEXT PRIMARY KEY, and the routing boundary only
		// requires it to be non-empty, so an id carrying a newline or a
		// terminal escape is storable and this is the first command
		// that prints one. The excerpt is payload text throughout and
		// spans leaves, so it carries the newline internal/store joins
		// them with.
		//
		// Quoting is also what keeps one hit to one block of three
		// lines, whatever bytes those three fields hold.
		_, _ = fmt.Fprintf(os.Stdout, "%s  %-11s  %s\n%.64q\n%q\n\n",
			stamp(h.ReceivedAtMS), h.Host, cutName(h.EventName, h.EventNameTruncated), h.ID, h.Excerpt)
	}

	// The second list, printed under its own heading and never interleaved
	// with the first (memory spec rev.2, M-2 decision 6). The two are ranked
	// by separate indexes and their scores are not comparable, so a merged
	// list would be putting them in an order that means nothing.
	if len(reply.MemoryHits) == 0 {
		return 0
	}
	_, _ = fmt.Fprintf(os.Stdout, "%d of %d native memory matches\n\n",
		len(reply.MemoryHits), reply.MemoryTotal)
	for _, h := range reply.MemoryHits {
		// Quoted on the same rule the event hits are: every one of these
		// but the host came out of a file this program did not write, so
		// a title carrying a terminal escape is a shape that can reach
		// here. The timestamp is the host's own and may be absent, which
		// is what "-" says.
		_, _ = fmt.Fprintf(os.Stdout, "%s  %-11s  %s\n%.64q\n%q\n%q\n\n",
			memoryStamp(h.HostModifiedMS), h.Host, h.Kind, h.ID, h.Title, h.Excerpt)
	}
	return 0
}

// memoryStamp is [stamp] for a host timestamp that may not exist. Zero means the
// host wrote none - 1 of the 18 Claude Code notes read on 2026-09-02 carries no
// modified key - and printing the epoch for it would be a date nobody wrote.
func memoryStamp(ms int64) string {
	if ms == 0 {
		return strings.Repeat(" ", len(stamp(0)))
	}
	return stamp(ms)
}

// displayNameRunes is how much of an event name one line of this program's
// output shows. It is a width and not the wire's bound: the service bounds a
// name at internal/service's maxEventNameRunes and says so on the hit, and
// this program cuts again for the terminal, so a name has two places it can
// have been shortened and [cutName] marks both the same way.
const displayNameRunes = 64

// cutName quotes name for one line, cut to at most [displayNameRunes] runes,
// and puts a mark after the closing quote when either this program or the
// service (cutByService, the hit's own flag) shortened it (backlog 17). The
// mark sits outside the quotes so that it cannot be read as the name's last
// character, which is what a marker inside the string would be.
func cutName(name string, cutByService bool) string {
	shown, cut := name, cutByService
	if r := []rune(name); len(r) > displayNameRunes {
		shown, cut = string(r[:displayNameRunes]), true
	}
	if cut {
		return fmt.Sprintf("%q…", shown)
	}
	return fmt.Sprintf("%q", shown)
}

// projectFlag is the one argument [search] interprets.
const projectFlag = "--project"

// searchScope splits args into the project to scope to and the words to search
// for.
//
// The flag is read only in first position, and only in its two-argument form.
// `--project=<path>` is not accepted, because accepting one spelling of a flag
// and not the other is a smaller surprise than an argument parser this binary
// does not otherwise have - main's whole argument handling is one switch in
// front of the relay path.
//
// A relative path is made absolute here rather than sent as typed. The service
// refuses a relative path outright, which is the only correct thing it can do
// with one: it would otherwise resolve against a long-lived process Task
// Scheduler started, in a directory that has nothing to do with the question.
func searchScope(args []string) (project string, words []string, err error) {
	if len(args) == 0 || args[0] != projectFlag {
		return "", args, nil
	}
	if len(args) < 2 {
		return "", nil, fmt.Errorf("%s needs a path", projectFlag)
	}
	abs, err := filepath.Abs(args[1])
	if err != nil {
		return "", nil, fmt.Errorf("resolve %.128q: %w", args[1], err)
	}
	return abs, args[2:], nil
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
func askSearch(req ipc.SearchRequest) (ipc.SearchReply, error) {
	var zero ipc.SearchReply

	payload, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("encode the request: %w", err)
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
		return zero, replied(err, raw)
	}
	return reply, nil
}
