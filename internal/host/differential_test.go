package host

import (
	"encoding/json/jsontext"
	"os"
	"path/filepath"
	"testing"
)

// TestTheGoMergeMatchesTheScriptByteForByte is the oracle for the port, and it
// is transitional: it exists so the Node installer can be deleted on evidence
// rather than on confidence, and it goes with the script.
//
// # Why the input is empty and not interesting
//
// The two implementations are deliberately NOT byte-equal in general. The
// script round-trips through JSON.parse and JSON.stringify, which normalises
// string escapes and number spellings; [MergeHooks] copies what it is not
// changing as raw bytes and so preserves both. On a document carrying an escape
// or a trailing zero they differ **by design**, and a test asserting equality
// there would be asserting the port failed.
//
// So the comparison is over a document that has nothing to normalise. What that
// leaves is exactly what the port could get wrong silently: the event set and
// its order, each host's hook object and its member order, which events carry a
// matcher, the per-event Codex timeout, the indentation, and the trailing
// newline. Fidelity has its own tests and is not this test's business.
func TestTheGoMergeMatchesTheScriptByteForByte(t *testing.T) {
	node, script, tmp := installerTree(t)

	codexPath := filepath.Join(tmp, "hooks.json")
	claudePath := filepath.Join(tmp, "settings.json")
	seed(t, codexPath, map[string]any{})
	seed(t, claudePath, map[string]any{})

	// An empty PATH, for the reason runInstallerWithPath documents: with one,
	// the MCP half would register a temporary directory's endpoint with the
	// developer's own Claude Code.
	out, err := runInstallerWithPath(t, node, script, tmp, codexPath, claudePath, "", "--apply")
	if err != nil {
		t.Fatalf("install-hooks.mjs --apply: %v\n%s", err, out)
	}

	// The relay path the script wrote is the one it derives from the
	// LOCALAPPDATA it was handed, so the Go side has to be given the same one
	// or the comparison is about a path and not about a merge.
	relay := filepath.Join(installerBin(tmp), "engramux.exe")

	for _, host := range []struct {
		name  string
		path  string
		entry func(string, string) jsontext.Value
	}{
		{"claude-code", claudePath, ClaudeEntry},
		{"codex", codexPath, CodexEntry},
	} {
		t.Run(host.name, func(t *testing.T) {
			want, err := os.ReadFile(host.path)
			if err != nil {
				t.Fatalf("read what the script wrote: %v", err)
			}
			got, err := MergeHooks([]byte("{}\n"), EventNames(),
				func(event string) jsontext.Value { return host.entry(event, relay) })
			if err != nil {
				t.Fatalf("MergeHooks: %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("the Go merge and the script disagree on a document with nothing to "+
					"normalise, so the disagreement is structural\n--- go ---\n%s\n--- script ---\n%s",
					got, want)
			}
		})
	}
}
