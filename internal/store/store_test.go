package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"modernc.org/sqlite"
)

// sqliteBusy is SQLITE_BUSY. modernc.org/sqlite/lib exports the constant, but
// that package is a multi-megabyte generated translation unit and this is the
// only value these tests need from it.
const sqliteBusy = 5

// requireBusy asserts that err is SQLITE_BUSY and not merely "some error".
// Every refusal these tests care about - a second connection, a second BeginTx
// - must be the lock refusing, not a typo in a DSN or a missing directory
// producing a different failure that happens to be non-nil.
func requireBusy(t *testing.T, err error, what string) {
	t.Helper()
	var serr *sqlite.Error
	if !errors.As(err, &serr) {
		t.Fatalf("%s error = %v (%T), want *sqlite.Error with code %d", what, err, err, sqliteBusy)
	}
	if serr.Code() != sqliteBusy {
		t.Fatalf("%s error code = %d (%v), want %d (SQLITE_BUSY)", what, serr.Code(), err, sqliteBusy)
	}
}

// closeAt registers db for close and asserts the close itself succeeds. On
// Windows an open handle makes t.TempDir()'s cleanup fail, and the WAL sidecar
// files count.
func closeAt(t *testing.T, db *sql.DB) {
	t.Helper()
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}

// dbPath returns a fresh database path under the test's own temp dir.
func dbPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "engramux.db")
}

// fastBusy is the production DSN with busy_timeout dropped from 10 s to 200 ms.
// A test that expects SQLITE_BUSY otherwise waits the full production timeout
// for every refusal. The driver applies busy_timeout before every other pragma
// regardless of DSN order, so the shortened timeout governs the connect itself.
func fastBusy(t *testing.T, uri string) string {
	t.Helper()
	out := strings.Replace(uri, "busy_timeout(10000)", "busy_timeout(200)", 1)
	if out == uri {
		t.Fatalf("test setup: the production DSN no longer contains busy_timeout(10000): %s", uri)
	}
	return out
}

// TestDSN pins spec 5.4's DSN as one exact string: every pragma, every value,
// and _txlock=immediate, which is the one setting Open cannot read back. The
// path goes in as written - a Windows path with backslashes and a drive letter
// is neither escaped nor rewritten.
func TestDSN(t *testing.T) {
	got := dsn(`D:\Users\example\AppData\Local\engramux\engramux.db`)
	want := `file:D:\Users\example\AppData\Local\engramux\engramux.db` +
		`?_pragma=journal_mode(wal)` +
		`&_pragma=locking_mode(exclusive)` +
		`&_pragma=foreign_keys(1)` +
		`&_pragma=recursive_triggers(1)` +
		`&_pragma=synchronous(2)` +
		`&_pragma=busy_timeout(10000)` +
		`&_pragma=journal_size_limit(67108864)` +
		`&_pragma=secure_delete(1)` +
		`&_txlock=immediate`
	if got != want {
		t.Errorf("dsn()\n got %s\nwant %s", got, want)
	}
}

// TestOpenReadsBackEveryPragma is I-11's positive half: on a database Open
// accepted, every pragma spec 5.4 sets reads back as the value it was set to,
// in the type the driver returns it as. These expectations are measured against
// modernc.org/sqlite v1.57.0 and written independently of the table store.go
// verifies against, so a wrong expectation in that table is caught here rather
// than agreeing with itself.
func TestOpenReadsBackEveryPragma(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, dbPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)

	want := []struct {
		pragma string
		value  any
	}{
		{"journal_mode", "wal"},
		{"locking_mode", "exclusive"},
		{"foreign_keys", int64(1)},
		{"recursive_triggers", int64(1)},
		{"synchronous", int64(2)},
		{"busy_timeout", int64(10000)},
		{"journal_size_limit", int64(67108864)},
		{"secure_delete", int64(1)},
	}
	for _, w := range want {
		t.Run(w.pragma, func(t *testing.T) {
			var got any
			if err := db.QueryRowContext(ctx, "PRAGMA "+w.pragma).Scan(&got); err != nil {
				t.Fatalf("PRAGMA %s: %v", w.pragma, err)
			}
			if got != w.value {
				t.Errorf("PRAGMA %s = %#v, want %#v", w.pragma, got, w.value)
			}
		})
	}
}

