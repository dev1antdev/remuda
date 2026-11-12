package tasks

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// deniedComment is remuda's comment on a task after a run's agent was denied calls nobody was there to approve: each
// call, whether the agent stopped on them, and what the person can do.
func deniedComment(kind string, denied transcript.Denials) string {
	var b strings.Builder
	agent := agentLabel(kind)
	if len(denied.Calls) == 1 {
		fmt.Fprintf(&b, "%s was denied a call that would have waited for your permission, since nobody was there to answer:\n\n", agent)
	} else {
		fmt.Fprintf(&b, "%s was denied %d calls that would have waited for your permission, since nobody was there to answer:\n\n", agent, len(denied.Calls))
	}
	for _, call := range denied.Calls {
		if call.What == "" {
			fmt.Fprintf(&b, "- %s\n", call.Tool)
			continue
		}
		fmt.Fprintf(&b, "- %s: %s\n", call.Tool, codeSpan(call.What))
	}
	if denied.EndsTurn {
		b.WriteString("\nIt stopped there, before writing an answer.")
	}
	b.WriteString("\nIf the work needs them, run them yourself in its terminal, or allow them with a permission rule or a permission mode in the project's agent settings.")
	return b.String()
}

// codeSpan is a text as inline code, fenced past any backticks in it.
func codeSpan(text string) string {
	fence := "`"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		return fence + " " + text + " " + fence
	}
	return fence + text + fence
}

// agentLabel is an agent kind as the page names it, "Claude Code" for claude.
func agentLabel(kind string) string {
	for _, a := range sessions.Agents {
		if a.ID == kind {
			return a.Label
		}
	}
	return kind
}
