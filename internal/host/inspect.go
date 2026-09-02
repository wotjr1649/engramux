package host

import (
	"encoding/json/jsontext"
	"errors"
	"strings"
)

// HookCommands returns, for each of events, the `command` of every Engramux
// hook the document holds under that event. An event with no Engramux hook is
// absent from the result, and so is an event the document does not mention.
//
// It is the read side of [MergeHooks] and shares its recognition rule
// ([isEngramux]), so what this finds is exactly what an install would replace.
// That sharing is the point: a second, stricter rule here would report a hook
// as missing that the installer would then decline to add, because it already
// found one.
//
// A shape this package does not understand is skipped rather than guessed at,
// the same way [rewriteEntries] preserves one: an event whose value is not an
// array, an entry with no `hooks` member, a hook with no string `command`. The
// caller reads that as "no Engramux hook here", which is the direction to be
// wrong in - it says re-run the installer, and the installer leaves the
// unrecognised shape alone.
func HookCommands(src []byte, events []string) (map[string][]string, error) {
	doc := jsontext.Value(src)
	if !doc.IsValid() {
		return nil, errors.New("host: the configuration file is not valid JSON")
	}
	table, ok, err := getMember(doc, hooksKey)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	out := make(map[string][]string, len(events))
	for _, event := range events {
		entries, ok, err := getMember(table, event)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if commands := commandsIn(entries); len(commands) > 0 {
			out[event] = commands
		}
	}
	return out, nil
}

// commandsIn walks one event's entry list and returns the command of every
// Engramux hook in it.
func commandsIn(entries jsontext.Value) []string {
	list, err := unmarshalValues(entries)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range list {
		inner, ok, err := getMember(entry, hooksKey)
		if err != nil || !ok {
			continue
		}
		hooks, err := unmarshalValues(inner)
		if err != nil {
			continue
		}
		for _, hook := range hooks {
			if !isEngramux(hook) {
				continue
			}
			command, ok, err := getMember(hook, "command")
			if err != nil || !ok || command.Kind() != '"' {
				continue
			}
			out = append(out, unquote(command))
		}
	}
	return out
}

// PointsAt reports whether a command [HookCommands] returned names relay.
//
// Three normalisations, each for a shape this package itself writes rather than
// for a shape it tolerates:
//
//   - [forwardSlashes] rewrites the separator on the way in, so the file holds
//     a path spelled differently from the one a caller computes with
//     filepath.Join.
//   - Codex takes its command as one string rather than an argument vector, so
//     [CodexEntry] quotes the path inside the value and the quotes come back
//     out with it.
//   - Windows paths are case-insensitive, and the two spellings that reach here
//     come from different places - one from an installer's filepath.Join, one
//     out of a file a person may have edited.
//
// It is not a general path comparison: no symlink resolution, no short-name
// expansion, no relative path. Both sides are absolute paths this product
// wrote, and a spelling neither of those produces reads as "points somewhere
// else", which sends the user to `install --apply` rather than to silence.
func PointsAt(command, relay string) bool {
	return normalizeCommand(command) == normalizeCommand(relay)
}

func normalizeCommand(v string) string {
	return strings.ToLower(forwardSlashes(strings.Trim(strings.TrimSpace(v), `"`)))
}
