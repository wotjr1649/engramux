// Package store opens the one SQLite database Engramux has, and refuses to
// hand it back unless it opened exactly the way spec 5.4 says.
//
// # One connection, held exclusively
//
// The service is the only process that ever opens the database - not even a
// read-only reader joins it (I-07), which is what forces every CLI read over
// the pipe instead (I-08). Two properties buy that: locking_mode=exclusive, and
// a pool capped at one connection.
//
// locking_mode=exclusive takes the file lock on first *access*, not at open, so
// [Open] does not return until the lock is actually held: it reads back every
// pragma and then runs one empty immediate transaction. A window between "Open
// returned" and "the lock exists" is precisely when another process could slip
// in, and I-07 does not have that window.
//
// The exclusive lock is also why no -shm wal-index file is ever created. That
// file is the one modernc.org/sqlite cannot defend: it is built with
// SQLITE_OMIT_SEH=1, so it cannot catch the structured exception upstream
// SQLite retries when a filter driver faults the -shm mapping (spec 5.4).
//
// # Transactions
//
// Ordinary [database/sql.DB.BeginTx] and [database/sql.Tx.Commit], with
// _txlock=immediate in the DSN making the driver issue BEGIN IMMEDIATE. Raw
// BEGIN IMMEDIATE SQL is banned: with exactly one connection, a single missed
// ROLLBACK wedges that connection permanently and there is no second one to
// recover on.
//
// # Every pragma is read back (I-11)
//
// Only _pragma values skip the driver's DSN validation, so a misspelling
// returns err == nil and SQLite ignores the setting silently. Readback alone
// cannot catch that, because a misspelled pragma leaves the real one at its
// default and some defaults - synchronous is one - are the value production
// wants. So [Open] checks two things and they catch different failures:
//
//   - the DSN's pragma names are exactly the set below, checked before opening,
//     which catches a misspelled or dropped pragma whatever its default is;
//   - every one of those pragmas reads back as its expected value, which
//     catches a setting that was named correctly and still did not take.
//
// Either failure fails startup. Running misconfigured is not an option this
// package offers.
//
// Checkpointing is deliberately not here: spec 5.4 specifies a straight
// TRUNCATE on a timer and a size threshold, and no Phase 1 gate depends on it.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// driverName is what modernc.org/sqlite registers itself as.
const driverName = "sqlite"

// Errors that fail startup. Each names a distinct way the database can end up
// configured differently from spec 5.4; compare with [errors.Is].
var (
	// ErrUnknownPragma means the DSN sets a pragma this package does not
	// verify. That is either a misspelling - the whole reason I-11 exists -
	// or a new setting nobody added to the table below.
	ErrUnknownPragma = errors.New("store: DSN sets a pragma that is not verified")

	// ErrMissingPragma means the DSN does not set a pragma this package
	// verifies. Readback cannot always see this: a pragma left unset reads
	// back as SQLite's default, and synchronous's default is the value spec
	// 5.4 asks for.
	ErrMissingPragma = errors.New("store: DSN does not set a verified pragma")

	// ErrPragmaMismatch means a pragma was set and did not take.
	ErrPragmaMismatch = errors.New("store: pragma read back with the wrong value")
)

// pragma is one setting from spec 5.4, and the value it must read back as.
// want's dynamic type is part of the comparison: the driver returns TEXT
// pragmas as string and the rest as int64.
type pragma struct {
	name string
	want any
}

// pragmas is spec 5.4's list. It is both the set of names the DSN may contain
// and the set of values verified after opening, so a name that appears in one
// and not the other cannot exist.
var pragmas = []pragma{
	{"journal_mode", "wal"},
	{"locking_mode", "exclusive"},
	{"foreign_keys", int64(1)},
	{"recursive_triggers", int64(1)},
	{"synchronous", int64(2)}, // 2 is FULL. 3 is EXTRA.
	{"busy_timeout", int64(10000)},
	{"journal_size_limit", int64(67108864)},
	{"secure_delete", int64(1)},
}

// dsnParams is spec 5.4's DSN, less the path. The driver applies _pragma values
// in its own order regardless of how they appear here, so this order is for
// readers only - it follows the spec's.
const dsnParams = "?_pragma=journal_mode(wal)" +
	"&_pragma=locking_mode(exclusive)" +
	"&_pragma=foreign_keys(1)" +
	"&_pragma=recursive_triggers(1)" +
	"&_pragma=synchronous(2)" +
	"&_pragma=busy_timeout(10000)" +
	"&_pragma=journal_size_limit(67108864)" +
	"&_pragma=secure_delete(1)" +
	"&_txlock=immediate"

