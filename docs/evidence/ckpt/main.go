// The previous run measured TRUNCATE right after a full PASSIVE had already
// copied every page, so 22ms was not the cost of a checkpoint - it was the cost
// of truncating an already-checkpointed WAL. This measures a COLD TRUNCATE at
// several WAL sizes, which is the number the checkpoint policy actually needs.
//
// The design worried that a blind TRUNCATE could hold the single connection for
// the full 10s busy_timeout, ten times the relay's entire budget, pushing every
// concurrent relay into the spool. That worry was inherited from a design with
// multiple connections. Under locking_mode=exclusive with SetMaxOpenConns(1)
// there is no other connection to contend with - so measure it and find out.
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

func fmtB(b int64) string {
	if b >= 1<<20 {
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	}
	return fmt.Sprintf("%.1f KiB", float64(b)/(1<<10))
}

// Fill to roughly targetMiB of WAL, then time one cold TRUNCATE.
func run(targetMiB int) {
	dir, _ := os.MkdirTemp("", "ckpt")
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
	db.Exec(`CREATE TABLE events(id INTEGER PRIMARY KEY, host TEXT NOT NULL, payload TEXT NOT NULL, ts INTEGER NOT NULL)`)

	body := strings.Repeat("x", 1182) // corpus p50
	target := int64(targetMiB) << 20
	n := 0
	for {
		if _, err := db.Exec(`INSERT INTO events(host,payload,ts) VALUES(?,?,?)`, "claude-code", body, n); err != nil {
			panic(err)
		}
		n++
		if n%500 == 0 {
			if fi, err := os.Stat(path + "-wal"); err == nil && fi.Size() >= target {
				break
			}
		}
	}
	fi, _ := os.Stat(path + "-wal")
	before := fi.Size()

	t := time.Now()
	var busy, logf, ckpt int
	err = db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logf, &ckpt)
	d := time.Since(t)
	after := int64(0)
	if fi, e := os.Stat(path + "-wal"); e == nil {
		after = fi.Size()
	}
	status := "ok"
	if err != nil {
		status = err.Error()
	}
	fmt.Printf("WAL %-10s (%6d rows)  cold TRUNCATE %-11v busy=%d pages=%d  -> %-10s %s\n",
		fmtB(before), n, d.Round(time.Microsecond), busy, logf, fmtB(after), status)
}

func main() {
	fmt.Println("cold TRUNCATE checkpoint, exclusive lock, single connection, nothing else touching the db")
	fmt.Println()
	for _, mib := range []int{8, 32, 64, 128, 256} {
		run(mib)
	}
	fmt.Println()
	fmt.Println("relay total budget is 1s; busy_timeout is 10s")
}
