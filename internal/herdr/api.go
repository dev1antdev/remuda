package herdr

import "time"

// API is a machine's herdr as remuda asks it: its snapshot, its event stream and every request remuda sends.
// remuda-core's adapter (internal/core) is the one implementation, so the packages that ask a machine never name it.
type API interface {
	SessionSnapshot() (*Snapshot, error)
	Subscribe(subs []Subscription) (EventStream, error)

	WorkspaceCreate(cwd, label string) (*WorkspaceCreateResult, error)
	WorkspaceRename(workspaceID, label string) error
	WorkspaceClose(workspaceID string) error
	TabCreate(workspaceID, label string) (*TabCreateResult, error)
	TabCreateIn(workspaceID, label, cwd string, env map[string]string) (*TabCreateResult, error)
	TabRename(tabID, label string) error
	TabClose(tabID string) error
	WorktreeCreate(workspaceID, branch, base, label string) (*WorktreeCreateResult, error)
	WorktreeCreateAt(workspaceID, branch, base, label, path string) (*WorktreeCreateResult, error)
	WorktreeOpen(workspaceID, path, label string) (*WorktreeCreateResult, error)
	LayoutExport(tabID string) (*LayoutDescription, error)
	LayoutApply(workspaceID, tabID, tabLabel string, root LayoutNode) (*LayoutDescription, error)

	PaneSplit(paneID, direction, cwd string) (*PaneInfo, error)
	PaneClose(paneID string) error
	PaneRead(paneID string) (string, error)
	PaneRecent(paneID string, n int) (text string, truncated bool, err error)
	PaneScreen(paneID string) (Screen, error)
	PaneSendText(paneID, text string) error
	PaneSendKeys(paneID string, keys []string) error
	PaneAtPrompt(paneID string) (bool, error)
	PaneShellPID(paneID string) (int, error)

	AgentStart(paneID, kind string) error
	AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error
	AgentPrompt(target, text string) error
	AgentGet(target string) (AgentInfo, error)
}

// EventStream is one open event stream: the events in order, then the error it ended on.
type EventStream interface {
	Next() (Event, error)
	Close() error
}

// Terminals is a machine's live terminals: a viewer opening one, and how many of this remuda's viewers watch it.
// remuda-core's Terminals (internal/core) is the one implementation: a machine's own attach for each viewer.
type Terminals interface {
	Open(termID string, view View) (Viewer, error)
	Viewers(termID string) int
}

// View is how one viewer opens a terminal: its grid, and whether it leaves the size to the others (Keep) or only
// watches (Observe).
type View struct {
	Cols, Rows int
	Keep       bool
	Observe    bool
}

// Viewer is one viewer's stream of a terminal: the program's bytes out, the keys and the size back.
type Viewer interface {
	Out() <-chan Out // closed once the terminal is no longer attached
	Write(data []byte) error
	Resize(cols, rows int) error
	Close()
}

// Out is one thing a viewer is handed, in order: the program's bytes, or, with Cols and Rows set and no bytes, the
// terminal's grid, which remuda-core tells a viewer that keeps the size or only watches ahead of the paint drawn at it.
type Out struct {
	Data       []byte
	Cols, Rows int
}

// Grid says the Out is a grid rather than bytes.
func (o Out) Grid() bool { return o.Cols > 0 && o.Rows > 0 }

// ValidSize says a viewer's grid is one to give a terminal: a collapsed or hidden mount measures nonsense.
func ValidSize(cols, rows int) bool {
	return cols >= 20 && rows >= 5 && cols <= 500 && rows <= 300
}
