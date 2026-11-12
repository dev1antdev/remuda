package schedule

import (
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/text"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// Started is the place a run of an agent schedule made, watched until the agent has answered: the next run waits for it,
// and a schedule that closes its tabs takes it down then.
type Started struct {
	Machine       string    `json:"machine"`
	Pane          string    `json:"pane"`
	Tab           string    `json:"tab"`
	Workspace     string    `json:"workspace"`
	MadeWorkspace bool      `json:"made_workspace"` // the workspace was made for it: closing takes the workspace, not just the tab
	Worked        bool      `json:"worked"`         // the agent has been seen working since the start
	At            time.Time `json:"at"`
}

// Agents is how the store starts an agent for a schedule, takes its place down, reads what it answered and how it stands
// now; wired by the app.
type Agents struct {
	Start  func(s Schedule) (Started, error)
	Close  func(run Started) error
	Answer func(machine, pane string) string // the agent's last answer, "" for none
	Status func(machine, pane string) string // herdr's agent status of the pane, events.Gone when no agent runs there now, "" when the machine has not said
	// Denied is the calls its conversation was denied without asking, and whether its latest turn stopped on them
	Denied func(machine, pane string) transcript.Denials
	// Hold is why the schedule's agent kind should not start on its machine now — its plan window at 95 % or forecast
	// to run out within the half hour (limits.Holding) — or "".
	Hold func(s Schedule) string
	// Offer hands a run the plan window held to the person, to run on the other vendor's agent.
	Offer func(s Schedule, why string)
}

// Swap is the agent a run starts instead of the schedule's own — an offer the person took — the saved schedule unchanged.
type Swap struct {
	Agent string
	Model string // "" for the agent's default
}

// checkAgent trims and checks an agent schedule: a machine and a folder, an agent that is not a shell, a prompt. Its
// command line is checked once the switch is known (Start).
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
	s.Since = ""
	if s.Label == "" {
		s.Label = text.FirstLine(s.Prompt, 60)
	}
	return nil
}

