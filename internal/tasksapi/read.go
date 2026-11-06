package tasksapi

import (
	"net/url"
	"time"
)

// The read side of the door: what the agents are doing, as the roster, a conversation's recap and the standup
// say it to the person — for an agent that wants to know who is on what before it files or takes a task.

// Roster is the cockpit's roster as a program needs it: every machine with its workspaces, tabs and panes.
type Roster struct {
	Machines []Machine `json:"machines"`
	Version  string    `json:"version"`
}

// Machine is one machine of the roster: a connected one carries its workspaces.
type Machine struct {
	ID       string      `json:"id"`
	Label    string      `json:"label"`
	State    string      `json:"state"` // connected | connecting | error
	Error    string      `json:"error,omitempty"`
	Sessions []Workspace `json:"sessions"`
}

// Workspace is one of herdr's workspaces with its tabs.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Dir  string `json:"dir,omitempty"`
	Tabs []Tab  `json:"tabs"`
}

// Tab is one tab with its panes.
type Tab struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Panes []Pane `json:"panes"`
}

// Pane is one pane: a shell when Agent is empty, else an agent with the sidebar's reading of it.
type Pane struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	Agent  string       `json:"agent"`
	CWD    string       `json:"cwd"`
	Status string       `json:"status"`
	Radar  *Radar       `json:"radar,omitempty"`
	Usage  *Usage       `json:"usage,omitempty"`
	Said   string       `json:"said,omitempty"`
	PR     *PullRequest `json:"pr,omitempty"`
}

// Radar is the sidebar's reading of an agent: its state, when it began, the watchdog's note.
type Radar struct {
	State      string `json:"state"` // working | blocked | done | idle | unknown
	LastActive int64  `json:"last_active,omitempty"`
	Since      int64  `json:"since,omitempty"`
	Alert      string `json:"alert,omitempty"`
}

// Usage is the meter of an agent's conversation.
type Usage struct {
	Model   string  `json:"model"`
	Name    string  `json:"name"`
	Cost    float64 `json:"cost"`
	Context int64   `json:"context"`
	Window  int     `json:"window"`
}

// PullRequest is the request an agent named.
type PullRequest struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// Recap is one conversation's recap as the cockpit answers it: the report as Markdown.
type Recap struct {
	Since    string `json:"since"`
	Markdown string `json:"markdown"`
}

// Standup is the standup over every agent as the cockpit answers it: the report as Markdown.
type Standup struct {
	Since    string `json:"since"`
	Markdown string `json:"markdown"`
}

// AgentPane is an agent pane with the workspace and tab it sits in.
type AgentPane struct {
	Workspace Workspace
	Tab       Tab
	Pane      Pane
}

// Roster reads the roster.
func (c *Client) Roster() (Roster, error) {
	var out Roster
	return out, c.do("GET", "/api/roster", nil, &out)
}

// Recap reads one conversation's recap: the pane's live one, or a session of it; a zero since is the whole conversation.
func (c *Client) Recap(machine, pane, session string, since time.Time) (Recap, error) {
	v := url.Values{"machine": {machine}, "pane": {pane}}
	if session != "" {
		v.Set("session", session)
	}
	if !since.IsZero() {
		v.Set("since", since.UTC().Format(time.RFC3339))
	}
	var out Recap
	return out, c.do("GET", "/api/recap?"+v.Encode(), nil, &out)
}

// Standup reads the standup since the moment given, or since local midnight in the zone named when it is zero.
func (c *Client) Standup(since time.Time, tz string) (Standup, error) {
	v := url.Values{}
	if tz != "" {
		v.Set("tz", tz)
	}
	if !since.IsZero() {
		v.Set("since", since.UTC().Format(time.RFC3339))
	}
	var out Standup
	return out, c.do("GET", "/api/standup?"+v.Encode(), nil, &out)
}

// AgentPanes is every agent pane of a machine with its place, in the roster's order; a shell is left out.
func (m Machine) AgentPanes() []AgentPane {
	var out []AgentPane
	for _, ws := range m.Sessions {
		for _, tab := range ws.Tabs {
			for _, pane := range tab.Panes {
				if pane.Agent != "" {
					out = append(out, AgentPane{Workspace: ws, Tab: tab, Pane: pane})
				}
			}
		}
	}
	return out
}
