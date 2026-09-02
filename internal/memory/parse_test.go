package memory

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Every fixture below is synthesised, on spec 6.2's rule and on the rule the
// memory spec's M-2 reading is under: the two hosts' memory files are the
// owner's private notes, so what this package is tested against is their
// *shape*, written here, and never a copy of one. The shapes are the ones read
// on 2026-09-02 and recorded in that section - the frontmatter keys and their
// counts, the index entry's form, the section delimiter, the extended-length
// path prefix.

// write puts one fixture on disk and returns the [Source] a walk would have
// produced for it, so that a parser test says what it is about rather than
// repeating a directory layout.
func write(t *testing.T, dir, name, body string, s Source) Source {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
	s.Path = path
	return s
}

const claudeNote = `---
name: the pipe name moves with the SID
description: what the override is for, and what it does not isolate
metadata:
  node_type: memory
  type: project
  originSessionId: 0198f0c1-1111-7222-8333-444444444444
  modified: 2026-08-04T14:33:05.123Z
---

The listener name is derived from the user SID, so redirecting LOCALAPPDATA
isolates nothing.
`

// TestAClaudeNoteKeepsItsValuesAndDropsItsKeys. The values are what a person
// searches for and the keys are what spec 5.7 measured the cost of indexing: a
// key is a token of every document, and `cwd` matched 900 of 901 that way. So
// `description` must not be findable and the description itself must be.
func TestAClaudeNoteKeepsItsValuesAndDropsItsKeys(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "abc12-pipe-name.md", claudeNote,
		Source{Host: HostClaude, Kind: KindClaudeNote, ProjectPath: "d--work-engramux"})

	items, warns, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v, want none for a note whose keys are all known", warns)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	got := items[0]
	for _, key := range []string{"description", "node_type", "originSessionId", "metadata"} {
		if strings.Contains(got.Body, key) {
			t.Errorf("the body carries the key %q; keys are dropped and values are kept", key)
		}
	}
	for _, value := range []string{"what the override is for", "redirecting LOCALAPPDATA"} {
		if !strings.Contains(got.Body, value) {
			t.Errorf("the body does not carry %q; the value must survive", value)
		}
	}
	if got.Title != "the pipe name moves with the SID" {
		t.Errorf("title = %q, want the name value", got.Title)
	}
	if got.Kind != KindClaudeNote+":project" {
		t.Errorf("kind = %q, want the note kind carrying the type value", got.Kind)
	}
	// 2026-08-04T14:33:05.123Z
	if want := int64(1785853985123); got.HostModifiedMS != want {
		t.Errorf("HostModifiedMS = %d, want %d", got.HostModifiedMS, want)
	}
	if got.ProjectPath != "d--work-engramux" {
		t.Errorf("ProjectPath = %q, want the directory key the walk carried", got.ProjectPath)
	}
}

// TestANoteWithNoModifiedKeepsNoHostTimestamp is not a defensive test. One of
// the 18 notes read on 2026-09-02 carries no `modified` key, and that field is
// what spec 3's P3 compares against - so a parser that requires it fails on real
// data, and a zero has to mean "the host wrote none" rather than the epoch.
func TestANoteWithNoModifiedKeepsNoHostTimestamp(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "no-modified.md", `---
name: a note the host never stamped
description: one of eighteen
metadata:
  node_type: memory
  type: user
  originSessionId: 0198f0c1-1111-7222-8333-444444444444
---
body
`, Source{Host: HostClaude, Kind: KindClaudeNote})

	items, warns, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v, want none: a missing modified is an ordinary note", warns)
	}
	if items[0].HostModifiedMS != 0 {
		t.Fatalf("HostModifiedMS = %d, want 0", items[0].HostModifiedMS)
	}
	if !strings.Contains(items[0].Body, "one of eighteen") {
		t.Fatalf("the body lost the description")
	}
}

