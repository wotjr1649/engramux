package memory

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ClaudeHome is where Claude Code keeps its configuration, and therefore where
// its memory is: the memory of a project lives at
// <home>/projects/<project key>/memory.
//
// It is resolved and never hardcoded, and the mechanism is the environment
// rather than a setting. The published settings schema carries exactly one
// memory property, the boolean that turns the feature on, and no property names
// a path; what moves is the whole configuration home, through
// CLAUDE_CONFIG_DIR, whose documented default is ~/.claude. Memory spec rev.2's
// M-2 reading is where that was measured, and it corrected an earlier line in
// that same section which said the location came from settings.
func ClaudeHome() string {
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// CodexHome is where Codex keeps its configuration, and its memory directory is
// a fixed name below it. CODEX_HOME is the override; ~/.codex is the default.
func CodexHome() string {
	if dir := strings.TrimSpace(os.Getenv("CODEX_HOME")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// Sources lists every memory file under the two homes without opening one.
//
// An absent home is not a warning and not an error: a machine with one host
// installed is an ordinary machine, and so is one where the feature has never
// been switched on. What *is* a warning is a directory that exists and cannot be
// read, and a file inside one whose name no parser claims - M2's "an unknown
// file name warns and continues", and the reason this returns the warnings
// rather than logging them is that the gate has to be able to count them.
//
// Either home may be "", which skips that host.
func Sources(claudeHome, codexHome string) ([]Source, []Warning) {
	var (
		out   []Source
		warns []Warning
	)
	if claudeHome != "" {
		s, w := claudeSources(claudeHome)
		out, warns = append(out, s...), append(warns, w...)
	}
	if codexHome != "" {
		s, w := codexSources(codexHome)
		out, warns = append(out, s...), append(warns, w...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, warns
}

// claudeSources walks <home>/projects/*/memory. A project directory holds the
// session transcripts too, which is why only the memory subdirectory is opened:
// on the machine this was measured, that parent held 3,823 files and its
// contents change continuously, and none of them is memory.
func claudeSources(home string) ([]Source, []Warning) {
	projects := filepath.Join(home, "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Warning{{Path: projects, Reason: "the project directory could not be read: " + err.Error()}}
	}
	var (
		out   []Source
		warns []Warning
	)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(projects, e.Name(), "memory")
		files, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				warns = append(warns, Warning{Path: dir, Reason: "the memory directory could not be read: " + err.Error()})
			}
			continue
		}
		for _, f := range files {
			s, w, ok := claudeSource(dir, e.Name(), f)
			if w != nil {
				warns = append(warns, *w)
			}
			if ok {
				out = append(out, s)
			}
		}
	}
	return out, warns
}

// claudeSource classifies one entry of a memory directory.
//
// The project key is the directory name as Claude Code wrote it, kept rather
// than decoded. Decoding is lossy and needs the filesystem: the key folds a
// drive colon and every separator to a hyphen, so recovering a path means
// walking candidates, and a project that has moved cannot be walked at all. What
// a scoped query does instead is encode the project it is asking about the same
// way - see [ProjectKeys].
func claudeSource(dir, projectKey string, f fs.DirEntry) (Source, *Warning, bool) {
	path := filepath.Join(dir, f.Name())
	if f.IsDir() {
		return Source{}, &Warning{Path: path, Reason: "a directory inside a memory directory, which no parser claims"}, false
	}
	if !strings.EqualFold(filepath.Ext(f.Name()), ".md") {
		return Source{}, &Warning{Path: path, Reason: "a file that is not .md, which no parser claims"}, false
	}
	info, err := f.Info()
	if err != nil {
		return Source{}, &Warning{Path: path, Reason: "the file could not be stat-ed: " + err.Error()}, false
	}
	kind := KindClaudeNote
	if f.Name() == "MEMORY.md" {
		kind = KindClaudeIndex
	}
	return Source{
		Path:          path,
		Host:          HostClaude,
		Kind:          kind,
		ProjectPath:   strings.ToLower(projectKey),
		ModTimeUnixMS: info.ModTime().UnixMilli(),
		Size:          info.Size(),
	}, nil, true
}

// codexSources walks <home>/memories. The .git directory is skipped by name and
// not by a warning: the directory being a git repository is documented behaviour
// of the host, so its internals are not an unrecognised shape.
//
// The index being absent *is* a warning, because M2 names it: a memory directory
// with files and no index is a shape that has to be reported rather than
// quietly read as "no index needed".
func codexSources(home string) ([]Source, []Warning) {
	root := filepath.Join(home, "memories")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Warning{{Path: root, Reason: "the memory directory could not be read: " + err.Error()}}
	}
	var (
		out      []Source
		warns    []Warning
		sawIndex bool
		sawAnyMD bool
	)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			warns = append(warns, Warning{Path: path, Reason: "walking stopped here: " + err.Error()})
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		sawAnyMD = true
		kind, known := codexKind(root, path)
		if !known {
			warns = append(warns, Warning{Path: path, Reason: "an .md file in the memory directory whose name no parser claims; read as one whole document"})
		}
		if kind == KindCodexIndex {
			sawIndex = true
		}
		info, err := d.Info()
		if err != nil {
			warns = append(warns, Warning{Path: path, Reason: "the file could not be stat-ed: " + err.Error()})
			return nil
		}
		out = append(out, Source{
			Path:          path,
			Host:          HostCodex,
			Kind:          kind,
			ModTimeUnixMS: info.ModTime().UnixMilli(),
			Size:          info.Size(),
		})
		return nil
	})
	if err != nil {
		warns = append(warns, Warning{Path: root, Reason: "the memory directory could not be walked: " + err.Error()})
	}
	if sawAnyMD && !sawIndex {
		warns = append(warns, Warning{Path: filepath.Join(root, "MEMORY.md"), Reason: "the memory directory has files and no index; the files are read without it"})
	}
	return out, warns
}

// codexKind names the parser for a file, and says whether the name was one it
// knows. An unknown .md is read as one whole document rather than skipped, which
// is what makes the warning beside it a report and not a refusal.
func codexKind(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return KindCodexRollout, false
	}
	switch {
	case rel == "MEMORY.md":
		return KindCodexIndex, true
	case rel == "raw_memories.md":
		return KindCodexRaw, true
	case rel == "memory_summary.md":
		return KindCodexDigest, true
	case filepath.Dir(rel) == "rollout_summaries":
		return KindCodexRollout, true
	}
	return KindCodexRollout, false
}

// ProjectKeys are the two strings a project's memory could be filed under, and a
// scoped query asks about both because the two hosts identify a project
// differently. root is the project root as internal/project normalised it.
//
// The first is the root itself, which is what Codex writes into a cwd field. The
// second is Claude Code's directory key for it: the drive colon and every
// separator folded to a hyphen. Encoding the question rather than decoding the
// answer is what keeps this off the filesystem - see [claudeSource].
func ProjectKeys(root string) []string {
	if root == "" {
		return nil
	}
	// Lowercase on both sides, because the two are written by different
	// programs and only one of them folds case: internal/project lowercases a
	// root when it derives one, and Claude Code writes its directory key with
	// the drive letter and path segments as the user typed them. Everything
	// this package stores in a project field is lowered on the way in, so the
	// comparison is between two lowered strings and never between one of each.
	root = strings.ToLower(root)
	var b strings.Builder
	for _, r := range root {
		switch r {
		case ':', '\\', '/', '_', '.':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	return []string{root, b.String()}
}
