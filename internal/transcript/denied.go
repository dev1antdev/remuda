package transcript

import (
	"strconv"
	"strings"
)

// The words Claude Code answers a tool call with when it refused the call without asking, nobody being there to answer
// its permission prompt: under permissions.denyInsteadOfPrompting or --permission-prompts none (2.1.322's wording),
// in a session that can show no prompt, and in its don't-ask mode. A call the auto mode classifier, a deny rule or the
// person refused is not one of them.
var deniedOpenings = []string{
	"Permission for this tool use was denied. It requires approval, and this session has no approval surface",
	"Permission for this tool use was denied: it requires interactive approval",
}

const deniedDontAsk = "has been denied because Claude Code is running in don't ask mode"

// Denial is a tool call Claude Code refused without asking: the tool, what the call was about, when it was made.
type Denial struct {
	Tool   string `json:"tool"`
	What   string `json:"what,omitempty"` // the command, the path or the address the call named, on one line and cut
	At     string `json:"at,omitempty"`
	ToolID string `json:"tool_id,omitempty"`
}

// Denials is what Claude Code refused without asking in a stretch of a conversation, and whether the agent's latest
// turn ends on such a call: a run of refusals stops the turn before the agent writes an answer.
type Denials struct {
	Calls    []Denial `json:"calls"`
	EndsTurn bool     `json:"ends_turn,omitempty"`
}

// IsDenied says whether a tool call's result is Claude Code's refusal for want of anyone to answer its prompt.
func IsDenied(p Part) bool {
	if p.Type != "tool" || !p.IsError {
		return false
	}
	out := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p.Output), "Error: "))
	for _, opening := range deniedOpenings {
		if strings.HasPrefix(out, opening) {
			return true
		}
	}
	return strings.HasPrefix(out, "Permission to use ") && strings.Contains(out, deniedDontAsk)
}

// DeniedAfter is the calls refused without asking in the turns after the one holding the record after ("" for the
// whole conversation, and the whole of it when that record is not among the turns).
func DeniedAfter(turns []Turn, after string) Denials {
	from := 0
	if after != "" {
		for i, turn := range turns {
			for _, id := range turn.IDs {
				if id == after {
					from = i + 1
				}
			}
		}
	}
	var out Denials
	for _, turn := range turns[min(from, len(turns)):] {
		for _, part := range turn.Parts {
			if IsDenied(part) {
				out.Calls = append(out.Calls, Denial{Tool: part.ToolName, What: deniedWhat(part), At: part.At, ToolID: part.ToolID})
			}
		}
	}
	if n := len(turns); n > from && turns[n-1].Role == "assistant" {
		parts := turns[n-1].Parts
		out.EndsTurn = len(parts) > 0 && IsDenied(parts[len(parts)-1])
	}
	return out
}

// deniedWhat is what a refused call named: a file first, else its command, pattern, address or description.
func deniedWhat(p Part) string {
	if path := InputField(p.Input, "file_path", "notebook_path", "path"); path != "" {
		return oneLine(path, 160)
	}
	return oneLine(InputField(p.Input, "command", "cmd", "url", "pattern", "query", "description", "input"), 120)
}

// oneLine is a text on one line, cut to n runes with an ellipsis.
func oneLine(text string, n int) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > n {
		return string(runes[:n-1]) + "…"
	}
	return text
}

// DeniedWords is the calls as a line for a schedule's result or a push: "2 calls denied: Bash touch x; Write a.txt",
// the first few named and the rest counted.
func DeniedWords(calls []Denial, named int) string {
	if len(calls) == 0 {
		return ""
	}
	var names []string
	for i, call := range calls {
		if i == named {
			names = append(names, "+"+strconv.Itoa(len(calls)-named)+" more")
			break
		}
		names = append(names, strings.TrimSpace(call.Tool+" "+oneLine(call.What, 60)))
	}
	return DeniedCount(len(calls)) + ": " + strings.Join(names, "; ")
}

// DeniedCount is how many calls were denied without asking, in words: "1 call denied", "3 calls denied".
func DeniedCount(n int) string {
	if n == 1 {
		return "1 call denied"
	}
	return strconv.Itoa(n) + " calls denied"
}
