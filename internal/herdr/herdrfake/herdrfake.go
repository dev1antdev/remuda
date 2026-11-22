// Package herdrfake is a machine's API, its terminals and its event stream as a test wants them: function fields for the
// calls a test cares about, and nothing done for the rest. It stood herdrtest's socket before Core 26j: remuda asks
// remuda-core now, so a test gives its machine this in place of a client.
package herdrfake

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// API is a herdr.API whose calls are the functions given; a call with none answers nothing and no error.
type API struct {
	Memory     bool // the snapshot is held here, not asked of a core
	Snapshot   func() (*herdr.Snapshot, error)
	SubscribeF func(subs []herdr.Subscription) (herdr.EventStream, error)

	WorkspaceCreateF func(cwd, label string) (*herdr.WorkspaceCreateResult, error)
	WorkspaceRenameF func(workspaceID, label string) error
	WorkspaceCloseF  func(workspaceID string) error
	TabCreateF       func(workspaceID, label string) (*herdr.TabCreateResult, error)
	TabCreateInF     func(workspaceID, label, cwd string, env map[string]string) (*herdr.TabCreateResult, error)
	TabRenameF       func(tabID, label string) error
	TabCloseF        func(tabID string) error
	WorktreeCreateF  func(workspaceID, branch, base, label string) (*herdr.WorktreeCreateResult, error)
	WorktreeOpenF    func(workspaceID, path, label string) (*herdr.WorktreeCreateResult, error)
	LayoutExportF    func(tabID string) (*herdr.LayoutDescription, error)
	LayoutApplyF     func(workspaceID, tabID, label string, root herdr.LayoutNode) (*herdr.LayoutDescription, error)

	PaneSplitF    func(paneID, direction, cwd string) (*herdr.PaneInfo, error)
	PaneCloseF    func(paneID string) error
	PaneReadF     func(paneID string) (string, error)
	PaneRecentF   func(paneID string, n int) (string, bool, error)
	PaneScreenF   func(paneID string) (herdr.Screen, error)
	PaneSendTextF func(paneID, text string) error
	PaneSendKeysF func(paneID string, keys []string) error
	PaneAtPromptF func(paneID string) (bool, error)
	PaneShellPIDF func(paneID string) (int, error)

	AgentStartF     func(paneID, kind string) error
	AgentStartWithF func(paneID, kind, name string, args []string, timeout time.Duration) error
	AgentPromptF    func(target, text string) error
	AgentGetF       func(target string) (herdr.AgentInfo, error)
}

var _ herdr.API = (*API)(nil)

func (a *API) SessionSnapshot() (*herdr.Snapshot, error) {
	if a.Snapshot == nil {
		return &herdr.Snapshot{}, nil
	}
	return a.Snapshot()
}

func (a *API) SnapshotFromMemory() bool { return a.Memory }

func (a *API) Subscribe(subs []herdr.Subscription) (herdr.EventStream, error) {
	if a.SubscribeF == nil {
		return nil, nil
	}
	return a.SubscribeF(subs)
}

func (a *API) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	if a.WorkspaceCreateF == nil {
		return nil, nil
	}
	return a.WorkspaceCreateF(cwd, label)
}

func (a *API) WorkspaceRename(workspaceID, label string) error {
	if a.WorkspaceRenameF == nil {
		return nil
	}
	return a.WorkspaceRenameF(workspaceID, label)
}

func (a *API) WorkspaceClose(workspaceID string) error {
	if a.WorkspaceCloseF == nil {
		return nil
	}
	return a.WorkspaceCloseF(workspaceID)
}

func (a *API) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	if a.TabCreateF == nil {
		return nil, nil
	}
	return a.TabCreateF(workspaceID, label)
}

func (a *API) TabCreateIn(workspaceID, label, cwd string, env map[string]string) (*herdr.TabCreateResult, error) {
	if a.TabCreateInF == nil {
		return nil, nil
	}
	return a.TabCreateInF(workspaceID, label, cwd, env)
}

func (a *API) TabRename(tabID, label string) error {
	if a.TabRenameF == nil {
		return nil
	}
	return a.TabRenameF(tabID, label)
}

func (a *API) TabClose(tabID string) error {
	if a.TabCloseF == nil {
		return nil
	}
	return a.TabCloseF(tabID)
}

func (a *API) WorktreeCreate(workspaceID, branch, base, label string) (*herdr.WorktreeCreateResult, error) {
	if a.WorktreeCreateF == nil {
		return nil, nil
	}
	return a.WorktreeCreateF(workspaceID, branch, base, label)
}

func (a *API) WorktreeCreateAt(workspaceID, branch, base, label, path string) (*herdr.WorktreeCreateResult, error) {
	return a.WorktreeCreate(workspaceID, branch, base, label)
}

func (a *API) WorktreeOpen(workspaceID, path, label string) (*herdr.WorktreeCreateResult, error) {
	if a.WorktreeOpenF == nil {
		return nil, nil
	}
	return a.WorktreeOpenF(workspaceID, path, label)
}

func (a *API) LayoutExport(tabID string) (*herdr.LayoutDescription, error) {
	if a.LayoutExportF == nil {
		return nil, nil
	}
	return a.LayoutExportF(tabID)
}

