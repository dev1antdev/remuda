package jobs

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestAJobStreamsItsOutputAndSaysWhenItEnded(t *testing.T) {
	bus := events.NewBus()
	var mu sync.Mutex
	var ended []events.JobDone
	events.Subscribe(bus, func(e events.JobDone) {
		mu.Lock()
		ended = append(ended, e)
		mu.Unlock()
	})
	host := &shell.Fake{Name: "box", Answer: func(script string) (string, error) {
		if strings.Contains(script, "fail") {
			return "so far\n", &shell.ExitError{Code: 2}
		}
		return "hello\n", nil
	}}
	runner := New(bus)

	job := runner.StartThen(host, "say hello", "echo hello", func(say func(string)) { say("before") }, func(say func(string)) error { say("after"); return nil })
	failed := runner.Start(host, "fail", "fail now")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a, _ := runner.Get(job.ID)
		b, _ := runner.Get(failed.ID)
		if a.Done && b.Done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, ok := runner.Get(job.ID)
	if !ok || !got.Done || !got.OK || got.Output != "before\nhello\nafter\n" || got.Machine != "box" {
		t.Fatalf("job: %+v", got)
	}
	bad, _ := runner.Get(failed.ID)
	if !bad.Done || bad.OK || bad.Error != "exit status 2" || bad.Output != "so far\n" {
		t.Fatalf("failed job: %+v", bad)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ended) != 2 {
		t.Fatalf("job ends on the bus: %+v", ended)
	}
	for _, e := range ended {
		if e.Machine != "box" || (e.Title == "say hello") != e.OK {
			t.Fatalf("job end: %+v", e)
		}
	}
	list := runner.List("box", 8)
	if len(list) != 2 || list[0].ID != failed.ID || list[0].Output != "" {
		t.Fatalf("list is newest first, without output: %+v", list)
	}
}
