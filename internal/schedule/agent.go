package schedule

import (
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/text"
)

// Started is the place a run of an agent schedule made: watched, when the schedule closes its tabs, until the agent has answered.
type Started struct {
	Machine       string    `json:"machine"`
	Pane          string    `json:"pane"`
	Tab           string    `json:"tab"`
	Workspace     string    `json:"workspace"`
	MadeWorkspace bool      `json:"made_workspace"` // the workspace was made for it: closing takes the workspace, not just the tab
	Worked        bool      `json:"worked"`         // the agent has been seen working since the start
	At            time.Time `json:"at"`
}

// Agents is how the store starts an agent for a schedule, takes its place down, and reads what it answered; wired by the app.
type Agents struct {
	Start  func(s Schedule) (Started, error)
	Close  func(run Started) error
	Answer func(machine, pane string) string // the agent's last answer, "" for none
}

// checkAgent trims and checks an agent schedule: a machine and a folder, an agent that is not a shell, a prompt, arguments
// that split, a model the agent's CLI takes.
func checkAgent(s *Schedule) error {
	s.Agent, s.Model, s.Args = strings.TrimSpace(s.Agent), strings.TrimSpace(s.Model), strings.TrimSpace(s.Args)
	if s.Machine == "" || !strings.HasPrefix(s.Dir, "/") {
		return errors.New("a machine and an absolute folder are required")
	}
	if s.Agent == "" || s.Agent == "shell" || !sessions.KnownAgent(s.Agent) {
		return errors.New("an agent is required, not a shell")
	}
	if s.Prompt == "" {
		return errors.New("the prompt is empty")
	}
	if _, err := sessions.SplitArgs(s.Args); err != nil {
		return err
	}
	if s.Model != "" {
		if _, ok := sessions.ModelArgs(s.Agent, s.Model); !ok {
			return errors.New(s.Agent + " takes no model flag")
		}
	}
	s.Since = ""
	if s.Label == "" {
		s.Label = text.FirstLine(s.Prompt, 60)
	}
	return nil
}

// fireAgent starts the agent through the app's Start and notes where; with Close on, the place is kept to be watched.
func (st *Store) fireAgent(s *Schedule) {
	st.mu.Lock()
	cp := *s
	st.mu.Unlock()
	now := st.now()
	var started Started
	err := errors.New("no way to start an agent")
	if st.Agents.Start != nil {
		started, err = st.Agents.Start(cp)
	}
	st.mu.Lock()
	cur, ok := st.items[s.ID]
	if !ok {
		st.mu.Unlock()
		return
	}
	cur.LastRun = now
	cur.LastPane = nil
	if err != nil {
		cur.LastOK, cur.LastResult = false, err.Error()
	} else {
		cur.LastOK, cur.LastResult = true, "started in "+started.Pane
		if cur.Close {
			started.Machine, started.At = cur.Machine, now
			cur.LastPane = &started
		}
	}
	if serr := st.saveLocked(); serr != nil {
		log.Printf("schedules: %v", serr)
	}
	label, machine, dir := cur.Label, cur.Machine, cur.Dir
	st.mu.Unlock()
	if err != nil && st.notify != nil {
		st.notify(push.Payload{
			Machine: machine, Kind: "schedule",
			Title: "Schedule “" + label + "” could not run",
			Body:  err.Error() + " · " + filepath.Base(dir),
			Tag:   "schedule|" + s.ID,
		})
	}
}

// OnStatus hears every pane's status change: an agent a schedule started and watches is taken down once it has answered,
// never while it asks something (the person decides); a pane gone on its own is forgotten.
func (st *Store) OnStatus(e events.PaneStatus) {
	type due struct {
		id, label string
		run       Started
	}
	var closing []due
	changed := false
	st.mu.Lock()
	for _, s := range st.items {
		run := s.LastPane
		if run == nil || run.Machine != e.Pane.Machine || run.Pane != e.Pane.Pane {
			continue
		}
		switch {
		case e.To == events.Gone:
			s.LastPane = nil
			changed = true
		case e.To == "working":
			if !run.Worked {
				run.Worked = true
				changed = true
			}
		case answered(e.To, e.From, run.Worked):
			closing = append(closing, due{s.ID, s.Label, *run})
		}
	}
	if changed {
		if err := st.saveLocked(); err != nil {
			log.Printf("schedules: %v", err)
		}
	}
	st.mu.Unlock()
	for _, d := range closing {
		st.inflight.Add(1)
		go func(d due) { // herdr's close and the transcript read must not hold the watcher's poll
			defer st.inflight.Done()
			st.closeAnswered(d.id, d.label, d.run)
		}(d)
	}
}

// answered says whether a status change means the agent is through: done, or idle after it worked.
func answered(to, from string, worked bool) bool {
	switch to {
	case "done":
		return true
	case "idle":
		return worked || from == "working"
	}
	return false
}

// closeAnswered reads the answer, takes the place down and notes the outcome on the schedule and in a push.
func (st *Store) closeAnswered(id, label string, run Started) {
	answer := ""
	if st.Agents.Answer != nil {
		answer = text.FirstLine(st.Agents.Answer(run.Machine, run.Pane), 120)
	}
	err := errors.New("no way to close the tab")
	if st.Agents.Close != nil {
		err = st.Agents.Close(run)
	}
	st.mu.Lock()
	cur, ok := st.items[id]
	// herdr's close makes the pane gone, which OnStatus may have heard (and cleared) before this lock: the outcome still goes on
	// the schedule unless a newer run replaced the pane meanwhile
	if ok && (cur.LastPane == nil || cur.LastPane.Pane == run.Pane) {
		cur.LastPane = nil
		switch {
		case err != nil:
			cur.LastOK, cur.LastResult = false, "answered, but the tab could not be closed: "+err.Error()
		case answer != "":
			cur.LastResult = "answered: " + answer
		default:
			cur.LastResult = "answered and closed"
		}
		if serr := st.saveLocked(); serr != nil {
			log.Printf("schedules: %v", serr)
		}
	}
	st.mu.Unlock()
	if st.notify == nil {
		return
	}
	body := answer
	if body == "" {
		body = "The tab is closed"
	}
	st.notify(push.Payload{
		Machine: run.Machine, Kind: "schedule",
		Title: "Scout “" + label + "” answered",
		Body:  body + " · the conversation stays readable through the search",
		Tag:   "schedule|" + id,
	})
}
