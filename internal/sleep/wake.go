package sleep

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/sessions"
)

// Wake starts every agent a sleep stopped in the workspace again on its session, in a job: each in its own shell
// when that stands free in the same folder, a new tab of the workspace for a pane gone, never in a shell that runs
// something. What does not start stays asleep for the next wake, and the job says why.
func (s *Service) Wake(mc *machines.Machine, workspaceID string) (Started, error) {
	w, ok := s.Store.Of(mc.ID(), workspaceID)
	if !ok {
		return Started{}, ErrNotAsleep
	}
	client, _, err := mc.Ready()
	if err != nil {
		return Started{}, err
	}
	if err := s.claim(mc.ID(), workspaceID, "wake"); err != nil {
		return Started{}, err
	}
	var lines []string
	for _, a := range w.Agents {
		lines = append(lines, fmt.Sprintf("%s in %s   (%s)", restart.Title(a.Kind), a.Pane, restart.Resume(a.Kind, a.Session)))
	}
	command := "# on " + mc.Label() + ", through herdr: each agent started again on its session, in its own shell or a new tab\n" + strings.Join(lines, "\n")
	checks := func(a Agent) error { return restart.CanResume(mc, a.Kind, a.Session, a.Dir) }
	job := s.Jobs.Run(mc, "Wake "+w.Name, command, func(say func(string)) error {
		defer s.release(mc.ID(), workspaceID)
		defer s.refresh(mc)
		return s.wakeAll(client, mc.ID(), workspaceID, w, checks, say)
	})
	return Started{Job: job.ID, Agents: len(w.Agents)}, nil
}

// outcome is what a wake did with one agent: back, still asleep (with the agent as it stands now: a new tab's pane),
// or forgotten, since it can never come back.
type outcome struct {
	back bool
	kept *Agent
	why  string
}

// wakeAll wakes the agents one after another and keeps asleep the ones that did not come back.
func (s *Service) wakeAll(client Client, machineID, workspaceID string, w Workspace, checks func(Agent) error, say func(string)) error {
	var kept []Agent
	var still, gone []string
	back := 0
	for _, a := range w.Agents {
		o := s.wakeOne(client, machineID, workspaceID, a, checks, say)
		switch {
		case o.back:
			back++
		case o.kept != nil:
			kept = append(kept, *o.kept)
			still = append(still, fmt.Sprintf("%s in %s (%s)", restart.Title(a.Kind), a.Pane, o.why))
		default:
			gone = append(gone, fmt.Sprintf("%s in %s (%s)", restart.Title(a.Kind), a.Pane, o.why))
		}
	}
	s.Store.keep(machineID, workspaceID, kept)
	if len(gone) > 0 {
		say("forgotten, since they cannot come back: " + strings.Join(gone, ", "))
	}
	if len(still) > 0 {
		return fmt.Errorf("%s is still partly asleep · not restarted (busy or gone): %s", w.Name, strings.Join(still, ", "))
	}
	if back == 0 {
		return fmt.Errorf("%s woke with nothing to resume: %s", w.Name, strings.Join(gone, ", "))
	}
	say(fmt.Sprintf("%s is awake: %s resumed", w.Name, agentsWord(back)))
	return nil
}

