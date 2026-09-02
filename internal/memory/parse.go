package memory

import (
	"os"
	"regexp"
	"strings"
	"time"
)

// Parse reads one located file and splits it into items.
//
// The split is a partition: every line of the file lands in exactly one item, so
// nothing a parser fails to understand is lost. A file whose delimiters are not
// recognised at all comes back as a single item with an empty
// [Item.EntryKey], which is M2's warn-and-continue applied to the unit rather
// than only to the fields.
//
// A file that cannot be read is an error and not a warning: the caller decided
// to open it by listing it, and a read that fails is a fact about this machine
// rather than about the format.
func Parse(s Source) ([]Item, []Warning, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, nil, err
	}
	// Normalising line endings here rather than in each parser: a memory file
	// written by a Windows editor is the same document as one written by the
	// host, and every offset below is a line index.
	text := strings.ReplaceAll(string(b), "\r\n", "\n")

	switch s.Kind {
	case KindClaudeIndex:
		return parseClaudeIndex(s, text)
	case KindClaudeNote:
		return parseClaudeNote(s, text)
	default:
		return parseCodex(s, text)
	}
}

// claudeIndexEntry is one line of a Claude Code MEMORY.md: a markdown link to a
// note, then a dash, then a description that is not the note's own. Measured on
// 2026-09-02 over 18 entries: the index description and the note's frontmatter
// description are similar at a median ratio of 0.14 and identical on none, so
// the index carries text that exists nowhere else and is worth indexing rather
// than being navigation only.
var claudeIndexEntry = regexp.MustCompile(`^-\s+\[(.*?)\]\(([^)]*)\)\s*[\x{2014}\x{2013}-]?\s*(.*)$`)

// parseClaudeIndex splits the index at its entries. Anything that is not an
// entry - a heading, a bullet with no link - accumulates into the leading block,
// which is what keeps the partition total. A bullet that looks like an entry and
// is not gets a warning, because that is the drift M2 exists to see: one such
// line was already present in the corpus read on 2026-09-02.
func parseClaudeIndex(s Source, text string) ([]Item, []Warning, error) {
	var (
		items    []Item
		warns    []Warning
		preamble []string
	)
	for _, line := range strings.Split(text, "\n") {
		m := claudeIndexEntry.FindStringSubmatch(line)
		if m == nil {
			if strings.HasPrefix(strings.TrimSpace(line), "- ") {
				warns = append(warns, Warning{Path: s.Path, Reason: "an index bullet that carries no link; kept in the index's own document"})
			}
			preamble = append(preamble, line)
			continue
		}
		title, target, desc := strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), strings.TrimSpace(m[3])
		items = append(items, Item{
			Host:        s.Host,
			Kind:        KindClaudeIndex + "-entry",
			SourcePath:  s.Path,
			EntryKey:    target,
			ProjectPath: s.ProjectPath,
			Title:       title,
			Body:        strings.TrimSpace(title + "\n" + desc),
		})
	}
	if body := strings.TrimSpace(strings.Join(preamble, "\n")); body != "" {
		items = append(items, Item{
			Host:        s.Host,
			Kind:        KindClaudeIndex,
			SourcePath:  s.Path,
			ProjectPath: s.ProjectPath,
			Title:       firstLine(body),
			Body:        body,
		})
	}
	return items, warns, nil
}

// claudeKnownKeys are the frontmatter keys measured on 2026-09-02: name,
// description and metadata on 18 of 18 notes, and under metadata node_type, type
// and originSessionId on 18 and modified on 17. A key outside this set is a
// warning and its value is still indexed, which is M2's first clause.
var claudeKnownKeys = map[string]bool{
	"name": true, "description": true, "metadata": true,
	"node_type": true, "type": true, "originSessionId": true, "modified": true,
}

var yamlKey = regexp.MustCompile(`^(\s*)([A-Za-z0-9_-]+):\s*(.*)$`)

// parseClaudeNote reads a note: a YAML frontmatter block, then the body.
//
// The key names are dropped and the values kept, for the reason spec 5.7 gives
// about JSON keys - `description` on every note is the same defect `cwd` on
// every payload was, and that one matched 900 of 901 documents. What is not
// dropped is any value, including the value of a key nobody has seen before.
//
// A note with no frontmatter is not an error. It is one item whose body is the
// whole file, with a warning, because a note that lost its block is exactly the
// format change this reader must survive rather than crash on.
func parseClaudeNote(s Source, text string) ([]Item, []Warning, error) {
	item := Item{
		Host:        s.Host,
		Kind:        KindClaudeNote,
		SourcePath:  s.Path,
		ProjectPath: s.ProjectPath,
	}
	var warns []Warning

	front, body, ok := splitFrontmatter(text)
	if !ok {
		warns = append(warns, Warning{Path: s.Path, Reason: "a note with no frontmatter block; read as one whole document"})
		item.Title = firstLine(text)
		item.Body = strings.TrimSpace(text)
		return []Item{item}, warns, nil
	}

	var values []string
	for _, line := range strings.Split(front, "\n") {
		m := yamlKey.FindStringSubmatch(line)
		if m == nil {
			if strings.TrimSpace(line) != "" {
				values = append(values, strings.TrimSpace(line))
			}
			continue
		}
		key, value := m[2], strings.TrimSpace(strings.Trim(m[3], `"'`))
		if !claudeKnownKeys[key] {
			warns = append(warns, Warning{Path: s.Path, Reason: "a frontmatter key this reader does not know: " + key})
		}
		switch key {
		case "name":
			item.Title = value
		case "modified":
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				item.HostModifiedMS = t.UnixMilli()
			} else if value != "" {
				warns = append(warns, Warning{Path: s.Path, Reason: "a modified value that is not RFC 3339; the item keeps no host timestamp"})
			}
		case "type":
			item.Kind = KindClaudeNote + ":" + value
		}
		if value != "" && key != "modified" {
			values = append(values, value)
		}
	}
	item.Body = strings.TrimSpace(strings.Join(values, "\n") + "\n" + body)
	if item.Title == "" {
		item.Title = firstLine(item.Body)
	}
	return []Item{item}, warns, nil
}

