package herdr

import "time"

// API is a machine's herdr as remuda asks it: its snapshot, its event stream and every request remuda sends. *Client
// is herdr's own; remuda-core's adapter is the other, so the packages that ask a machine never name either.
type API interface {
	Ping() (version string, protocol int, err error)
	SessionSnapshot() (*Snapshot, error)
	Subscribe(subs []Subscription) (Events, error)
	Bridged() bool

	WorkspaceCreate(cwd, label string) (*WorkspaceCreateResult, error)
	WorkspaceRename(workspaceID, label string) error
	WorkspaceClose(workspaceID string) error
	WorkspaceReportMetadata(m WorkspaceMetadata) error
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
	PaneScroll(paneID string, offset int) (Scroll, error)
	PaneSendText(paneID, text string) error
	PaneSendKeys(paneID string, keys []string) error
	PaneSendInput(paneID, text string, keys []string) error
	PaneAtPrompt(paneID string) (bool, error)
	PaneShellPID(paneID string) (int, error)
	PaneReportMetadata(m PaneMetadata) error

	AgentStart(paneID, kind string) error
	AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error
	AgentPrompt(target, text string) error
	AgentGet(target string) (AgentInfo, error)

	NotificationShow(t Toast) (Shown, error)
}

// Events is one open event stream: the events in order, then the error it ended on.
type Events interface {
	Next() (Event, error)
	Close() error
}

// Terminals is a machine's live terminals: a viewer joining one, typing into it, resizing it, and how many watch it.
type Terminals interface {
	Subscribe(termID string, cols, rows int) (ch chan []byte, cancel func(), err error)
	Write(termID string, data []byte) error
	Resize(termID string, cols, rows int) error
	Viewers(termID string) int
}

var (
	_ API       = (*Client)(nil)
	_ Terminals = (*AttachManager)(nil)
)
