package memory

import (
	"os"
	"strings"
	"testing"
)

// Gates M1 and M2 of the memory spec's section 5.
//
// # Nothing here prints a path or a line of a memory file
//
// These are the owner's private notes. A failure reports the index of a source
// and the number of a line and nothing else - not the file's name, which for a
// Claude Code note is a slug of its own title, and not the text, which is the
// whole of what must not leave the machine. That is the same rule the memory
// spec's M-2 reading is under, and it is why this file's failure messages read
// as awkwardly as they do.
//
// M1 runs over whatever is on this machine and skips when there is nothing,
// which is the shape TestPhase4Gate's corpus mode already has. On the machine it
// was written on, both hosts' memory is switched off and both directories are
// frozen snapshots - which costs M1 nothing, because M1 is about files that
// exist rather than about files that are still being written.

// TestGateM1EveryNativeMemoryFileParsesAndKeepsItsText is M1: over every native
// memory file present, no crash, the fields extracted where they exist, and no
// text silently dropped.
//
// "No text silently dropped" is asserted as a partition rather than as a byte
// comparison, because the parsers deliberately drop one thing - a field's key
// name, on spec 5.7's rule about `cwd` matching 900 of 901 documents. So the
// check is that every line's *value* lands in some item's body, which fails on a
// parser that skipped a block and passes on one that dropped a label.
func TestGateM1EveryNativeMemoryFileParsesAndKeepsItsText(t *testing.T) {
	sources, _ := Sources(ClaudeHome(), CodexHome())
	if len(sources) == 0 {
		t.Skip("no native memory on this machine; M1 is over the files that exist")
	}

	var items, files, withStamp, withProject int
	byHost := map[string]int{}
	for i, s := range sources {
		got, _, err := Parse(s)
		if err != nil {
			t.Fatalf("source %d (%s, %s): Parse: %v", i, s.Host, s.Kind, err)
		}
		files++
		items += len(got)
		byHost[s.Host] += len(got)
		for _, it := range got {
			if it.HostModifiedMS != 0 {
				withStamp++
			}
			if it.ProjectPath != "" {
				withProject++
			}
			if it.Host != s.Host {
				t.Errorf("source %d: item host = %q, want %q", i, it.Host, s.Host)
			}
			if it.SourcePath != s.Path {
				t.Errorf("source %d: an item does not carry the file it came from", i)
			}
		}
		if line, ok := missingValue(t, s, got); !ok {
			t.Fatalf("source %d (%s, %s): line %d has a value that reached no item's body", i, s.Host, s.Kind, line)
		}
	}
	t.Logf("M1: %d files, %d items (claude-code %d, codex %d), %d with a host timestamp, %d with a project",
		files, items, byHost[HostClaude], byHost[HostCodex], withStamp, withProject)
}

// missingValue is M1's lossless clause. It returns the 1-based number of the
// first line whose value is in no item, and never the line itself.
func missingValue(t *testing.T, s Source, items []Item) (int, bool) {
	t.Helper()
	b, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatalf("read a source: %v", err)
	}
	bodies := make([]string, len(items))
	for i, it := range items {
		bodies[i] = it.Body
	}
	for n, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		value := lineValue(s, line)
		if value == "" {
			continue
		}
		found := false
		for _, body := range bodies {
			if strings.Contains(body, value) {
				found = true
				break
			}
		}
		if !found {
			return n + 1, false
		}
	}
	return 0, true
}

// lineValue is what a line contributes to an item's body: the line with a label
// this reader recognises taken off, and "" for a line that is only a label, only
// a fence, or empty.
func lineValue(s Source, line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed == "---" {
		return ""
	}
	if s.Kind == KindClaudeIndex {
		// An index entry becomes its link text and its description; the two
		// are separated in the item, so the line as a whole is in no body and
		// its halves are checked instead.
		if m := claudeIndexEntry.FindStringSubmatch(line); m != nil {
			return strings.TrimSpace(m[3])
		}
		return trimmed
	}
	if s.Kind == KindClaudeNote {
		if m := yamlKey.FindStringSubmatch(line); m != nil {
			if m[2] == "modified" {
				return "" // the host timestamp is a column, not indexed text
			}
			return strings.TrimSpace(strings.Trim(m[3], `"'`))
		}
		return trimmed
	}
	if strings.HasPrefix(line, "## ") {
		// A Codex section heading becomes the item's key rather than part of
		// its body, which is the split itself.
		return ""
	}
	if m := codexFieldLine.FindStringSubmatch(line); m != nil {
		if m[1] == "updated_at" {
			return "" // as above, and on both hosts for the same reason
		}
		return strings.TrimSpace(m[2])
	}
	return trimmed
}

// TestGateM2DriftWarnsAndContinues is M2, all three of its shapes, each asserted
// on both halves: something came back as a warning, and nothing was lost.
//
// It runs over synthesised files rather than the machine's, and that is the
// point of it - a drift canary that only fires on drift somebody already has is
// a canary that reports the past. The corpus's own drift is what M1 above walks.
func TestGateM2DriftWarnsAndContinues(t *testing.T) {
	t.Run("an unknown frontmatter key", func(t *testing.T) {
		dir := t.TempDir()
		src := write(t, dir, "n.md", `---
name: n
description: d
metadata:
  node_type: memory
  type: project
  arrivedInAVersionNobodyHasSeen: a value that must survive
---
body
`, Source{Host: HostClaude, Kind: KindClaudeNote})
		items, warns, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		requireWarned(t, warns, "arrivedInAVersionNobodyHasSeen")
		requireKept(t, items, "a value that must survive")
	})

	t.Run("an unknown file name", func(t *testing.T) {
		home := t.TempDir()
		write(t, home, "memories/MEMORY.md", "# index\n", Source{})
		write(t, home, "memories/an_artefact_from_a_newer_host.md", "## s\nsomething only this file says\n", Source{})
		sources, warns := Sources("", home)
		requireWarned(t, warns, "no parser claims")
		var all []Item
		for _, s := range sources {
			got, _, err := Parse(s)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			all = append(all, got...)
		}
		requireKept(t, all, "something only this file says")
	})

	t.Run("a missing index", func(t *testing.T) {
		home := t.TempDir()
		write(t, home, "memories/raw_memories.md", "## s\nstill has to be read\n", Source{})
		sources, warns := Sources("", home)
		requireWarned(t, warns, "no index")
		if len(sources) != 1 {
			t.Fatalf("sources = %d, want the files beside the missing index to still be listed", len(sources))
		}
		items, _, err := Parse(sources[0])
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		requireKept(t, items, "still has to be read")
	})
}

func requireWarned(t *testing.T, warns []Warning, substring string) {
	t.Helper()
	for _, w := range warns {
		if strings.Contains(w.Reason, substring) {
			return
		}
	}
	var reasons []string
	for _, w := range warns {
		reasons = append(reasons, w.Reason)
	}
	t.Fatalf("no warning mentions %q; a silent skip is the failure M2 is shaped against. got %v", substring, reasons)
}

func requireKept(t *testing.T, items []Item, substring string) {
	t.Helper()
	for _, it := range items {
		if strings.Contains(it.Body, substring) {
			return
		}
	}
	t.Fatalf("no item carries the text the drift arrived with; warning about a shape and then dropping it is still dropping it")
}
