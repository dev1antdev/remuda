package said

import (
	"sync"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// heldReader holds every answer read until the test lets it go, so a status change can land while a read is in flight.
type heldReader struct {
	entered chan struct{}
	release chan struct{}
	mu      sync.Mutex
	answers []string
	version string
}

func newHeldReader() *heldReader {
	return &heldReader{entered: make(chan struct{}), release: make(chan struct{}), version: "1-1"}
}

func (h *heldReader) versionOf(string, string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.version, true
}

func (h *heldReader) answer(string, string) (string, string) {
	h.entered <- struct{}{}
	<-h.release
	return "Shipped the release.", "id"
}

func (h *heldReader) link(string, string) (transcript.Link, bool) {
	return transcript.Link{URL: "https://github.com/o/r/pull/7", Label: "PR #7"}, true
}

func paneStatus(to string) events.PaneStatus {
	return events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "w1:p1"}, Kind: "claude", From: herdr.StatusWorking, To: to}
}

func (t *Tracker) settled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.running["local"]
}

func TestAReadInFlightWritesNoStaleLineAfterWork(t *testing.T) {
	h := newHeldReader()
	tr := New(Inputs{Version: h.versionOf, Answer: h.answer, Link: h.link}, nil)
	var answered []string
	var answeredMu sync.Mutex
	tr.OnAnswer = func(_, _, text string) {
		answeredMu.Lock()
		answered = append(answered, text)
		answeredMu.Unlock()
	}

	tr.OnStatus(paneStatus(herdr.StatusDone))
	<-h.entered
	tr.OnStatus(paneStatus(herdr.StatusWorking)) // the person sent the next prompt while the finish was being read
	h.release <- struct{}{}
	waitFor(t, "the read to end", tr.settled)
	if line := tr.Line("local", "w1:p1"); line != "" {
		t.Fatalf("a working agent says its old line %q", line)
	}
	answeredMu.Lock()
	if len(answered) != 1 {
		t.Fatalf("the finish's answer was not heard: %v", answered)
	}
	answeredMu.Unlock()

	h.mu.Lock()
	h.version = "2-2"
	h.mu.Unlock()
	tr.OnStatus(paneStatus(herdr.StatusDone))
	<-h.entered
	h.release <- struct{}{}
	waitFor(t, "the next finish's line", func() bool { return tr.Line("local", "w1:p1") == "Shipped the release." })

	tr.OnStatus(paneStatus(herdr.StatusWorking))
	h.mu.Lock()
	h.version = "3-3"
	h.mu.Unlock()
	tr.OnStatus(paneStatus(herdr.StatusDone))
	<-h.entered
	tr.OnStatus(paneStatus(events.Gone)) // the pane closed while its finish was being read
	h.release <- struct{}{}
	waitFor(t, "the last read to end", tr.settled)
	tr.mu.Lock()
	_, kept := tr.lines["local|w1:p1"]
	tr.mu.Unlock()
	if kept {
		t.Fatal("a closed pane was given its line back")
	}
}
