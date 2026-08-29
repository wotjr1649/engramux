package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// TestTokenizerComparesTheLiveIndexWithTheMigration closes backlog 18: nothing
// in this product verified a live index's tokenizer against the migration that
// declared it.
//
// # The failure it is for
//
// goose records that a migration ran and never re-runs it, and it does not
// checksum the file. Editing an applied migration in place therefore leaves an
// index built by the old clause and a migration file claiming the new one, on
// every machine that already ran that version, with nothing saying so. That was
// safe only while the migration was not an ancestor of the default branch, and
// that expired at merge.
//
// # Why the drift is simulated on the live side
//
// The migration is embedded in the binary, so a test cannot edit it. The live
// index can be dropped and recreated with a different clause, which produces
// exactly the state the defect produces - a live index whose tokenizer is not
// the one the file declares - from the other direction. What is being asserted
// is that the two are compared at all.
func TestTokenizerComparesTheLiveIndexWithTheMigration(t *testing.T) {
	t.Run("a migrated database agrees with itself", func(t *testing.T) {
		db := migrated(t)
		live, expected, err := Tokenizer(t.Context(), db)
		if err != nil {
			t.Fatalf("Tokenizer: %v", err)
		}
		if live != expected {
			t.Fatalf("a freshly migrated database disagrees: live %q, migration %q", live, expected)
		}
		// Not an empty string on both sides, which would "agree" and
		// measure nothing. The migration declares a clause, so both
		// sides must carry one.
		if live == "" {
			t.Error("both sides are empty, so the comparison proves nothing")
		}
	})

	t.Run("an index rebuilt with another tokenizer disagrees", func(t *testing.T) {
		db := migrated(t)
		_, expected, err := Tokenizer(t.Context(), db)
		if err != nil {
			t.Fatalf("Tokenizer: %v", err)
		}

		const other = "ascii"
		if other == expected {
			t.Fatalf("the substitute tokenizer is the migration's own (%q), so this asserts nothing", other)
		}
		rebuildIndexWith(t, db, other)

		live, again, err := Tokenizer(t.Context(), db)
		if err != nil {
			t.Fatalf("Tokenizer after the rebuild: %v", err)
		}
		if live != other {
			t.Fatalf("the live tokenizer reads back as %q, want the %q it was rebuilt with", live, other)
		}
		if again != expected {
			t.Errorf("the migration's tokenizer changed to %q: it is embedded and cannot", again)
		}
		if live == again {
			t.Error("the two agree after the index was rebuilt with a different tokenizer")
		}
	})

	t.Run("a database with no index says so", func(t *testing.T) {
		db := migrated(t)
		if _, err := db.ExecContext(t.Context(), `DROP TABLE `+ftsTable); err != nil {
			t.Fatalf("drop the index: %v", err)
		}
		if _, _, err := Tokenizer(t.Context(), db); !errors.Is(err, ErrNoFTSTable) {
			t.Fatalf("Tokenizer = %v, want ErrNoFTSTable", err)
		}
	})
}

// rebuildIndexWith drops the index and recreates it with the tokenizer given,
// which is the state an edited-in-place migration leaves behind.
//
// The triggers are left alone: they name the table and go on working against
// the new one, which is exactly why the drift is silent in production.
func rebuildIndexWith(t *testing.T, db *sql.DB, tokenizer string) {
	t.Helper()
	var createSQL string
	if err := db.QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = ?`, ftsTable).Scan(&createSQL); err != nil {
		t.Fatalf("read the index schema: %v", err)
	}
	replaced := tokenizeClause.ReplaceAllString(createSQL, "tokenize = '"+tokenizer+"'")
	if replaced == createSQL {
		t.Fatalf("the tokenize clause was not replaced, so the rebuild would not change anything")
	}
	for _, stmt := range []string{`DROP TABLE ` + ftsTable, replaced} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("rebuild the index: %v", err)
		}
	}
}

// TestTheMigrationThisReadsIsTheOneThatCreatesTheIndex guards [Tokenizer]'s own
// assumption: the file it reads has to be the one that creates the index. A
// renamed or split migration would make it read a file declaring nothing and
// report an empty expected value - which would "agree" with an index that also
// carried no clause, and the comparison would pass while measuring nothing.
func TestTheMigrationThisReadsIsTheOneThatCreatesTheIndex(t *testing.T) {
	body, err := migrationFiles.ReadFile(migrationDir + "/" + ftsMigration)
	if err != nil {
		t.Fatalf("read %s: %v", ftsMigration, err)
	}
	if !strings.Contains(string(body), "CREATE VIRTUAL TABLE "+ftsTable) {
		t.Fatalf("%s does not create %s, so Tokenizer is reading the wrong file", ftsMigration, ftsTable)
	}
	if firstSubmatch(string(body)) == "" {
		t.Fatalf("%s declares no tokenize clause, so the expected side is empty", ftsMigration)
	}
}
