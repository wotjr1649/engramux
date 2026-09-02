package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/wotjr1649/engramux/internal/secret"
)

// Collector re-reads the two hosts' memory on an interval and keeps the
// memory_items table equal to what is on disk (M-2 decision 3).
//
// # Why a ticker and not a watcher
//
// A watcher was rejected on a measurement rather than a preference. Claude
// Code's memory directory is a *subdirectory of the transcript directory*: on
// the machine this was measured, the memory directories' siblings were 36, 104
// and 4 transcript files and the parent held 3,823, written continuously by the
// very sessions this product's hooks already capture. A recursive watch there
// fires on all of that, and a per-directory watch needs its watch set rebuilt
// every time a project appears. Either costs handles and threads, which are two
// of the three instruments the Phase 6 soak reads, so the next series would be
// measuring the watcher rather than the service.
//
// What a ticker costs instead is one os.Stat per file per tick over about 80
// files, and a re-read only of what moved. On the machine this was written for,
// both hosts' memory is switched off and nothing ever moves, so the steady state
// is the stat and nothing else.
//
// # Why the whole file is re-read when it moves
//
// A changed file's rows are deleted and rewritten rather than diffed. The corpus
// is hundreds of rows, a diff would need the old blocks in hand to compare
// against, and the external-content index follows either path through the same
// triggers. Simplicity wins at this size; if the corpus ever reaches a size
// where it does not, the mtime pair this already keeps is what a diff would be
// built on.
type Collector struct {
	// ClaudeHome and CodexHome are the two configuration homes. An empty one
	// skips that host, which is what a machine with one host installed wants.
	ClaudeHome string
	CodexHome  string

	// seen is the short-circuit: a path to the modification time and size it
	// had when it was last parsed. A file whose pair is unchanged is not
	// reopened.
	seen map[string]fileStamp
}

type fileStamp struct {
	modMS int64
	size  int64
}

// Report is what one pass did. Every field is a count rather than a list,
// because the caller logs it and a log line must not carry a path.
type Report struct {
	// Files is how many sources the walk listed, Skipped how many of those
	// were unchanged since the last pass and not reopened.
	Files   int
	Skipped int
	// Items is how many items the files that *were* read produced.
	Items int
	// Written is rows inserted, Removed rows deleted - both including the
	// churn of rewriting a changed file.
	Written int
	Removed int
	// Warnings is every shape no parser recognised, kept rather than
	// swallowed (M2).
	Warnings []Warning
}

// Collect runs one pass. It is safe to call on a database no other goroutine is
// writing; the service calls it from one loop.
//
// A pass that cannot list a home is not an error - [Sources] reports that as a
// warning and lists what it could - so the only errors here are the database's.
func (c *Collector) Collect(ctx context.Context, db *sql.DB, now time.Time) (Report, error) {
	if c.seen == nil {
		c.seen = map[string]fileStamp{}
	}
	sources, warns := Sources(c.ClaudeHome, c.CodexHome)
	rep := Report{Files: len(sources), Warnings: warns}

	live := make(map[string]bool, len(sources))
	for _, s := range sources {
		live[s.Path] = true
		if was, ok := c.seen[s.Path]; ok && was.modMS == s.ModTimeUnixMS && was.size == s.Size {
			rep.Skipped++
			continue
		}
		items, w, err := Parse(s)
		if err != nil {
			// A file listed a moment ago and unreadable now is this
			// machine's problem and not the format's, so it is a warning
			// and the pass continues. The stamp is deliberately not
			// recorded, so the next tick tries again.
			rep.Warnings = append(rep.Warnings, Warning{Path: s.Path, Reason: "the file could not be read: " + err.Error()})
			continue
		}
		rep.Warnings = append(rep.Warnings, w...)
		rep.Items += len(items)
		written, removed, err := replaceFile(ctx, db, s, items, now)
		if err != nil {
			return rep, err
		}
		rep.Written += written
		rep.Removed += removed
		c.seen[s.Path] = fileStamp{modMS: s.ModTimeUnixMS, size: s.Size}
	}

	// Files that have gone. The sweep is skipped when the walk found nothing
	// at all, because "both homes read as empty" is what a transient failure
	// looks like as well as what an uninstalled host looks like, and the two
	// are not worth telling apart by deleting the whole table.
	if len(sources) > 0 {
		removed, err := sweep(ctx, db, live)
		if err != nil {
			return rep, err
		}
		rep.Removed += removed
	}
	for path := range c.seen {
		if !live[path] {
			delete(c.seen, path)
		}
	}
	return rep, nil
}

