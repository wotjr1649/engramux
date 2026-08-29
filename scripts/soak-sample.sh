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
set -u

data="${LOCALAPPDATA}/engramux"
cli="${data}/bin/engramux.exe"
log="${SOAK_LOG:-$(git rev-parse --show-toplevel)/.capture/soak/soak.tsv}"

every=0
if [ "${1:-}" = "--every" ]; then
	every="${2:-1800}"
fi

sample() {
	local ts uptime events spool db wal rss handles threads status
	ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

	# One status read. It takes the same read gate a tool call takes, which
	# is the point: the sampler is a client like any other.
	status="$("$cli" status 2>/dev/null)" || status=""
	uptime="$(printf '%s\n' "$status" | awk '$1=="uptime"{print $2}')"
	events="$(printf '%s\n' "$status" | awk '$1=="events"{print $2}')"
	spool="$(printf '%s\n' "$status" | awk '$1=="spool"{print $2}')"

	db="$(stat -c %s "${data}/engramux.db" 2>/dev/null || echo 0)"
	wal="$(stat -c %s "${data}/engramux.db-wal" 2>/dev/null || echo 0)"

	# Working set, handle count and thread count. The handle and thread
	# counts are what a leaked pipe connection or a leaked goroutine shows up
	# as long before the working set moves; the working set is the only
	# instrument the MCP session map has, because SessionTimeout is zero on
	# purpose (spec 5.9) and nothing outside the process can count its
	# entries.
	read -r rss handles threads <<-EOF
		$(powershell -NoProfile -NonInteractive -Command \
			'$p = Get-Process engramux-service -ErrorAction SilentlyContinue | Select-Object -First 1; if ($p) { "{0} {1} {2}" -f $p.WorkingSet64, $p.HandleCount, $p.Threads.Count } else { "0 0 0" }' 2>/dev/null)
	EOF

	printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$ts" "${uptime:-down}" "${events:-0}" "${spool:-0}" \
		"$db" "$wal" "${rss:-0}" "${handles:-0}" "${threads:-0}" >>"$log"
}

mkdir -p "$(dirname "$log")"
if [ ! -s "$log" ]; then
	printf 'ts\tuptime\tevents\tspool\tdb_bytes\twal_bytes\trss_bytes\thandles\tthreads\n' >"$log"
fi

sample
tail -1 "$log"
if [ "$every" -gt 0 ]; then
	while sleep "$every"; do
		sample
		tail -1 "$log"
	done
fi
