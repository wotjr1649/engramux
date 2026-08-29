package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
)

// ftsTable is the name the search index is created under. It appears in the
// migration and in sqlite_schema, and this is the one place Go spells it.
const ftsTable = "events_fts"

// ftsMigration is the file that creates the index. The tokenizer this package
// expects is whatever that file declares, read at run time rather than copied
// into a constant here - a constant would be a third spelling to keep in step,
// and keeping two in step is the problem this exists to detect.
const ftsMigration = "00002_events_fts.sql"

// tokenizeClause captures the tokenizer out of a `tokenize = '...'` clause. It
// is deliberately loose about whitespace and about which quote character is
// used, because it reads two different texts: the migration as it was written,
// and the CREATE statement as SQLite stored it.
var tokenizeClause = regexp.MustCompile(`(?i)tokenize\s*=\s*['"]([^'"]*)['"]`)

// ErrNoFTSTable means the database has no search index at all, which is a
// database that predates the migration rather than one that disagrees with it.
var ErrNoFTSTable = errors.New("store: the database has no search index")

// Tokenizer returns the tokenizer the live index was created with and the one
// the migration declares.
//
// # Why this exists
//
// goose records that a migration ran and never re-runs it, and it does not
// checksum the file. So editing an applied migration in place leaves an index
// whose contents were built by the old clause and a migration file that claims
// the new one, on every machine that already ran that version, with nothing
// saying so. That was safe only while the migration was not an ancestor of the
// default branch; it is now.
//
// I-07 leaves the service as the only process that can look at the live schema,
// which is why this is reachable through a request rather than by a tool
// pointing at the file. And it returns both strings rather than a verdict,
// because `doctor` reports the comparison and a caller that only got "they
// disagree" could not say what to do about it.
//
// The live side is read out of sqlite_schema, which stores the CREATE statement
// as it was executed, so it is the text that built the index and not a
// re-derivation of it.
func Tokenizer(ctx context.Context, db *sql.DB) (live, expected string, err error) {
	var createSQL string
	err = db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = ?`, ftsTable).Scan(&createSQL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("%w: %s is not in sqlite_schema", ErrNoFTSTable, ftsTable)
	}
	if err != nil {
		return "", "", fmt.Errorf("store: read the index schema: %w", err)
	}

	file, err := fs.ReadFile(migrationFiles, migrationDir+"/"+ftsMigration)
	if err != nil {
		return "", "", fmt.Errorf("store: read the embedded migration: %w", err)
	}

	// An absent clause reads back as the empty string on either side rather
	// than as an error. FTS5's own default is `unicode61` with no arguments,
	// so "no clause" is a real state, and reporting it as one lets `doctor`
	// show the disagreement instead of failing to look.
	return firstSubmatch(createSQL), firstSubmatch(string(file)), nil
}

func firstSubmatch(s string) string {
	if m := tokenizeClause.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}
