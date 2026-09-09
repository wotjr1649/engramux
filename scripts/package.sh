#!/usr/bin/env bash
# Builds a release and the archive that carries it, and prints the archive's
# SHA-256 on stdout.
#
# Everything else goes to stderr, so a caller can take the hash with a plain
# command substitution and a person running it still sees what happened. The
# release workflow is that caller: it re-runs this at the tagged commit and
# refuses the release when the hash it gets back is not the one the marketplace
# entry already names. That check is what replaces a workflow with write access
# to `main` - the hash is committed by the owner, beside the version it names,
# and CI verifies rather than writes.
#
# Usage: scripts/package.sh <version>          # 0.1.0, not v0.1.0
#
# # The two things that make the hash reproducible, and neither is obvious
#
# `-buildvcs=false`. Without it the binary records the commit it was built at,
# and the release order is build, then commit the hash, then tag - so the local
# build would carry the parent commit and the workflow's would carry the tag,
# and two builds of one source tree would not agree. What is lost is the
# `vcs.revision` stamp that `-trimpath` otherwise leaves behind; what replaces
# it is the tag, which names the commit on the release page.
#
# The version reaches the archive through the link flag alone. `plugin.json`
# carries no `version` field, and that is deliberate rather than an omission:
# Claude Code takes the version from the plugin manifest, else the marketplace
# entry, else the archive's digest, so the entry answering it is supported. Had
# this script rewritten `plugin.json` before staging it, the archive's bytes
# would depend on whichever `jq` was on the machine, and two `jq` versions that
# indent differently would produce two different hashes from one commit.
#
# # What it does not do
#
# It does not tag, does not create a release and does not push. Those are
# publication acts and they are the owner's own hands (memory spec M-7, and the
# plan's Step 6 rev.6). It also does not clean up its staging directory: that is
# a `mktemp -d` under `dist/`, which is gitignored, and a script that removes
# directories is a worse thing to have around than a few empty ones.
set -euo pipefail

module=github.com/wotjr1649/engramux
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

v="${1:-}"
if [ -z "$v" ]; then
    echo "usage: scripts/package.sh <version>    # 0.1.0, not v0.1.0" >&2
    exit 2
fi
# Semantic versioning's own shape, minus the build-metadata field nothing here
# uses. The leading `v` is refused rather than stripped: it is the tag's, not
# the version's, and accepting both spellings is how the two drift apart.
if ! printf '%s' "$v" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
    echo "package.sh: '$v' is not a semantic version, or carries a leading v" >&2
    exit 1
fi

cd "$repo"
mkdir -p dist
# Relative paths from here down, and that is not style. This shell converts an
# absolute POSIX path on its way into a Windows program, so an absolute
# /tmp/... handed to `go run` would be resolved against the current drive and
# land somewhere else entirely. A relative path is not converted and both
# shells resolve it the same way.
stage="$(mktemp -d -p dist stage.XXXXXX)"
archive="dist/engramux-$v-windows-amd64.zip"

echo "package.sh: building $v into $stage" >&2
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X $module/internal/version.linked=$v" \
    -o "$stage/engramux.exe" ./cmd/engramux
# -H=windowsgui, or the service pops a console window every time it spawns a
# child (spec 5.1).
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
    -ldflags "-s -w -H=windowsgui -X $module/internal/version.linked=$v" \
    -o "$stage/engramux-service.exe" ./cmd/engramux-service

# Ask the binary what version it is, because nothing else can. `go tool link -X`
# sets a package-level string variable, and against a const, a mistyped import
# path or a non-constant initializer it does nothing at all and reports nothing
# - there is no linker error. `go version -m` is not the check either: -trimpath
# removes the recorded -ldflags line entirely, measured 2026-09-04. The shipped
# surface is the check, and this is it.
#
# `|| true` because doctor's exit code is about the installation and not about
# this, and pipefail would otherwise make a machine with no service a build
# failure. The report itself is never printed: it carries this machine's state.
report="$("$stage/engramux.exe" doctor 2>&1 || true)"
linked="$(printf '%s\n' "$report" | awk '$1 == "version" { sub(/,$/, "", $2); print $2; exit }')"
if [ "$linked" != "$v" ]; then
    echo "package.sh: the binary reports '${linked:-nothing}', not '$v' - the -X flag did not take" >&2
    exit 1
fi
echo "package.sh: the binary reports $linked" >&2

mkdir -p "$stage/.claude-plugin"
cp .claude-plugin/plugin.json "$stage/.claude-plugin/plugin.json"
cp README.md LICENSE "$stage/"
# `skills/` reaches the host through the archive and through nothing else.
# Claude Code discovers a plugin's skills from this directory with no manifest
# field declaring it - verified 2026-09-08 against the installed `codex` and
# `superpowers` plugins, whose manifests declare none and whose skills the host
# had loaded - so leaving it out of the staging list is how the search surface
# stays unreachable without anything failing.
cp -r skills "$stage/skills"

sha="$(go run ./scripts/mkzip "$stage" "$archive")"
echo "package.sh: $archive" >&2
echo "package.sh: sha256 $sha" >&2

# The marketplace entry, written here so that the version, the URL and the hash
# reach one commit together - which is the whole of M-7's "cannot be committed
# apart". jq rather than sed: this is JSON, and a regular expression that edits
# it is a bug waiting for the first line to move.
url="https://github.com/wotjr1649/engramux/releases/download/v$v/engramux-$v-windows-amd64.zip"
# `tr -d` because jq on Windows writes its stdout in text mode: measured
# 2026-09-07, the first line of the file it produced here was `{\r\n`. It is
# cosmetic in the commit - .gitattributes says `*.json text eol=lf`, so the blob
# is normalised on `add` and the diff is right - and it is not cosmetic in the
# working copy, where a mixed tree makes an end-of-line-anchored pattern behave
# differently on the two halves of it.
jq --arg v "$v" --arg url "$url" --arg sha "$sha" \
    '(.plugins[] | select(.name == "engramux")) |= (.version = $v | .source.url = $url | .source.sha256 = $sha)' \
    .claude-plugin/marketplace.json | tr -d '\r' > .claude-plugin/marketplace.json.tmp
mv .claude-plugin/marketplace.json.tmp .claude-plugin/marketplace.json
# Read it back, because the edit above can succeed at doing nothing: `|=` on a
# `select` that matches no entry rewrites nothing, exits 0, and would leave this
# script announcing a version it did not write. Verify the insertion, never the
# exit code.
if [ "$(jq -r '.plugins[] | select(.name == "engramux") | .source.sha256' .claude-plugin/marketplace.json)" != "$sha" ]; then
    echo "package.sh: the catalogue does not name $sha after the rewrite - is there an entry named engramux in it?" >&2
    exit 1
fi
echo "package.sh: .claude-plugin/marketplace.json now names $v" >&2
echo "package.sh: review it, commit it, and tag that commit v$v" >&2

echo "$sha"
