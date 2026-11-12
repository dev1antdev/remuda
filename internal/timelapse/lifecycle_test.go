package timelapse

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestAMachineNotHeardFromForAwayHasItsTurnWrittenWhereItWasLastSeen(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	seen := b.clock.Add(-2 * time.Second)
	b.clock = b.clock.Add(Away)
	b.Tick(b.clock)
	list := b.List("local", "w1:p1", "")
	if len(list) != 1 || list[0].Ended != EndedAway || !list[0].End.Equal(seen) {
		t.Fatalf("listed %+v, last seen %v", list, seen)
	}
	b.clock = b.clock.Add(time.Hour)
	b.work(2*time.Minute, "w1:p1")
	b.rest(herdr.StatusDone, Grace+2*time.Second, "w1:p1")
	if list := b.List("local", "w1:p1", ""); len(list) != 2 || list[0].Ended != herdr.StatusDone || list[0].End.Sub(list[0].Start) > 3*time.Minute {
		t.Fatalf("back from away: %+v", list)
	}
}

func TestATickReadingAnOlderSnapshotAgainKeepsWhenThePaneWasSeen(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	seen := b.clock.Add(-2 * time.Second)
	b.clock = b.clock.Add(TickFor - 3*time.Second)
	b.Tick(b.clock)
	b.Close()
	if list := b.List("local", "w1:p1", ""); len(list) != 1 || list[0].Ended != EndedStopped || list[0].End.After(seen.Add(Interval)) {
		t.Fatalf("listed %+v, last seen %v: a tick's own clock taken for a sighting", list, seen)
	}
}

func TestAMachineBackAfterAwayWithNoTickBetweenStartsANewTurn(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	b.clock = b.clock.Add(Away + time.Hour)
	b.work(2*time.Minute, "w1:p1")
	b.rest(herdr.StatusDone, Grace+2*time.Second, "w1:p1")
	list := b.List("local", "w1:p1", "")
	if len(list) != 2 || list[1].Ended != EndedAway || list[0].Ended != herdr.StatusDone || list[0].End.Sub(list[0].Start) > 3*time.Minute {
		t.Fatalf("a turn spans the machine's absence: %+v", list)
	}
}

// held keeps the bench's jobs from running: they wait in the queue, as behind a reader busy on a slow machine
func (b *bench) held() (release func()) {
	var waiting []func()
	b.Recorder.run = func(fn func()) { waiting = append(waiting, fn) }
	return func() {
		b.reopen()
		for _, fn := range waiting {
			fn()
		}
	}
}

func TestClosingWritesATurnThatEndedAndStillWaitsInTheQueue(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	b.held()
	b.Observe("local", snap(herdr.StatusWorking), b.clock)
	b.Close()
	if list := b.List("local", "w1:p1", ""); len(list) != 1 || list[0].Ended != EndedGone {
		t.Fatalf("listed %+v", list)
	}
}

func TestOffAndDeleteAllDropATurnThatEndedAndStillWaitsInTheQueue(t *testing.T) {
	for name, drop := range map[string]func(*bench){
		"off":        func(b *bench) { b.SetOn(false) },
		"delete all": func(b *bench) { b.DeleteAll() },
	} {
		b := newBench(t)
		b.work(2*time.Minute, "w1:p1")
		release := b.held()
		b.Observe("local", snap(herdr.StatusWorking), b.clock)
		drop(b)
		release()
		if got := files(t, b.dir); len(got) != 0 {
			t.Fatalf("%s: the turn landed afterwards: %v", name, got)
		}
	}
}

func TestAReadOfAPaneThatClosedMeanwhileIsNoFailure(t *testing.T) {
	var logged bytes.Buffer
	before := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(before) })
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	release := b.held()
	b.clock = b.clock.Add(4 * time.Second)
	b.Observe("local", snap(herdr.StatusWorking, "w1:p1"), b.clock)
	b.Observe("local", snap(herdr.StatusWorking), b.clock)
	delete(b.screen, "w1:p1")
	release()
	if strings.Contains(logged.String(), "reading w1:p1") {
		t.Fatalf("logged %q", logged.String())
	}
}
