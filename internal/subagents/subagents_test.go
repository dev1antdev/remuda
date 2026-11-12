package subagents

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/transcript"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) pass(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// files is what each pane's conversation lists, the reads made and the machines connected.
type files struct {
	mu         sync.Mutex
	lists      map[string][]transcript.Subagent
	failing    map[string]error
	reads      map[string]int
	down       map[string]bool
	hold       chan struct{} // while set, a read waits for it to close
	inflight   int
	mostAtOnce int
}

func newFiles() *files {
	return &files{lists: map[string][]transcript.Subagent{}, failing: map[string]error{}, reads: map[string]int{}, down: map[string]bool{}}
}

func (f *files) read(machineID, paneID string, _ time.Time) ([]transcript.Subagent, error) {
	k := machineID + "|" + paneID
	f.mu.Lock()
	f.reads[k]++
	f.inflight++
	f.mostAtOnce = max(f.mostAtOnce, f.inflight)
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inflight--
	return f.lists[k], f.failing[k]
}

func (f *files) ready(machineID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.down[machineID]
}

func (f *files) set(k string, list ...transcript.Subagent) {
	f.mu.Lock()
	f.lists[k] = list
	f.mu.Unlock()
}

func (f *files) count(k string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[k]
}

func agents(statuses ...string) []transcript.Subagent {
	out := make([]transcript.Subagent, len(statuses))
	for i, status := range statuses {
		out[i] = transcript.Subagent{ID: "agent-" + string(rune('a'+i)), Status: status}
	}
	return out
}

func snapshot(panes ...herdr.PaneInfo) *herdr.Snapshot { return &herdr.Snapshot{Panes: panes} }

func claude(id, status string) herdr.PaneInfo {
	return herdr.PaneInfo{PaneID: id, Agent: "claude", AgentStatus: status}
}

// tracker is a Tracker on the files with a clock of its own, and a count of its OnChange calls.
func tracker(f *files) (*Tracker, *clock, func() int) {
	c := &clock{now: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	t := New(f.read, f.ready)
	t.now = c.read
	var mu sync.Mutex
	changes := 0
	t.OnChange = func() {
		mu.Lock()
		changes++
		mu.Unlock()
	}
	return t, c, func() int {
		mu.Lock()
		defer mu.Unlock()
		return changes
	}
}

// settle waits until no sweep runs on the machine.
func settle(t *testing.T, tr *Tracker, machineID string) {
	t.Helper()
	waitFor(t, "the sweep to end", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return !tr.sweeping[machineID]
	})
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waited in vain for %s", what)
}

func TestCountOfSumsByStatus(t *testing.T) {
	got := CountOf(agents(transcript.SubagentWorking, transcript.SubagentDone, transcript.SubagentWorking, transcript.SubagentQuiet, "odd"))
	if got != (Count{Working: 2, Quiet: 1, Done: 1}) {
		t.Fatalf("count: %+v", got)
	}
	if CountOf(nil) != (Count{}) {
		t.Fatal("no subagents is a zero count")
	}
}

func TestNothingIsReadWhileNoPageLooks(t *testing.T) {
	f := newFiles()
	f.set("local|w1:p1", agents(transcript.SubagentWorking)...)
	tr, c, _ := tracker(f)
	tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusWorking)))
	settle(t, tr, "local")
	if f.count("local|w1:p1") != 0 || tr.Count("local", "w1:p1") != nil {
		t.Fatal("a pane was read before any page asked for the roster")
	}

	tr.Touch()
	waitFor(t, "the count", func() bool { return tr.Count("local", "w1:p1") != nil })
	if got := *tr.Count("local", "w1:p1"); got != (Count{Working: 1}) {
		t.Fatalf("count: %+v", got)
	}

	c.pass(WatchedFor)
	tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusWorking)))
	settle(t, tr, "local")
	if n := f.count("local|w1:p1"); n != 1 {
		t.Fatalf("a page gone for WatchedFor still has the pane read: %d reads", n)
	}
}

