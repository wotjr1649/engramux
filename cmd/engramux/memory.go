package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/wotjr1649/engramux/internal/ipc"
)

// showMemory is `engramux memory <id> [project]`: one native memory item in
// full, which is what a memory hit from `engramux search` is an excerpt of
// (memory spec rev.2, M-2 decision 9).
//
// The project is optional here where it is optional-but-usually-given for an
// event, and the difference is the data rather than the interface: Codex's
// memory is global and Claude Code files its own under a directory key that is
// not a path, so a large part of what this reads belongs to no project this
// database has a row for. On the machine this was measured, 155 of 303 items.
func showMemory(args []string) int {
	if len(args) == 0 || len(args) > 2 {
		warn("usage: engramux memory <id> [project]")
		return 2
	}
	var root string
	if len(args) == 2 {
		var err error
		root, err = projectArg(args[1:])
		if err != nil {
			warn("memory: %v", err)
			return 1
		}
	}

	reply, err := askGetMemory(ipc.GetMemoryRequest{ID: args[0], Project: root})
	if err != nil {
		warn("memory: %v", err)
		return 1
	}
	// Reached only after Verify, which is what makes a nil item mean "no such
	// item in that scope" rather than "the service refused".
	if reply.Item == nil {
		_, _ = fmt.Fprintln(os.Stdout, "no such memory item in this scope")
		return 1
	}
	printMemory(*reply.Item)
	return 0
}

// printMemory writes one memory item as a header block and its body.
//
// Every field but the host is quoted, and the rule is the schema rather than
// the field's name - the same rule [printEvent] follows. The host comes from
// which directory the file was found under and is one of two values this
// program knows; everything else came out of a file this program did not write,
// so a title or a key carrying a terminal escape is a shape that can reach here.
//
// The body is quoted too, and that is the difference from [printEvent]: an
// event's payload is JSON, which [json.Indent] will only accept if it carries
// no unescaped control byte, and a memory body is markdown with no such
// guarantee.
func printMemory(m ipc.MemoryDocument) {
	_, _ = fmt.Fprintf(os.Stdout,
		"id        %q\nhost      %-11s\nkind      %q\nsource    %q\nentry     %q\nproject   %q\ntitle     %q\nmodified  %s\nprivacy   %q\nbody      %d bytes\n",
		m.ID, m.Host, m.Kind, m.SourcePath, m.EntryKey, m.ProjectPath, m.Title,
		memoryStamp(m.HostModifiedMS), m.PrivacyClass, m.BodyBytes)

	if m.Body == "" && m.BodyBytes > 0 {
		_, _ = fmt.Fprintf(os.Stdout,
			"\nthe body is over the %d byte reply bound and was not sent\n", ipc.MaxMemoryBodyBytes)
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, "\n%q\n", m.Body)
}

// askGetMemory sends one [ipc.GetMemory] request and returns the verified reply.
// It is [askGetEvent] with a different document, on the same terms: the bound on
// the error's copy of the reply is there because those are bytes off the wire.
func askGetMemory(req ipc.GetMemoryRequest) (ipc.GetMemoryReply, error) {
	var zero ipc.GetMemoryReply

	payload, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("encode the request: %w", err)
	}
	raw, err := roundTrip(ipc.GetMemory, payload)
	if err != nil {
		return zero, err
	}

	var reply ipc.GetMemoryReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return zero, fmt.Errorf("decode the reply: %w", err)
	}
	if err := reply.Verify(); err != nil {
		return zero, replied(err, raw)
	}
	return reply, nil
}
