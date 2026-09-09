## What changed since 0.1.1

**`engramux update` with no arguments now reads Claude Code's plugin cache.** Until this, updating
the plugin moved bytes into a directory nothing read: the host fetched the release archive, checked
its SHA-256 and unpacked it, and the service went on running the build it already had until somebody
typed the path out by hand. A delivery channel that delivers where nobody looks is not one.

It takes the newest version in the cache that carries both binaries — a half-removed one is not
offered, because the host keeps an old version for about a fortnight — and it names that version
before it stops anything. **An explicit `--from` still outranks it**, which is what keeps a `dist/`
build from being undone by the released one, and a `--from` that names nothing is still refused
rather than quietly falling back. With no plugin installed it says how to install one.

**One catch, and it is unavoidable: this only works once *this* build is the installed one.** Take
it the old way once —

    engramux update --from %USERPROFILE%\.claude\plugins\cache\engramux\engramux\0.1.2

— and from then on `engramux update` is the whole command.

**`CLAUDE_CONFIG_DIR` is now honoured, and backlog 54 is closed.** Every path this product derives
under the Claude Code configuration home moved with it: the settings file, the plugin cache, and the
global application-state file that `install` reads before deciding whether registration is already
done. Previously only the native-memory reader honoured the variable, so on a machine that had moved
its configuration home `doctor` reported the eleven hook entries missing and `install --apply` wrote
them where the host would never look.

The rule has two shapes and that is measured rather than assumed. By default the application-state
file is a *sibling* of the configuration home; with the variable set it moves *inside* the directory
that variable names. A resolver that joined onto the home in both arms would have looked obviously
right and broken every machine that has never set the variable.

## Still true from 0.1.1

`bin/engramux` makes the CLI a bare command inside Claude Code, as a shim onto the installed binary
rather than a second copy of it. Installing still puts nothing on your own shell's `PATH`; the README
says where the binaries land and warns that `setx` truncates `PATH` at 1024 characters. Hook-time
context injection is built and ships disabled.
