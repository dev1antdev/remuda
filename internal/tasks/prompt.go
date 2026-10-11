package tasks

import (
	"html"
	"regexp"
	"strings"
)

// Prompt is what an agent gets for a task: the title, the description as
// text, and which task it is — with /grill-me in front for a grooming pass.
// The same words the UI's "Send to agent" composes.
func Prompt(t Task, project string, groom bool) string {
	parts := []string{strings.TrimSpace(t.Title)}
	if d := HTMLToText(t.Description); d != "" {
		parts = append(parts, d)
	}
	where := "(remuda task " + t.Identifier
	if project != "" {
		where += " in " + project
	}
	parts = append(parts, where+")")
	out := strings.Join(parts, "\n\n")
	if groom {
		out = "/grill-me " + out
	}
	return out
}

var (
	htmlBreak = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlBlock = regexp.MustCompile(`(?i)</(p|div|li|h[1-6]|pre|blockquote|tr)>`)
	htmlItem  = regexp.MustCompile(`(?i)<li[^>]*>`)
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
	manyNL    = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText flattens the description's HTML the way the UI does: blocks
// end a line, list items get a dash, the rest of the tags go, entities
// unescape.
func HTMLToText(s string) string {
	if !strings.Contains(s, "<") {
		return strings.TrimSpace(html.UnescapeString(s))
	}
	s = htmlBreak.ReplaceAllString(s, "\n")
	s = htmlItem.ReplaceAllString(s, "- ")
	s = htmlBlock.ReplaceAllString(s, "\n")
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(manyNL.ReplaceAllString(s, "\n\n"))
}