// codexFieldLine is a lowercase snake_case label at the start of a line, which is
// how Codex writes every field it has: cwd, thread_id, updated_at, rollout_path,
// git_branch, task, task_group, task_outcome, description, keywords,
// rollout_summary_file. The label is dropped and the value kept, on spec 5.7's
// rule about keys.
//
// The bound is deliberate and is where the measurement stops. Codex also writes
// two capitalised prose labels on 55 of 55 rollout summaries, and those are left
// alone: dropping a capitalised label risks eating a sentence, and 55 documents
// out of about 200 is nothing like the 900 of 901 that made the rule.
var codexFieldLine = regexp.MustCompile(`^([a-z][a-z0-9_]*):\s*(.*)$`)

// parseCodex splits a Codex artefact at its second-level headings, which is the
// delimiter every one of its files uses: 55 thread sections in the raw-memories
// file, 46 in the index. A rollout summary has one and is therefore one item,
// which is the answer this parser should give for it anyway.
func parseCodex(s Source, text string) ([]Item, []Warning, error) {
	var (
		items []Item
		warns []Warning
		key   string
		block []string
	)
	flush := func() {
		body, mod, cwd, w := codexBlock(s, block)
		if body == "" {
			return
		}
		warns = append(warns, w...)
		items = append(items, Item{
			Host:           s.Host,
			Kind:           s.Kind,
			SourcePath:     s.Path,
			EntryKey:       key,
			ProjectPath:    cwd,
			Title:          firstLine(body),
			Body:           body,
			HostModifiedMS: mod,
		})
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			key, block = strings.TrimSpace(strings.TrimPrefix(line, "## ")), nil
			continue
		}
		block = append(block, line)
	}
	flush()
	if len(items) == 0 {
		warns = append(warns, Warning{Path: s.Path, Reason: "a Codex artefact with no section heading and no text; nothing was indexed"})
	}
	return items, warns, nil
}

// codexBlock turns one block's lines into its indexed text, its host timestamp
// and the project its fields name, dropping the field labels and keeping every
// value.
func codexBlock(s Source, lines []string) (body string, modMS int64, cwd string, warns []Warning) {
	var kept []string
	for _, line := range lines {
		m := codexFieldLine.FindStringSubmatch(line)
		if m == nil {
			kept = append(kept, line)
			continue
		}
		key, value := m[1], strings.TrimSpace(m[2])
		switch key {
		case "updated_at":
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				modMS = t.UnixMilli()
			} else if value != "" {
				warns = append(warns, Warning{Path: s.Path, Reason: "an updated_at value that is not RFC 3339; the item keeps no host timestamp"})
			}
		case "cwd":
			cwd = normalisePath(value)
		}
		// The host timestamp goes to its column and not into the indexed
		// text, on both hosts, and spec 5.7's rule is why: it is written on
		// every document, so its parts - a year, a month, an offset - would
		// become tokens of all of them, which is the defect `cwd` at 900 of
		// 901 documents measured. What a time-qualified query gets instead is
		// the column, which is spec 3's P3.
		if value != "" && key != "updated_at" {
			kept = append(kept, value)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), modMS, cwd, warns
}

// normalisePath folds the two things that make one directory read as two paths
// in this corpus: the extended-length prefix, which 39 of 55 lines of one file
// carried against 16 that did not, and case, which internal/project already
// folds when it derives a project root.
func normalisePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, `\\?\`)
	return strings.ToLower(p)
}

// splitFrontmatter returns the block between the opening and closing --- and the
// text after it. An unterminated block is not a block.
func splitFrontmatter(text string) (front, body string, ok bool) {
	if !strings.HasPrefix(text, "---\n") {
		return "", "", false
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", false
	}
	body = rest[end+len("\n---"):]
	return rest[:end], strings.TrimPrefix(body, "\n"), true
}

// firstLine is the display title when the format gave none: the first line that
// has anything on it, with markdown heading marks and bullets taken off.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#-*> "))
		if line != "" {
			return line
		}
	}
	return ""
}