// TestOpenRejectsMisspelledPragma is the case spec 5.4 names to justify I-11:
// `syncronous(3)`. Only _pragma values skip the driver's own DSN validation, so
// the misspelling opens with err == nil and SQLite ignores it silently.
//
// Readback cannot catch this one. The subtest below is the measurement of why:
// the ignored setting leaves synchronous at 2, which is both SQLite's default
// and exactly the value production wants, so an assertion that synchronous == 2
// passes on the typo'd DSN. Validating the DSN's pragma names against the set
// Open verifies is what catches it, and it catches it before the database is
// opened at all.
func TestOpenRejectsMisspelledPragma(t *testing.T) {
	ctx := t.Context()
	typo := strings.Replace(dsn(dbPath(t)), "synchronous(2)", "syncronous(3)", 1)

	db, err := open(ctx, typo)
	if err == nil {
		closeAt(t, db)
		t.Fatalf("open(%s) succeeded, want ErrUnknownPragma", typo)
	}
	if !errors.Is(err, ErrUnknownPragma) {
		t.Fatalf("open error = %v, want errors.Is(_, ErrUnknownPragma)", err)
	}

	t.Run("readback alone cannot catch it", func(t *testing.T) {
		typo := strings.Replace(dsn(dbPath(t)), "synchronous(2)", "syncronous(3)", 1)
		raw, err := sql.Open(driverName, typo)
		if err != nil {
			t.Fatalf("sql.Open: %v", err)
		}
		closeAt(t, raw)

		var got any
		if err := raw.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&got); err != nil {
			t.Fatalf("PRAGMA synchronous: %v", err)
		}
		if got != int64(2) {
			t.Fatalf("PRAGMA synchronous on the typo'd DSN = %#v, want int64(2).\n"+
				"The misspelling no longer reads back as the production value, which means "+
				"readback would now catch it and this gate's premise has changed.", got)
		}
	})
}

// TestOpenRejectsPragmaThatDidNotTake is I-11's other half, and the half only
// readback can catch: `synchronous(9)` is a known pragma name with a value
// SQLite will not accept. The DSN passes name validation, the open returns nil,
// and the setting silently stays at 1.
func TestOpenRejectsPragmaThatDidNotTake(t *testing.T) {
	bogus := strings.Replace(dsn(dbPath(t)), "synchronous(2)", "synchronous(9)", 1)

	db, err := open(t.Context(), bogus)
	if err == nil {
		closeAt(t, db)
		t.Fatalf("open(%s) succeeded, want ErrPragmaMismatch", bogus)
	}
	if !errors.Is(err, ErrPragmaMismatch) {
		t.Fatalf("open error = %v, want errors.Is(_, ErrPragmaMismatch)", err)
	}
}

// TestOpenRejectsDSNMissingAPragma covers the deletion readback cannot see
// either: synchronous(2) removed outright leaves synchronous at 2, because 2 is
// SQLite's default. Only comparing the DSN against the set of pragmas Open
// verifies can tell that the setting was never asked for.
func TestOpenRejectsDSNMissingAPragma(t *testing.T) {
	short := strings.Replace(dsn(dbPath(t)), "&_pragma=synchronous(2)", "", 1)

	db, err := open(t.Context(), short)
	if err == nil {
		closeAt(t, db)
		t.Fatalf("open(%s) succeeded, want ErrMissingPragma", short)
	}
	if !errors.Is(err, ErrMissingPragma) {
		t.Fatalf("open error = %v, want errors.Is(_, ErrMissingPragma)", err)
	}
}

