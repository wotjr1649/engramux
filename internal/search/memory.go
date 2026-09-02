package search

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/wotjr1649/engramux/internal/secret"
)

// MemoryHit is one matching native memory item, in the order FTS5 ranked it.
//
// It is a separate type from [Hit] and the two lists are returned separately,
// which is memory spec rev.2's M-2 decision 6 and not a convenience: bm25 is
// computed against the document frequencies of one index, and the two indexes
// here are built over populations of about 300 and 17,000. Interleaving their
// scores would put an unmeasured normalisation rule in front of M3's own recall
// number.
type MemoryHit struct {
	// ID is memory_items.id, derived from the host, the file and the block,
	// so it survives the collection tick that reads the same block again -
	// which is what makes it usable as get_memory's argument.
	ID string
	// Host is which host wrote it and Kind is which artefact and which block
	// within it.
	Host string
	Kind string
	// SourcePath is the file it came from. It is a user path, and it is
	// masked here rather than by the caller for the same reason the excerpt
	// is (I-10, and spec 8's Phase 5 egress clause).
	SourcePath string
	// Title is a display line, masked with the same rule.
	Title string
	// HostModifiedMS is the host's own timestamp, 0 when it wrote none.
	HostModifiedMS int64
	// Excerpt is a window of the masked body around the match.
	Excerpt string
}

// SearchMemory returns up to limit native memory items whose body matches text,
// best first, and how many matched before the limit.
//
// projectKeys scopes the search, and it is a list rather than an id because the
// two hosts identify a project differently and neither string can be turned into
// the other without the filesystem:
// [github.com/wotjr1649/engramux/internal/memory.ProjectKeys] is what builds it
// from a project root. An empty list is every project, on the same terms
// [Search]'s empty projectID is.
//
// The two phases are [Search]'s and for the same reason: the single connection
// (spec 5.4) is held by an open *sql.Rows, and masking is the expensive part. So
// the rows are read, the cursor is closed, and only then is anything masked.
func SearchMemory(ctx context.Context, db *sql.DB, text string, projectKeys []string, limit int) (hits []MemoryHit, total int64, err error) {
	tokens, err := queryTokens(text)
	if err != nil {
		return nil, 0, err
	}

	query, args := memoryMatchQuery(tokens, projectKeys, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search: match memory: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		hit  MemoryHit
		body string
	}
	var scanned []row
	for rows.Next() {
		var r row
		var mod sql.NullInt64
		if err := rows.Scan(&r.hit.ID, &r.hit.Host, &r.hit.Kind, &r.hit.SourcePath,
			&r.hit.Title, &r.body, &mod, &total); err != nil {
			return nil, 0, fmt.Errorf("search: scan a memory hit: %w", err)
		}
		// NULL is "the host wrote no timestamp" and 0 is what that becomes on
		// the wire; the column is nullable because 1 of the 18 Claude Code
		// notes read on 2026-09-02 carries no modified key.
		r.hit.HostModifiedMS = mod.Int64
		scanned = append(scanned, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search: read the memory hits: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, fmt.Errorf("search: close the memory hits: %w", err)
	}

	hits = make([]MemoryHit, len(scanned))
	for i, r := range scanned {
		hits[i] = r.hit
		hits[i].SourcePath = secret.MaskString(hits[i].SourcePath)
		hits[i].Title = secret.MaskString(hits[i].Title)
		hits[i].Excerpt = excerptText(secret.MaskString(r.body), tokens)
	}
	return hits, total, nil
}

// memoryMatchQuery builds the statement and its arguments.
//
// The scope is a predicate on memory_items and it compares project_path, not
// project_id (M-2 decision 8). The foreign key is a convenience that is empty
// whenever no projects row matched, which on Claude Code's side is always -
// scoping on it would make that host's memory unreachable through MCP.
//
// The unscoped statement is byte-identical to the scoped one without its
// predicate, on the same rule [matchQuery] follows: an always-true disjunction
// would put an unmeasured expression in front of every invocation.
func memoryMatchQuery(tokens []string, projectKeys []string, limit int) (string, []any) {
	const (
		head = `
		SELECT memory_items.id, memory_items.host, memory_items.kind,
		       memory_items.source_path, memory_items.title, memory_items.body,
		       memory_items.host_modified_at, count(*) OVER ()
		FROM memory_fts
		JOIN memory_items ON memory_items.rowid = memory_fts.rowid
		WHERE memory_fts MATCH ?`
		tail = `
		ORDER BY rank
		LIMIT ?`
	)
	if len(projectKeys) == 0 {
		return head + tail, []any{matchExpression(tokens), limit}
	}
	args := []any{matchExpression(tokens)}
	for _, k := range projectKeys {
		args = append(args, k)
	}
	args = append(args, limit)
	return head + `
		  AND memory_items.project_path IN (` +
		strings.TrimSuffix(strings.Repeat("?, ", len(projectKeys)), ", ") + `)` + tail, args
}
