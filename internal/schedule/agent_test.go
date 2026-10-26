package schedule

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/push"
)

// scoutDoor records what the store asked of the app for an agent schedule and answers on paper.
type scoutDoor struct {
	mu       sync.Mutex
	starts   []Schedule
	closed   []Started
	startErr error
	closeErr error
	answer   string
	pushed   []push.Payload
}

func (d *scoutDoor) agents() Agents {
	return Agents{
		Start: func(s Schedule) (Started, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.starts = append(d.starts, s)
			if d.startErr != nil {
				return Started{}, d.startErr
			}
			return Started{Pane: "w5:p2", Tab: "w5:t2", Workspace: "w5"}, nil
		},
		Close: func(run Started) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.closed = append(d.closed, run)
			return d.closeErr
		},
		Answer: func(machine, pane string) string { return d.answer },
	}
}

func (d *scoutDoor) notify(p push.Payload) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pushed = append(d.pushed, p)
}

func scoutStore(t *testing.T, dir string) (*Store, *scoutDoor) {
	t.Helper()
	door := &scoutDoor{answer: "Made three tasks."}
	st := New(dir, nil, door.notify, nil)
	st.Agents = door.agents()
	return st, door
}

var scout = Schedule{Kind: KindAgent, Machine: "local", Dir: "/home/me/remuda", Agent: "codex", Model: "gpt-5", Args: "--full-auto", Cron: "0 */3 * * *", Prompt: "Look for what could be better", Close: true, Enabled: true}

func status(pane, from, to string) events.PaneStatus {
	return events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: pane}, From: from, To: to}
}

