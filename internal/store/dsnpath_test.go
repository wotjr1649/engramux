package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenAcceptsPathsWithURISignificantCharacters guards the seam between a
// Windows path and the URI the DSN embeds it in.
//
// dsn() concatenates the path into "file:" + path + params, and SQLite parses
// what it gets as a URI. Windows filenames legally contain characters that mean
// something there - '%' introduces a percent-escape and '#' starts a fragment -
// and the database lives under the user's local application data directory
// (spec 5.6), so the path carries a username nobody here chose.
//
// Three of these opened the wrong file before uriPath existed, and all three
// returned a nil error while doing it: `hash#tag` opened `hash`, `pct%41hex`
// opened `pctAhex`, `pct%25five` opened `pct%five`. `100%done` passed
// throughout, which is the tell - `%do` is not a valid hex escape, so the bug
// reaches the user whose directory happens to contain a valid one and nobody
// else.
//
// The assertion that catches it is the os.Stat at the end, not the write. A DSN
// pointing somewhere else opens, creates, and inserts perfectly happily.
func TestOpenAcceptsPathsWithURISignificantCharacters(t *testing.T) {
	for _, dirName := range []string{
		"plain",
		"with space",
		"100%done",
		"a&b",
		"hash#tag",
		"plus+sign",
		"semi;colon",
		"at@sign",
		"paren(s)",
		"equals=sign",
		"pct%41hex",
		"pct%25five",
		// Both escapes in one name: the '%' that uriPath writes for '#' must
		// not be re-escaped by the '%' rule.
		"both%41#tag",
	} {
		t.Run(dirName, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), dirName, "engramux.db")
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			ctx := t.Context()
			db, err := Open(ctx, path)
			if err != nil {
				t.Fatalf("Open(%q): %v", path, err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Errorf("close: %v", err)
				}
			})

			// Opening is not enough: a DSN that silently pointed somewhere
			// else would still open. Write, then confirm the bytes landed in
			// the file we asked for.
			if _, err := db.ExecContext(ctx, `CREATE TABLE t(v TEXT NOT NULL)`); err != nil {
				t.Fatalf("create: %v", err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO t(v) VALUES('landed')`); err != nil {
				t.Fatalf("insert: %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("no database at the path we asked for: %v", err)
			}
		})
	}
}

// TestDSNEscapesOnlyWhatSQLiteConsumes pins the other half: an escape that is
// too broad is as wrong as one that is too narrow, and it fails on paths no
// test directory can be named after. A Windows filename cannot contain '?' or
// '*', and a UNC share cannot be created without a server, so these are
// asserted on the DSN string rather than by opening a file.
//
// The second assertion is the same class of bug one layer on: the DSN this
// produces is itself parsed - by checkPragmaNames here, and by the driver - by
// splitting at the first '?'. Escaping '?' in the path is what makes that split
// unambiguous.
func TestDSNEscapesOnlyWhatSQLiteConsumes(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"plain", `C:\Users\fixture\engramux.db`, `file:C:\Users\fixture\engramux.db`},
		{"percent", `C:\Users\100%done\engramux.db`, `file:C:\Users\100%25done\engramux.db`},
		{"hash", `C:\Users\hash#tag\engramux.db`, `file:C:\Users\hash%23tag\engramux.db`},
		{"valid escape", `C:\Users\pct%41hex\engramux.db`, `file:C:\Users\pct%2541hex\engramux.db`},
		{"question mark", `C:\Users\q?mark\engramux.db`, `file:C:\Users\q%3Fmark\engramux.db`},
		// Every character here is legal in a Windows filename and harmless in
		// the path component of a URI. Escaping any of them would be too broad.
		{"untouched", `C:\a&b (x)=y;z@w+v,u'-t\engramux.db`, `file:C:\a&b (x)=y;z@w+v,u'-t\engramux.db`},
		// SQLite reads an authority component only after "//", with forward
		// slashes, so a UNC path's leading backslashes are just path bytes.
		{"unc", `\\server\share\engramux\engramux.db`, `file:\\server\share\engramux\engramux.db`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dsn(c.path)
			if want := c.want + dsnParams; got != want {
				t.Errorf("dsn(%s)\n got %s\nwant %s", c.path, got, want)
			}
			if err := checkPragmaNames(got); err != nil {
				t.Errorf("the DSN for %s no longer parses: %v", c.path, err)
			}
		})
	}
}
