package sleep

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/watch"
)

// Client is what sleeping and waking need of herdr's client: the leave, a place for a pane gone, and whether a
// shell is free.
type Client interface {
	restart.Leaver
	sessions.Placer
	PaneAtPrompt(paneID string) (bool, error)
}

// Service puts workspaces to sleep and wakes them, a job each the person can follow.
type Service struct {
	Store *Store
	Watch *watch.Watcher
	Jobs  *jobs.Runner
	// Lock is the per-pane typing lock: a leave and a start hold it, so nothing else lands in the bare shell between.
	Lock func(machineID, paneID string) sync.Locker
	// Typed hears what a sleep types into a pane — an agent's exit, or an Esc as a key — for the activity; nil hears none.
	Typed func(machineID, paneID, text string, keys bool)
	// Skip names a workspace remuda keeps for itself (the concierge's): it never sleeps.
	Skip func(machineID, workspaceID string) bool

	mu    sync.Mutex
	doing map[string]string // machine|workspace → "sleep" | "wake" while one runs
}

// ErrNotAsleep is a wake of a workspace that is awake.
var ErrNotAsleep = errors.New("this workspace is not asleep")

// ErrNoWorkspace is a sleep or wake of a workspace the machine does not have.
var ErrNoWorkspace = errors.New("no such workspace there")

// Busy is an agent still at work or asking the person something when a sleep was asked for.
type Busy struct {
	Pane   string `json:"pane"`
	Agent  string `json:"agent"`
	Status string `json:"status"` // working | blocked
}

// BusyError is a sleep refused until the person confirms: agents there still work or ask.
type BusyError struct {
	Workspace string
	Busy      []Busy
}

func (e *BusyError) Error() string {
	var still []string
	for _, b := range e.Busy {
		still = append(still, restart.Title(b.Agent)+" is "+map[string]string{herdr.StatusWorking: "working", herdr.StatusBlocked: "asking you something"}[b.Status])
	}
	return fmt.Sprintf("Sleep %s? Still busy: %s. Sleeping interrupts them. Waking resumes their conversations where they stopped.", e.Workspace, strings.Join(still, ", "))
}

// Started is what a sleep or a wake answers at once: its job, and how many agents it is about.
type Started struct {
	Job    string `json:"job"`
	Agents int    `json:"agents"`
}

// claim takes the one sleep or wake of a workspace at a time; the error says what runs there.
func (s *Service) claim(machineID, workspaceID, what string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doing == nil {
		s.doing = map[string]string{}
	}
	key := machineID + "|" + workspaceID
	switch s.doing[key] {
	case "sleep":
		return errors.New("this workspace is going to sleep right now")
	case "wake":
		return errors.New("this workspace is waking right now")
	}
	s.doing[key] = what
	return nil
}

func (s *Service) release(machineID, workspaceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.doing, machineID+"|"+workspaceID)
}

// Plan is what a sleep of the workspace would record: every agent remuda can bring back on a session, with the ones
// still at work or asking; the agents with no session to resume are left running, named in left.
func Plan(snap *herdr.Snapshot, workspaceID string) (w Workspace, busy []Busy, left []string, err error) {
	ws := snap.Workspace(workspaceID)
	if ws == nil {
		return Workspace{}, nil, nil, ErrNoWorkspace
	}
	w = Workspace{Name: ws.Label, Dir: snap.WorkspaceDir(ws), At: time.Now()}
	tabs := map[string]herdr.TabInfo{}
	for _, tab := range snap.Tabs {
		tabs[tab.TabID] = tab
	}
	for i := range snap.Panes {
		p := &snap.Panes[i]
		if p.WorkspaceID != workspaceID || p.Agent == "" {
			continue
		}
		session := restart.SessionOf(p)
		if !restart.Knows(p.Agent) || session == "" {
			left = append(left, fmt.Sprintf("%s in %s", restart.Title(p.Agent), p.PaneID))
			continue
		}
		tab := tabs[p.TabID]
		name := ""
		if tab.Label != "" && tab.Label != strconv.Itoa(tab.Number) {
			name = tab.Label
		}
		dir := p.Dir()
		if dir == "" {
			dir = w.Dir
		}
		w.Agents = append(w.Agents, Agent{Pane: p.PaneID, Tab: p.TabID, TabName: name, Kind: p.Agent, Session: session, Dir: dir, Title: p.DisplayTitle()})
		if p.AgentStatus == herdr.StatusWorking || p.AgentStatus == herdr.StatusBlocked {
			busy = append(busy, Busy{Pane: p.PaneID, Agent: p.Agent, Status: p.AgentStatus})
		}
	}
	sort.Slice(w.Agents, func(i, j int) bool { return w.Agents[i].Pane < w.Agents[j].Pane })
	return w, busy, left, nil
}