// wakeOne brings one agent back under its pane's lock: a fresh look at the pane, then its shell or a new tab.
func (s *Service) wakeOne(client Client, machineID, workspaceID string, a Agent, checks func(Agent) error, say func(string)) outcome {
	if s.Lock != nil {
		l := s.Lock(machineID, a.Pane)
		l.Lock()
		defer l.Unlock()
	}
	title := restart.Title(a.Kind)
	keep := func(why string, as Agent) outcome {
		say(fmt.Sprintf("  ✗ %s — still asleep", why))
		return outcome{kept: &as, why: why}
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		say(fmt.Sprintf("· %s (%s): herdr did not answer", title, a.Pane))
		return keep("herdr did not answer: "+err.Error(), a)
	}
	p, found := restart.Lookup(snap, a.Pane)
	switch {
	case found && p.Agent == a.Kind && restart.SessionOf(&p) == a.Session:
		say(fmt.Sprintf("· %s (%s): back already", title, a.Pane))
		return outcome{back: true}
	case found && p.Agent != "":
		say(fmt.Sprintf("· %s (%s): %s runs there now", title, a.Pane, restart.Title(p.Agent)))
		return keep(restart.Title(p.Agent)+" runs in its pane now", a)
	case found && p.Dir() != a.Dir:
		say(fmt.Sprintf("· %s (%s): its shell is in %s now, not %s", title, a.Pane, p.Dir(), a.Dir))
		return keep("its shell is in another folder now", a)
	}
	if err := checks(a); err != nil {
		say(fmt.Sprintf("· %s (%s): %v", title, a.Pane, err))
		return outcome{why: err.Error()}
	}
	if !found {
		return s.wakeInTab(client, snap, workspaceID, a, say, keep)
	}
	if free, err := client.PaneAtPrompt(a.Pane); err != nil || !free {
		say(fmt.Sprintf("· %s (%s): something runs in its shell", title, a.Pane))
		if err != nil {
			return keep("herdr cannot say its shell is free: "+err.Error(), a)
		}
		return keep("something runs in its shell", a)
	}
	return start(client, a, say, keep)
}

// wakeInTab starts an agent whose pane is gone in a new tab of its workspace, on its folder, else wherever a workspace
// stands on that folder or a new one; the tab's shell is the agent's from then on, so a start that fails there is
// tried in that same shell next time.
func (s *Service) wakeInTab(client Client, snap *herdr.Snapshot, workspaceID string, a Agent, say func(string), keep func(string, Agent) outcome) outcome {
	title := restart.Title(a.Kind)
	if ws := snap.Workspace(workspaceID); ws != nil && snap.WorkspaceDir(ws) == a.Dir {
		res, err := client.TabCreate(workspaceID, a.TabName)
		if err != nil {
			say(fmt.Sprintf("· %s (%s): its pane is gone, and no tab could be made: %v", title, a.Pane, err))
			return keep("its pane is gone and herdr made no tab: "+err.Error(), a)
		}
		say(fmt.Sprintf("· %s (%s): its pane is gone — a new tab, %s", title, a.Pane, res.RootPane.PaneID))
		a.Pane, a.Tab = res.RootPane.PaneID, res.Tab.TabID
		return start(client, a, say, keep)
	}
	place, err := sessions.StartIn(client, snap, a.Dir, a.TabName, a.Kind, sessions.Start{Resume: a.Session})
	if place.Pane != "" {
		a.Pane, a.Tab = place.Pane, place.Tab
	}
	if err != nil {
		say(fmt.Sprintf("· %s: its pane is gone, and it did not start in %s: %v", title, a.Dir, err))
		if place.Pane != "" {
			return keep("it did not start in its new tab: "+err.Error(), a)
		}
		return keep("its pane is gone and herdr made no place for it: "+err.Error(), a)
	}
	say(fmt.Sprintf("· %s: its pane is gone — back in %s, session %s resumed", title, place.Pane, short(a.Session)))
	return outcome{back: true}
}

// start is the agent on its session in the pane's shell.
func start(client Client, a Agent, say func(string), keep func(string, Agent) outcome) outcome {
	say(fmt.Sprintf("· %s (%s): %s", restart.Title(a.Kind), a.Pane, restart.Resume(a.Kind, a.Session)))
	if err := sessions.StartAgentResumed(client, a.Pane, a.Kind, a.Session); err != nil {
		return keep("it did not start: "+err.Error(), a)
	}
	say(fmt.Sprintf("  ✓ back, session %s resumed", short(a.Session)))
	return outcome{back: true}
}
