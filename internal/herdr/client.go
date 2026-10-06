// Package herdr speaks herdr's newline-JSON RPC over its Unix socket and owns
// the `herdr terminal attach` PTYs. herdr-ui is a bridge: herdr keeps every
// process, pane and agent status; nothing here mutates state beyond what the
// user asked for (send text/keys, create/close workspaces).
package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultSocket is herdr's socket under a home directory.
func DefaultSocket(home string) string {
	return filepath.Join(home, ".config", "herdr", "herdr.sock")
}

// FindBinary resolves the herdr binary: the given name on PATH, else the
// places installers put it — a process started by an IDE or a service has a
// bare PATH without ~/.local/bin.
func FindBinary(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if strings.Contains(name, "/") {
		return "", fmt.Errorf("%s: not an executable", name)
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{filepath.Join(home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"} {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found in PATH, ~/.local/bin, /opt/homebrew/bin or /usr/local/bin", name)
}

// SocketPath returns $HERDR_SOCKET or herdr's default socket location.
func SocketPath() string {
	if p := os.Getenv("HERDR_SOCKET"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return DefaultSocket(home)
}

// Error is a structured herdr RPC error (code + message): herdr answered,
// and said no.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("herdr %s: %s", e.Code, e.Message) }

// IsCode reports whether err is a herdr error with the given code.
func IsCode(err error, code string) bool {
	var he *Error
	return errors.As(err, &he) && he.Code == code
}

type Client struct {
	Socket  string
	Timeout time.Duration
	// Dial, when set, replaces the unix-socket dial — e.g. an `ssh host nc -U`
	// process for hosts whose sshd cannot forward unix sockets.
	Dial func(timeout time.Duration) (net.Conn, error)
	// OnTransportError fires when a call fails to reach herdr at all (as
	// opposed to a structured *Error): the owner marks the machine dead.
	OnTransportError func(error)
	reqSeq           atomic.Int64
}

func NewClient(socket string) *Client {
	return &Client{Socket: socket, Timeout: 10 * time.Second}
}

func (c *Client) dial(timeout time.Duration) (net.Conn, error) {
	if c.Dial != nil {
		return c.Dial(timeout)
	}
	conn, err := net.DialTimeout("unix", c.Socket, timeout)
	if err != nil {
		return nil, fmt.Errorf("cannot reach herdr at %s: %w", c.Socket, err)
	}
	return conn, nil
}

type rpcFrame struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Call performs one request on one connection — herdr closes the socket after
// a single response, so connections are never pooled or reused.
func (c *Client) Call(method string, params any, out any) error {
	return c.CallTimeout(method, params, out, c.Timeout)
}

// CallTimeout is Call with a custom deadline (worktree git operations etc.).
func (c *Client) CallTimeout(method string, params any, out any, timeout time.Duration) error {
	err := c.call(method, params, out, timeout)
	var he *Error
	if err != nil && !errors.As(err, &he) && c.OnTransportError != nil {
		c.OnTransportError(err)
	}
	return err
}

func (c *Client) call(method string, params any, out any, timeout time.Duration) error {
	conn, err := c.dial(timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	if params == nil {
		params = map[string]any{}
	}
	req := map[string]any{
		"id":     fmt.Sprintf("herdr-ui-%d", c.reqSeq.Add(1)),
		"method": method,
		"params": params,
	}
	enc, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(enc, '\n')); err != nil {
		return err
	}

	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var frame rpcFrame
		if err := json.Unmarshal(line, &frame); err != nil {
			return fmt.Errorf("herdr sent unparseable frame: %.200s", line)
		}
		if frame.Error != nil {
			return frame.Error
		}
		if out != nil && frame.Result != nil {
			return json.Unmarshal(frame.Result, out)
		}
		return nil
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return fmt.Errorf("herdr closed the connection before answering %s", method)
}

// ---- agent status ----

// herdr's agent statuses, and the coarser vocabulary the UI shows.
const (
	StatusWorking = "working"
	StatusBlocked = "blocked"
	StatusDone    = "done"
	StatusIdle    = "idle"
	StatusUnknown = "unknown"

	UIRun   = "run"
	UIInput = "input"
	UIDone  = "done"
	UIIdle  = "idle"
	UIShell = "shell"
)

// UIStatus maps a herdr status to the UI's: a pane, tab or workspace with no
// agent at all is "shell".
func UIStatus(status string, hasAgent bool) string {
	switch status {
	case StatusWorking:
		return UIRun
	case StatusBlocked:
		return UIInput
	case StatusDone:
		return UIDone
	case StatusIdle:
		return UIIdle
	}
	if !hasAgent {
		return UIShell
	}
	return UIIdle
}

// ---- snapshot types (herdr protocol 22; only the fields herdr-ui reads) ----

type AgentSession struct {
	Agent string `json:"agent"`
	Kind  string `json:"kind"` // "id" | "path"
	Value string `json:"value"`
}

type PaneInfo struct {
	PaneID        string        `json:"pane_id"`
	TabID         string        `json:"tab_id"`
	WorkspaceID   string        `json:"workspace_id"`
	TerminalID    string        `json:"terminal_id"`
	Agent         string        `json:"agent"`
	DisplayAgent  string        `json:"display_agent"`
	AgentStatus   string        `json:"agent_status"` // blocked|done|idle|unknown|working
	AgentSession  *AgentSession `json:"agent_session"`
	CWD           string        `json:"cwd"`
	ForegroundCWD string        `json:"foreground_cwd"`
	Label         string        `json:"label"`
	Title         string        `json:"title"`
	TitleStripped string        `json:"terminal_title_stripped"`
	Focused       bool          `json:"focused"`
}

// Dir is the pane's folder: the shell's cwd, else the foreground process's.
func (p *PaneInfo) Dir() string {
	if p.CWD != "" {
		return p.CWD
	}
	return p.ForegroundCWD
}

// AgentName is what to call the agent in the pane; "" for a plain shell.
func (p *PaneInfo) AgentName() string {
	if p.DisplayAgent != "" {
		return p.DisplayAgent
	}
	return p.Agent
}

// DisplayTitle is the user's label, else the terminal title, else the agent.
func (p *PaneInfo) DisplayTitle() string {
	for _, t := range []string{p.Label, p.TitleStripped, p.Title, p.AgentName()} {
		if strings.TrimSpace(t) != "" {
			return t
		}
	}
	return ""
}

type TabInfo struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Number      int    `json:"number"`
	Label       string `json:"label"`
	AgentStatus string `json:"agent_status"`
}

type WorktreeInfo struct {
	CheckoutPath     string `json:"checkout_path"`
	IsLinkedWorktree bool   `json:"is_linked_worktree"`
	RepoKey          string `json:"repo_key"`
	RepoName         string `json:"repo_name"`
}

type WorkspaceInfo struct {
	WorkspaceID string        `json:"workspace_id"`
	Label       string        `json:"label"`
	AgentStatus string        `json:"agent_status"`
	Focused     bool          `json:"focused"`
	ActiveTabID string        `json:"active_tab_id"`
	Worktree    *WorktreeInfo `json:"worktree"`
}

type Rect struct {
	Width  int `json:"width"` // cells
	Height int `json:"height"`
}

type LayoutPane struct {
	PaneID string `json:"pane_id"`
	Rect   Rect   `json:"rect"`
}

type Layout struct {
	Panes []LayoutPane `json:"panes"`
}

type Snapshot struct {
	Version       string          `json:"version"`
	FocusedPaneID string          `json:"focused_pane_id"`
	Workspaces    []WorkspaceInfo `json:"workspaces"`
	Tabs          []TabInfo       `json:"tabs"`
	Panes         []PaneInfo      `json:"panes"`
	Layouts       []Layout        `json:"layouts"`
}

// PaneSize returns a pane's grid size in cells from the layouts, or 0,0.
func (s *Snapshot) PaneSize(paneID string) (cols, rows int) {
	for _, l := range s.Layouts {
		for _, p := range l.Panes {
			if p.PaneID == paneID {
				return p.Rect.Width, p.Rect.Height
			}
		}
	}
	return 0, 0
}

func (c *Client) SessionSnapshot() (*Snapshot, error) {
	var out struct {
		Snapshot Snapshot `json:"snapshot"`
	}
	if err := c.Call("session.snapshot", nil, &out); err != nil {
		return nil, err
	}
	return &out.Snapshot, nil
}

func (c *Client) Ping() (version string, protocol int, err error) {
	var out struct {
		Version  string `json:"version"`
		Protocol int    `json:"protocol"`
	}
	if err := c.Call("ping", nil, &out); err != nil {
		return "", 0, err
	}
	return out.Version, out.Protocol, nil
}

func (c *Client) PaneSendText(paneID, text string) error {
	return c.Call("pane.send_text", map[string]any{"pane_id": paneID, "text": text}, nil)
}

func (c *Client) PaneSendKeys(paneID string, keys []string) error {
	return c.Call("pane.send_keys", map[string]any{"pane_id": paneID, "keys": keys}, nil)
}

// AgentPrompt is herdr's own submit: bracketed paste + Enter 300ms later.
// Refuses with agent_blocked while the agent waits for a menu answer, and
// agent_not_found / agent_not_ready without an agent in front.
func (c *Client) AgentPrompt(target, text string) error {
	return c.Call("agent.prompt", map[string]any{"target": target, "text": text}, nil)
}

type WorkspaceCreateResult struct {
	Workspace WorkspaceInfo `json:"workspace"`
	RootPane  PaneInfo      `json:"root_pane"`
}

func (c *Client) WorkspaceCreate(cwd, label string) (*WorkspaceCreateResult, error) {
	params := map[string]any{"focus": false}
	if cwd != "" {
		params["cwd"] = cwd
	}
	if label != "" {
		params["label"] = label
	}
	var out WorkspaceCreateResult
	if err := c.Call("workspace.create", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) WorkspaceClose(workspaceID string) error {
	return c.Call("workspace.close", map[string]any{"workspace_id": workspaceID}, nil)
}

type TabCreateResult struct {
	Tab      TabInfo  `json:"tab"`
	RootPane PaneInfo `json:"root_pane"`
}

// TabCreate opens another tab in the workspace's folder. herdr keeps the
// label as given, so empty means unnamed.
func (c *Client) TabCreate(workspaceID, label string) (*TabCreateResult, error) {
	params := map[string]any{"workspace_id": workspaceID, "focus": false}
	if label != "" {
		params["label"] = label
	}
	var out TabCreateResult
	if err := c.Call("tab.create", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TabClose closes the tab and every pane in it; a workspace's last tab takes
// the workspace with it.
func (c *Client) TabClose(tabID string) error {
	return c.Call("tab.close", map[string]any{"tab_id": tabID}, nil)
}

type WorktreeCreateResult struct {
	Workspace WorkspaceInfo `json:"workspace"`
	RootPane  PaneInfo      `json:"root_pane"`
	Worktree  struct {
		Path   string `json:"path"`
		Branch string `json:"branch"`
	} `json:"worktree"`
}

// WorktreeCreate runs `git worktree add` for the workspace's repository and
// opens the checkout as a new workspace grouped under it. git on a large
// checkout can take well over the default timeout.
func (c *Client) WorktreeCreate(workspaceID, branch, base, label string) (*WorktreeCreateResult, error) {
	params := map[string]any{"workspace_id": workspaceID, "branch": branch, "focus": false}
	if base != "" {
		params["base"] = base
	}
	if label != "" {
		params["label"] = label
	}
	var out WorktreeCreateResult
	if err := c.CallTimeout("worktree.create", params, &out, 60*time.Second); err != nil {
		return nil, err
	}
	return &out, nil
}

// AgentStart launches an agent (by herdr manifest kind) in a pane.
func (c *Client) AgentStart(paneID, kind string) error {
	return c.CallTimeout("agent.start", map[string]any{
		"pane_id": paneID, "kind": kind, "name": kind,
	}, nil, 30*time.Second)
}
