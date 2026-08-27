// Measures what it costs to spawn a Node.js process for a hook, which is the
// mechanism upstream claude-mem uses. The design claim under test is
// "node hook spawn costs 56-60ms"; the comparison point is the measured Go
// relay round trip of p50 11.3ms / p95 14.2ms.
//
// Bare `node -e ""` is the floor: a real hook additionally resolves and loads
// its entry module, so the real cost is strictly higher than what this reports.
package main

import (
	"fmt"
	"os/exec"
	"sort"
	"time"
)

func bench(label string, n int, argv ...string) {
	// Warm the file cache so the first sample does not dominate.
	for i := 0; i < 3; i++ {
		_ = exec.Command(argv[0], argv[1:]...).Run()
	}

	samples := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		t := time.Now()
		cmd := exec.Command(argv[0], argv[1:]...)
		if err := cmd.Run(); err != nil {
			fmt.Printf("%-28s error: %v\n", label, err)
			return
		}
		samples = append(samples, time.Since(t))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })

	var sum time.Duration
	for _, s := range samples {
		sum += s
	}
	p := func(q float64) time.Duration { return samples[int(float64(len(samples)-1)*q)] }
	fmt.Printf("%-28s n=%-4d min %-8v p50 %-8v p95 %-8v max %-8v mean %v\n",
		label, n, samples[0].Round(time.Microsecond), p(0.50).Round(time.Microsecond),
		p(0.95).Round(time.Microsecond), samples[len(samples)-1].Round(time.Microsecond),
		(sum / time.Duration(len(samples))).Round(time.Microsecond))
}

func main() {
	bench("node -e \"\"", 40, "node", "-e", "")
	bench("node --version", 40, "node", "--version")
	bench("bun -e \"\"", 40, "bun", "-e", "")
	// A Go binary that exits immediately, as the lower bound for any spawn on
	// this machine. This isolates process-creation cost from runtime startup.
	bench("go binary, immediate exit", 40, "./noop.exe")
}
