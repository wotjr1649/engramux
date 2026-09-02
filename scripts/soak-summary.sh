#!/usr/bin/env bash
# Summarise the Phase 6 soak series (spec 8, Phase 6 row; spec 7.3's soak row)
# from the sampler's TSV, reading from the first row after the *last* uptime
# reset. Everything before that row is a previous service instance and not
# the series - and a reset inside the window is a restart, which is exactly
# what the `resets` figure is there to show.
#
# Prints the figures the post-soak write-up needs, as key<TAB>value lines, and
# nothing else: no path, no pid. A reading the sampler wrote as `-` is skipped,
# never read as 0.
#
# Usage:
#   bash scripts/soak-summary.sh                                 # the live log, .capture/soak/soak.tsv
#   bash scripts/soak-summary.sh docs/evidence/soak/soak.tsv     # the series that closed Phase 6
#   bash scripts/soak-summary.sh --selftest                      # exit 0 when the parser agrees
set -u

summarise() {
	awk -F'\t' '
	# Go duration "69h31m27.116s" -> seconds.
	function secs(d,   s, n, u) {
		s = 0
		while (match(d, /^[0-9.]+[hms]/)) {
			n = substr(d, 1, RLENGTH - 1)
			u = substr(d, RLENGTH, 1)
			s += (u == "h") ? n * 3600 : (u == "m") ? n * 60 : n
			d = substr(d, RLENGTH + 1)
		}
		return s
	}
	# "2026-09-02T01:53:01Z" -> epoch seconds (the zone cancels in differences).
	function epoch(t) {
		return mktime(substr(t, 1, 4) " " substr(t, 6, 2) " " substr(t, 9, 2) " " substr(t, 12, 2) " " substr(t, 15, 2) " " substr(t, 18, 2))
	}
	function lo(a, v) { return (a == "" || v + 0 < a + 0) ? v : a }
	function hi(a, v) { return (a == "" || v + 0 > a + 0) ? v : a }
	function out(k, v) { printf "%s\t%s\n", k, v }
	NR == 1 { next }
	{ row[NR] = $0 }
	$2 ~ /^[0-9]/ {
		u = secs($2)
		if (prev != "" && u < prev) { start = NR; resets++ }
		if (start == "") start = NR
		prev = u
	}
	END {
		if (start == "") { print "soak-summary: no numeric uptime in the log" > "/dev/stderr"; exit 1 }
		for (i = start; i <= NR; i++) {
			if (!(i in row)) continue
			n = split(row[i], f, "\t")
			if (n < 10) { malformed++; continue }
			rows++
			ts = f[1]; up = f[2]
			if (first_ts == "") first_ts = ts
			last_ts = ts
			e = epoch(ts)
			if (last_e != "" && e - last_e > 45 * 60) gaps++
			last_e = e
			if (up ~ /^[0-9]/) { ok++; last_up = secs(up) } else state[up]++
			if (f[3] != "-") { if (ev_first == "") ev_first = f[3]; ev_last = f[3] }
			if (f[4] != "-") { spool_max = hi(spool_max, f[4]); if (f[4] + 0 > 0) spool_nonzero++ }
			if (f[5] != "-") { if (db_first == "") db_first = f[5]; db_last = f[5]; db_min = lo(db_min, f[5]); db_max = hi(db_max, f[5]) } else db_unread++
			if (f[6] != "-") { wal_min = lo(wal_min, f[6]); wal_max = hi(wal_max, f[6]); if (f[6] + 0 > 0) wal_nonzero++ } else wal_unread++
			if (f[7] != "-") { if (rss_first == "") rss_first = f[7]; rss_last = f[7]; rss_min = lo(rss_min, f[7]); rss_max = hi(rss_max, f[7]) } else proc_unread++
			if (f[8] != "-") { h_min = lo(h_min, f[8]); h_max = hi(h_max, f[8]) }
			if (f[9] != "-") { t_min = lo(t_min, f[9]); t_max = hi(t_max, f[9]) }
			if (f[10] in pid) pid_repeats++
			pid[f[10]] = 1
		}
		span_h = (epoch(last_ts) - epoch(first_ts)) / 3600
		out("series_first_ts", first_ts)
		out("series_last_ts", last_ts)
		out("series_span_h", sprintf("%.2f", span_h))
		out("rows", rows)
		out("rows_malformed", malformed + 0)
		out("gaps_over_45min", gaps + 0)
		out("resets_in_file", resets + 0)
		out("uptime_last_h", sprintf("%.2f", last_up / 3600))
		out("uptime_past_72h", (last_up >= 72 * 3600) ? "yes" : "no")
		out("state_ok", ok + 0)
		out("state_read-failed", state["read-failed"] + 0)
		out("state_down", state["down"] + 0)
		out("state_unknown", state["unknown"] + 0)
		out("state_parse-failed", state["parse-failed"] + 0)
		out("events_first", ev_first)
		out("events_last", ev_last)
		out("spool_max", spool_max)
		out("spool_rows_nonzero", spool_nonzero + 0)
		out("db_first_bytes", db_first)
		out("db_last_bytes", db_last)
		out("db_min_bytes", db_min)
		out("db_max_bytes", db_max)
		out("db_growth_mb_per_h", (span_h > 0) ? sprintf("%.3f", (db_last - db_first) / 1e6 / span_h) : "-")
		out("db_rows_unread", db_unread + 0)
		out("wal_min_bytes", wal_min)
		out("wal_max_bytes", wal_max)
		out("wal_rows_nonzero", wal_nonzero + 0)
		out("wal_rows_unread", wal_unread + 0)
		out("rss_first_bytes", rss_first)
		out("rss_last_bytes", rss_last)
		out("rss_min_bytes", rss_min)
		out("rss_max_bytes", rss_max)
		out("proc_rows_unread", proc_unread + 0)
		out("handles_min", h_min)
		out("handles_max", h_max)
		out("threads_min", t_min)
		out("threads_max", t_max)
		out("pid_repeats", pid_repeats + 0)
	}' "$1"
}

