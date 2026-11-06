package said

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// busyReader counts the reads in flight per machine and the reads made per pane.
type busyReader struct {
	mu       sync.Mutex
	inFlight map[string]int
	most     map[string]int
	reads    map[string]int
}

func (b *busyReader) version(machine, pane string) (string, bool) { return "1-1", true }

func (b *busyReader) answer(machine, pane string) (string, string) {
	b.mu.Lock()
	b.inFlight[machine]++
	b.most[machine] = max(b.most[machine], b.inFlight[machine])
	b.reads[machine+"|"+pane]++
	b.mu.Unlock()
	time.Sleep(time.Millisecond)
	b.mu.Lock()
	b.inFlight[machine]--
	b.mu.Unlock()
	return "Answered " + pane + ".", "id"
}

func TestTrackerReadsOneAtATimePerMachineAndEachFileOnce(t *testing.T) {
	b := &busyReader{inFlight: map[string]int{}, most: map[string]int{}, reads: map[string]int{}}
	tr := New(Inputs{Version: b.version, Answer: b.answer}, nil)
	machines, panes := []string{"local", "gog"}, []string{"w1:p1", "w1:p2", "w2:p1"}
	var wg sync.WaitGroup
	for _, machine := range machines {
		for _, pane := range panes {
			for range 20 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					tr.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: machine, Pane: pane}, Kind: "claude", From: herdr.StatusWorking, To: herdr.StatusDone})
				}()
			}
		}
	}
	wg.Wait()
	for _, machine := range machines {
		for _, pane := range panes {
			waitFor(t, machine+" "+pane, func() bool { return tr.Line(machine, pane) == "Answered "+pane+"." })
		}
	}
	waitFor(t, "the readers to stop", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return !tr.running["local"] && !tr.running["gog"]
	})
	b.mu.Lock()
	defer b.mu.Unlock()
	var problems []string
	for _, machine := range machines {
		if b.most[machine] != 1 {
			problems = append(problems, fmt.Sprintf("%s read %d panes at once", machine, b.most[machine]))
		}
		for _, pane := range panes {
			if n := b.reads[machine+"|"+pane]; n != 1 {
				problems = append(problems, fmt.Sprintf("%s %s read %d times", machine, pane, n))
			}
		}
	}
	if len(problems) > 0 {
		t.Fatal(strings.Join(problems, "; "))
	}
}
