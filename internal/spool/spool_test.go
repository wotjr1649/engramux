package spool

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const testID = "0192f0c0-0000-7000-8000-000000000001"

// entries returns the names of every file in dir, so a test can assert what
// Write left behind rather than only that the record it wanted exists. A
// leftover temp file is a real failure - the drain must see one record per
// completed write and nothing else.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read spool dir: %v", err)
	}
	names := make([]string, 0, len(des))
	for _, de := range des {
		names = append(names, de.Name())
	}
	return names
}

// TestWriteNamesTheRecordAfterTheID is the assertion the relay's whole
// idempotency story rests on (I-05): the record's identity is the id the relay
// minted, and it is carried by the file name, so a body that will not parse
// still has an id.
func TestWriteNamesTheRecordAfterTheID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spool")
	payload := []byte(`{ "hook_event_name": "SessionEnd", "cwd": "C:\\Users\\x" }`)

	if err := Write(dir, testID, payload, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}

	names := entries(t, dir)
	if len(names) != 1 || names[0] != testID+ext {
		t.Fatalf("spool dir holds %q, want exactly [%q]", names, testID+ext)
	}

	//nolint:gosec // G304: reading a record this test just wrote into its own t.TempDir
	got, err := os.ReadFile(filepath.Join(dir, testID+ext))
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("record bytes\n got %q\nwant %q", got, payload)
	}
}

// TestWritePreservesBytesExactly guards the reason the record is the raw
// payload and not a re-encoded document. Phase 1 gates on a byte-for-byte
// round trip, and stdin is whatever the host wrote - which is not always
// valid JSON and not always valid UTF-8.
func TestWritePreservesBytesExactly(t *testing.T) {
	for name, payload := range map[string][]byte{
		"empty":        {},
		"not json":     []byte("this is not json"),
		"invalid utf8": {0x7b, 0x22, 0x61, 0x22, 0x3a, 0x22, 0xff, 0xfe, 0x22, 0x7d},
		"embedded nul": {0x7b, 0x00, 0x7d},
		"html chars":   []byte(`{"a":"<b>&</b>"}`),
		"whitespace":   []byte("{\n\t\"a\" : 1\n}\n"),
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := Write(dir, testID, payload, nil); err != nil {
				t.Fatalf("Write: %v", err)
			}
			//nolint:gosec // G304: reading a record this test just wrote into its own t.TempDir
			got, err := os.ReadFile(filepath.Join(dir, testID+ext))
			if err != nil {
				t.Fatalf("read record: %v", err)
			}
			if string(got) != string(payload) {
				t.Errorf("record bytes\n got %x\nwant %x", got, payload)
			}
		})
	}
}

// TestWriteRejectsAnIDThatIsNotAUUID stops the id from being a path. It is
// minted by uuid.NewV7 today, so this never fires in production - but the id
// is concatenated into a file name, and "..\\..\\x" is a working directory
// escape. Removing the shape is cheaper than trusting every future caller.
//
// The last four are the ones a bare uuid.Validate accepts and this package must
// not: they are the same UUID spelled four other ways, and each one either
// cannot be a Windows file name or reaches events.id as a different string from
// the canonical spelling - two rows for one event, which is I-05 broken. See
// canonicalUUID.
func TestWriteRejectsAnIDThatIsNotAUUID(t *testing.T) {
	for _, id := range []string{
		"",
		"..",
		`..\..\escape`,
		"../../escape",
		"not-a-uuid",
		"0192f0c0-0000-7000-8000-00000000000",   // 35 chars
		"0192f0c0-0000-7000-8000-0000000000011", // 37 chars
		"0192f0c0_0000_7000_8000_000000000001",
		"urn:uuid:0192f0c0-0000-7000-8000-000000000001",
		"{0192f0c0-0000-7000-8000-000000000001}",
		"0192f0c0000070008000000000000001",
		"0192F0C0-0000-7000-8000-000000000001",
	} {
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			err := Write(dir, id, []byte(`{}`), nil)
			if !errors.Is(err, ErrID) {
				t.Fatalf("Write(%q, nil) error = %v, want ErrID", id, err)
			}
			if names := entries(t, dir); len(names) != 0 {
				t.Errorf("Write(%q, nil) left %q behind, want nothing", id, names)
			}
		})
	}
}

// TestWriteCreatesTheSpoolDirectory: the relay is a fresh process on a machine
// that may never have spooled anything, and an event that cannot be delivered
// must not also fail to be saved because a directory was missing (I-04).
func TestWriteCreatesTheSpoolDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "spool")
	if err := Write(dir, testID, []byte(`{}`), nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("spool dir holds %q, want one record", names)
	}
}

// TestWriteTwiceUnderOneIDLeavesOneRecord. The id is the idempotency key, so
// two writes under it are one record by definition - not two files the drain
// would replay twice.
func TestWriteTwiceUnderOneIDLeavesOneRecord(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, testID, []byte(`{"n":1}`), nil); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := Write(dir, testID, []byte(`{"n":2}`), nil); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("spool dir holds %q, want one record", names)
	}
	//nolint:gosec // G304: reading a record this test just wrote into its own t.TempDir
	got, err := os.ReadFile(filepath.Join(dir, testID+ext))
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	if string(got) != `{"n":2}` {
		t.Errorf("record bytes = %q, want the second write", got)
	}
}

// TestDirIsUnderTheLocalApplicationDataDirectory pins spec 5.6's placement,
// and pins the seam the relay's subprocess tests steer with: os.UserCacheDir
// reads %LocalAppData% on Windows, so a child process given a different
// LOCALAPPDATA spools somewhere a test can read. Nothing in production code
// knows that is happening.
func TestDirIsUnderTheLocalApplicationDataDirectory(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	want := filepath.Join(local, "engramux", "spool")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}