func TestAgentScheduleStartsAndWatchesThePane(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	s, err := st.Put(scout)
	if err != nil {
		t.Fatal(err)
	}
	if s.Label != "Look for what could be better" || s.Kind != KindAgent {
		t.Fatalf("put %+v", s)
	}
	got, err := st.Run(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(door.starts) != 1 || door.starts[0].Agent != "codex" || door.starts[0].Model != "gpt-5" || door.starts[0].Prompt != scout.Prompt {
		t.Fatalf("start asked with %+v", door.starts)
	}
	if !got.LastOK || got.LastResult != "started in w5:p2" || got.LastPane == nil || got.LastPane.Pane != "w5:p2" || got.LastPane.Machine != "local" {
		t.Fatalf("after the run: %+v pane %+v", got, got.LastPane)
	}

	// a question open is the person's: nothing closes; idle before any work is the agent coming up
	st.OnStatus(status("w5:p2", "idle", "blocked"))
	st.OnStatus(status("w5:p2", "unknown", "idle"))
	st.Wait()
	if len(door.closed) != 0 {
		t.Fatalf("closed while blocked or coming up: %+v", door.closed)
	}
	st.OnStatus(status("w5:p2", "idle", "working"))
	st.OnStatus(status("w9:p1", "working", "done")) // another pane
	st.Wait()
	if len(door.closed) != 0 || !st.List()[0].LastPane.Worked {
		t.Fatalf("another pane's finish closed ours, or working was not noted: %+v %+v", door.closed, st.List()[0].LastPane)
	}
	st.OnStatus(status("w5:p2", "working", "done"))
	st.Wait()
	if len(door.closed) != 1 || door.closed[0].Tab != "w5:t2" {
		t.Fatalf("closed %+v", door.closed)
	}
	after := st.List()[0]
	if after.LastPane != nil || after.LastResult != "answered: Made three tasks." || !after.LastOK {
		t.Fatalf("after the answer: %+v", after)
	}
	if len(door.pushed) != 1 || door.pushed[0].Kind != "schedule" || !strings.Contains(door.pushed[0].Title, "answered") || !strings.Contains(door.pushed[0].Body, "Made three tasks.") {
		t.Fatalf("pushed %+v", door.pushed)
	}
	st.OnStatus(status("w5:p2", "done", "gone")) // the tab went: nothing more to do
	st.Wait()
	if len(door.closed) != 1 {
		t.Fatalf("closed again: %+v", door.closed)
	}
}

func TestAgentScheduleNotesTheAnswerWhenTheCloseIsHeardFirst(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	s, _ := st.Put(scout)
	_, _ = st.Run(s.ID)
	agents := door.agents()
	agents.Close = func(run Started) error {
		st.OnStatus(status(run.Pane, "done", "gone")) // the watcher sees the pane go while herdr is still answering the close
		return nil
	}
	st.Agents = agents
	st.OnStatus(status("w5:p2", "working", "done"))
	st.Wait()
	if after := st.List()[0]; after.LastResult != "answered: Made three tasks." || after.LastPane != nil {
		t.Fatalf("after a close heard first: %+v", after)
	}
}

func TestAgentScheduleIdleAfterWorkIsAnAnswerToo(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	s, _ := st.Put(scout)
	_, _ = st.Run(s.ID)
	st.OnStatus(status("w5:p2", "working", "idle"))
	st.Wait()
	if len(door.closed) != 1 {
		t.Fatalf("idle after working did not close: %+v", door.closed)
	}
}

func TestAgentScheduleWithoutCloseLeavesTheTab(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	open := scout
	open.Close = false
	s, _ := st.Put(open)
	got, _ := st.Run(s.ID)
	if got.LastPane != nil {
		t.Fatalf("watched without Close: %+v", got.LastPane)
	}
	st.OnStatus(status("w5:p2", "working", "done"))
	st.Wait()
	if len(door.closed) != 0 {
		t.Fatalf("closed: %+v", door.closed)
	}
}

func TestAgentScheduleRemembersThePaneAcrossARestart(t *testing.T) {
	dir := t.TempDir()
	st, _ := scoutStore(t, dir)
	s, _ := st.Put(scout)
	_, _ = st.Run(s.ID)

	again, door := scoutStore(t, dir)
	if got := again.List()[0]; got.LastPane == nil || got.LastPane.Pane != "w5:p2" {
		t.Fatalf("the pane was not remembered: %+v", got)
	}
	again.OnStatus(status("w5:p2", "working", "done"))
	again.Wait()
	if len(door.closed) != 1 || again.List()[0].LastPane != nil {
		t.Fatalf("closed %+v, list %+v", door.closed, again.List()[0])
	}
}

func TestAgentScheduleNotesAFailedStartAndAFailedClose(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	door.startErr = errors.New("the tab is there, but the agent did not start")
	s, _ := st.Put(scout)
	got, _ := st.Run(s.ID)
	if got.LastOK || got.LastResult != door.startErr.Error() || got.LastPane != nil || len(door.pushed) != 1 || !strings.Contains(door.pushed[0].Title, "could not run") {
		t.Fatalf("failed start: %+v pushed %+v", got, door.pushed)
	}

	door.startErr, door.closeErr, door.pushed = nil, errors.New("no such tab"), nil
	_, _ = st.Run(s.ID)
	st.OnStatus(status("w5:p2", "working", "done"))
	st.Wait()
	if after := st.List()[0]; after.LastOK || !strings.HasPrefix(after.LastResult, "answered, but the tab could not be closed") || after.LastPane != nil {
		t.Fatalf("failed close: %+v", after)
	}
}

func TestAgentSchedulePutChecksItsFields(t *testing.T) {
	st, _ := scoutStore(t, t.TempDir())
	bad := map[string]func(s *Schedule){
		"a shell":           func(s *Schedule) { s.Agent = "shell" },
		"an unknown agent":  func(s *Schedule) { s.Agent = "nobody" },
		"no prompt":         func(s *Schedule) { s.Prompt = " " },
		"an unclosed quote": func(s *Schedule) { s.Args = `--flag "oops` },
		"no folder":         func(s *Schedule) { s.Dir = "" },
		"a bad cron":        func(s *Schedule) { s.Cron = "0 25 * * *" },
	}
	for name, spoil := range bad {
		s := scout
		spoil(&s)
		if _, err := st.Put(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	s, err := st.Put(scout)
	if err != nil || s.Since != "" || s.Model != "gpt-5" {
		t.Fatalf("a sound one refused: %v %+v", err, s)
	}
}
