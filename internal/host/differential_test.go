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

// TestTheGoSpliceMatchesTheScriptByteForByte is the same oracle for the Codex
// MCP table, and it is transitional in the same way.
//
// Unlike the hook merge, this one has no fidelity difference to work around:
// both implementations splice lines and neither reformats what it keeps. So the
// comparison is over a file with comments, another product's tables, and a
// table of ours to replace - which is the shape that would catch a splice
// swallowing the table after it.
func TestTheGoSpliceMatchesTheScriptByteForByte(t *testing.T) {
	node, script, tmp := installerTree(t)

	// The script reads mcp.json out of the LOCALAPPDATA it is handed, and
	// spec 5.9 assigns that file to the service - so this seeds it the way a
	// service would have, and neither implementation writes it.
	local := filepath.Join(tmp, "local", "engramux")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ep := &Endpoint{URL: "http://127.0.0.1:8867/mcp", Token: "abcdef0123456789"}
	if err := os.WriteFile(filepath.Join(local, "mcp.json"),
		[]byte(`{"url":"`+ep.URL+`","token":"`+ep.Token+`"}`), 0o600); err != nil {
		t.Fatalf("seed mcp.json: %v", err)
	}

	// The script falls back to HOME/.codex/config.toml, which is inside tmp.
	codexDir := filepath.Join(tmp, "home", ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	config := filepath.Join(codexDir, "config.toml")
	const before = `# a comment the user wrote
model = "gpt-5.4"

[mcp_servers.engramux]
url = "http://127.0.0.1:1/mcp"
http_headers = { Authorization = "Bearer stale" }

[mcp_servers.other]
url = "http://127.0.0.1:2/mcp"
`
	if err := os.WriteFile(config, []byte(before), 0o600); err != nil {
		t.Fatalf("seed config.toml: %v", err)
	}

	codexPath := filepath.Join(tmp, "hooks.json")
	claudePath := filepath.Join(tmp, "settings.json")
	seed(t, codexPath, map[string]any{})
	seed(t, claudePath, map[string]any{})

	out, err := runInstallerWithPath(t, node, script, tmp, codexPath, claudePath, "", "--apply")
	if err != nil {
		t.Fatalf("install-hooks.mjs --apply: %v\n%s", err, out)
	}

	//nolint:gosec // G304: a path this test built under t.TempDir.
	want, err := os.ReadFile(config)
	if err != nil {
		t.Fatalf("read what the script wrote: %v", err)
	}
	got, err := SpliceCodex(before, ep)
	if err != nil {
		t.Fatalf("SpliceCodex: %v", err)
	}
	if got != string(want) {
		t.Errorf("the Go splice and the script disagree\n--- go ---\n%s\n--- script ---\n%s", got, want)
	}
}
