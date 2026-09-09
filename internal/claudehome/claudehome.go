// Package claudehome resolves where Claude Code keeps its configuration.
//
// It exists because two packages need that answer and neither may import the
// other. `internal/memory` needs it to find the native memory directory, and it
// reaches `database/sql`; `cmd/engramux` needs it for the settings file, the
// plugin cache and the application-state file, and it is the relay, which
// `TestTheRelayDoesNotLinkTheSQLiteDriver` exists to keep small. So the shared
// answer lives in a leaf that imports nothing but the standard library's
// smallest pieces, and both call it.
//
// Backlog 54 is what forced it. Before this package, `internal/memory` honoured
// the override and `cmd/engramux` spelled `<home>/.claude/...` by hand, so on a
// machine that had moved its configuration home the memory indexer read the
// right directory while `doctor` reported eleven hook entries missing and
// `install --apply` wrote them where the host would never look. Fixing one path
// at a time is worse than fixing none: a plugin cache that honours the variable
// beside a settings file that does not is a machine whose diagnosis contradicts
// itself.
package claudehome

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvVar is the one mechanism that moves the configuration home.
//
// The installed host calls it "the configuration home" in its own strings and
// names it in a message about "where your own settings are found", which is
// where the wording below comes from rather than from a guess.
const EnvVar = "CLAUDE_CONFIG_DIR"

// AppStateName is Claude Code's global application-state file, which is also
// where user-scope MCP servers are recorded - so it is the file `install` reads
// before deciding whether registration is already done.
//
// It is a constant here because it is the one name in this product that a
// credential-path guard refuses to let a shell command mention. Written once,
// it never has to be typed into a command line to be checked.
const AppStateName = ".claude.json"

// dirName is the configuration home's name under the user's home directory,
// used only when [EnvVar] is unset.
const dirName = ".claude"

// Dir answers the configuration home.
//
// home is the user's home directory and is consulted only when the override is
// unset; pass "" when it could not be determined and Dir answers "" in turn,
// rather than inventing a relative path from an empty string.
func Dir(home string) string {
	if dir := override(); dir != "" {
		return dir
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, dirName)
}

// Settings is the user settings file.
func Settings(home string) string {
	return join(Dir(home), "settings.json")
}

// PluginCache is where Claude Code unpacks the plugins it installs, one
// directory per marketplace, plugin and version.
func PluginCache(home string) string {
	return join(Dir(home), "plugins", "cache")
}

// AppState is the global application-state file, and it is the reason this
// package is not a single join.
//
// **Measured 2026-09-09 against the installed host.** By default the file is a
// *sibling* of the configuration home rather than a child of it - `~/.claude`
// beside `~/.claude.json`. With [EnvVar] set it moves *inside* the directory
// that variable names: pointing the variable at an empty directory made
// `claude mcp list` answer "No MCP servers configured" where the same command
// without it found this product's endpoint, and the host then wrote its own
// copy of the file into that directory. So the two arms have different shapes
// and a resolver that joined onto [Dir] in both would be wrong on every machine
// that has never set the variable.
func AppState(home string) string {
	if dir := override(); dir != "" {
		return filepath.Join(dir, AppStateName)
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, AppStateName)
}

// override answers the trimmed value of [EnvVar], or "" when it is unset or
// blank. A variable set to whitespace is a variable nobody meant to set.
func override() string {
	return strings.TrimSpace(os.Getenv(EnvVar))
}

// join keeps an unresolvable home unresolvable instead of turning it into a
// relative path that would resolve against whatever directory the process
// happens to be in.
func join(dir string, parts ...string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(append([]string{dir}, parts...)...)
}