// fireAgent starts the agent through the app's Start and keeps the place to watch it, unless the agent the last run started
// is still at it — then the run is skipped, so a schedule every few minutes never piles agents up — or the agent's plan
// window holds the start: then it is skipped too, and the person offered to run it on the other vendor. A swap starts
// another agent kind this once.
func (st *Store) fireAgent(s *Schedule, swap *Swap) {
	st.mu.Lock()
	cur, ok := st.items[s.ID]
	if !ok {
		st.mu.Unlock()
		return
	}
	cp := *cur
	st.mu.Unlock()
	if cp.LastPane != nil {
		if why, words := st.previousRun(cp, *cp.LastPane); why != "" {
			st.noteSkip(cp.ID, why, words)
			return
		}
	}
	if swap != nil {
		cp.Agent, cp.Model, cp.Args = swap.Agent, swap.Model, "" // the arguments were the other agent's
	}
	if st.Agents.Hold != nil {
		if why := st.Agents.Hold(cp); why != "" {
			st.noteSkip(cp.ID, SkipLimit, why)
			if swap == nil && st.Agents.Offer != nil {
				st.Agents.Offer(cp, why)
			}
			return
		}
	}
	now := st.now()
	var started Started
	err := errors.New("no way to start an agent")
	if st.Agents.Start != nil {
		started, err = st.Agents.Start(cp)
	}
	st.mu.Lock()
	cur, ok = st.items[s.ID]
	if !ok {
		st.mu.Unlock()
		return
	}
	cur.LastRun, cur.Skipped, cur.LastDenied = now, "", nil
	if err != nil {
		cur.LastOK, cur.LastResult = false, err.Error()
	} else {
		cur.LastOK, cur.LastResult = true, "started in "+started.Pane
		if swap != nil {
			cur.LastResult += " with " + agentLabel(swap.Agent) + ", for the plan limit"
		}
		started.Machine, started.At = cur.Machine, now
		cur.LastPane = &started
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

// agentLabel is an agent kind as the page names it, "Codex" for codex.
func agentLabel(kind string) string {
	for _, a := range sessions.Agents {
		if a.ID == kind {
			return a.Label
		}
	}
	return kind
}

// startingFor is how long an agent that has not worked yet counts as coming up: the first prompt's tries and a minute more.
var startingFor = sessions.FirstPromptTimeout + time.Minute

// previousRun looks at the agent the last run started as it stands now and says why the next run must wait for it, or ""
// once it is through — answered (and taken down, when the schedule closes its tabs, as the status change would have), gone,
// or never having taken its prompt (the launcher pushed that; its tab is left for the person to look at).
func (st *Store) previousRun(s Schedule, run Started) (why, words string) {
	status := ""
	if st.Agents.Status != nil {
		status = st.Agents.Status(run.Machine, run.Pane)
	}
	agent := "its previous agent (" + run.Pane + ")"
	switch status {
	case "":
		return SkipUnsure, "no word yet on how " + agent + " stands"
	case events.Gone:
		st.forget(s.ID, run)
		return "", ""
	case herdr.StatusWorking:
		return SkipWorking, agent + " is still working"
	case herdr.StatusBlocked:
		return SkipAsking, agent + " waits for an answer"
	case herdr.StatusDone:
	default: // idle or unknown: through, if it ever worked or has an answer to show
		if !run.Worked && (st.Agents.Answer == nil || st.Agents.Answer(run.Machine, run.Pane) == "") {
			if st.now().Sub(run.At) < startingFor {
				return SkipStarting, agent + " has not started on the prompt yet"
			}
			st.forget(s.ID, run)
			return "", ""
		}
	}
	if st.claim(s.ID, run) && s.Close {
		st.settle(s.ID, s.Label, run, true)
	}
	return "", ""
}

// claim lets go of a watched run for the one who settles it; false when it is no longer the run watched (a status change
// settled it meanwhile).
func (st *Store) claim(id string, run Started) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	cur, ok := st.items[id]
	if !ok || cur.LastPane == nil || cur.LastPane.Machine != run.Machine || cur.LastPane.Pane != run.Pane {
		return false
	}
	cur.LastPane = nil
	if err := st.saveLocked(); err != nil {
		log.Printf("schedules: %v", err)
	}
	return true
}

// forget stops watching a run that is gone, or never took its prompt.
func (st *Store) forget(id string, run Started) { st.claim(id, run) }

// OnStatus hears every pane's status change: an agent a schedule started and watches is through once it has answered —
// its answer noted, its tab taken down when the schedule closes its tabs — never while it asks something (the person
// decides); a pane gone on its own is forgotten.
func (st *Store) OnStatus(e events.PaneStatus) {
	type due struct {
		id, label string
		run       Started
		close     bool
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
			// the close is on its way: a further change of the pane before it lands (herdr reads idle once the finished
			// pane is focused) must not close and push a second time
			s.LastPane = nil
			changed = true
			closing = append(closing, due{s.ID, s.Label, *run, s.Close})
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
			st.settle(d.id, d.label, d.run, d.close)
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

// settle reads the answer of a run that is through, and the calls it was denied without asking, and notes them on the
// schedule — a turn that stopped on those calls answered nothing, and is said to have stopped; with close, the place is
// taken down too and a push says what the agent answered.
func (st *Store) settle(id, label string, run Started, close bool) {
	answer := ""
	if st.Agents.Answer != nil {
		answer = text.FirstLine(st.Agents.Answer(run.Machine, run.Pane), 120)
	}
	var denied transcript.Denials
	if st.Agents.Denied != nil {
		denied = st.Agents.Denied(run.Machine, run.Pane)
	}
	outcome := "answered"
	stopped := answer == "" && denied.EndsTurn
	if stopped {
		outcome = "stopped on calls it was denied without asking"
	}
	var err error
	if close {
		err = errors.New("no way to close the tab")
		if st.Agents.Close != nil {
			err = st.Agents.Close(run)
		}
	}
	st.mu.Lock()
	cur, ok := st.items[id]
	// herdr's close makes the pane gone, which OnStatus may have heard (and cleared) before this lock: the outcome still goes on
	// the schedule unless a newer run replaced the pane meanwhile
	if ok && (cur.LastPane == nil || cur.LastPane.Pane == run.Pane) {
		cur.LastPane, cur.Skipped, cur.LastDenied = nil, "", denied.Calls
		switch {
		case err != nil:
			cur.LastOK, cur.LastResult = false, outcome+", but the tab could not be closed: "+err.Error()
		case stopped:
			cur.LastOK, cur.LastResult = false, outcome
		case answer != "":
			cur.LastOK, cur.LastResult = true, "answered: "+answer
		case close:
			cur.LastOK, cur.LastResult = true, "answered and closed"
		default:
			cur.LastOK, cur.LastResult = true, "answered"
		}
		if n := len(denied.Calls); n > 0 {
			cur.LastResult += " · " + transcript.DeniedCount(n)
		}
		if serr := st.saveLocked(); serr != nil {
			log.Printf("schedules: %v", serr)
		}
	}
	st.mu.Unlock()
	if !close || st.notify == nil {
		return
	}
	body := answer
	if body == "" {
		body = "The tab is closed"
	}
	if words := transcript.DeniedWords(denied.Calls, 3); words != "" {
		body += " · " + words
	}
	title := "Scout “" + label + "” answered"
	if stopped {
		title = "Scout “" + label + "” stopped on calls it was denied"
	}
	st.notify(push.Payload{
		Machine: run.Machine, Kind: "schedule",
		Title: title,
		Body:  body + " · the conversation stays readable through the search",
		Tag:   "schedule|" + id,
	})
}