// TestAnUnknownFrontmatterKeyWarnsAndItsValueIsStillIndexed is M2's first
// clause, and both halves matter: the warning is what makes drift visible, and
// the value surviving is what makes the reader a partition rather than a filter.
func TestAnUnknownFrontmatterKeyWarnsAndItsValueIsStillIndexed(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "drifted.md", `---
name: a note from a newer host
description: it grew a key
metadata:
  node_type: memory
  type: project
  originSessionId: 0198f0c1-1111-7222-8333-444444444444
  confidence: highly-plausible
---
body
`, Source{Host: HostClaude, Kind: KindClaudeNote})

	items, warns, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Reason, "confidence") {
		t.Fatalf("warnings = %v, want exactly one naming the unknown key", warns)
	}
	if !strings.Contains(items[0].Body, "highly-plausible") {
		t.Fatalf("the unknown key's value was dropped; a silent skip is the failure M2 is shaped against")
	}
}

// TestANoteWithNoFrontmatterIsOneWholeDocument. A note that lost its block is a
// format change, and the reader has to survive it as a document rather than as a
// crash or as nothing.
func TestANoteWithNoFrontmatterIsOneWholeDocument(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "bare.md", "# just a heading\n\nand a body\n",
		Source{Host: HostClaude, Kind: KindClaudeNote})

	items, warns, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Reason, "no frontmatter") {
		t.Fatalf("warnings = %v, want one about the missing block", warns)
	}
	if items[0].Title != "just a heading" {
		t.Fatalf("title = %q, want the first line with its heading mark off", items[0].Title)
	}
	if !strings.Contains(items[0].Body, "and a body") {
		t.Fatalf("the body was lost")
	}
}

// TestAClaudeIndexSplitsAtItsEntriesAndKeepsWhatIsNotOne. The index's own
// descriptions are not the notes' - measured at a median similarity of 0.14 over
// 18 entries on 2026-09-02, identical on none - so each entry is a document. A
// bullet with no link is the drift already in that corpus, and it has to warn
// and still be indexed.
func TestAClaudeIndexSplitsAtItsEntriesAndKeepsWhatIsNotOne(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "MEMORY.md", `# Project memory

- [the pipe name moves with the SID](abc12-pipe-name.md) — the override and what it does not isolate
- [what the soak measured](def34-soak.md) — nine refusals, one read
- **a standing note with no target at all**
`, Source{Host: HostClaude, Kind: KindClaudeIndex, ProjectPath: "d--work-engramux"})

	items, warns, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var keys []string
	for _, it := range items {
		keys = append(keys, it.EntryKey)
	}
	slices.Sort(keys)
	if want := []string{"", "abc12-pipe-name.md", "def34-soak.md"}; !slices.Equal(keys, want) {
		t.Fatalf("entry keys = %v, want %v - two entries and one leading block", keys, want)
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Reason, "no link") {
		t.Fatalf("warnings = %v, want one about the bullet with no link", warns)
	}
	var leading Item
	for _, it := range items {
		if it.EntryKey == "" {
			leading = it
		}
	}
	if !strings.Contains(leading.Body, "a standing note with no target") {
		t.Fatalf("the unlinked bullet was dropped; the parse is a partition and never a filter")
	}
	for _, it := range items {
		if it.EntryKey != "" && !strings.Contains(it.Body, "the override") && !strings.Contains(it.Body, "nine refusals") {
			t.Fatalf("entry %q carries neither description; the index's own text is what makes it a document", it.EntryKey)
		}
	}
}

const codexRaw = `# Raw memories

## thread ` + "`0198f0c1-1111-7222-8333-444444444444`" + `
updated_at: 2026-08-17T10:49:00+09:00
cwd: \\?\D:\work\Engramux
rollout_path: D:\rollouts\one.jsonl
task: close the soak
task_outcome: closed at 72h5m
description: the covering index came out of it

## thread ` + "`0198f0c1-2222-7222-8333-444444444444`" + `
updated_at: 2026-08-17T10:49:00+09:00
cwd: D:\work\other
task: something else
`

