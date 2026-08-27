// I-09 says the singleton is enforced by ListenPipe exclusivity on a fixed pipe
// name, and the Phase 3 gate says 30 concurrent starts must leave exactly one
// service. Nothing measured that. The inherited claim was about a "squatter
// winning 35 of 40 races", which is a different question and one the trust
// boundary decision already answered - a same-SID process is inside the boundary.
//
// What actually needs to be true: among N processes racing to create the same
// pipe name at the same instant, exactly one succeeds and the rest get a clean,
// distinguishable error. Not "usually one". Exactly one, every time.
//
// Children synchronise on a shared wall-clock deadline so they really do race,
// rather than being staggered by process spawn cost.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
)

const pipeName = `\\.\pipe\engramux-probe-v1`

func child(startNanos int64) {
	time.Sleep(time.Until(time.Unix(0, startNanos)))

	l, err := winio.ListenPipe(pipeName, &winio.PipeConfig{
		SecurityDescriptor: "", // default DACL; this probe is about exclusivity, not ACLs
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	})
	if err != nil {
		fmt.Printf("LOSE %s\n", err)
		return
	}
	fmt.Println("WIN")
	// Hold it briefly so late racers still see it occupied, then release.
	time.Sleep(300 * time.Millisecond)
	l.Close()
}

func main() {
	if len(os.Args) > 2 && os.Args[1] == "-child" {
		n, _ := strconv.ParseInt(os.Args[2], 10, 64)
		child(n)
		return
	}

	self, _ := os.Executable()
	const rounds = 20
	const racers = 30

	winTally := map[int]int{}
	loseReasons := map[string]int{}

	for r := 0; r < rounds; r++ {
		// Give every child the same start instant, far enough out that all of
		// them are already sleeping on it before it arrives.
		start := time.Now().Add(700 * time.Millisecond).UnixNano()

		var wg sync.WaitGroup
		var mu sync.Mutex
		wins := 0

		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, _ := exec.Command(self, "-child", strconv.FormatInt(start, 10)).Output()
				s := strings.TrimSpace(string(out))
				mu.Lock()
				defer mu.Unlock()
				if s == "WIN" {
					wins++
				} else if strings.HasPrefix(s, "LOSE ") {
					loseReasons[strings.TrimPrefix(s, "LOSE ")]++
				} else {
					loseReasons["unexpected output: "+s]++
				}
			}()
		}
		wg.Wait()
		winTally[wins]++
		fmt.Printf("round %2d: %d winner(s) of %d\n", r+1, wins, racers)
	}

	fmt.Printf("\n%d rounds x %d concurrent ListenPipe on the same fixed name\n", rounds, racers)
	fmt.Println("winners per round:")
	for w, c := range winTally {
		marker := ""
		if w != 1 {
			marker = "   <-- I-09 VIOLATED"
		}
		fmt.Printf("  %d winner(s): %d round(s)%s\n", w, c, marker)
	}
	fmt.Println("loser errors:")
	for e, c := range loseReasons {
		fmt.Printf("  x%-4d %s\n", c, e)
	}
}
