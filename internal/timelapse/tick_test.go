package timelapse

import (
	"fmt"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// tick moves the bench's clock for d, the recorder ticking every TickEvery with no snapshot in between, the pane's
// screen moving on every 4 s
func (b *bench) tick(d time.Duration, pane string) {
	for end := b.clock.Add(d); b.clock.Before(end); {
		b.clock = b.clock.Add(TickEvery)
		b.screen[pane] = fmt.Sprintf("working\nstep %d\n> ", b.clock.Sub(t0)/(4*time.Second))
		b.Tick(b.clock)
	}
}

func TestATurnIsReadAtItsOwnPaceBetweenSparseSnapshots(t *testing.T) {
	b := newBench(t)
	b.screen["w1:p1"] = "working\nstep 0\n> "
	b.Observe("local", snap(herdr.StatusWorking, "w1:p1"), b.clock) // herdr's stream is quiet: the next snapshot comes late
	b.tick(40*time.Second, "w1:p1")
	b.Observe("local", snap(herdr.StatusWorking, "w1:p1"), b.clock)
	b.tick(40*time.Second, "w1:p1")
	if b.reads < 18 {
		t.Fatalf("two snapshots 40 s apart, the screen read %d times: the turn is read at the snapshots' pace, not every %v", b.reads, Interval)
	}
	b.screen["w1:p1"] = "Done.\n> "
	b.Observe("local", snap(herdr.StatusDone, "w1:p1"), b.clock)
	for end := b.clock.Add(Grace + TickEvery); b.clock.Before(end); {
		b.clock = b.clock.Add(TickEvery)
		b.Tick(b.clock)
	}
	list := b.List("local", "w1:p1", "")
	if len(list) != 1 || list[0].Ended != herdr.StatusDone {
		t.Fatalf("a turn at rest is written Grace after it stopped, with no snapshot since: %+v", list)
	}
	if list[0].Count < 18 {
		t.Fatalf("%d frames kept of a turn whose screen moved every 4 s for 80 s", list[0].Count)
	}
}

func TestATickReadsNothingOnAMachineNotHeardFrom(t *testing.T) {
	b := newBench(t)
	b.screen["w1:p1"] = "working\n> "
	b.Observe("local", snap(herdr.StatusWorking, "w1:p1"), b.clock)
	b.clock = b.clock.Add(TickFor + time.Second)
	reads := b.reads
	b.Tick(b.clock)
	if b.reads != reads {
		t.Fatal("a snapshot older than TickFor was observed again")
	}
	b.Forget("local")
	b.Observe("gog", snap(herdr.StatusWorking, "w1:p1"), b.clock)
	b.clock = b.clock.Add(Interval)
	reads = b.reads
	b.Tick(b.clock)
	if b.reads != reads+1 {
		t.Fatalf("one machine's tick read %d screens, not one", b.reads-reads)
	}
	b.Close()
	b.clock = b.clock.Add(Interval)
	reads = b.reads
	b.Tick(b.clock)
	if b.reads != reads {
		t.Fatal("read after Close")
	}
}
