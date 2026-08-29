#!/usr/bin/env bash
# One sample of the Phase 6 soak (spec 8, Phase 6), appended as one TSV line.
#
# Everything here is read from *outside* the service. The soak's precondition is
# that the binary stops changing, so nothing may be added to the process to
# instrument it - what can be observed is what the shipped `status` reply says,
# what the files on disk weigh, and what the OS reports about the process.
#
# Usage:
#   bash scripts/soak-sample.sh                 # one sample
#   bash scripts/soak-sample.sh --every 1800    # a sample every 30 minutes
#   SOAK_LOG=/some/path bash scripts/soak-sample.sh
#
# The default log is under .capture/, which is gitignored. The line itself
# carries no path - `status` masks the database path and nothing else here
# prints one - but the log is a measurement record, not a repository artefact.
#
# # Every failure here is loud, and that is the whole design
#
# This runs unattended for 72 hours and its output is the evidence a phase gate
# rests on. A sampler that keeps looping while writing nothing, or that writes a
# row saying something that did not happen, is worse than one that stops: the
# series looks complete and is not. So a missing prerequisite exits non-zero
# before the first sample, a write that fails exits rather than continues, and a
# reading that could not be taken is written as its own token and never as 0.
set -u

usage() {
	printf 'usage: %s [--every SECONDS]\n' "${0##*/}" >&2
	exit 2
}

# --every is accepted in both spellings because getting it wrong is silent
# otherwise: the earlier version read only `--every N`, so `--every=1800` left
# the loop off and produced one sample and exit 0, which is what an operator
# starting a 72-hour run would read as success.
every=0
case "${1:-}" in
"") ;;
--every)
	[ $# -ge 2 ] || usage
	every="$2"
	;;
--every=*) every="${1#--every=}" ;;
*) usage ;;
esac
case "$every" in
'' | *[!0-9]*) usage ;;
esac

data="${LOCALAPPDATA:-}"
if [ -z "$data" ]; then
	# A scheduled task runs with whatever environment it was given, and the
	# brief tells the user to create one. Without this the script would fail
	# on an unbound variable at the first sample instead of here.
	echo "soak-sample: LOCALAPPDATA is not set, so the data directory cannot be located" >&2
	exit 1
fi
data="$data/engramux"
cli="$data/bin/engramux.exe"

log="${SOAK_LOG:-}"
if [ -z "$log" ]; then
	# An empty repository root would make the default log path /.capture/...,
	# which in Git Bash lands under the MSYS root and not under this
	# repository - so the samples would go somewhere nobody looks.
	root="$(git rev-parse --show-toplevel 2>/dev/null)" || root=""
	if [ -z "$root" ]; then
		echo "soak-sample: not inside a git worktree and SOAK_LOG is not set" >&2
		exit 1
	fi
	log="$root/.capture/soak/soak.tsv"
fi

# write appends one line, and stops the run when it cannot. A loop that keeps
# going after a full disk or a locked file prints the previous last line on
# every iteration and looks alive.
write() {
	printf '%s\n' "$1" >>"$log" || {
		echo "soak-sample: cannot append to the log" >&2
		exit 1
	}
}

sample() {
	local ts uptime events spool db wal rss handles threads proc status
	ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

	# The process reading comes first, and the order is what lets the row
	# below tell a service that is not running from one that is running and
	# did not answer in time. Read after the status call, those are the same
	# cell.
	#
	# PowerShell answers `none` for "there is no such process" so that it is
	# distinguishable from "the reading could not be taken", which is empty
	# output. Collapsing the two into 0 is what the first version did, and a
	# 0 in a working-set series that otherwise reads 29,000,000 is a data
	# point rather than a gap.
	#
	# The handle and thread counts are what a leaked pipe connection or a
	# leaked goroutine shows as long before the working set moves; the
	# working set is the only instrument the MCP session map has, because
	# SessionTimeout is zero on purpose (spec 5.9) and nothing outside the
	# process can count its entries.
	proc="$(powershell -NoProfile -NonInteractive -Command \
		'$p = Get-Process engramux-service -ErrorAction SilentlyContinue | Select-Object -First 1; if ($p) { "{0} {1} {2}" -f $p.WorkingSet64, $p.HandleCount, $p.Threads.Count } else { "none" }' \
		2>/dev/null | tr -d '\r')" || proc=""
	case "$proc" in
	none) rss=0 handles=0 threads=0 ;;
	'') rss=- handles=- threads=- ;;
	*)
		read -r rss handles threads <<<"$proc"
		if [ -z "$rss" ] || [ -z "$handles" ] || [ -z "$threads" ]; then
			rss=- handles=- threads=-
		fi
		;;
	esac

	# One status read. It takes the same read gate a tool call takes, which
	# is the point: the sampler is a client like any other. It is also the
	# one thing here that can fail while the service is healthy - a cold read
	# on a large database can outlast the read deadline (spec 7.1) - so a
	# failure is recorded as what it is and the three fields it would have
	# filled are left unknown rather than zero.
	if status="$("$cli" status 2>/dev/null)"; then
		uptime="$(printf '%s\n' "$status" | awk '$1=="uptime"{print $2}')"
		events="$(printf '%s\n' "$status" | awk '$1=="events"{print $2}')"
		spool="$(printf '%s\n' "$status" | awk '$1=="spool"{print $2}')"
		if [ -z "$uptime" ] || [ -z "$events" ] || [ -z "$spool" ]; then
			# status answered in a shape this does not parse. Three
			# empty cells would be a row that says nothing and fails
			# nothing.
			uptime=parse-failed events=- spool=-
		fi
	elif [ "$rss" = "-" ]; then
		uptime=unknown events=- spool=-
	elif [ "$rss" = "0" ]; then
		uptime=down events=- spool=-
	else
		uptime=read-failed events=- spool=-
	fi

	db="$(stat -c %s "${data}/engramux.db" 2>/dev/null || echo -)"
	wal="$(stat -c %s "${data}/engramux.db-wal" 2>/dev/null || echo -)"

	# $$ is this script's own pid, constant for the life of an --every loop,
	# so it names the run that wrote the row. Two loops appending to one log
	# interleave silently otherwise - which happened, twice, in one session:
	# stopping a loop from outside killed the wrapper and left the bash, and
	# a loop keeps running the sample() it parsed at startup even after the
	# file is rewritten. Neither is preventable from inside the script.
	# Making the interleave legible is.
	write "$(printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s' \
		"$ts" "$uptime" "$events" "$spool" "$db" "$wal" "$rss" "$handles" "$threads" "$$")"
}

header="$(printf 'ts\tuptime\tevents\tspool\tdb_bytes\twal_bytes\trss_bytes\thandles\tthreads\tpid')"

mkdir -p "$(dirname "$log")" || exit 1
if [ ! -s "$log" ]; then
	write "$header"
elif [ "$(head -1 "$log")" != "$header" ]; then
	# An existing log with a different header is one this script's columns do
	# not line up with. Appending to it would produce a file that parses and
	# means two different things by column.
	echo "soak-sample: the log has a different header, so it was written by another version" >&2
	exit 1
fi

sample
tail -1 "$log"
if [ "$every" -gt 0 ]; then
	while sleep "$every"; do
		sample
		tail -1 "$log"
	done
fi
