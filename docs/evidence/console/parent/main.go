// parent 는 GUI(Subsystem 2) 로 빌드한다 — 콘솔이 없는 부모다.
// Task Scheduler 처럼 콘솔 없는 컨텍스트에서 자식을 띄우는 상황을 모사한다.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func spawn(childExe, label, outFile string, attr *syscall.SysProcAttr) {
	cmd := exec.Command(childExe, label, outFile)
	if attr != nil {
		cmd.SysProcAttr = attr
	}
	cmd.Run()
}

func main() {
	dir := os.Args[1]
	child := filepath.Join(dir, "child.exe")

	// 1) 기본 플래그 — Task Scheduler 가 무엇을 쓰는지 모를 때의 최악 가정
	spawn(child, "GUI부모_기본플래그", filepath.Join(dir, "r1.txt"), nil)

	// 2) CREATE_NO_WINDOW — 부모가 명시적으로 창을 막는 경우
	spawn(child, "GUI부모_CREATE_NO_WINDOW", filepath.Join(dir, "r2.txt"),
		&syscall.SysProcAttr{CreationFlags: 0x08000000})

	// 3) DETACHED_PROCESS — 콘솔을 아예 붙이지 않게 하는 경우
	spawn(child, "GUI부모_DETACHED", filepath.Join(dir, "r3.txt"),
		&syscall.SysProcAttr{CreationFlags: 0x00000008})

	os.WriteFile(filepath.Join(dir, "done.txt"), []byte(time.Now().String()), 0o644)
}