// TestACodexArtefactSplitsAtItsSectionHeadings. The second-level heading is the
// delimiter every Codex artefact uses - 55 thread sections in the raw-memories
// file, 46 in the index - and it is what keeps a 291,566 B file from being one
// document.
func TestACodexArtefactSplitsAtItsSectionHeadings(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "raw_memories.md", codexRaw, Source{Host: HostCodex, Kind: KindCodexRaw})

	items, _, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3: the leading block and two sections", len(items))
	}
	var withKey int
	for _, it := range items {
		if it.EntryKey != "" {
			withKey++
		}
		if strings.Contains(it.Body, "updated_at") || strings.Contains(it.Body, "rollout_path") {
			t.Errorf("item %q carries a field label; the labels are dropped and the values kept", it.EntryKey)
		}
	}
	if withKey != 2 {
		t.Fatalf("sections with a key = %d, want 2", withKey)
	}
	if !strings.Contains(items[1].Body, "the covering index came out of it") {
		t.Fatalf("a field value was dropped")
	}
	// 2026-08-17T10:49:00+09:00
	if want := int64(1786931340000); items[1].HostModifiedMS != want {
		t.Errorf("HostModifiedMS = %d, want %d", items[1].HostModifiedMS, want)
	}
}

// TestACodexPathIsNormalisedPastTheExtendedLengthPrefix. Of 55 path lines in one
// artefact read on 2026-09-02, 39 carried the extended-length prefix and 16 did
// not, so a comparison that does not fold it misses 39 of its own corpus. Case
// is folded for the same reason internal/project folds it when it derives a
// root: the two strings are written by different programs.
func TestACodexPathIsNormalisedPastTheExtendedLengthPrefix(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "raw_memories.md", codexRaw, Source{Host: HostCodex, Kind: KindCodexRaw})

	items, _, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := items[1].ProjectPath; got != `d:\work\engramux` {
		t.Fatalf("ProjectPath = %q, want the prefix stripped and the case folded", got)
	}
	if got := items[2].ProjectPath; got != `d:\work\other` {
		t.Fatalf("ProjectPath = %q, want a plain path to survive unchanged but for its case", got)
	}
}

// TestProjectKeysAsksAboutBothFormsAProjectCanBeFiledUnder. The two hosts
// identify a project differently and neither can be converted into the other
// without the filesystem, so a scoped query asks about both. Encoding the
// question is what keeps this off the disk.
func TestProjectKeysAsksAboutBothFormsAProjectCanBeFiledUnder(t *testing.T) {
	got := ProjectKeys(`D:\AI_DEV\engramux`)
	want := []string{`d:\ai_dev\engramux`, "d--ai-dev-engramux"}
	if !slices.Equal(got, want) {
		t.Fatalf("ProjectKeys = %q, want %q", got, want)
	}
	if ProjectKeys("") != nil {
		t.Fatalf("ProjectKeys(\"\") = %q, want nil: an unscoped query asks about no project", ProjectKeys(""))
	}
}

// TestSourcesFindsBothHostsAndSaysWhatItCouldNotClaim. Everything M2 asks a
// walk to notice, in one layout: a memory directory beside the transcripts it
// must not read, a .git directory that is documented rather than unrecognised,
// and a file whose name no parser knows.
func TestSourcesFindsBothHostsAndSaysWhatItCouldNotClaim(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()

	write(t, claude, filepath.Join("projects", "d--work-engramux", "memory", "MEMORY.md"), "# index\n", Source{})
	write(t, claude, filepath.Join("projects", "d--work-engramux", "memory", "abc12-note.md"), claudeNote, Source{})
	write(t, claude, filepath.Join("projects", "d--work-engramux", "memory", "notes.txt"), "not markdown", Source{})
	// A transcript beside the memory directory rather than inside it: the walk
	// must not reach it, which is why only */memory is opened.
	write(t, claude, filepath.Join("projects", "d--work-engramux", "0198f0c1.jsonl"), "{}\n", Source{})
	write(t, claude, filepath.Join("projects", "d--no-memory-here", "0198f0c2.jsonl"), "{}\n", Source{})

	write(t, codex, filepath.Join("memories", "MEMORY.md"), "# index\n", Source{})
	write(t, codex, filepath.Join("memories", "raw_memories.md"), codexRaw, Source{})
	write(t, codex, filepath.Join("memories", "rollout_summaries", "2026-08-17T10-49-00-a_b.md"), "# one\n", Source{})
	write(t, codex, filepath.Join("memories", "notes_from_a_newer_host.md"), "# who knows\n", Source{})
	write(t, codex, filepath.Join("memories", ".git", "objects", "ab", "cdef.md"), "not a memory\n", Source{})

	sources, warns := Sources(claude, codex)

	var paths []string
	for _, s := range sources {
		paths = append(paths, filepath.Base(s.Path))
	}
	slices.Sort(paths)
	want := []string{
		"2026-08-17T10-49-00-a_b.md", "MEMORY.md", "MEMORY.md",
		"abc12-note.md", "notes_from_a_newer_host.md", "raw_memories.md",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("sources = %v, want %v", paths, want)
	}
	for _, s := range sources {
		if strings.Contains(s.Path, ".git") {
			t.Errorf("the walk reached the git directory: %s", filepath.Base(filepath.Dir(s.Path)))
		}
		if strings.HasSuffix(s.Path, ".jsonl") {
			t.Errorf("the walk reached a transcript: %s", filepath.Base(s.Path))
		}
		if s.ModTimeUnixMS == 0 || s.Size == 0 {
			t.Errorf("%s carries no mtime/size pair, which is the whole short-circuit", filepath.Base(s.Path))
		}
	}
	var reasons []string
	for _, w := range warns {
		reasons = append(reasons, w.Reason)
	}
	if len(warns) != 2 {
		t.Fatalf("warnings = %v, want two: the .txt and the unknown .md name", reasons)
	}
}

