package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

func probe(label, extra string, maxConns int) {
	dir, _ := os.MkdirTemp("", "shm")
	path := filepath.Join(dir, "t.db")
	dsn := "file:" + path + "?_pragma=journal_mode(wal)&_pragma=synchronous(3)&_pragma=busy_timeout(10000)" + extra

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		fmt.Printf("%-26s open: %v\n", label, err)
		return
	}
	defer db.Close()
	db.SetMaxOpenConns(maxConns)

	if _, err := db.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		fmt.Printf("%-26s create: %v\n", label, err)
		return
	}
	var lm, jm string
	db.QueryRow("PRAGMA locking_mode").Scan(&lm)
	db.QueryRow("PRAGMA journal_mode").Scan(&jm)

	// 쓰기를 좀 해서 WAL 과 shm 이 실제로 만들어지게 한다
	for i := 0; i < 200; i++ {
		if _, err := db.Exec(`INSERT INTO t(v) VALUES(?)`, fmt.Sprint(i)); err != nil {
			fmt.Printf("%-26s insert: %v\n", label, err)
			return
		}
	}

	shm := statSize(path + "-shm")
	wal := statSize(path + "-wal")

	// 두 번째 프로세스(여기서는 두 번째 *sql.DB)가 열 수 있는가
	second := "OK"
	db2, err := sql.Open("sqlite", dsn)
	if err == nil {
		var n int
		if err := db2.QueryRow(`SELECT count(*) FROM t`).Scan(&n); err != nil {
			second = "DENIED: " + err.Error()
		}
		db2.Close()
	} else {
		second = "OPEN FAIL: " + err.Error()
	}

	// 수동 checkpoint 가 되는가
	ck := "OK"
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		ck = err.Error()
	}

	fmt.Printf("%-26s locking=%-9s journal=%-4s shm=%-8s wal=%-9s 2nd-conn=%-28s ckpt=%s\n",
		label, lm, jm, shm, wal, trunc(second, 28), ck)
}

func statSize(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return "없음"
	}
	return fmt.Sprintf("%dB", fi.Size())
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// 동시 읽기/쓰기 + 수동 checkpoint 를 섞어 EXCLUSIVE 가 실제 부하에서 견디는지 본다.
func stress(label, extra string, maxConns int) {
	dir, _ := os.MkdirTemp("", "stress")
	path := filepath.Join(dir, "s.db")
	dsn := "file:" + path + "?_pragma=journal_mode(wal)&_pragma=synchronous(3)&_pragma=busy_timeout(10000)" + extra
	db, _ := sql.Open("sqlite", dsn)
	defer db.Close()
	db.SetMaxOpenConns(maxConns)
	db.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)`)

	var wg sync.WaitGroup
	var writeErr, readErr, ckErr int
	var mu sync.Mutex
	done := make(chan struct{})
	time.AfterFunc(2*time.Second, func() { close(done) })

	wg.Add(3)
	go func() { // writer
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				if _, err := db.Exec(`INSERT INTO t(v) VALUES('x')`); err != nil {
					mu.Lock()
					writeErr++
					mu.Unlock()
				}
			}
		}
	}()
	go func() { // reader
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				var n int
				if err := db.QueryRow(`SELECT count(*) FROM t`).Scan(&n); err != nil {
					mu.Lock()
					readErr++
					mu.Unlock()
				}
			}
		}
	}()
	go func() { // 수동 checkpoint — Tailscale 을 물었던 바로 그 패턴
		defer wg.Done()
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
					mu.Lock()
					ckErr++
					mu.Unlock()
				}
			}
		}
	}()
	wg.Wait()

	var rows int
	db.QueryRow(`SELECT count(*) FROM t`).Scan(&rows)
	var ic string
	db.QueryRow(`PRAGMA integrity_check`).Scan(&ic)
	fmt.Printf("%-26s rows=%-7d writeErr=%-5d readErr=%-4d ckptErr=%-4d integrity=%s\n",
		label, rows, writeErr, readErr, ckErr, ic)
}

func main() {
	fmt.Println("=== -shm 파일 생성 여부와 2차 커넥션 ===")
	probe("기본(플랜 현재)", "", 4)
	probe("EXCLUSIVE, 1 conn", "&_pragma=locking_mode(exclusive)", 1)
	probe("EXCLUSIVE, 4 conn", "&_pragma=locking_mode(exclusive)", 4)
	fmt.Println()
	fmt.Println("=== 동시 읽기/쓰기 + 50ms 수동 checkpoint, 2초 ===")
	stress("기본(플랜 현재)", "", 4)
	stress("EXCLUSIVE, 1 conn", "&_pragma=locking_mode(exclusive)", 1)
}