func TestAWorkingPaneIsReadMoreOftenThanAResting(t *testing.T) {
	f := newFiles()
	tr, c, _ := tracker(f)
	tr.Touch()
	panes := snapshot(claude("w1:p1", herdr.StatusWorking), claude("w1:p2", herdr.StatusIdle))
	observe := func() {
		tr.Touch()
		tr.Observe("local", panes)
		settle(t, tr, "local")
	}
	observe()
	if f.count("local|w1:p1") != 1 || f.count("local|w1:p2") != 1 {
		t.Fatalf("a pane first seen is read at once: %v", f.reads)
	}

	c.pass(WorkingEvery - time.Second)
	observe()
	if f.count("local|w1:p1") != 1 {
		t.Fatal("a working pane was read again before WorkingEvery")
	}
	c.pass(time.Second)
	observe()
	if f.count("local|w1:p1") != 2 || f.count("local|w1:p2") != 1 {
		t.Fatalf("after WorkingEvery the working pane alone is read again: %v", f.reads)
	}

	c.pass(RestingEvery - WorkingEvery - time.Second)
	observe()
	if f.count("local|w1:p2") != 1 {
		t.Fatal("a resting pane was read again before RestingEvery")
	}
	c.pass(time.Second)
	observe()
	if f.count("local|w1:p2") != 2 {
		t.Fatalf("after RestingEvery the resting pane is read again: %v", f.reads)
	}
}

func TestARestingPaneWithSubagentsAtWorkIsReadAtTheWorkingPace(t *testing.T) {
	f := newFiles()
	f.set("local|w1:p1", agents(transcript.SubagentWorking, transcript.SubagentDone)...)
	tr, c, _ := tracker(f)
	observe := func(d time.Duration) {
		c.pass(d)
		tr.Touch()
		tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusIdle)))
		settle(t, tr, "local")
	}
	observe(0)
	observe(WorkingEvery)
	if n := f.count("local|w1:p1"); n != 2 {
		t.Fatalf("a resting pane with a subagent working waits for RestingEvery: %d reads", n)
	}

	f.set("local|w1:p1", agents(transcript.SubagentQuiet, transcript.SubagentDone)...)
	observe(WorkingEvery)
	observe(WorkingEvery)
	if n := f.count("local|w1:p1"); n != 3 {
		t.Fatalf("a subagent gone quiet may never finish and must not hold the working pace: %d reads", n)
	}
	observe(RestingEvery - WorkingEvery)
	if n := f.count("local|w1:p1"); n != 4 {
		t.Fatalf("after RestingEvery it is read again: %d reads", n)
	}
}

func TestOnlyClaudeCodePanesAreRead(t *testing.T) {
	f := newFiles()
	tr, _, _ := tracker(f)
	tr.Touch()
	tr.Observe("local", snapshot(
		herdr.PaneInfo{PaneID: "w1:p1", Agent: "codex", AgentStatus: herdr.StatusWorking},
		herdr.PaneInfo{PaneID: "w1:p2", AgentStatus: herdr.StatusIdle},
		claude("w1:p3", herdr.StatusIdle),
	))
	settle(t, tr, "local")
	if f.count("local|w1:p1") != 0 || f.count("local|w1:p2") != 0 || f.count("local|w1:p3") != 1 {
		t.Fatalf("reads: %v", f.reads)
	}
}

func TestAMachineNotConnectedIsSkipped(t *testing.T) {
	f := newFiles()
	f.down["gog"] = true
	tr, _, _ := tracker(f)
	tr.Touch()
	tr.Observe("gog", snapshot(claude("w1:p1", herdr.StatusWorking)))
	settle(t, tr, "gog")
	if f.count("gog|w1:p1") != 0 {
		t.Fatal("a machine away was read")
	}
	f.mu.Lock()
	f.down["gog"] = false
	f.mu.Unlock()
	tr.Touch()
	settle(t, tr, "gog")
	if f.count("gog|w1:p1") != 1 {
		t.Fatal("the machine back was not read on the next look")
	}
}

func TestOneSweepAtATimePerMachine(t *testing.T) {
	f := newFiles()
	f.hold = make(chan struct{})
	tr, _, _ := tracker(f)
	tr.Touch()
	tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusWorking), claude("w1:p2", herdr.StatusWorking)))
	waitFor(t, "the first read", func() bool { return f.count("local|w1:p1") == 1 })
	for range 5 {
		tr.Touch()
		tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusWorking), claude("w1:p2", herdr.StatusWorking)))
	}
	close(f.hold)
	settle(t, tr, "local")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mostAtOnce != 1 || f.reads["local|w1:p1"] != 1 || f.reads["local|w1:p2"] != 1 {
		t.Fatalf("one machine read %d panes at once, reads %v", f.mostAtOnce, f.reads)
	}
}

