package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// migrationDir is the directory inside [migrationFiles] goose reads. It is also
// the directory the //go:embed pattern names; the two must agree or [provider]
// hands goose an empty filesystem and every migration silently does nothing.
const migrationDir = "migrations"

// migrationFiles carries the schema into the binary, so a deployed service needs
// nothing on disk but its own executable and the database. The migrations live
// in the package that owns the database on purpose: the DDL is the truth (spec
// 6), and the truth should not be reachable only through a path that a working
// directory can change.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies every pending migration to db, and is a no-op once db is
// current. It is separate from [Open] because opening and schema changes fail
// for different reasons and a caller may want to tell them apart - Open failing
// means the database is unusable, Migrate failing means it is usable and out of
// date.
//
// db must be the pool [Open] returned. goose runs each migration in its own
// transaction through [database/sql.DB.BeginTx], which is what the one
// connection allows; raw BEGIN IMMEDIATE is banned here for the same reason it
// is banned everywhere else in this package.
func Migrate(ctx context.Context, db *sql.DB) error {
	p, err := provider(db)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("store: migrate up: %w", err)
	}
	return nil
}

// provider builds the goose provider for db.
//
// The [goose.Provider] API is used rather than goose's package-level functions
// because those keep the dialect, the base filesystem and the logger in package
// globals: two callers in one process configure each other, and a test that
// forgets to reset one leaks into the next. It is also the half of goose that
// takes a [context.Context], which is what the noctx linter wants.
//
// The returned provider must not be Closed: [goose.Provider.Close] closes the
// *sql.DB it was handed, and this package's callers own that pool.
func provider(db *sql.DB) (*goose.Provider, error) {
	sub, err := fs.Sub(migrationFiles, migrationDir)
	if err != nil {
		return nil, fmt.Errorf("store: locate embedded migrations: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		return nil, fmt.Errorf("store: build the migration provider: %w", err)
	}
	return p, nil
}
