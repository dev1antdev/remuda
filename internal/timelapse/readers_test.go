package timelapse

import (
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestAScreenReadNeverHoldsUpTheSnapshotsAndOneMachineReadsOneAtATime(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	inFlight, most := 0, 0
	r := Open(t.TempDir(), Inputs{Screen: func(machineID, paneID string) (string, error) {
		mu.Lock()
		inFlight++
		most = max(most, inFlight)
		mu.Unlock()
		<-release
		mu.Lock()
		inFlight--
		mu.Unlock()
		return "screen of " + paneID, nil
	}})
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			r.Observe("gog", snap(herdr.StatusWorking, "w1:p1", "w1:p2"), t0.Add(time.Duration(i)*Interval))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Observe waited on a screen read")
	}
	close(release)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		r.mu.Lock()
		running := r.running["gog"]
		r.mu.Unlock()
		if !running {
			break
		}
	}
	r.Close()
	mu.Lock()
	defer mu.Unlock()
	if most != 1 {
		t.Fatalf("%d reads of one machine at once", most)
	}
}