func (a *API) LayoutApply(workspaceID, tabID, label string, root herdr.LayoutNode) (*herdr.LayoutDescription, error) {
	if a.LayoutApplyF == nil {
		return nil, nil
	}
	return a.LayoutApplyF(workspaceID, tabID, label, root)
}

func (a *API) PaneSplit(paneID, direction, cwd string) (*herdr.PaneInfo, error) {
	if a.PaneSplitF == nil {
		return nil, nil
	}
	return a.PaneSplitF(paneID, direction, cwd)
}

func (a *API) PaneClose(paneID string) error {
	if a.PaneCloseF == nil {
		return nil
	}
	return a.PaneCloseF(paneID)
}

func (a *API) PaneRead(paneID string) (string, error) {
	if a.PaneReadF == nil {
		return "", nil
	}
	return a.PaneReadF(paneID)
}

func (a *API) PaneRecent(paneID string, n int) (string, bool, error) {
	if a.PaneRecentF == nil {
		return "", false, nil
	}
	return a.PaneRecentF(paneID, n)
}

func (a *API) PaneScreen(paneID string) (herdr.Screen, error) {
	if a.PaneScreenF == nil {
		return herdr.Screen{}, nil
	}
	return a.PaneScreenF(paneID)
}

func (a *API) PaneSendText(paneID, text string) error {
	if a.PaneSendTextF == nil {
		return nil
	}
	return a.PaneSendTextF(paneID, text)
}

func (a *API) PaneSendKeys(paneID string, keys []string) error {
	if a.PaneSendKeysF == nil {
		return nil
	}
	return a.PaneSendKeysF(paneID, keys)
}

func (a *API) PaneAtPrompt(paneID string) (bool, error) {
	if a.PaneAtPromptF == nil {
		return false, nil
	}
	return a.PaneAtPromptF(paneID)
}

func (a *API) PaneShellPID(paneID string) (int, error) {
	if a.PaneShellPIDF == nil {
		return 0, nil
	}
	return a.PaneShellPIDF(paneID)
}

func (a *API) AgentStart(paneID, kind string) error {
	if a.AgentStartF == nil {
		return nil
	}
	return a.AgentStartF(paneID, kind)
}

func (a *API) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	if a.AgentStartWithF == nil {
		return nil
	}
	return a.AgentStartWithF(paneID, kind, name, args, timeout)
}

func (a *API) AgentPrompt(target, text string) error {
	if a.AgentPromptF == nil {
		return nil
	}
	return a.AgentPromptF(target, text)
}

func (a *API) AgentGet(target string) (herdr.AgentInfo, error) {
	if a.AgentGetF == nil {
		return herdr.AgentInfo{}, nil
	}
	return a.AgentGetF(target)
}

// Stream is an event stream a test feeds: Push hands one event to its reader, End gives it the error it ends on.
type Stream struct {
	mu     sync.Mutex
	events chan herdr.Event
	done   chan struct{}
	err    error
	closed bool
}

// NewStream is a stream with room for n events before its reader must take them.
func NewStream(n int) *Stream {
	return &Stream{events: make(chan herdr.Event, n), done: make(chan struct{})}
}

var _ herdr.EventStream = (*Stream)(nil)

var (
	herdrErrClosed     = net.ErrClosed
	herdrErrNoTerminal = errors.New("no terminal here")
)

// Next hands the next event over, then the error the stream ended on; net.ErrClosed once it is closed here.
func (s *Stream) Next() (herdr.Event, error) {
	select {
	case ev, ok := <-s.events:
		if ok {
			return ev, nil
		}
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = herdrErrClosed
		}
		return herdr.Event{}, err
	case <-s.done:
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = herdrErrClosed
		}
		return herdr.Event{}, err
	}
}

// Push hands one event over.
func (s *Stream) Push(ev herdr.Event) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return
	}
	s.events <- ev
}

// End ends the stream with err.
func (s *Stream) End(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed, s.err = true, err
	s.mu.Unlock()
	close(s.done)
}

// Close ends the stream here.
func (s *Stream) Close() error {
	s.End(nil)
	return nil
}

// Terminals is a machine's terminals as a test wants them: Open hands over what its function answers.
type Terminals struct {
	OpenF    func(termID string, view herdr.View) (herdr.Viewer, error)
	ViewersF func(termID string) int
}

var _ herdr.Terminals = (*Terminals)(nil)

func (t *Terminals) Open(termID string, view herdr.View) (herdr.Viewer, error) {
	if t.OpenF == nil {
		return nil, herdrErrNoTerminal
	}
	return t.OpenF(termID, view)
}

func (t *Terminals) Viewers(termID string) int {
	if t.ViewersF == nil {
		return 0
	}
	return t.ViewersF(termID)
}

// Manager is a fleet whose local machine answers this package's fakes and reaches no core: what a test that builds a
// registry, a route or a watcher without asking a machine needs. A test that wants a real core starts coretest.Server
// and machines.NewManager instead.
func Manager(dir string) *machines.Manager {
	return machines.NewManagerWith(dir, nil, false, &API{}, &Terminals{})
}