// TestOpenRefusesSecondConnection is I-07: no other process opens the database,
// ever. The refusal is asserted with nothing executed on the first connection
// in between, because locking_mode=exclusive takes the lock on first access
// rather than at open - a test that let something incidental touch the file
// first would pass whether or not Open established the lock itself.
//
// The second half is the control. A test that only shows a second connection
// failing cannot distinguish a held lock from a path that never opens at all;
// once the first connection closes, the same second connection must succeed.
func TestOpenRefusesSecondConnection(t *testing.T) {
	ctx := t.Context()
	path := dbPath(t)

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Registering a cleanup executes no SQL: the database is untouched between
	// Open returning and the second connection below. The close that matters is
	// asserted inline; this one only keeps a failed test from leaving a handle
	// open, which would fail t.TempDir()'s cleanup on Windows.
	firstClosed := false
	t.Cleanup(func() {
		if !firstClosed {
			_ = db.Close()
		}
	})

	second, err := sql.Open(driverName, fastBusy(t, dsn(path)))
	if err != nil {
		t.Fatalf("sql.Open (second): %v", err)
	}
	// Closed at the end rather than here: when the assertion below fails the
	// second connection is a live one holding the file, and an unclosed handle
	// buries the real failure under t.TempDir()'s cleanup error. It holds
	// nothing when the assertion passes, since its only connect attempt failed.
	closeAt(t, second)
	var n int
	err = second.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&n)
	requireBusy(t, err, "second connection while the first is open")

	if err := db.Close(); err != nil {
		t.Fatalf("Close (first): %v", err)
	}
	firstClosed = true

	third, err := sql.Open(driverName, fastBusy(t, dsn(path)))
	if err != nil {
		t.Fatalf("sql.Open (third): %v", err)
	}
	closeAt(t, third)
	if err := third.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&n); err != nil {
		t.Fatalf("after the first connection closed a new connection still failed: %v", err)
	}
	if n != 0 {
		t.Errorf("sqlite_schema row count = %d, want 0", n)
	}
}