// uriPath escapes the three characters SQLite's URI parser consumes from a
// `file:` path, and no others. Measured against modernc.org/sqlite v1.57.0:
//
//   - '#' starts a fragment. `hash#tag` opens `hash` instead, with no error.
//   - '%HH' is an escape. `pct%41hex` opens `pctAhex`, `pct%25five` opens
//     `pct%five`. `100%done` survives only because `%do` is not valid hex,
//     which is what makes this class of bug reach some users and not others.
//   - '?' starts the query. A Windows filename cannot contain one, so this is
//     insurance rather than a fix - but it is also what guarantees that the
//     first '?' in the DSN is always the separator, which both this package's
//     [checkPragmaNames] and the driver's own parser assume.
//
// Nothing else is escaped, and that is deliberate. '&', spaces, '+', ';', '@',
// '(', ')' and '=' are all legal in a Windows filename and all pass through
// SQLite untouched, because the path is what comes *before* the query. Reaching
// for [net/url] here is the trap: url.QueryEscape also escapes the backslashes
// and spaces a Windows path is made of, and fails half these cases.
//
// One [strings.Replacer] pass rather than three: it does not rescan its own
// output, so '%' -> "%25" cannot be re-escaped into "%2525" by a later rule.
var uriPath = strings.NewReplacer("%", "%25", "#", "%23", "?", "%3F")

// dsn is the connection string for the database at path. A Windows path keeps
// its backslashes, its drive letter, and its leading `\\` if it is a UNC path -
// SQLite only reads an authority component after `//`, with forward slashes.
func dsn(path string) string { return "file:" + uriPath.Replace(path) + dsnParams }

// Open opens the database at path, creating it if it does not exist, and
// returns a pool holding exactly one connection.
//
// It fails rather than returning a misconfigured database: every pragma spec
// 5.4 sets is read back and compared, and a DSN naming a pragma this package
// does not verify is rejected before the file is touched (I-11).
//
// When Open returns without error the exclusive lock is already held, so no
// other process can open the file (I-07). Callers own the returned pool and
// must Close it; Close releases the lock.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	return open(ctx, dsn(path))
}

// open is Open with the DSN supplied, so tests can open with a deliberately
// broken one.
func open(ctx context.Context, uri string) (*sql.DB, error) {
	if err := checkPragmaNames(uri); err != nil {
		return nil, err
	}

	db, err := sql.Open(driverName, uri)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", uri, err)
	}
	// One connection, and it stays open. MaxIdleConns below the cap would let
	// the pool close the connection when it goes idle, which releases the
	// exclusive lock and reopens the I-07 window every time the service is
	// quiet.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := verifyPragmas(ctx, db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err := takeExclusiveLock(ctx, db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

// checkPragmaNames reports whether uri sets exactly the pragmas [pragmas]
// verifies - no unknown name, none missing. This is the half of I-11 that runs
// before the database is opened, and the only half that can catch a
// misspelling: `syncronous(3)` is not a pragma, so SQLite ignores it and
// `synchronous` reads back as its default, which is the value spec 5.4 wanted
// anyway.
//
// Parameters other than _pragma are not checked here. _txlock is not a pragma
// and cannot be read back either; it is pinned by a test instead (spec 5.4).
func checkPragmaNames(uri string) error {
	// A Windows path cannot contain '?', so the first one starts the
	// parameters.
	_, params, _ := strings.Cut(uri, "?")

	seen := make(map[string]bool, len(pragmas))
	for _, param := range strings.Split(params, "&") {
		value, ok := strings.CutPrefix(param, "_pragma=")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(value, "(")
		if !known(name) {
			return fmt.Errorf("%w: %q", ErrUnknownPragma, name)
		}
		seen[name] = true
	}
	for _, p := range pragmas {
		if !seen[p.name] {
			return fmt.Errorf("%w: %q", ErrMissingPragma, p.name)
		}
	}
	return nil
}

func known(name string) bool {
	for _, p := range pragmas {
		if p.name == name {
			return true
		}
	}
	return false
}

// verifyPragmas reads every pragma back and compares it, value and type, with
// what spec 5.4 asked for (I-11). This is the half that catches a pragma named
// correctly whose value SQLite would not accept: `synchronous(9)` opens without
// error and leaves synchronous at 1.
func verifyPragmas(ctx context.Context, db *sql.DB) error {
	for _, p := range pragmas {
		var got any
		// p.name comes from the table above, never from a caller.
		if err := db.QueryRowContext(ctx, "PRAGMA "+p.name).Scan(&got); err != nil {
			return fmt.Errorf("store: read back pragma %s: %w", p.name, err)
		}
		if got != p.want {
			return fmt.Errorf("%w: %s = %#v, want %#v", ErrPragmaMismatch, p.name, got, p.want)
		}
	}
	return nil
}

// takeExclusiveLock makes "Open returned" mean "the lock is held" (I-07).
//
// locking_mode=exclusive acquires the lock on first access, not at open: after
// sql.Open and a Ping alone, another process can still open the database and
// read it. One statement is enough to take the lock, and exclusive mode retains
// it until the connection closes.
//
// An empty immediate transaction is the statement, rather than relying on the
// pragma readback above having incidentally done it, because "which of these
// eight reads happens to touch the file" is not something to leave to a driver
// or a SQLite version. It writes nothing and commits.
//
// This depends on _txlock=immediate, and measurably so: with a deferred BEGIN
// an empty transaction takes no lock at all, and the test that asserts a second
// connection is refused fails. That makes the second-connection test a check on
// _txlock in the production DSN too, not only on locking_mode.
func takeExclusiveLock(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: take the exclusive lock: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: take the exclusive lock: %w", err)
	}
	return nil
}
