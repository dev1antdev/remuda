package recap

import (
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/watch"
)

// Service reads the conversations the recaps are made of: a pane's through the chat service, every agent's from the watcher's snapshots.
type Service struct {
	Machines *machines.Manager
	Watch    *watch.Watcher
	Chat     *chat.Service
	Radar    *radar.Tracker
}

// Conversation is the recap of one conversation: a pane's live one, a session of its folder, or a transcript on its own.
func (s *Service) Conversation(mc *machines.Machine, paneID, sessionID, dir string, since time.Time) (Recap, error) {
	conv, err := s.Chat.Open(mc, paneID, sessionID, dir, false)
	if err != nil {
		return Recap{}, err
	}
	if conv.Path == "" {
		return Of(nil, since), nil
	}
	turns, err := conv.Turns()
	if err != nil {
		return Recap{}, err
	}
	r := Of(turns, since)
	r.Spans = Spans(turns, since, time.Now())
	return r, nil
}

// Standup is one item per agent pane on every connected machine, the machines read side by side, in the roster's order.
func (s *Service) Standup(since time.Time) []Item {
	list := s.Machines.List()
	perMachine := make([][]Item, len(list))
	var wg sync.WaitGroup
	for i, mc := range list {
		if state, _ := mc.State(); state != machines.StateConnected {
			continue
		}
		snap, _ := s.Watch.Snapshot(mc.ID())
		if snap == nil {
			continue
		}
		wg.Add(1)
		go func(i int, mc *machines.Machine) {
			defer wg.Done()
			for j := range snap.Panes {
				pane := &snap.Panes[j]
				if pane.Agent == "" {
					continue
				}
				item := Item{Machine: mc.ID(), Label: mc.Label(), Workspace: snap.WorkspaceLabel(pane.WorkspaceID), Pane: pane.PaneID, Dir: pane.Dir(), Agent: pane.AgentName()}
				display := s.Radar.Display(mc.ID(), pane)
				item.State, item.Alert = display.State, display.Alert
				recap, err := s.Conversation(mc, pane.PaneID, "", "", since)
				if err != nil {
					item.Error = err.Error()
				}
				item.Recap = recap
				perMachine[i] = append(perMachine[i], item)
			}
		}(i, mc)
	}
	wg.Wait()
	items := []Item{}
	for _, part := range perMachine {
		items = append(items, part...)
	}
	return items
}
