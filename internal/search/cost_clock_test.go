package search_test

import (
	"testing"
	"time"
)

// testing.B.Elapsed uses QPC on this Go/Windows toolchain. Use the standard
// benchmark lifecycle rather than owning an unsafe native call or changing
// the machine's timer resolution. Each operation is a complete measured group;
// calibration repeats groups, and callers retain the final group's samples.
func runCostBenchmark(t *testing.T, work func(*testing.B)) {
	t.Helper()
	var failed bool
	testing.Benchmark(func(b *testing.B) {
		defer func() { failed = failed || b.Failed() }()
		for range b.N {
			work(b)
		}
	})
	if failed {
		t.Fatal("cost benchmark failed its correctness check")
	}
}

func bestSearchGroup(t *testing.T, work func(*testing.B)) time.Duration {
	t.Helper()
	var best time.Duration
	runCostBenchmark(t, func(b *testing.B) {
		best = 0
		for i := 0; i < 4; i++ {
			start := b.Elapsed()
			work(b)
			d := b.Elapsed() - start
			if d <= 0 {
				b.Fatal("nonpositive search interval")
			}
			if i > 0 && (best == 0 || d < best) {
				best = d
			}
		}
	})
	return best
}

func TestDiagnoseSearchClockResolution(t *testing.T) {
	var sameGo, sameBenchmark int
	var largestGoStep, goSpan, benchmarkSpan time.Duration
	runCostBenchmark(t, func(b *testing.B) {
		sameGo, sameBenchmark, largestGoStep = 0, 0, 0
		prevGo, prevBenchmark := time.Now(), b.Elapsed()
		firstGo, firstBenchmark := prevGo, prevBenchmark
		for range 50000 {
			g, q := time.Now(), b.Elapsed()
			if g.Sub(prevGo) == 0 {
				sameGo++
			}
			if step := g.Sub(prevGo); step > largestGoStep {
				largestGoStep = step
			}
			if q == prevBenchmark {
				sameBenchmark++
			}
			if q < prevBenchmark {
				b.Fatal("benchmark clock moved backwards")
			}
			prevGo, prevBenchmark = g, q
		}
		goSpan, benchmarkSpan = prevGo.Sub(firstGo), prevBenchmark-firstBenchmark
		if benchmarkSpan <= 0 {
			b.Fatal("benchmark clock did not resolve the sampling interval")
		}
	})
	t.Logf("50000 adjacent samples: Go unchanged=%d; benchmark unchanged=%d; largest Go step=%s; Go span=%s; benchmark span=%s", sameGo, sameBenchmark, largestGoStep, goSpan, benchmarkSpan)
}
