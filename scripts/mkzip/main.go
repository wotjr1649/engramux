// Command mkzip writes a zip whose bytes depend on nothing but its input, and
// prints the SHA-256 of what it wrote.
//
// # Why this exists rather than a call to tar or Compress-Archive
//
// The release procedure commits the archive's hash beside the version it names
// and the workflow re-builds the archive and refuses the release when the two
// disagree. That check is only meaningful if two builds of one commit produce
// the same bytes, and neither obvious tool gives that. Windows' bsdtar stamps
// each entry with the file's real modification time, so the same directory
// zipped twice is two different archives. PowerShell's Compress-Archive has
// shipped versions that write path separators as backslashes, which is a
// portability bug rather than a determinism one but is no better a foundation.
// archive/zip is in the standard library, takes the timestamp as an argument,
// and is already a dependency of every build.
//
// # What determinism costs here, and what it does not cover
//
// Entries are sorted, every timestamp is [epoch], and no header field is read
// from the filesystem - so the archive is a function of the file contents and
// their names. What it does not cover is the contents: two builds of the two
// binaries agree only because the release build is -trimpath, CGO_ENABLED=0,
// -buildvcs=false and a toolchain pinned by go.mod. -buildvcs=false is the one
// that is not obvious. Without it the binary records the commit it was built
// at, and the release procedure builds before committing the hash, so the local
// build would carry the parent commit and the workflow's would carry the tag -
// two different binaries from one source tree. scripts/package.sh owns that
// line; this program owns only the container.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// epoch is the modification time every entry carries.
//
// 1980-01-01 is the zip format's own epoch - the earliest instant its MS-DOS
// time fields can encode - so it is the one value that needs no explanation
// when somebody reads it out of an archive and wonders what happened.
var epoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mkzip <directory> <out.zip>")
		os.Exit(2)
	}
	sum, err := run(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkzip: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(sum)
}

// run zips dir into out and answers the hex SHA-256 of the archive.
//
// The hash is taken from the bytes on their way to the file rather than by
// reading the file back, so what is reported is what was written and not what
// a second read happened to find.
func run(dir, out string) (string, error) {
	names, err := entries(dir)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", fmt.Errorf("%s holds no files", dir)
	}

	// #nosec G304 G703 -- out is this program's own second argument. The caller is
	// scripts/package.sh or the release workflow; there is no untrusted path
	// anywhere in this program's input.
	f, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	sum := sha256.New()
	zw := zip.NewWriter(io.MultiWriter(f, sum))
	for _, name := range names {
		if err := add(zw, dir, name); err != nil {
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// entries lists every regular file under dir as a slash-separated path relative
// to it, sorted.
//
// Sorted because [filepath.WalkDir] already walks in lexical order and relying
// on that would make the guarantee somebody else's; slices.Sort here says the
// order is this program's own. Directories produce no entry of their own: a zip
// reader creates them from the paths, and an empty directory is not something
// this archive ever carries.
func entries(dir string) ([]string, error) {
	var out []string
	// #nosec G703 -- dir is this program's own first argument, for the same
	// reason as above.
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("%s is not a regular file", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(out)
	return out, nil
}

// add writes one file into the archive.
//
// The header is built field by field rather than through [zip.FileInfoHeader],
// which would read the mode and the modification time off the filesystem and
// put the machine back into the bytes.
func add(zw *zip.Writer, dir, name string) error {
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: epoch,
	})
	if err != nil {
		return err
	}
	// #nosec G304 G703 -- name came from walking dir, which is this program's own
	// first argument.
	src, err := os.Open(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	if _, err := io.Copy(w, src); err != nil {
		return err
	}
	return src.Close()
}