// replaceFile makes the table's rows for one file equal to items, in one
// transaction so that a failure leaves the file's previous rows rather than
// half of its new ones.
func replaceFile(ctx context.Context, db *sql.DB, s Source, items []Item, now time.Time) (written, removed int, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("memory: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM memory_items WHERE host = ? AND source_path = ?`, s.Host, s.Path)
	if err != nil {
		return 0, 0, fmt.Errorf("memory: clear a file's rows: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("memory: count the cleared rows: %w", err)
	}
	removed = int(n)

	for _, it := range items {
		// project_id is filled by looking the project path up as a root,
		// which matches on Codex's side - its cwd is an absolute path and
		// projects.root is one - and does not on Claude Code's, whose
		// project path is that host's directory key. That asymmetry is the
		// decision rather than a gap: the key is a convenience and
		// project_path is what a scoped query compares, through
		// [ProjectKeys], which asks about both forms.
		//
		// privacy_class is what internal/secret makes of the body, on the
		// same terms events are tagged under: a memory note is full of
		// paths, and I-10 says the original is stored and the masking
		// happens at egress.
		class := secret.Detect([]byte(it.Body))
		var mod any
		if it.HostModifiedMS != 0 {
			mod = it.HostModifiedMS
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO memory_items (id, host, kind, source_path, entry_key, project_path,
			                          project_id, title, body, host_modified_at,
			                          privacy_class, redaction_version, indexed_at)
			VALUES (?, ?, ?, ?, ?, ?, (SELECT id FROM projects WHERE root = ?), ?, ?, ?, ?, ?, ?)`,
			ItemID(it), it.Host, it.Kind, it.SourcePath, it.EntryKey, it.ProjectPath,
			it.ProjectPath, it.Title, it.Body, mod,
			class.String(), int64(secret.Version), now.UnixMilli()); err != nil {
			return 0, 0, fmt.Errorf("memory: write an item: %w", err)
		}
		written++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("memory: commit: %w", err)
	}
	return written, removed, nil
}

// sweep deletes the rows of files that are no longer on disk. It reads the
// distinct paths back rather than building a NOT IN list, because the list would
// be as long as the corpus and this runs every tick.
func sweep(ctx context.Context, db *sql.DB, live map[string]bool) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT source_path FROM memory_items`)
	if err != nil {
		return 0, fmt.Errorf("memory: list the indexed files: %w", err)
	}
	var gone []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("memory: scan an indexed file: %w", err)
		}
		if !live[path] {
			gone = append(gone, path)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("memory: read the indexed files: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("memory: close the indexed files: %w", err)
	}

	var removed int
	for _, path := range gone {
		res, err := db.ExecContext(ctx, `DELETE FROM memory_items WHERE source_path = ?`, path)
		if err != nil {
			return removed, fmt.Errorf("memory: delete a vanished file's rows: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return removed, fmt.Errorf("memory: count a vanished file's rows: %w", err)
		}
		removed += int(n)
	}
	return removed, nil
}

// ItemID is the primary key: a hash of the three columns migration 00004 makes
// unique. Derived rather than minted, so that re-reading an unchanged block
// produces the same row and a caller can hold an id across a restart - which is
// what get_memory needs from a search hit.
//
// The separator is a NUL because none of the three can contain one: a Windows
// path cannot, and the other two are text this package cut out of a file whose
// NULs would have made it unreadable long before here.
func ItemID(it Item) string {
	sum := sha256.Sum256([]byte(it.Host + "\x00" + it.SourcePath + "\x00" + it.EntryKey))
	return hex.EncodeToString(sum[:16])
}