// TestSourcesWarnsWhenTheIndexIsMissing is M2 by name: "a missing index" is one
// of the three shapes that must warn and continue, and continuing means the
// files beside it are still listed.
func TestSourcesWarnsWhenTheIndexIsMissing(t *testing.T) {
	codex := t.TempDir()
	write(t, codex, filepath.Join("memories", "raw_memories.md"), codexRaw, Source{})

	sources, warns := Sources("", codex)
	if len(sources) != 1 {
		t.Fatalf("sources = %d, want the file beside the missing index to still be listed", len(sources))
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Reason, "no index") {
		t.Fatalf("warnings = %v, want one naming the missing index", warns)
	}
}

// TestAnAbsentHomeIsNotAWarning. A machine with one host installed, or one where
// the feature was never switched on, is ordinary - and on the machine this was
// written for, both are switched off. A reader that warned here would warn on
// every tick forever.
func TestAnAbsentHomeIsNotAWarning(t *testing.T) {
	sources, warns := Sources(filepath.Join(t.TempDir(), "nothing"), filepath.Join(t.TempDir(), "nothing"))
	if len(sources) != 0 || len(warns) != 0 {
		t.Fatalf("sources = %d, warnings = %v, want none of either", len(sources), warns)
	}
}

// TestNothingHereWrites. M-2 is read-only and never qualified, and the
// directories this walks are two other programs' working state. Asserting it
// rather than commenting it: the whole tree's modification times and sizes are
// taken before and after a full read.
func TestNothingHereWrites(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	write(t, claude, filepath.Join("projects", "d--work-engramux", "memory", "MEMORY.md"), "# index\n", Source{})
	write(t, claude, filepath.Join("projects", "d--work-engramux", "memory", "abc12-note.md"), claudeNote, Source{})
	write(t, codex, filepath.Join("memories", "MEMORY.md"), "# index\n", Source{})
	write(t, codex, filepath.Join("memories", "raw_memories.md"), codexRaw, Source{})

	before := treeState(t, claude, codex)
	sources, _ := Sources(claude, codex)
	if len(sources) == 0 {
		t.Fatal("no sources, so the read this test is about did not happen")
	}
	for _, s := range sources {
		if _, _, err := Parse(s); err != nil {
			t.Fatalf("Parse %s: %v", filepath.Base(s.Path), err)
		}
	}
	if after := treeState(t, claude, codex); !slices.Equal(before, after) {
		t.Fatalf("the tree changed across a full read:\nbefore %v\nafter  %v", before, after)
	}
}

// treeState is every path under the given roots with its size, which is what a
// write of any kind would move. Modification time is deliberately not in it:
// reading a file updates the last-access time on some volumes and this test is
// about writes.
func treeState(t *testing.T, roots ...string) []string {
	t.Helper()
	var out []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if d.IsDir() {
				out = append(out, rel+"/")
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			out = append(out, rel+":"+strconv.FormatInt(info.Size(), 10))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	slices.Sort(out)
	return out
}
