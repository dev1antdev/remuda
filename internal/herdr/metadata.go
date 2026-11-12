package herdr

import (
	"strings"
	"time"
	"unicode"
)

// What a client may write into herdr's own UI: display values on a pane or a workspace (tokens a person's sidebar rows
// name as $name, a label for each agent state) and a toast. herdr keeps them per source, normalizes the text and lets
// a value with a TTL lapse by itself.

// Metadata limits as herdr 0.9.5 states them.
const (
	MaxTokens     = 16 // token keys one report may name
	MaxValueRunes = 80 // a token's value, a state label: herdr cuts there
	MaxTTL        = 24 * time.Hour
	MaxToastTitle = 80
	MaxToastBody  = 240
)

// PaneMetadata is one pane.report_metadata: Tokens set a value per name, a nil value clears that name, a name left
// out stays as it is; Labels replace the source's state labels, ClearLabels drops them (herdr refuses both at once).
type PaneMetadata struct {
	Pane        string
	Source      string
	Tokens      map[string]*string
	Labels      map[string]string // idle | working | blocked | done | unknown → the words state_text shows
	ClearLabels bool
	TTL         time.Duration // 0 keeps the values until replaced, cleared or the pane closes
}

// WorkspaceMetadata is one workspace.report_metadata: tokens alone, the same patch rules.
type WorkspaceMetadata struct {
	Workspace string
	Source    string
	Tokens    map[string]*string
	TTL       time.Duration
}

// PaneReportMetadata writes display values onto a pane; herdr answers pane_not_found for a pane gone.
func (c *Client) PaneReportMetadata(m PaneMetadata) error {
	params := map[string]any{"pane_id": m.Pane, "source": m.Source, "tokens": tokenParams(m.Tokens)}
	if len(m.Labels) > 0 {
		params["state_labels"] = m.Labels
	}
	if m.ClearLabels {
		params["clear_state_labels"] = true
	}
	if ms := ttlMillis(m.TTL); ms > 0 {
		params["ttl_ms"] = ms
	}
	return c.Call("pane.report_metadata", params, nil)
}

// WorkspaceReportMetadata writes display values onto a workspace.
func (c *Client) WorkspaceReportMetadata(m WorkspaceMetadata) error {
	params := map[string]any{"workspace_id": m.Workspace, "source": m.Source, "tokens": tokenParams(m.Tokens)}
	if ms := ttlMillis(m.TTL); ms > 0 {
		params["ttl_ms"] = ms
	}
	return c.Call("workspace.report_metadata", params, nil)
}

func tokenParams(tokens map[string]*string) map[string]any {
	out := make(map[string]any, len(tokens))
	for name, value := range tokens {
		if value == nil {
			out[name] = nil
		} else {
			out[name] = *value
		}
	}
	return out
}

func ttlMillis(ttl time.Duration) int64 {
	if ttl <= 0 {
		return 0
	}
	return min(ttl, MaxTTL).Milliseconds()
}

// Toast is one notification.show: a sound of none, done or request plays only when herdr shows it.
type Toast struct {
	Title string
	Body  string
	Sound string
}

// Shown is herdr's answer to a toast: whether anything showed and why not — disabled (ui.toast.delivery is off),
// rate_limited, no_foreground_client, busy (a toast already up was not replaced).
type Shown struct {
	Shown  bool   `json:"shown"`
	Reason string `json:"reason"`
}

// NotificationShow pops a toast through herdr's configured delivery on its foreground client.
func (c *Client) NotificationShow(t Toast) (Shown, error) {
	params := map[string]any{"title": t.Title}
	if t.Body != "" {
		params["body"] = t.Body
	}
	if t.Sound != "" {
		params["sound"] = t.Sound
	}
	var out Shown
	err := c.Call("notification.show", params, &out)
	return out, err
}

// DisplayText is a value as herdr stores it: control characters removed, the ends trimmed, at most n runes. A value
// written this way reads back the same, so what was written can be told from what herdr holds.
func DisplayText(s string, n int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	if runes := []rune(s); len(runes) > n {
		s = strings.TrimSpace(string(runes[:n]))
	}
	return s
}