// TestOpenCreatesNoSharedMemoryFile is the observable consequence of
// locking_mode=exclusive taking effect before first access (spec 5.4): the -shm
// wal-index is never created. That file is the one modernc.org/sqlite cannot
// defend, since it is built with SQLITE_OMIT_SEH=1 and cannot catch the
// structured exception a faulting filter driver raises on the mapping. A -shm
// appearing at any point here means the locking mode did not take.
func TestOpenCreatesNoSharedMemoryFile(t *testing.T) {
	ctx := t.Context()
	path := dbPath(t)

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = db.Close()
		}
	})
	requireAbsent(t, path+"-shm", "after Open")

	if _, err := db.ExecContext(ctx, `CREATE TABLE t(v TEXT NOT NULL)`); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO t(v) VALUES('x')`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	requireAbsent(t, path+"-shm", "after a write")
	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Errorf("stat -wal after a write: %v, want the WAL to exist", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	closed = true
	requireAbsent(t, path+"-shm", "after Close")
}

func requireAbsent(t *testing.T, path, when string) {
	t.Helper()
	_, err := os.Stat(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat %s %s: err = %v, want os.ErrNotExist", filepath.Base(path), when, err)
	}
}

// TestOpenCapsThePoolAtOne pins spec 5.4's one connection. The cap is asserted
// on the pool's own accounting, and then behaviourally: eight concurrent
// queries must still leave exactly one connection open. Under exclusive locking
// a second connection would be refused anyway, so a pool free to open one is a
// pool free to fail.
func TestOpenCapsThePoolAtOne(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, dbPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("Stats().MaxOpenConnections = %d, want 1", got)
	}

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var n int
			errs[i] = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&n)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent query %d: %v", i, err)
		}
	}
	if got := db.Stats().OpenConnections; got != 1 {
		t.Errorf("Stats().OpenConnections after 8 concurrent queries = %d, want 1", got)
	}
}

// TestTransactionsDoNotWedgeTheConnection is the cost of one connection: a
// transaction that is neither committed nor rolled back wedges it permanently,
// and there is no second connection to recover on. Commit, rollback and commit
// again on the same pool must all work, and the rolled-back row must be absent
// from the committed result.
//
// The exact rows are the assertion. A count, or a check that the error was nil,
// survives a Commit that silently became a no-op.
func TestTransactionsDoNotWedgeTheConnection(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, dbPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	closeAt(t, db)

	if _, err := db.ExecContext(ctx, `CREATE TABLE t(v TEXT NOT NULL)`); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	commit := func(v string) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx(%q): %v", v, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t(v) VALUES(?)`, v); err != nil {
			t.Fatalf("INSERT %q: %v", v, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(%q): %v", v, err)
		}
	}

	commit("first")

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx (to roll back): %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t(v) VALUES('rolled-back')`); err != nil {
		t.Fatalf("INSERT (to roll back): %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	commit("third")

	rows, err := db.QueryContext(ctx, `SELECT v FROM t ORDER BY rowid`)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close: %v", err)
		}
	}()
	var got []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if want := "first,third"; strings.Join(got, ",") != want {
		t.Errorf("rows = %q, want [%s]", got, want)
	}
}

// TestTxlockImmediate pins _txlock=immediate, the one DSN setting I-11's
// readback cannot check because it is not a pragma (spec 5.4).
//
// The technique: connection A begins a transaction and writes nothing, then
// connection B begins one. Under _txlock=immediate the driver issues BEGIN
// IMMEDIATE, so A takes the write lock at BEGIN and B's BeginTx is refused with
// SQLITE_BUSY. Without it the driver issues a plain BEGIN, A holds nothing, and
// B's BeginTx succeeds.
//
// Two things this test must not do, either of which makes it decorative:
//
//   - Assert on B's first write instead of B's BeginTx. Under a deferred BEGIN
//     the write contends too; the entire difference between the two settings is
//     *when* the lock is taken, so an assertion on the write cannot tell them
//     apart.
//   - Let A write something first. Then both settings take the lock at the same
//     moment and the two DSNs behave identically.
//
// Limitation, and spec 5.4 asks for exactly this and no more: this pins the
// driver's _txlock handling, not the production DSN end to end. Observing
// contention needs a second connection and locking_mode=exclusive means there
// is no such thing, so locking_mode is dropped here and everything else from
// spec 5.4 is kept. Two other tests cover the production DSN itself: TestDSN
// holds _txlock=immediate in it literally, and TestOpenRefusesSecondConnection
// fails without it, because Open's empty transaction only takes the exclusive
// lock when the driver begins it IMMEDIATE.
func TestTxlockImmediate(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// uri is the production DSN minus locking_mode, with the short busy
	// timeout, optionally minus _txlock.
	uri := func(t *testing.T, name string, txlock bool) string {
		t.Helper()
		out := fastBusy(t, strings.Replace(dsn(filepath.Join(dir, name)), "&_pragma=locking_mode(exclusive)", "", 1))
		if !txlock {
			out = strings.Replace(out, "&_txlock=immediate", "", 1)
			if strings.Contains(out, "_txlock") {
				t.Fatalf("test setup: could not remove _txlock from %s", out)
			}
			return out
		}
		if !strings.Contains(out, "_txlock=immediate") {
			t.Fatalf("test setup: the production DSN no longer carries _txlock=immediate: %s", out)
		}
		return out
	}

	// beginBoth opens two connections to the same database, begins a
	// transaction on A that writes nothing, and returns B's BeginTx error.
	beginBoth := func(t *testing.T, uri string) error {
		t.Helper()
		a, err := sql.Open(driverName, uri)
		if err != nil {
			t.Fatalf("sql.Open (A): %v", err)
		}
		closeAt(t, a)
		b, err := sql.Open(driverName, uri)
		if err != nil {
			t.Fatalf("sql.Open (B): %v", err)
		}
		closeAt(t, b)

		// A's transaction writes nothing. Under _txlock=immediate the write
		// lock is already held once BeginTx returns.
		tx, err := a.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("A BeginTx: %v", err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(); err != nil {
				t.Errorf("A Rollback: %v", err)
			}
		})

		btx, err := b.BeginTx(ctx, nil)
		if err == nil {
			if err := btx.Rollback(); err != nil {
				t.Errorf("B Rollback: %v", err)
			}
		}
		return err
	}

	// Each subtest gets its own database file: sharing one would let the first
	// subtest's lock decide the second's outcome.
	t.Run("immediate refuses the second BeginTx", func(t *testing.T) {
		requireBusy(t, beginBoth(t, uri(t, "immediate.db", true)),
			"B BeginTx under _txlock=immediate")
	})

	t.Run("deferred allows it", func(t *testing.T) {
		if err := beginBoth(t, uri(t, "deferred.db", false)); err != nil {
			t.Fatalf("B BeginTx without _txlock = %v, want nil. "+
				"Without this half the immediate case above proves nothing.", err)
		}
	})
}