// Sleep puts a workspace of the machine to sleep: its agents recorded at once, so the roster says asleep, then each
// asked to leave in a job. Agents at work or asking refuse it with a BusyError unless the person said anyway.
func (s *Service) Sleep(mc *machines.Machine, workspaceID string, anyway bool) (Started, error) {
	if s.Skip != nil && s.Skip(mc.ID(), workspaceID) {
		return Started{}, errors.New("the concierge's workspace never sleeps: stop it from its own place")
	}
	client, _, err := mc.Ready()
	if err != nil {
		return Started{}, err
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		return Started{}, err
	}
	w, busy, left, err := Plan(snap, workspaceID)
	if err != nil {
		return Started{}, err
	}
	if len(w.Agents) == 0 {
		why := "no agent runs there"
		if len(left) > 0 {
			why = "no agent there has a session to resume: " + strings.Join(left, ", ")
		}
		return Started{}, fmt.Errorf("nothing to put to sleep in %s: %s", w.Name, why)
	}
	if len(busy) > 0 && !anyway {
		return Started{}, &BusyError{Workspace: w.Name, Busy: busy}
	}
	if err := s.claim(mc.ID(), workspaceID, "sleep"); err != nil {
		return Started{}, err
	}
	s.Store.put(mc.ID(), workspaceID, w)
	var lines []string
	for _, a := range w.Agents {
		lines = append(lines, fmt.Sprintf("%s in %s   (%s)", restart.Title(a.Kind), a.Pane, restart.Resume(a.Kind, a.Session)))
	}
	command := "# on " + mc.Label() + ", through herdr: each agent asked to leave with its own exit, its shell, pane and tab kept\n" + strings.Join(lines, "\n")
	job := s.Jobs.Run(mc, "Sleep "+w.Name, command, func(say func(string)) error {
		defer s.release(mc.ID(), workspaceID)
		defer s.refresh(mc)
		if len(left) > 0 {
			say("left running, with no session to resume: " + strings.Join(left, ", "))
		}
		return s.sleepAll(client, mc.ID(), workspaceID, w, say)
	})
	return Started{Job: job.ID, Agents: len(w.Agents)}, nil
}

// sleepAll asks every recorded agent to leave, side by side; one that would not stays awake and off the record.
func (s *Service) sleepAll(client restart.Leaver, machineID, workspaceID string, w Workspace, say func(string)) error {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		awake   = map[string]string{} // pane → why it still runs
		stopped int
	)
	for _, a := range w.Agents {
		wg.Add(1)
		go func(a Agent) {
			defer wg.Done()
			var lines []string
			note := func(line string) { lines = append(lines, line) }
			why := s.sleepOne(client, machineID, a, note)
			mu.Lock()
			defer mu.Unlock()
			for _, line := range lines {
				say(line)
			}
			if why != "" {
				awake[a.Pane] = why
			} else {
				stopped++
			}
		}(a)
	}
	wg.Wait()
	if len(awake) == 0 {
		say(fmt.Sprintf("%s is asleep: %s stopped", w.Name, agentsWord(stopped)))
		return nil
	}
	var kept []Agent
	var names []string
	for _, a := range w.Agents {
		if why, ok := awake[a.Pane]; ok {
			names = append(names, fmt.Sprintf("%s in %s (%s)", restart.Title(a.Kind), a.Pane, why))
		} else {
			kept = append(kept, a)
		}
	}
	s.Store.keep(machineID, workspaceID, kept)
	if stopped == 0 {
		return fmt.Errorf("%s did not go to sleep: %s", w.Name, strings.Join(names, ", "))
	}
	return fmt.Errorf("%s is asleep, but %s did not stop: %s", w.Name, agentsWord(len(names)), strings.Join(names, ", "))
}

