package search

import (
	"context"
	"database/sql"
	"errors"
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
func SearchMemory(ctx context.Context, db *sql.DB, text string, projectKeys []string, limit int, m Match) (hits []MemoryHit, total int64, err error) {
	tokens, err := queryTokens(text)
	if err != nil {
		return nil, 0, err
	}

	query, args := memoryMatchQuery(tokens, projectKeys, limit, m)
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

// / MemoryItem is one whole native memory item, already masked.
//
// It is the read behind get_memory, and it lives here rather than in
// internal/store because this package owns the memory read path end to end -
// the search half masks everything it returns, and splitting the read so that
// half of it masked somewhere else is two conventions for one feature.
type MemoryItem struct {
	ID             string
	Host           string
	Kind           string
	SourcePath     string
	EntryKey       string
	ProjectPath    string
	Title          string
	Body           string
	PrivacyClass   string
	HostModifiedMS int64
}

// GetMemoryItem reads one item by id, within projectKeys, or nil when the pair
// matches no row.
//
// nil and an error are different answers: nil is "no such item in that scope",
// which is the same answer as "no such item" and is deliberately not
// distinguished from it - the same rule store.GetEvent follows, so that a caller
// cannot use this to learn what exists in a project it did not ask about.
//
// An empty projectKeys is every project (memory spec rev.2, M-2 decision 9): a
// memory item may belong to no project this database has a row for.
func GetMemoryItem(ctx context.Context, db *sql.DB, id string, projectKeys []string) (*MemoryItem, error) {
	query := `
		SELECT id, host, kind, source_path, entry_key, project_path, title, body,
		       privacy_class, host_modified_at
		FROM memory_items
		WHERE id = ?`
	args := []any{id}
	if len(projectKeys) > 0 {
		query += `
		  AND project_path IN (` + strings.TrimSuffix(strings.Repeat("?, ", len(projectKeys)), ", ") + `)`
		for _, k := range projectKeys {
			args = append(args, k)
		}
	}

	var it MemoryItem
	var mod sql.NullInt64
	err := db.QueryRowContext(ctx, query, args...).Scan(&it.ID, &it.Host, &it.Kind,
		&it.SourcePath, &it.EntryKey, &it.ProjectPath, &it.Title, &it.Body,
		&it.PrivacyClass, &mod)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("search: read a memory item: %w", err)
	}
	it.HostModifiedMS = mod.Int64

	// Everything that came out of a file is masked, and the id is not: it is
	// this build own derivation, 32 hex characters, and masking a value the
	// caller has to hand back would break the round trip get_memory exists
	// for.
	it.SourcePath = secret.MaskString(it.SourcePath)
	it.EntryKey = secret.MaskString(it.EntryKey)
	it.ProjectPath = secret.MaskString(it.ProjectPath)
	it.Title = secret.MaskString(it.Title)
	it.Body = secret.MaskString(it.Body)
	return &it, nil
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
func memoryMatchQuery(tokens []string, projectKeys []string, limit int, m Match) (string, []any) {
	// The body is joined after the LIMIT, for [matchQuery]'s reason and by its
	// shape: `count(*) OVER ()` materialises every matching row, so a body in
	// the same SELECT list is a body read per match rather than per hit. The
	// native corpus is three hundred items where the event corpus is twenty
	// thousand, so the cost here is small - but it is the same defect, and
	// one of the two statements quietly keeping it is how it comes back.
	const (
		inner = `
		SELECT memory_fts.rowid AS rid, count(*) OVER () AS total, rank AS score
		FROM memory_fts
		JOIN memory_items ON memory_items.rowid = memory_fts.rowid
		WHERE memory_fts MATCH ?`
		tail = `
		ORDER BY score
		LIMIT ?`
		outer = `
		SELECT mi.id, mi.host, mi.kind, mi.source_path, mi.title, mi.body,
		       mi.host_modified_at, k.total
		FROM (`
		join = `) AS k
		JOIN memory_items mi ON mi.rowid = k.rid
		ORDER BY k.score`
	)
	if len(projectKeys) == 0 {
		return outer + inner + tail + join, []any{matchExpression(tokens, m), limit}
	}
	args := []any{matchExpression(tokens, m)}
	for _, k := range projectKeys {
		args = append(args, k)
	}
	args = append(args, limit)
	return outer + inner + `
		  AND memory_items.project_path IN (` +
		strings.TrimSuffix(strings.Repeat("?, ", len(projectKeys)), ", ") + `)` + tail + join, args
}
