package mcp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/tasksapi"
)

// The read tools: who is on what, from the roster, a conversation's recap and the standup the cockpit shows
// the person — so an agent looks before it files a task or takes one.

// RecapArgs names one agent's conversation.
type RecapArgs struct {
	Machine string `json:"machine"`
	Pane    string `json:"pane"`
	Since   string `json:"since"`
}

// StandupArgs is the standup's period.
type StandupArgs struct {
	Since string `json:"since"`
}

// Agents is agents_list: a table of every agent on every machine, the agent's own pane marked.
func (t *Tools) Agents() (string, error) {
	roster, err := t.Client.Roster()
	if err != nil {
		return "", err
	}
	now := t.now()
	var rows []string
	var away []string
	for _, mc := range roster.Machines {
		if mc.State != "connected" {
			note := mc.State
			if mc.Error != "" {
				note += ": " + mc.Error
			}
			away = append(away, fmt.Sprintf("%s (%s)", mc.Label, note))
			continue
		}
		for _, ap := range mc.AgentPanes() {
			pane := ap.Pane
			id := pane.ID
			if id == t.Source && t.Source != "" {
				id += " (you)"
			}
			rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s |",
				cell(mc.Label), cell(ap.Workspace.Name), cell(id), cell(pane.Agent), cell(agentState(pane.Radar, now)), cell(oneLine(pane.Said, 80)), cell(prLabel(pane.PR)), cell(money(pane.Usage))))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d agent%s", len(rows), plural(len(rows)))
	if len(away) > 0 {
		fmt.Fprintf(&b, "; not reachable now: %s", strings.Join(away, ", "))
	}
	b.WriteString("\n\n")
	if len(rows) == 0 {
		return b.String(), nil
	}
	b.WriteString("| machine | workspace | pane | agent | state | last answer | PR | cost |\n|---|---|---|---|---|---|---|---|\n")
	b.WriteString(strings.Join(rows, "\n") + "\n")
	return b.String(), nil
}

// AgentRecap is agent_recap: what one agent's conversation came to, headed by where it is and how it stands.
func (t *Tools) AgentRecap(a RecapArgs) (string, error) {
	if strings.TrimSpace(a.Pane) == "" {
		return "", errors.New("agent_recap needs the pane, as agents_list lists it")
	}
	since, err := sinceOf(a.Since, t.now())
	if err != nil {
		return "", err
	}
	roster, err := t.Client.Roster()
	if err != nil {
		return "", err
	}
	mc, err := findMachine(roster, a.Machine)
	if err != nil {
		return "", err
	}
	ap, err := findPane(mc, strings.TrimSpace(a.Pane))
	if err != nil {
		return "", err
	}
	rec, err := t.Client.Recap(mc.ID, ap.Pane.ID, "", since)
	if err != nil {
		return "", err
	}
	where := fmt.Sprintf("%s in %s", ap.Pane.Agent, ap.Workspace.Name)
	if mc.ID != "local" {
		where += " on " + mc.Label
	}
	return fmt.Sprintf("%s (%s) — %s\n\n%s", where, ap.Pane.ID, agentState(ap.Pane.Radar, t.now()), rec.Markdown), nil
}

// Standup is standup: the report over every agent, since local midnight unless the period says otherwise.
func (t *Tools) Standup(a StandupArgs) (string, error) {
	since, err := sinceOf(a.Since, t.now())
	if err != nil {
		return "", err
	}
	standup, err := t.Client.Standup(since, time.Local.String())
	if err != nil {
		return "", err
	}
	return standup.Markdown, nil
}

func (t *Tools) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// sinceOf reads a period: nothing (the tool's own default), midnight, 24h, 7d, a date or an RFC 3339 moment.
func sinceOf(s string, now time.Time) (time.Time, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return time.Time{}, nil
	case "midnight", "today":
		local := now.In(time.Local)
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local), nil
	case "24h":
		return now.Add(-24 * time.Hour), nil
	case "7d":
		return now.Add(-7 * 24 * time.Hour), nil
	}
	if d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local); err == nil {
		return d, nil
	}
	if at, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
		return at, nil
	}
	return time.Time{}, fmt.Errorf("since is midnight, 24h, 7d, a date YYYY-MM-DD or an RFC 3339 moment, not %q", s)
}

