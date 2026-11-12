package triggers

import (
	"regexp"
	"strings"
)

// MaxShown is how many matching rows a test answers.
const MaxShown = 20

// Tried is a rule tried on a pane's rows: the rows it matches (the last MaxShown), how many in all, and for a preview
// the port the first of them names.
type Tried struct {
	Lines []string `json:"lines"`
	Count int      `json:"count"`
	Port  int      `json:"port,omitempty"`
}

// Try checks the rule and runs it over text a line at a time, as herdr would.
func Try(rule Rule, text string) (Tried, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		rule.Name = "test"
	}
	if err := Check(&rule); err != nil {
		return Tried{}, err
	}
	match := func(line string) bool { return strings.Contains(line, rule.Pattern) }
	if rule.Match == "regex" {
		re := regexp.MustCompile(rule.Pattern) // Check read it
		match = re.MatchString
	}
	out := Tried{Lines: []string{}}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r ")
		if !match(line) {
			continue
		}
		out.Count++
		if out.Port == 0 && rule.Action == ActionPreview {
			out.Port, _, _ = ServedAt(line)
		}
		out.Lines = append(out.Lines, cut(line, 300))
		if len(out.Lines) > MaxShown {
			out.Lines = out.Lines[1:]
		}
	}
	return out, nil
}
