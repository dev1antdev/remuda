package scenes

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// fakeHerdr is a machine's herdr on paper: what was asked of it, in order, and what it answers.
type fakeHerdr struct {
	mu        sync.Mutex
	calls     []string
	next      int      // the number the next workspace, tab and pane are made under
	atPrompt  []string // the panes whose shell is at its prompt
	refuse    string   // a call, by its first word, herdr refuses
	prompted  []string // pane: text, as agent.prompt took it
	startArgs [][]string
}

func (f *fakeHerdr) note(format string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := fmt.Sprintf(format, args...)
	f.calls = append(f.calls, call)
	if f.refuse != "" && strings.HasPrefix(call, f.refuse) {
		return &herdr.Error{Code: "refused", Message: "herdr says no to " + f.refuse}
	}
	return nil
}

func (f *fakeHerdr) id() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	return f.next + 6
}

func (f *fakeHerdr) AgentStart(pane, kind string) error {
	return f.note("start %s %s", pane, kind)
}

func (f *fakeHerdr) AgentStartWith(pane, kind, name string, args []string, _ time.Duration) error {
	f.mu.Lock()
	f.startArgs = append(f.startArgs, args)
	f.mu.Unlock()
	return f.note("start %s %s %s", pane, kind, strings.Join(args, " "))
}

func (f *fakeHerdr) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	if err := f.note("workspace %s", cwd); err != nil {
		return nil, err
	}
	n := f.id()
	ws := fmt.Sprintf("w%d", n)
	return &herdr.WorkspaceCreateResult{Workspace: herdr.WorkspaceInfo{WorkspaceID: ws}, RootPane: herdr.PaneInfo{PaneID: ws + ":p1", TabID: ws + ":t1", WorkspaceID: ws}}, nil
}

func (f *fakeHerdr) TabCreate(workspace, label string) (*herdr.TabCreateResult, error) {
	if err := f.note("tab %s %q", workspace, label); err != nil {
		return nil, err
	}
	n := f.id()
	return &herdr.TabCreateResult{Tab: herdr.TabInfo{TabID: fmt.Sprintf("%s:t%d", workspace, n)}, RootPane: herdr.PaneInfo{PaneID: fmt.Sprintf("%s:p%d", workspace, n)}}, nil
}

func (f *fakeHerdr) TabRename(tab, label string) error { return f.note("rename %s %q", tab, label) }

func (f *fakeHerdr) LayoutExport(tab string) (*herdr.LayoutDescription, error) {
	return nil, f.note("export %s", tab)
}

func (f *fakeHerdr) LayoutApply(workspace, tab, label string, root herdr.LayoutNode) (*herdr.LayoutDescription, error) {
	if err := f.note("layout %s %s %q %d", workspace, tab, label, len(root.Panes())); err != nil {
		return nil, err
	}
	n := f.id()
	laid := root
	for i, pane := range laid.Panes() {
		pane.PaneID = fmt.Sprintf("w1:p%d", n*10+i)
	}
	return &herdr.LayoutDescription{WorkspaceID: workspace, TabID: fmt.Sprintf("w1:t%d", n), Root: laid}, nil
}

func (f *fakeHerdr) PaneClose(pane string) error { return f.note("close %s", pane) }

func (f *fakeHerdr) PaneAtPrompt(pane string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.atPrompt {
		if p == pane {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeHerdr) AgentPrompt(target, text string) error {
	f.mu.Lock()
	f.prompted = append(f.prompted, target+": "+text)
	f.mu.Unlock()
	return f.note("prompt %s %q", target, text)
}

func (f *fakeHerdr) PaneSendText(pane, text string) error { return f.note("text %s %q", pane, text) }
func (f *fakeHerdr) PaneSendKeys(pane string, keys []string) error {
	return f.note("keys %s %v", pane, keys)
}

func (f *fakeHerdr) said() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// fakeMachine is one machine of the fleet on paper.
type fakeMachine struct {
	id, label string
	client    *fakeHerdr
	away      error
	snap      *herdr.Snapshot
	leftovers []worktrees.Leftover
	tidied    []string
	refreshed int
	missing   []string // the folders that are not there
}

func (m *fakeMachine) ID() string    { return m.id }
func (m *fakeMachine) Label() string { return m.label }
func (m *fakeMachine) Herdr() (Herdr, error) {
	if m.away != nil {
		return nil, m.away
	}
	return m.client, nil
}
func (m *fakeMachine) Snapshot() *herdr.Snapshot { return m.snap }
func (m *fakeMachine) Refresh()                  { m.refreshed++ }
func (m *fakeMachine) Leftovers(open []string) ([]worktrees.Leftover, error) {
	return m.leftovers, nil
}
func (m *fakeMachine) Tidy(folder string) worktrees.Outcome {
	m.tidied = append(m.tidied, folder)
	return worktrees.Outcome{Path: folder, Removed: true, BranchDeleted: true}
}

// fleetOf is a service over the machines given, its store in a temp dir, the radar reading every agent idle since states
// says otherwise, the meter reading contexts.
func fleetOf(t *testing.T, list ...*fakeMachine) (*Service, map[string]radar.Display, map[string]int) {
	t.Helper()
	states := map[string]radar.Display{}
	contexts := map[string]int{}
	svc := &Service{
		Store:  New(t.TempDir()),
		Recipe: func(id string) (recipes.Recipe, bool) { return recipes.Recipe{}, false },
		Machines: func() []Machine {
			out := make([]Machine, len(list))
			for i, m := range list {
				out[i] = m
			}
			return out
		},
		Machine: func(id string) (Machine, error) {
			for _, m := range list {
				if m.id == id {
					return m, nil
				}
			}
			return nil, errors.New("no machine " + id)
		},
		Readings: Readings{
			State: func(machine string, pane *herdr.PaneInfo) radar.Display {
				if d, ok := states[machine+"|"+pane.PaneID]; ok {
					return d
				}
				return radar.Display{State: herdr.StatusIdle}
			},
			Context: func(machine, pane string) (int, bool) {
				share, ok := contexts[machine+"|"+pane]
				return share, ok
			},
			Scratch: func(machine, workspace string) bool { return workspace == "w99" },
		},
		Input: sessions.NewInput(nil, nil),
	}
	return svc, states, contexts
}

// waitDone reads a run until it is done.
func waitDone(t *testing.T, svc *Service, id string) Run {
	t.Helper()
	for i := 0; i < 200; i++ {
		if run, ok := svc.Get(id); ok && run.Done {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the run never finished")
	return Run{}
}

func (f *fakeHerdr) prompts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompted...)
}

func (m *fakeMachine) Folder(dir string) bool {
	for _, gone := range m.missing {
		if gone == dir {
			return false
		}
	}
	return true
}