// sleepOne makes one agent leave under its pane's lock: a fresh look, an Esc for one at work or asking, its own exit;
// "" once it left, else why it still runs.
func (s *Service) sleepOne(client restart.Leaver, machineID string, a Agent, say func(string)) string {
	if s.Lock != nil {
		l := s.Lock(machineID, a.Pane)
		l.Lock()
		defer l.Unlock()
	}
	title := restart.Title(a.Kind)
	typed := func(text string, keys bool) {
		if s.Typed != nil {
			s.Typed(machineID, a.Pane, text, keys)
		}
	}
	exit := func(text string) { typed(text, false) }
	p, found, err := look(client, a.Pane)
	switch {
	case err != nil:
		say(fmt.Sprintf("· %s (%s): herdr did not answer: %v", title, a.Pane, err))
		return "herdr did not answer"
	case !found:
		say(fmt.Sprintf("· %s (%s): the pane is gone — waking opens a tab for it", title, a.Pane))
		return ""
	case p.Agent == "":
		say(fmt.Sprintf("· %s (%s): left already", title, a.Pane))
		return ""
	case p.Agent != a.Kind || restart.SessionOf(&p) != a.Session:
		say(fmt.Sprintf("· %s (%s): runs another conversation now — left as it is", title, a.Pane))
		return "another conversation runs there now"
	}
	if busy(p.AgentStatus) {
		interrupt(client, a.Pane, typed, say)
	}
	say(fmt.Sprintf("· %s (%s): %s", title, a.Pane, restart.Exit(a.Kind)))
	state, err := restart.Leave(client, a.Pane, a.Kind, exit, say)
	if herdr.IsCode(err, "agent_blocked") { // a menu opened under the exit: closed, and the exit typed once more
		interrupt(client, a.Pane, typed, say)
		state, err = restart.Leave(client, a.Pane, a.Kind, exit, say)
	}
	if err != nil {
		say("  ✗ " + err.Error())
		return err.Error()
	}
	switch state {
	case restart.LeftShell:
		say(fmt.Sprintf("  ✓ asleep, session %s kept to resume", short(a.Session)))
		return ""
	case restart.LeftGone:
		say("  ✓ asleep; the pane closed with it — waking opens a tab for it")
		return ""
	case restart.LeftOther:
		say("  ✗ something else runs in the pane now")
		return "something else runs there now"
	case restart.LeftUnseen:
		say("  ✗ herdr did not show the pane after the exit")
		return "herdr did not show the pane"
	}
	say(fmt.Sprintf("  ✗ %s did not leave — it runs on", title))
	return title + " did not leave"
}

// interruptWait is how long an agent interrupted with Esc is given to come to rest before it is asked to leave.
var interruptWait = 6 * time.Second

// interrupt presses Esc for an agent at work or asking, as the person would, and waits for it to come to rest.
func interrupt(client restart.Leaver, paneID string, typed func(string, bool), say func(string)) {
	say(fmt.Sprintf("  %s: Esc — it was busy", paneID))
	if err := client.PaneSendKeys(paneID, []string{"esc"}); err != nil {
		say("  Esc: " + err.Error())
		return
	}
	typed("Esc", true)
	deadline := time.Now().Add(interruptWait)
	for time.Now().Before(deadline) {
		time.Sleep(restart.LeavePoll)
		p, found, err := look(client, paneID)
		if err == nil && (!found || p.Agent == "" || !busy(p.AgentStatus)) {
			return
		}
	}
}

func busy(status string) bool { return status == herdr.StatusWorking || status == herdr.StatusBlocked }

// refresh asks the watcher for a fresh snapshot, so the roster says what the job did.
func (s *Service) refresh(mc *machines.Machine) {
	if s.Watch != nil {
		s.Watch.Refresh(mc)
	}
}

// look is the pane in a fresh snapshot.
func look(client interface {
	SessionSnapshot() (*herdr.Snapshot, error)
}, paneID string) (herdr.PaneInfo, bool, error) {
	snap, err := client.SessionSnapshot()
	if err != nil {
		return herdr.PaneInfo{}, false, err
	}
	p, found := restart.Lookup(snap, paneID)
	return p, found, nil
}

func agentsWord(n int) string {
	if n == 1 {
		return "1 agent"
	}
	return fmt.Sprintf("%d agents", n)
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