// findMachine is the machine named by id or by its label, case-folded; nothing names the cockpit's own.
func findMachine(roster tasksapi.Roster, name string) (tasksapi.Machine, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "local"
	}
	for _, mc := range roster.Machines {
		if mc.ID == name || strings.EqualFold(mc.Label, name) {
			if mc.State != "connected" {
				return tasksapi.Machine{}, fmt.Errorf("%s is %s now", mc.Label, mc.State)
			}
			return mc, nil
		}
	}
	var names []string
	for _, mc := range roster.Machines {
		names = append(names, fmt.Sprintf("%s (id %s)", mc.Label, mc.ID))
	}
	return tasksapi.Machine{}, fmt.Errorf("no machine %q; the machines: %s", name, strings.Join(names, ", "))
}

// findPane is the agent pane with the id given on the machine.
func findPane(mc tasksapi.Machine, id string) (tasksapi.AgentPane, error) {
	var ids []string
	for _, ap := range mc.AgentPanes() {
		if ap.Pane.ID == id {
			return ap, nil
		}
		ids = append(ids, ap.Pane.ID)
	}
	if len(ids) == 0 {
		return tasksapi.AgentPane{}, fmt.Errorf("no agent pane %s on %s: no agent runs there", id, mc.Label)
	}
	return tasksapi.AgentPane{}, fmt.Errorf("no agent pane %s on %s; its agents: %s", id, mc.Label, strings.Join(ids, ", "))
}

// agentState says how an agent stands and for how long: "working 12 min", "waiting for the person 3 min", "finished 1 h ago".
func agentState(r *tasksapi.Radar, now time.Time) string {
	if r == nil {
		return "unknown"
	}
	if r.Alert != "" {
		return "needs the person: " + r.Alert
	}
	span := ""
	if r.Since > 0 {
		span = ago(now.Sub(time.UnixMilli(r.Since)))
	}
	switch r.State {
	case "working":
		return withSpan("working", span, "")
	case "blocked":
		return withSpan("waiting for the person", span, "")
	case "done":
		return withSpan("finished", span, " ago")
	case "idle":
		if r.LastActive > 0 {
			return withSpan("idle, last worked", ago(now.Sub(time.UnixMilli(r.LastActive))), " ago")
		}
		return "idle"
	}
	return r.State
}

// withSpan is a state with how long: "finished 2 h ago", "finished just now", "finished" when nobody knows.
func withSpan(state, span, suffix string) string {
	switch span {
	case "":
		return state
	case "just now":
		return state + " just now"
	}
	return state + " " + span + suffix
}

// ago is a span as "12 min", "1 h 5 min", "3 d".
func ago(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	m := int(d.Minutes())
	switch {
	case m < 1:
		return "just now"
	case m < 60:
		return fmt.Sprintf("%d min", m)
	case m < 24*60:
		if m%60 == 0 {
			return fmt.Sprintf("%d h", m/60)
		}
		return fmt.Sprintf("%d h %d min", m/60, m%60)
	}
	return fmt.Sprintf("%d d", m/(24*60))
}

func prLabel(pr *tasksapi.PullRequest) string {
	if pr == nil {
		return ""
	}
	if pr.URL != "" {
		return fmt.Sprintf("[%s](%s)", pr.Label, pr.URL)
	}
	return pr.Label
}

// money is a conversation's cost as the page says it, nothing for a pane without a meter.
func money(u *tasksapi.Usage) string {
	if u == nil || u.Cost <= 0 {
		return ""
	}
	switch {
	case u.Cost >= 100:
		return fmt.Sprintf("$%.0f", u.Cost)
	case u.Cost >= 10:
		return fmt.Sprintf("$%.1f", u.Cost)
	}
	return fmt.Sprintf("$%.2f", u.Cost)
}

// oneLine is the first line of a text cut to n characters.
func oneLine(s string, n int) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	runes := []rune(s)
	if len(runes) > n {
		return strings.TrimSpace(string(runes[:n-1])) + "…"
	}
	return s
}