if [ "${1:-}" = "--selftest" ]; then
	# A pre-reboot tail, a reset, one read-failed row and one gap. The parser
	# has to find the reset, skip what precedes it, and keep `-` out of the
	# arithmetic.
	fixture="$(mktemp)"
	printf 'ts\tuptime\tevents\tspool\tdb_bytes\twal_bytes\trss_bytes\thandles\tthreads\tpid\n' >"$fixture"
	printf '2026-01-01T00:00:00Z\t50h0m0s\t100\t0\t1000\t0\t500\t10\t5\t1\n' >>"$fixture"
	printf '2026-01-01T00:30:00Z\t20m4s\t100\t0\t1000\t4000\t500\t10\t5\t2\n' >>"$fixture"
	printf '2026-01-01T01:00:00Z\tread-failed\t-\t-\t1200\t-\t600\t12\t6\t3\n' >>"$fixture"
	printf '2026-01-01T02:30:00Z\t72h0m0.5s\t150\t0\t3000\t2000\t550\t11\t5\t4\n' >>"$fixture"
	got="$(summarise "$fixture")"
	rm -f "$fixture"
	fail=0
	for want in "rows	3" "resets_in_file	1" "state_read-failed	1" "state_ok	2" "gaps_over_45min	1" \
		"uptime_past_72h	yes" "wal_max_bytes	4000" "wal_rows_unread	1" "db_growth_mb_per_h	0.001" \
		"events_first	100" "events_last	150" "pid_repeats	0" "series_span_h	2.00"; do
		printf '%s\n' "$got" | grep -qxF "$want" || { echo "soak-summary selftest: missing <$want>" >&2; fail=1; }
	done
	[ "$fail" -eq 0 ] && echo "soak-summary selftest: ok"
	exit "$fail"
fi

log="${1:-}"
if [ -z "$log" ]; then
	root="$(git rev-parse --show-toplevel 2>/dev/null)" || root=""
	[ -n "$root" ] || { echo "soak-summary: not inside a git worktree and no log given" >&2; exit 1; }
	log="$root/.capture/soak/soak.tsv"
fi
[ -s "$log" ] || { echo "soak-summary: the log is missing or empty" >&2; exit 1; }
summarise "$log"
