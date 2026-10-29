// Package recap sums up what an agent did in a conversation: the prompts and answers, the time and the
// tokens, the tools it called and which failed, the files it touched, the commands it ran — one Recap per
// conversation, and a standup report over every agent on every machine. Read from the transcript, never written.
package recap

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/said"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// Call is one tool call named in a recap: the longest one.
type Call struct {
	Name string  `json:"name"`
	Hint string  `json:"hint,omitempty"` // the file, the command, the pattern it was about
	Took float64 `json:"took"`           // seconds
}

// Recap is what one conversation came to over a period.
type Recap struct {
	Prompts   int            `json:"prompts"`
	Answers   int            `json:"answers"` // turns that ended in text
	Took      float64        `json:"took"`    // seconds the answers took, from each prompt to the end of its answer
	Tokens    int64          `json:"tokens"`
	Cost      float64        `json:"cost"` // USD at API prices
	Calls     int            `json:"calls"`
	Tools     map[string]int `json:"tools"` // calls by tool name
	Failed    int            `json:"failed"`
	Longest   *Call          `json:"longest,omitempty"`
	Commands  []string       `json:"commands"` // the shell commands, the first MaxCommands
	Edited    []string       `json:"edited"`   // files written, in the order first touched
	Read      []string       `json:"read"`     // files only read
	Subagents int            `json:"subagents"`
	Questions int            `json:"questions"`
	Asking    bool           `json:"asking"` // the last turn ends in a question still without an answer
	First     string         `json:"first,omitempty"`
	Last      string         `json:"last,omitempty"`
	Said      string         `json:"said,omitempty"`  // the first line of the last answer
	Spans     []Span         `json:"spans,omitempty"` // when it worked, waited and sat idle over the period, set by the Service
}

// MaxCommands is how many shell commands a recap lists.
const MaxCommands = 20

var (
	editTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}
	agentTool = map[string]bool{"Agent": true, "Task": true}
)

// Of sums up the turns from since on; a zero since takes them all. A turn without a time counts.
func Of(turns []transcript.Turn, since time.Time) Recap {
	r := Recap{Tools: map[string]int{}, Commands: []string{}, Edited: []string{}, Read: []string{}}
	edited, read := map[string]bool{}, map[string]bool{}
	lastAnswer, lastRole := "", ""
	var lastPart *transcript.Part
	for i := range turns {
		t := &turns[i]
		if !since.IsZero() && t.Ts != "" {
			if at, err := time.Parse(time.RFC3339Nano, t.Ts); err == nil && at.Before(since) {
				continue
			}
		}
		if r.First == "" {
			r.First = t.Ts
		}
		if t.Ts != "" {
			r.Last = t.Ts
		}
		lastRole = t.Role
		lastPart = nil
		if t.Role == "user" {
			r.Prompts++
			continue
		}
		if answer := t.Answer(); answer != "" {
			r.Answers++
			lastAnswer = answer
		}
		r.Took += t.Took
		r.Tokens += t.Tokens
		r.Cost += t.Cost
		for j := range t.Parts {
			p := &t.Parts[j]
			if p.Type != "tool" {
				continue
			}
			lastPart = p
			r.Calls++
			r.Tools[p.ToolName]++
			if p.IsError {
				r.Failed++
			}
			if r.Longest == nil || p.Took > r.Longest.Took {
				r.Longest = &Call{Name: p.ToolName, Hint: Hint(p), Took: p.Took}
			}
			switch {
			case transcript.ShellCommand(p) != "":
				if cmd := transcript.ShellCommand(p); len(r.Commands) < MaxCommands {
					r.Commands = append(r.Commands, oneLine(cmd, 160))
				}
			case editTools[p.ToolName]:
				if path := inputField(p.Input, "file_path", "notebook_path"); path != "" && !edited[path] {
					edited[path] = true
					r.Edited = append(r.Edited, path)
				}
			case p.ToolName == "Read":
				if path := inputField(p.Input, "file_path"); path != "" && !read[path] {
					read[path] = true
					r.Read = append(r.Read, path)
				}
			case agentTool[p.ToolName]:
				r.Subagents++
			case transcript.IsQuestion(p.ToolName):
				r.Questions++
			}
		}
	}
	if r.Longest != nil && r.Longest.Took == 0 {
		r.Longest = nil
	}
	kept := make([]string, 0, len(r.Read))
	for _, path := range r.Read {
		if !edited[path] {
			kept = append(kept, path)
		}
	}
	r.Read = kept
	r.Asking = lastRole == "assistant" && lastPart != nil && transcript.IsQuestion(lastPart.ToolName) && lastPart.Output == "" && !lastPart.IsError
	r.Said = said.FirstLine(lastAnswer)
	return r
}

// Quiet is a recap with nothing in it: no prompt, no answer, no call.
func (r Recap) Quiet() bool { return r.Prompts == 0 && r.Answers == 0 && r.Calls == 0 }

// inputField is the first of the keys found in a tool's input: an object, or the string the server cut a big input to.
func inputField(input json.RawMessage, keys ...string) string {
	return transcript.InputField(input, keys...)
}

// Hint is what a call was about in a few words: its file, else its command, pattern, address or description.
func Hint(p *transcript.Part) string {
	if path := inputField(p.Input, "file_path", "notebook_path", "path"); path != "" {
		return path
	}
	return oneLine(inputField(p.Input, "command", "cmd", "pattern", "url", "description", "query", "input"), 80)
}

// oneLine is a text on one line cut to n runes, an ellipsis when cut.
func oneLine(text string, n int) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > n {
		return string(runes[:n-1]) + "…"
	}
	return text
}
