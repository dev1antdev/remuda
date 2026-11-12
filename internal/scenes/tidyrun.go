package scenes

import (
	"strings"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// tidy does the actions of a run that the plan, read afresh, still holds; the rest are skipped with the reason.
func (s *Service) tidy(t Tidy, run *Run, by string) {
	fresh := map[string]Action{}
	for _, action := range s.Plan(t, s.now()).Actions {
		fresh[action.ID] = action
	}
	touched := map[string]Machine{}
	for i := range run.Items {
		s.mu.Lock()
		id := run.Items[i].Action.ID
		s.mu.Unlock()
		action, ok := fresh[id]
		if !ok {
			why := "no longer to tidy"
			if planned := parseID(id); planned.Machine != "" {
				if mc, err := s.Machine(planned.Machine); err == nil {
					why = staleWhy(planned, mc.Snapshot(), s.Readings)
				}
			}
			s.update(run, i, func(item *Item) { item.State, item.Note = StateSkipped, why })
			continue
		}
		s.update(run, i, func(item *Item) { item.Action, item.State = &action, StateRunning })
		mc, state, note := s.act(action, by)
		if mc != nil {
			touched[mc.ID()] = mc
		}
		s.update(run, i, func(item *Item) { item.State, item.Note = state, note })
	}
	for _, mc := range touched {
		mc.Refresh()
	}
}

// act does one action on its machine, and says how it went.
func (s *Service) act(action Action, by string) (Machine, string, string) {
	mc, err := s.Machine(action.Machine)
	if err != nil {
		return nil, StateAway, err.Error()
	}
	if action.Kind == ActWorktree {
		outcome := mc.Tidy(action.Path)
		if !outcome.Removed {
			return mc, StateRefused, outcome.Kept
		}
		if outcome.BranchKept != "" {
			return mc, StateDone, "its branch stays: " + outcome.BranchKept
		}
		return mc, StateDone, ""
	}
	client, err := mc.Herdr()
	if err != nil {
		return mc, StateAway, mc.Label() + ": " + err.Error()
	}
	if action.Kind == ActCompact {
		if err := s.Input.SubmitToAgent(client, events.PaneRef{Machine: action.Machine, Pane: action.Pane}, CompactText); err != nil {
			return mc, StateRefused, err.Error()
		}
		if s.Sent != nil {
			s.Sent(by, action.Machine, action.Pane, CompactText)
		}
		return mc, StateDone, ""
	}
	if err := client.PaneClose(action.Pane); err != nil {
		return mc, StateRefused, err.Error()
	}
	return mc, StateDone, ""
}

// parseID reads an action's id back into its kind, machine and pane or folder.
func parseID(id string) Action {
	parts := strings.SplitN(id, "|", 3)
	if len(parts) < 3 {
		return Action{}
	}
	action := Action{ID: id, Kind: parts[0], Machine: parts[1]}
	if action.Kind == ActWorktree {
		action.Path = parts[2]
	} else {
		action.Pane = parts[2]
	}
	return action
}

// AtWork names a machine the tidy covers where an agent works or asks now, "" when nobody's is: a scheduled tidy waits for a quiet evening.
func (s *Service) AtWork(t Tidy) string {
	for _, mc := range s.Machines() {
		if !t.Covers(mc.ID()) {
			continue
		}
		snap := mc.Snapshot()
		if snap == nil {
			continue
		}
		for i := range snap.Panes {
			pane := &snap.Panes[i]
			if pane.Agent == "" {
				continue
			}
			if state := s.Readings.State(mc.ID(), pane).State; state == herdr.StatusWorking || state == herdr.StatusBlocked {
				return mc.Label()
			}
		}
	}
	return ""
}
