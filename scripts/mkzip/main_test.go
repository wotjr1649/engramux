package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// plant writes a small tree and answers the directory holding it. The files are
// created in an order that is not their sorted order, because that is the only
// way the sorting assertion below can fail.
func plant(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("plant %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte("contents of "+name), 0o600); err != nil {
			t.Fatalf("plant %s: %v", name, err)
		}
	}
	return dir
}

// TestTheArchiveIsAFunctionOfItsInput is the invariant the release procedure
// rests on, and it is worth stating why a test that zips twice and compares the
// two hashes would not be it.
//
// The obvious mutation - stamping the entries with the wall clock instead of
// [epoch] - is one this machine's clock can hide: a formatted timestamp here
// moves about every 15.6 ms, so two runs a few milliseconds apart can carry the
// same one and a comparison of two hashes would pass with the mutation in
// place. So the timestamp is asserted to be exactly the epoch, read back out of
// the archive, which nothing about the clock can satisfy by accident. The
// two-run comparison is here as well, for everything a single run cannot see.
func TestTheArchiveIsAFunctionOfItsInput(t *testing.T) {
	want := []string{".claude-plugin/plugin.json", "LICENSE", "README.md", "engramux.exe"}

	first := filepath.Join(t.TempDir(), "first.zip")
	sum, err := run(plant(t, "engramux.exe", "README.md", ".claude-plugin/plugin.json", "LICENSE"), first)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	zr, err := zip.OpenReader(first)
	if err != nil {
		t.Fatalf("open the archive: %v", err)
	}
	defer func() { _ = zr.Close() }()

	var got []string
	for _, f := range zr.File {
		got = append(got, f.Name)
		if !f.Modified.Equal(epoch) {
			t.Errorf("%s carries %s, want %s - the archive took a timestamp off the machine",
				f.Name, f.Modified.UTC(), epoch)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("entries = %q, want %q", got, want)
	}

	// The reported hash is the archive's, and not of some other bytes: it is
	// taken on the way to the file, so a writer that reported a digest of
	// what it meant to write would pass everything above.
	// #nosec G304 G703 -- first is a path this test just built under t.TempDir.
	onDisk, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read the archive: %v", err)
	}
	if digest := sha256.Sum256(onDisk); hex.EncodeToString(digest[:]) != sum {
		t.Errorf("run reported %s; the file on disk hashes to %s", sum, hex.EncodeToString(digest[:]))
	}

	// A second tree with the same contents, written in a different order and
	// under a different parent, produces the same archive.
	second := filepath.Join(t.TempDir(), "second.zip")
	again, err := run(plant(t, "LICENSE", ".claude-plugin/plugin.json", "README.md", "engramux.exe"), second)
	if err != nil {
		t.Fatalf("run again: %v", err)
	}
	if again != sum {
		t.Errorf("two runs over one input gave %s and %s", sum, again)
	}
}

// TestAnEmptyDirectoryIsRefused. A release whose archive holds nothing is a
// release that installs nothing, and an empty zip is a valid file - so without
// this the failure would arrive at whoever downloaded it.
func TestAnEmptyDirectoryIsRefused(t *testing.T) {
	if _, err := run(t.TempDir(), filepath.Join(t.TempDir(), "empty.zip")); err == nil {
		t.Error("run accepted a directory with no files in it")
	}
}
