// Two claims under test, both load-bearing for I-05 and the Phase 1 replay gate:
//
//  1. A row committed with the production DSN survives a hard kill that happens
//     after COMMIT returns but before any ACK is written.
//  2. After that kill, a NEW process can open the same database — even though
//     the dead process held locking_mode=exclusive.
//
// (2) is the one nobody checked. If an exclusive lock survived process death the
// service could never restart after a crash, and the whole design would be wrong.
//
// The child inserts, commits, reports the id on stdout, then blocks forever, so
// the parent's TerminateProcess is what ends it. No defers run, nothing is closed.
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const dsn = "?_pragma=journal_mode(wal)&_pragma=locking_mode(exclusive)" +
	"&_pragma=foreign_keys(1)&_pragma=synchronous(2)&_pragma=busy_timeout(10000)" +
	"&_pragma=secure_delete(1)&_txlock=immediate"

func open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

func child(path, id string) {
	db, err := open(path)
	if err != nil {
		fmt.Println("CHILD-OPEN-FAIL", err)
		os.Exit(2)
	}
	tx, err := db.Begin()
	if err != nil {
		fmt.Println("CHILD-BEGIN-FAIL", err)
		os.Exit(2)
	}
	if _, err := tx.Exec(`INSERT INTO events(id, payload) VALUES(?, ?)`, id, "committed-then-killed"); err != nil {
		fmt.Println("CHILD-INSERT-FAIL", err)
		os.Exit(2)
	}
	if err := tx.Commit(); err != nil {
		fmt.Println("CHILD-COMMIT-FAIL", err)
		os.Exit(2)
	}
	// COMMIT has returned. Everything after this point is what a crash destroys.
	fmt.Println("COMMITTED")
	os.Stdout.Sync()
	select {} // hold the exclusive lock until the parent kills us
}

func main() {
	if len(os.Args) > 2 && os.Args[1] == "-child" {
		child(os.Args[2], os.Args[3])
		return
	}

	dir, err := os.MkdirTemp("", "crashtest")
	if err != nil {
		panic(err)
	}
	path := filepath.Join(dir, "t.db")

	// Seed the schema, then close so the child can take the exclusive lock.
	db, err := open(path)
	if err != nil {
		panic(err)
	}
	if _, err := db.Exec(`CREATE TABLE events(id TEXT PRIMARY KEY, payload TEXT NOT NULL)`); err != nil {
		panic(err)
	}
	db.Close()

	self, _ := os.Executable()
	const rounds = 20
	survived, reopened, reopenErrs := 0, 0, map[string]int{}

	for i := 0; i < rounds; i++ {
		id := fmt.Sprintf("evt-%03d", i)
		cmd := exec.Command(self, "-child", path, id)
		out, _ := cmd.StdoutPipe()
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		sc := bufio.NewScanner(out)
		got := ""
		if sc.Scan() {
			got = sc.Text()
		}
		if got != "COMMITTED" {
			fmt.Printf("round %d: child said %q\n", i, got)
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			continue
		}
		// Hard kill: TerminateProcess. No defers, no Close, no checkpoint.
		if err := cmd.Process.Kill(); err != nil {
			fmt.Println("kill:", err)
		}
		_ = cmd.Wait()

		// Claim 2: can a fresh connection take the exclusive lock the dead
		// process held? Retry briefly - the OS releases handles asynchronously.
		var db2 *sql.DB
		var openErr error
		deadline := time.Now().Add(3 * time.Second)
		for {
			db2, openErr = open(path)
			if openErr == nil {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if openErr != nil {
			reopenErrs[openErr.Error()]++
			continue
		}
		reopened++

		// Claim 1: is the committed row there?
		var n int
		if err := db2.QueryRow(`SELECT count(*) FROM events WHERE id = ?`, id).Scan(&n); err != nil {
			reopenErrs["select: "+err.Error()]++
		} else if n == 1 {
			survived++
		}
		var ic string
		_ = db2.QueryRow(`PRAGMA integrity_check`).Scan(&ic)
		if ic != "ok" {
			reopenErrs["integrity: "+ic]++
		}
		db2.Close()
	}

	shm := "absent"
	if fi, err := os.Stat(path + "-shm"); err == nil {
		shm = fmt.Sprintf("%dB", fi.Size())
	}
	fmt.Printf("\nrounds                       %d\n", rounds)
	fmt.Printf("reopened after hard kill     %d/%d\n", reopened, rounds)
	fmt.Printf("committed row survived       %d/%d\n", survived, rounds)
	fmt.Printf("-shm file after all of it    %s\n", shm)
	if len(reopenErrs) == 0 {
		fmt.Println("errors                       none")
	}
	for e, c := range reopenErrs {
		fmt.Printf("error x%-3d                   %s\n", c, e)
	}
}
