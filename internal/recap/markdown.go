package recap

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Item is one agent's line in a standup: where it is, how it stands, and its recap over the period.
type Item struct {
	Machine   string `json:"machine"`
	Label     string `json:"label"` // the machine's name
	Workspace string `json:"workspace"`
	Pane      string `json:"pane"`
	Dir       string `json:"dir"`
	Agent     string `json:"agent"`
	State     string `json:"state"` // the radar's: working | blocked | done | idle | unknown
	Alert     string `json:"alert,omitempty"`
	Recap     Recap  `json:"recap"`
	Error     string `json:"error,omitempty"` // the conversation could not be read
}

// Markdown is one standup report ready to paste: a heading per agent that did something, the numbers in a
// line, the files, the last answer; the agents with nothing to say listed at the end.
func Markdown(items []Item, since time.Time, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Standup · %s\n\n", now.Format("2 Jan 2006"))
	if !since.IsZero() {
		fmt.Fprintf(&b, "Since %s.\n\n", since.Format("2 Jan 15:04"))
	}
	var quiet []string
	busy := 0
	for _, item := range items {
		where := item.Workspace
		if item.Machine != "local" && item.Label != "" {
			where += " on " + item.Label
		}
		if item.Error != "" {
			fmt.Fprintf(&b, "## %s\n\n- could not be read: %s\n\n", where, item.Error)
			continue
		}
		r := item.Recap
		if r.Quiet() {
			quiet = append(quiet, where)
			continue
		}
		busy++
		fmt.Fprintf(&b, "## %s — %s\n\n", where, stateWords(item))
		for _, line := range Lines(r) {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	if busy == 0 && len(quiet) > 0 {
		b.WriteString("Nothing happened in this period.\n\n")
	}
	if len(quiet) > 0 {
		sort.Strings(quiet)
		fmt.Fprintf(&b, "Nothing to report from %s.\n", strings.Join(quiet, ", "))
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// Lines is one recap as the lines of a report: the figures, the tools, what ran, the files, the subagents and
// questions, the last answer — each without its list mark, for the standup and for a handover alike.
func Lines(r Recap) []string {
	lines := []string{Numbers(r)}
	if r.Calls > 0 {
		lines = append(lines, Calls(r))
	}
	if len(r.Commands) > 0 {
		lines = append(lines, "Ran: "+strings.Join(quoteAll(shortened(r.Commands, 60), 6), ", "))
	}
	if len(r.Edited) > 0 {
		lines = append(lines, "Edited: "+strings.Join(quoteAll(r.Edited, 12), ", "))
	}
	if len(r.Read) > 0 {
		lines = append(lines, "Read: "+strings.Join(quoteAll(r.Read, 12), ", "))
	}
	if r.Subagents > 0 || r.Questions > 0 {
		var parts []string
		if r.Subagents > 0 {
			parts = append(parts, plural(r.Subagents, "subagent"))
		}
		if r.Questions > 0 {
			parts = append(parts, plural(r.Questions, "question"))
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	if r.Said != "" {
		lines = append(lines, "Last answer: "+r.Said)
	}
	return lines
}

// Numbers is the one line of figures: prompts, answers, time, tokens, cost.
func Numbers(r Recap) string {
	parts := []string{plural(r.Prompts, "prompt"), plural(r.Answers, "answer")}
	if r.Took > 0 {
		parts = append(parts, Spent(r.Took))
	}
	if r.Tokens > 0 {
		parts = append(parts, tokens(r.Tokens)+" tokens")
	}
	if r.Cost > 0 {
		parts = append(parts, usd(r.Cost))
	}
	return strings.Join(parts, " · ")
}

// Calls is the tools line: how many calls, the busiest tools, the failures, the longest call.
func Calls(r Recap) string {
	names := make([]string, 0, len(r.Tools))
	for name := range r.Tools {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if r.Tools[names[i]] != r.Tools[names[j]] {
			return r.Tools[names[i]] > r.Tools[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > 4 {
		names = names[:4]
	}
	byTool := make([]string, 0, len(names))
	for _, name := range names {
		byTool = append(byTool, fmt.Sprintf("%s %d", name, r.Tools[name]))
	}
	line := plural(r.Calls, "tool call")
	if len(byTool) > 0 {
		line += " (" + strings.Join(byTool, ", ") + ")"
	}
	if r.Failed > 0 {
		line += " · " + plural(r.Failed, "failed call")
	}
	if r.Longest != nil {
		line += " · longest " + r.Longest.Name + " " + Spent(r.Longest.Took)
		if r.Longest.Hint != "" {
			line += " (" + r.Longest.Hint + ")"
		}
	}
	return line
}

func stateWords(item Item) string {
	if item.Alert != "" {
		return "needs you: " + item.Alert
	}
	switch item.State {
	case "working":
		return "working"
	case "blocked":
		return "needs you"
	case "done":
		return "finished"
	case "idle":
		return "idle"
	}
	return item.State
}

// Spent says how long a piece of work took, as the page does: "12 s", "4 min 12 s", "1 h 12 min".
func Spent(seconds float64) string {
	s := int(seconds + 0.5)
	if s < 60 {
		return fmt.Sprintf("%d s", s)
	}
	m := s / 60
	if m < 60 {
		if s%60 != 0 {
			return fmt.Sprintf("%d min %d s", m, s%60)
		}
		return fmt.Sprintf("%d min", m)
	}
	h := m / 60
	if m%60 != 0 {
		return fmt.Sprintf("%d h %d min", h, m%60)
	}
	return fmt.Sprintf("%d h", h)
}

func tokens(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%dk", (n+500)/1000)
	}
	return fmt.Sprint(n)
}

func usd(n float64) string {
	switch {
	case n >= 100:
		return fmt.Sprintf("$%.0f", n)
	case n >= 10:
		return fmt.Sprintf("$%.1f", n)
	}
	return fmt.Sprintf("$%.2f", n)
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// shortened is every text cut to n runes: a command is named, not quoted whole.
func shortened(texts []string, n int) []string {
	out := make([]string, len(texts))
	for i, text := range texts {
		out[i] = oneLine(text, n)
	}
	return out
}

// quoteAll wraps the first n texts in backticks, "+k more" for the rest.
func quoteAll(texts []string, n int) []string {
	out := make([]string, 0, n+1)
	for i, text := range texts {
		if i == n {
			out = append(out, fmt.Sprintf("+%d more", len(texts)-n))
			break
		}
		out = append(out, "`"+strings.ReplaceAll(text, "`", "'")+"`")
	}
	return out
}
