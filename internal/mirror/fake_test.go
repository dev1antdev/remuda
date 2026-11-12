package mirror

import (
	"sync"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// fakeHerdr keeps every write and toast, and holds the tokens as herdr would; err, when set, answers the next ones.
type fakeHerdr struct {
	mu     sync.Mutex
	panes  []herdr.PaneMetadata
	spaces []herdr.WorkspaceMetadata
	toasts []herdr.Toast
	held   map[string]map[string]string // by pane or workspace id
	err    error
	refuse map[string]error // by pane: herdr refusing that pane's values alone
	shown  herdr.Shown
}

func (f *fakeHerdr) PaneReportMetadata(m herdr.PaneMetadata) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.panes = append(f.panes, m)
	if err := f.refuse[m.Pane]; err != nil {
		return err
	}
	if f.err == nil {
		f.apply(m.Pane, m.Tokens)
	}
	return f.err
}

func (f *fakeHerdr) WorkspaceReportMetadata(m herdr.WorkspaceMetadata) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spaces = append(f.spaces, m)
	if f.err == nil {
		f.apply(m.Workspace, m.Tokens)
	}
	return f.err
}

func (f *fakeHerdr) NotificationShow(t herdr.Toast) (herdr.Shown, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toasts = append(f.toasts, t)
	return f.shown, f.err
}

func (f *fakeHerdr) apply(id string, tokens map[string]*string) {
	if f.held == nil {
		f.held = map[string]map[string]string{}
	}
	if f.held[id] == nil {
		f.held[id] = map[string]string{}
	}
	for name, v := range tokens {
		if v == nil {
			delete(f.held[id], name)
		} else {
			f.held[id][name] = *v
		}
	}
}

// restart is herdr's own restart: the tokens are not restored.
func (f *fakeHerdr) restart() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held = nil
}

func (f *fakeHerdr) take() ([]herdr.PaneMetadata, []herdr.WorkspaceMetadata) {
	f.mu.Lock()
	defer f.mu.Unlock()
	panes, spaces := f.panes, f.spaces
	f.panes, f.spaces = nil, nil
	return panes, spaces
}
