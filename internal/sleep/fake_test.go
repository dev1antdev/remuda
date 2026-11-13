package sleep

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/restart"
)

const (
	sidA = "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"
	sidB = "0a1b2c3d-4e5f-4061-8273-94a5b6c7d8e9"
	sidC = "1f2e3d4c-5b6a-4978-8695-a4b3c2d1e0f9"
)

// quick makes the leave and the interrupt look at the fake every millisecond instead of every second.
func quick(t *testing.T) {
	poll, wait := restart.LeavePoll, interruptWait
	restart.LeavePoll, interruptWait = time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { restart.LeavePoll, interruptWait = poll, wait })
}

func agentPane(id, tab, kind, session, status, dir string) herdr.PaneInfo {
	p := herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", TabID: tab, Agent: kind, AgentStatus: status, CWD: dir, Title: "✳ " + kind}
	if session != "" {
		p.AgentSession = &herdr.AgentSession{Kind: "id", Value: session}
	}
	return p
}

func shellPane(id, tab, dir string) herdr.PaneInfo {
	return herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", TabID: tab, CWD: dir, Title: "zsh"}
}

// fakeHerdr is herdr as a sleep and a wake see it: panes whose agents leave on their exit (or not), an Esc that brings
// a busy one to rest, shells that are free or not, tabs made, agents started.
type fakeHerdr struct {
	mu        sync.Mutex
	snap      herdr.Snapshot
	stubborn  map[string]bool  // panes whose agent never leaves, Ctrl-C or not
	busyShell map[string]bool  // panes whose shell runs something
	refuse    map[string]error // panes where a start fails
	typed     []string
	started   []string
	tabs      []string
}

func newFake(panes ...herdr.PaneInfo) *fakeHerdr {
	return &fakeHerdr{
		snap: herdr.Snapshot{
			Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/work/api"}}},
			Tabs:       []herdr.TabInfo{{TabID: "w1:t1", WorkspaceID: "w1", Number: 1, Label: "1"}, {TabID: "w1:t2", WorkspaceID: "w1", Number: 2, Label: "review"}},
			Panes:      panes,
		},
		stubborn: map[string]bool{}, busyShell: map[string]bool{}, refuse: map[string]error{},
	}
}

func (f *fakeHerdr) pane(id string) *herdr.PaneInfo {
	for i := range f.snap.Panes {
		if f.snap.Panes[i].PaneID == id {
			return &f.snap.Panes[i]
		}
	}
	return nil
}

func (f *fakeHerdr) SessionSnapshot() (*herdr.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := f.snap
	copied.Panes = append([]herdr.PaneInfo(nil), f.snap.Panes...)
	return &copied, nil
}

func (f *fakeHerdr) AgentPrompt(target, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.pane(target)
	if p == nil || p.Agent == "" {
		return errors.New("agent_not_found")
	}
	if p.AgentStatus == herdr.StatusBlocked {
		return &herdr.Error{Code: "agent_blocked", Message: "a menu is open"}
	}
	f.typed = append(f.typed, target+" "+text)
	if !f.stubborn[target] {
		p.Agent, p.AgentStatus, p.AgentSession = "", "", nil
	}
	return nil
}

func (f *fakeHerdr) PaneSendKeys(paneID string, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typed = append(f.typed, paneID+" "+strings.Join(keys, "+"))
	if p := f.pane(paneID); p != nil && len(keys) == 1 && keys[0] == "esc" {
		p.AgentStatus = herdr.StatusIdle
	}
	return nil
}

func (f *fakeHerdr) PaneAtPrompt(paneID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.busyShell[paneID], nil
}

func (f *fakeHerdr) AgentStart(paneID, kind string) error {
	return f.AgentStartWith(paneID, kind, kind, nil, 0)
}

func (f *fakeHerdr) AgentStartWith(paneID, kind, name string, args []string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.refuse[paneID]; err != nil {
		return err
	}
	f.started = append(f.started, fmt.Sprintf("%s %s %s", paneID, kind, strings.Join(args, " ")))
	if p := f.pane(paneID); p != nil {
		p.Agent, p.AgentStatus = kind, herdr.StatusIdle
	}
	return nil
}

func (f *fakeHerdr) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.tabs) + 7
	f.tabs = append(f.tabs, workspaceID+" "+label)
	res := &herdr.TabCreateResult{}
	res.Tab.TabID = fmt.Sprintf("%s:t%d", workspaceID, n)
	res.RootPane.PaneID = fmt.Sprintf("%s:p%d", workspaceID, n)
	f.snap.Panes = append(f.snap.Panes, shellPane(res.RootPane.PaneID, res.Tab.TabID, "/work/api"))
	return res, nil
}

func (f *fakeHerdr) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	return nil, errors.New("no new workspace in these tests")
}
