// Measures Windows PID reuse: how often a freshly created process gets a PID
// that a recently exited process already used, and how quickly reuse happens.
//
// The design claim under test is "PID reuse reaches 5.75% within 2.3s", which
// decides whether peer verification on the named pipe needs process creation
// time in addition to the PID.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"
)

func main() {
	const n = 3000

	// A process that exits immediately. Using the Go toolchain's own helper
	// would add startup cost; cmd.exe is not available as a nested shell here,
	// so spawn the smallest thing that reliably exists and exits: this very
	// binary with an argument that makes it return at once.
	self, err := os.Executable()
	if err != nil {
		fmt.Println("executable:", err)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "-exit" {
		return
	}

	type seen struct {
		at    time.Time
		index int
	}
	last := make(map[int]seen, n)

	var reuseGaps []time.Duration
	reused := 0
	start := time.Now()

	for i := 0; i < n; i++ {
		cmd := exec.Command(self, "-exit")
		if err := cmd.Start(); err != nil {
			fmt.Println("start:", err)
			return
		}
		pid := cmd.Process.Pid
		now := time.Now()

		if prev, ok := last[pid]; ok {
			reused++
			reuseGaps = append(reuseGaps, now.Sub(prev.at))
		}
		last[pid] = seen{at: now, index: i}
		_ = cmd.Wait()
	}

	elapsed := time.Since(start)
	fmt.Printf("spawned            %d processes in %v (%.1f/s)\n", n, elapsed.Round(time.Millisecond), float64(n)/elapsed.Seconds())
	fmt.Printf("distinct PIDs      %d\n", len(last))
	fmt.Printf("reuse events       %d  (%.2f%% of spawns)\n", reused, float64(reused)*100/float64(n))

	if len(reuseGaps) == 0 {
		fmt.Println("no reuse observed")
		return
	}
	sort.Slice(reuseGaps, func(i, j int) bool { return reuseGaps[i] < reuseGaps[j] })
	fmt.Printf("reuse gap  min     %v\n", reuseGaps[0].Round(time.Millisecond))
	fmt.Printf("reuse gap  p50     %v\n", reuseGaps[len(reuseGaps)/2].Round(time.Millisecond))
	fmt.Printf("reuse gap  max     %v\n", reuseGaps[len(reuseGaps)-1].Round(time.Millisecond))

	// How many reuses happened inside the window a pipe peer check would care
	// about: the relay's 1s total budget, and the 2.3s the old claim named.
	for _, w := range []time.Duration{time.Second, 2300 * time.Millisecond} {
		c := 0
		for _, g := range reuseGaps {
			if g <= w {
				c++
			}
		}
		fmt.Printf("reuse within %-6v %d  (%.2f%% of spawns)\n", w, c, float64(c)*100/float64(n))
	}
}