func TestOnChangeOnlyWhenACountMoves(t *testing.T) {
	f := newFiles()
	f.set("local|w1:p1", agents(transcript.SubagentWorking, transcript.SubagentWorking)...)
	tr, c, changes := tracker(f)
	read := func() {
		c.pass(WorkingEvery)
		tr.Touch()
		tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusWorking)))
		settle(t, tr, "local")
	}
	read()
	if changes() != 1 {
		t.Fatalf("the first count is a change: %d", changes())
	}
	read()
	if changes() != 1 {
		t.Fatalf("the same count again is no change: %d", changes())
	}
	f.set("local|w1:p1", agents(transcript.SubagentWorking, transcript.SubagentDone)...)
	read()
	if changes() != 2 || *tr.Count("local", "w1:p1") != (Count{Working: 1, Done: 1}) {
		t.Fatalf("a subagent finishing moves the count: %d %+v", changes(), tr.Count("local", "w1:p1"))
	}
	f.set("local|w1:p1")
	read()
	if changes() != 3 || tr.Count("local", "w1:p1") != nil {
		t.Fatalf("a conversation with none left reads as nil: %d %+v", changes(), tr.Count("local", "w1:p1"))
	}
}

func TestAFailedReadKeepsTheLastCountAndComplainsOnceAnHour(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)
	f := newFiles()
	f.set("gog|w1:p1", agents(transcript.SubagentQuiet)...)
	tr, c, changes := tracker(f)
	read := func(d time.Duration) {
		c.pass(d)
		tr.Touch()
		tr.Observe("gog", snapshot(claude("w1:p1", herdr.StatusWorking), claude("w1:p2", herdr.StatusWorking)))
		settle(t, tr, "gog")
	}
	read(0)
	f.mu.Lock()
	f.failing["gog|w1:p1"] = errors.New("ssh: connection reset")
	f.failing["gog|w1:p2"] = errors.New("ssh: connection reset")
	f.mu.Unlock()
	read(WorkingEvery)
	read(WorkingEvery)
	if got := tr.Count("gog", "w1:p1"); got == nil || *got != (Count{Quiet: 1}) || changes() != 1 {
		t.Fatalf("a failed read lost the count: %+v, %d changes", got, changes())
	}
	if n := strings.Count(logged.String(), "cannot be read"); n != 1 {
		t.Fatalf("logged %d times within the hour:\n%s", n, logged.String())
	}
	read(ComplainEvery)
	if n := strings.Count(logged.String(), "cannot be read"); n != 2 {
		t.Fatalf("an hour later it is logged again: %d", n)
	}
}

func TestAClosedPaneAndARemovedMachineAreForgotten(t *testing.T) {
	f := newFiles()
	f.set("local|w1:p1", agents(transcript.SubagentWorking)...)
	f.set("gog|w2:p1", agents(transcript.SubagentDone)...)
	tr, c, _ := tracker(f)
	tr.Touch()
	tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusWorking)))
	tr.Observe("gog", snapshot(claude("w2:p1", herdr.StatusIdle)))
	waitFor(t, "both counts", func() bool { return tr.Count("local", "w1:p1") != nil && tr.Count("gog", "w2:p1") != nil })
	settle(t, tr, "local")

	f.mu.Lock()
	f.hold = make(chan struct{})
	f.mu.Unlock()
	c.pass(WorkingEvery)
	tr.Touch()
	waitFor(t, "a read under way", func() bool { return f.count("local|w1:p1") == 2 })
	tr.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "w1:p1"}, From: herdr.StatusWorking, To: events.Gone})
	close(f.hold)
	settle(t, tr, "local")
	if tr.Count("local", "w1:p1") != nil {
		t.Fatal("a closed pane keeps its count, or a read under way wrote it back")
	}

	tr.Forget("gog")
	if tr.Count("gog", "w2:p1") != nil {
		t.Fatal("a removed machine keeps its counts")
	}
}

func TestAPaneNoLongerClaudeCodesIsDropped(t *testing.T) {
	f := newFiles()
	f.set("local|w1:p1", agents(transcript.SubagentDone)...)
	tr, _, _ := tracker(f)
	tr.Touch()
	tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusIdle)))
	waitFor(t, "the count", func() bool { return tr.Count("local", "w1:p1") != nil })
	tr.Observe("local", snapshot(herdr.PaneInfo{PaneID: "w1:p1", AgentStatus: herdr.StatusIdle}))
	if tr.Count("local", "w1:p1") != nil {
		t.Fatal("a pane whose agent left keeps its count")
	}
	settle(t, tr, "local")
}

func TestANilTrackerAnswersNothing(t *testing.T) {
	var tr *Tracker
	tr.Touch()
	tr.Observe("local", snapshot(claude("w1:p1", herdr.StatusWorking)))
	if tr.Count("local", "w1:p1") != nil {
		t.Fatal("a nil tracker counted")
	}
}
