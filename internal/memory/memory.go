// Package memory reads the two hosts' own memory directories, read-only
// (memory spec rev.2, M-2).
//
// # What one item is
//
// One item is one block the host's own format delimits, and one whole file
// where it does not (M-2 decision 2). File granularity was rejected on a
// measurement: Codex's raw-memories file is 291,566 B where the median document
// either side of it is about 5 KB, so a file-grained reader makes the bulk of
// Codex's content one document and an excerpt cut from it answers nothing.
//
// # The parse is a partition and never a filter
//
// M2 says a shape the reader does not recognise is preserved, warned about, and
// continued past - a silent skip is a failure. So every parser here splits its
// file into blocks that *cover it*: text before the first recognised delimiter
// becomes a leading block with an empty [Item.EntryKey], and a file with no
// recognised delimiter at all becomes one block. Concatenating a file's blocks
// in order reproduces its indexable text, which is what
// TestEveryByteOfAFileEndsUpInSomeItem holds.
//
// The one thing deliberately dropped is a frontmatter *key name*. Spec 5.7
// measured what indexing structure costs - `cwd`, a JSON key, matched 900 of 901
// documents - and a YAML key is the same defect in a different syntax:
// `description` would otherwise be a token of every Claude Code note. The
// values are kept and the keys are not, which is the same rule store.Leaves
// applies to a payload.
//
// # Nothing here writes
//
// M-2 is read-only and never qualified: this package opens files for reading and
// has no path that creates, truncates or renames one. That is a property of the
// code rather than a comment, and it is worth stating because the directories it
// reads are two other programs' working state.
package memory

// Item is one native memory document, in the shape the store writes.
//
// It is this package's own type and not the wire's, on the same seam
// [github.com/wotjr1649/engramux/internal/search.Hit] sits on: what travels the
// pipe is internal/ipc's, and the service turns one into the other.
type Item struct {
	// Host is "claude-code" or "codex", matching events.host and the CHECK
	// migration 00004 puts on the column.
	Host string
	// Kind names the shape this item was parsed out of - which parser and
	// which block within it. It is free text in the column on purpose: a new
	// artefact in either host's directory should widen this value rather than
	// fail a constraint.
	Kind string
	// SourcePath is the file on this machine. It is a user path, so every
	// egress masks it (spec 8, Phase 5).
	SourcePath string
	// EntryKey is the block within SourcePath, and "" means the item is the
	// whole file or its leading block. With Host and SourcePath it is the
	// identity migration 00004 makes unique, which is what lets a re-scan
	// update a row instead of adding one.
	EntryKey string
	// ProjectPath is the best project identifier the host gave us, and the
	// two hosts give different ones: Claude Code names a project by the
	// directory slug it files memory under, and Codex writes an absolute cwd
	// per entry. Both are stored as they were read - see [ProjectKeys] for
	// how a scoped query asks about either.
	ProjectPath string
	// Title is a short display line. It is cut from the same text Body holds,
	// so it is never the only place a word appears and is not indexed.
	Title string
	// Body is the indexed and stored text: the block's own text with
	// frontmatter key names removed. I-10 applies to it exactly as it applies
	// to a payload - what is stored is the original, and masking happens at
	// egress.
	Body string
	// HostModifiedMS is the host's own timestamp in milliseconds since the
	// Unix epoch, and 0 means the host wrote none. That is not defensive: 1 of
	// the 18 Claude Code notes read on 2026-09-02 carries no `modified` key,
	// and this is the field spec 3's P3 compares against.
	HostModifiedMS int64
}

// Warning is a shape a reader did not recognise. It is the whole of M2's
// mechanism: every one of these is a thing that was kept rather than skipped,
// and a reader that returns none while producing fewer items than it read files
// is the failure that gate is shaped against.
type Warning struct {
	// Path is the file the warning is about.
	Path string
	// Reason says what was not recognised, in terms of the format and never
	// by quoting the file - these are the user's private notes.
	Reason string
}

// Source is one file a reader knows how to parse, located but not read.
//
// Listing is separate from parsing because collection re-stats on an interval
// and re-reads only what changed (M-2 decision 3): [ModTimeUnixMS] and [Size]
// are what that comparison is made of, and nothing here has opened the file.
type Source struct {
	// Path is the file.
	Path string
	// Host is which host wrote it.
	Host string
	// Kind is which parser reads it.
	Kind string
	// ProjectPath is what the location says about the project, which for
	// Claude Code is the directory slug and for Codex is empty - Codex's
	// memory is global and its entries carry their own cwd.
	ProjectPath string
	// ModTimeUnixMS and Size are the short-circuit. A file whose pair is
	// unchanged since the last tick is not reopened.
	ModTimeUnixMS int64
	Size          int64
}

// Kinds a [Source] can carry. They name the artefact rather than the file, so a
// host that renames a file keeps its parser.
const (
	KindClaudeIndex  = "claude-index"
	KindClaudeNote   = "claude-note"
	KindCodexIndex   = "codex-index"
	KindCodexRaw     = "codex-raw"
	KindCodexDigest  = "codex-summary"
	KindCodexRollout = "codex-rollout"
)

// Hosts, matching events.host and migration 00004's CHECK.
const (
	HostClaude = "claude-code"
	HostCodex  = "codex"
)
