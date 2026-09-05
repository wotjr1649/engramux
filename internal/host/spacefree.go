package host

import (
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// spaceFree answers a spelling of path that carries no space, or path itself
// when Windows has none to give.
//
// # Why a path with a space needs one at all
//
// A Codex hook command is source text for a shell rather than an argument
// vector (spec 4.2). Which shell is not this product's decision and not
// knowable from here: codex-rs's hooks/src/engine/command_runner.rs runs
// COMSPEC with /C when no shell is configured and the session's own snapshotted
// shell otherwise. **Measured 2026-09-05** against a stub over cmd.exe,
// powershell.exe and pwsh.exe: the plain path is the only spelling cmd.exe
// runs and the only one PowerShell will not run when it has a space in it, the
// quoted path is printed rather than run by both PowerShells - which is backlog
// 50's whole defect - and `& "path"` is a syntax error in cmd.exe. **No
// quoting serves both shells.** What serves both is a path that needs none.
//
// # Only the part that has to change changes
//
// The rewrite covers the shortest prefix that holds every space, and nothing
// below it. The space in a real installation is the Windows account name, which
// %LOCALAPPDATA% carries, so the prefix ends well above this product's own
// directory - and that matters rather than being tidiness: [isEngramux]
// recognises this product's hooks by the word engramux in the command, and an
// 8.3 rewrite that reached `engramux\bin\engramux.exe` could collide its way to
// `ENGRAM~1` and make the next install add a second hook beside this one.
//
// # What is left open
//
// 8.3 name generation is per-volume and can be turned off, and a directory
// created while it was off has no short name at all. Then this returns the path
// it was given, the entry is written the way it has always been written, and a
// Codex whose snapshotted shell is PowerShell captures nothing - which
// `doctor`'s `codex received` line reports as nothing having arrived. Backlog
// 51 records that: neither remaining spelling is right for both shells, so
// there is nothing to choose between them without a machine to measure on.
func spaceFree(path string) string {
	p := filepath.FromSlash(path)
	space := strings.LastIndex(p, " ")
	if space < 0 {
		return path
	}
	prefix, rest := p, ""
	if end := strings.IndexAny(p[space:], `\/`); end >= 0 {
		prefix, rest = p[:space+end], p[space+end:]
	}
	short, err := shortName(prefix)
	if err != nil || strings.Contains(short, " ") {
		return path
	}
	return short + rest
}

// shortName asks Windows for the 8.3 spelling of a path that exists.
//
// The path has to exist, which is why [spaceFree] asks about a prefix and not
// about the relay: [Install] plans both host files before it copies a single
// binary, so on a first install neither the relay nor the directory holding it
// is there yet. The account directory is.
func shortName(path string) (string, error) {
	in, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	// The length is passed as the constant it is rather than as len(buf),
	// so the conversion the API needs is a constant one: gosec reads
	// uint32(len(...)) as an overflow whatever the slice was made from.
	const buflen = windows.MAX_LONG_PATH
	buf := make([]uint16, buflen)
	n, err := windows.GetShortPathName(in, &buf[0], buflen)
	if err != nil {
		return "", err
	}
	if n == 0 || n > buflen {
		return "", errors.New("host: GetShortPathName wants a buffer longer than a Windows path")
	}
	return windows.UTF16ToString(buf[:n]), nil
}
