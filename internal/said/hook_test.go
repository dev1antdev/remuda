package said

import (
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestOnAnswerHearsAFinishAndNeverASeed(t *testing.T) {
	f := &fake{
		versions: map[string]string{"local|w1:p1": "1-1", "local|w1:p2": "1-1"},
		answers:  map[string]string{"local|w1:p1": "Seeded.", "local|w1:p2": "Finished.\n\nREADY FOR REVIEW"},
	}
	tr := New(Inputs{Version: f.version, Answer: f.answer}, nil)
	var mu sync.Mutex
	var heard []string
	tr.OnAnswer = func(machineID, paneID, text string) {
		mu.Lock()
		defer mu.Unlock()
		heard = append(heard, machineID+"|"+paneID+": "+text)
	}
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(heard)
	}

	tr.Observe("local", &herdr.Snapshot{Panes: []herdr.PaneInfo{{PaneID: "w1:p1", Agent: "claude", AgentStatus: herdr.StatusDone}}})
	waitFor(t, "the seeded line", func() bool { return tr.Line("local", "w1:p1") == "Seeded." })
	if count() != 0 {
		t.Fatal("a seed from a snapshot was heard as an answer")
	}

	finish := events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "w1:p2"}, Kind: "claude", From: herdr.StatusWorking, To: herdr.StatusDone}
	tr.OnStatus(finish)
	waitFor(t, "the finish", func() bool { return count() == 1 })
	mu.Lock()
	if heard[0] != "local|w1:p2: Finished.\n\nREADY FOR REVIEW" {
		t.Fatalf("heard %q", heard[0])
	}
	mu.Unlock()

	finish.To = herdr.StatusIdle
	tr.OnStatus(finish)
	tr.OnStatus(status(herdr.StatusDone)) // the seeded pane finishes with the file it was seeded from
	time.Sleep(20 * time.Millisecond)
	if count() != 1 {
		t.Fatalf("an unchanged file was heard again: %d", count())
	}

	f.mu.Lock()
	f.versions["local|w1:p1"], f.answers["local|w1:p1"] = "2-2", "Now really done."
	f.mu.Unlock()
	tr.OnStatus(status(herdr.StatusDone))
	waitFor(t, "the new answer", func() bool { return count() == 2 })
	mu.Lock()
	if heard[1] != "local|w1:p1: Now really done." {
		t.Fatalf("heard %q", heard[1])
	}
	mu.Unlock()
}
