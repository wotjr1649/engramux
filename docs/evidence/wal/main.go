// Two things the checkpoint policy in the design rests on, neither measured before:
//
//  1. How fast the WAL actually grows per event, using the real payload size
//     distribution from the capture corpus (p50 1182 B, p90 7418 B, p99 32970 B).
//     The old claim was a bare "WAL grows to 491 MB", with no rate and no
//     reproduction, which is unusable for sizing a checkpoint interval.
//
//  2. How long a TRUNCATE checkpoint blocks the single connection at a given WAL
//     size. This is the number that decides whether checkpointing can run
//     opportunistically: the relay's entire budget is 1s, and busy_timeout is
//     10s, so a checkpoint that blocks for seconds pushes every concurrent
//     relay into the spool.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Sizes sampled from the real corpus, weighted to reproduce its shape.
var sizeBuckets = []struct {
	bytes  int
	weight int
}{
	{200, 25}, {1182, 35}, {3658, 20}, {7418, 12}, {20000, 6}, {32970, 2},
}

func payload(i int) string {
	total := 0
	for _, b := range sizeBuckets {
		total += b.weight
	}
	pick := (i * 37) % total // deterministic, no rand: the harness must be reproducible
	for _, b := range sizeBuckets {
		if pick < b.weight {
			return strings.Repeat("x", b.bytes)
		}
		pick -= b.weight
	}
	return strings.Repeat("x", 1182)
}

func walSize(path string) int64 {
	fi, err := os.Stat(path + "-wal")
	if err != nil {
		return 0
	}
	return fi.Size()
}

func main() {
	dir, _ := os.MkdirTemp("", "waltest")
	path := filepath.Join(dir, "t.db")

	db, err := sql.Open("sqlite", "file:"+path+
		"?_pragma=journal_mode(wal)&_pragma=locking_mode(exclusive)"+
		"&_pragma=synchronous(2)&_pragma=busy_timeout(10000)"+
		"&_pragma=wal_autocheckpoint(0)&_txlock=immediate")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE events(
		id INTEGER PRIMARY KEY, host TEXT NOT NULL, payload TEXT NOT NULL, ts INTEGER NOT NULL)`); err != nil {
		panic(err)
	}
	// Confirm auto-checkpoint really is off; a silently ignored pragma would
	// make this whole measurement a lie.
	var ac int
	_ = db.QueryRow(`PRAGMA wal_autocheckpoint`).Scan(&ac)
	fmt.Printf("wal_autocheckpoint = %d (must be 0 for this measurement to mean anything)\n\n", ac)

	const n = 20000
	fmt.Printf("%-10s %-14s %-16s %s\n", "events", "WAL bytes", "B/event", "elapsed")
	start := time.Now()
	for i := 1; i <= n; i++ {
		if _, err := db.Exec(`INSERT INTO events(host,payload,ts) VALUES(?,?,?)`,
			"claude-code", payload(i), i); err != nil {
			fmt.Println("insert:", err)
			return
		}
		if i%2500 == 0 {
			w := walSize(path)
			fmt.Printf("%-10d %-14s %-16.0f %v\n", i, fmtB(w), float64(w)/float64(i),
				time.Since(start).Round(time.Millisecond))
		}
	}

	w := walSize(path)
	perEvent := float64(w) / float64(n)
	fmt.Printf("\nWAL growth  %.0f B/event\n", perEvent)
	// Extrapolate at the measured real ingest rate: 2 events/s across 8 sessions.
	for _, rate := range []float64{0.247, 2.0, 24.7} {
		perHour := perEvent * rate * 3600
		fmt.Printf("  at %5.2f ev/s -> %8s/hour, reaches 64 MiB in %6.1f h\n",
			rate, fmtB(int64(perHour)), 64*1024*1024/perHour)
	}

	fmt.Printf("\ncheckpoint cost at WAL = %s\n", fmtB(w))
	for _, mode := range []string{"PASSIVE", "TRUNCATE"} {
		before := walSize(path)
		t := time.Now()
		var busy, logf, ckpt int
		err := db.QueryRow(fmt.Sprintf(`PRAGMA wal_checkpoint(%s)`, mode)).Scan(&busy, &logf, &ckpt)
		d := time.Since(t)
		if err != nil {
			fmt.Printf("  %-9s error after %v: %v\n", mode, d.Round(time.Microsecond), err)
			continue
		}
		fmt.Printf("  %-9s %-10v busy=%d pages_in_wal=%d checkpointed=%d  WAL %s -> %s\n",
			mode, d.Round(time.Microsecond), busy, logf, ckpt, fmtB(before), fmtB(walSize(path)))
	}
}

func fmtB(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}
