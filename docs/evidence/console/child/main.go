// child 는 CUI(Subsystem 3) 로 빌드한다.
// 콘솔이 붙었는지, 그 콘솔을 자기가 만들었는지(=창이 새로 떴는지) 파일로 보고한다.
// stdout 을 쓰지 않는 이유: stdio 리다이렉션과 콘솔 존재 여부를 분리하기 위해서다.
package main

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	k32                     = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleWindow    = k32.NewProc("GetConsoleWindow")
	procGetConsoleProcList  = k32.NewProc("GetConsoleProcessList")
	u32                     = windows.NewLazySystemDLL("user32.dll")
	procIsWindowVisible     = u32.NewProc("IsWindowVisible")
)

func main() {
	hwnd, _, _ := procGetConsoleWindow.Call()

	var pids [64]uint32
	n, _, _ := procGetConsoleProcList.Call(uintptr(unsafe.Pointer(&pids[0])), 64)

	visible := uintptr(0)
	if hwnd != 0 {
		visible, _, _ = procIsWindowVisible.Call(hwnd)
	}

	// 콘솔이 있고 그 콘솔에 붙은 프로세스가 우리 하나뿐이면 = 우리가 만든 것 = 새 창
	created := hwnd != 0 && n == 1

	out := fmt.Sprintf(
		"label=%s\nhasConsole=%v\nconsoleProcCount=%d\nconsoleWindowVisible=%v\nconsoleCreatedByUs=%v\nstdoutValid=%v\n",
		os.Args[1], hwnd != 0, n, visible != 0, created, os.Stdout != nil && func() bool {
			_, err := os.Stdout.Stat()
			return err == nil
		}())

	os.WriteFile(os.Args[2], []byte(out), 0o644)
}
