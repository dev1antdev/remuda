// Package handover passes a conversation to another agent: the gist of it so far — the recap, the last exchanges in
// full — becomes the first prompt of a fresh agent in a new tab of the same workspace. Claude got stuck, the budget
// says a cheaper model, the person wants Codex's take: a resume works only for the same agent, this works across them.
package handover

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// MaxText is what the first prompt is kept under: an agent's prompt box takes a long paste, not a whole transcript.
const MaxText = 60 * 1024

// MaxLastN is the most exchanges a handover carries in full.
const MaxLastN = 10

// Exchange is one prompt of the person's and what the agent did and said on it, in order.
type Exchange struct {
	Prompt string
	Lines  []string // the agent's text as it came, a tool call as one line each
}

// Text is the first prompt for the agent taking over: who it takes over from, what was done so far (the recap's
// lines), the last n exchanges in full, and what to do next. Kept under MaxText: the oldest full exchanges go
// first, then the recap's lists, then the text is cut.
func Text(from string, summary recap.Recap, turns []transcript.Turn, lastN int) string {
	exchanges := Exchanges(turns)
	if lastN < 0 {
		lastN = 0
	}
	if lastN > MaxLastN {
		lastN = MaxLastN
	}
	if lastN > len(exchanges) {
		lastN = len(exchanges)
	}
	for n := lastN; n >= 0; n-- {
		if text := compose(from, summary, exchanges[len(exchanges)-n:]); len(text) <= MaxText {
			return text
		}
	}
	summary.Commands, summary.Edited, summary.Read = nil, nil, nil
	text := compose(from, summary, nil)
	if len(text) > MaxText {
		text = cut(text, MaxText)
	}
	return text
}

// Exchanges pairs every prompt of the person's with what the agent did on it until the next prompt.
func Exchanges(turns []transcript.Turn) []Exchange {
	var out []Exchange
	for i := range turns {
		t := &turns[i]
		if t.Role == "user" {
			out = append(out, Exchange{Prompt: userText(t)})
			continue
		}
		if len(out) == 0 {
			out = append(out, Exchange{})
		}
		last := &out[len(out)-1]
		for j := range t.Parts {
			p := &t.Parts[j]
			switch p.Type {
			case "text":
				if text := strings.TrimSpace(p.Text); text != "" {
					last.Lines = append(last.Lines, text)
				}
			case "tool":
				last.Lines = append(last.Lines, callLine(p))
			}
		}
	}
	return out
}

func compose(from string, summary recap.Recap, exchanges []Exchange) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Handover from %s\n\n", from)
	then := ""
	if len(exchanges) > 0 {
		then = ", then the last exchanges in full"
	}
	fmt.Fprintf(&b, "I am handing this conversation over to you. %s worked on it with me in this workspace until now; below is what it did%s. Carry on from here. Ask me if anything is unclear.\n\n", from, then)
	b.WriteString("## What was done so far\n\n")
	for _, line := range recap.Lines(summary) {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	if len(exchanges) > 0 {
		fmt.Fprintf(&b, "\n## The last %s in full\n", plural(len(exchanges), "exchange"))
		for _, ex := range exchanges {
			b.WriteString("\n### Me\n\n")
			if ex.Prompt == "" {
				b.WriteString("(no text)\n")
			} else {
				b.WriteString(ex.Prompt + "\n")
			}
			fmt.Fprintf(&b, "\n### %s\n\n", from)
			if len(ex.Lines) == 0 {
				b.WriteString("(no answer)\n")
			}
			for _, line := range ex.Lines {
				b.WriteString(line + "\n")
			}
		}
	}
	b.WriteString("\n---\n\nCarry on from here. Ask me if anything is unclear.\n")
	return b.String()
}

// userText is what the person wrote in a turn of theirs, the text parts joined.
func userText(t *transcript.Turn) string {
	var texts []string
	for _, p := range t.Parts {
		if p.Type == "text" {
			if text := strings.TrimSpace(p.Text); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return strings.Join(texts, "\n\n")
}

// callLine is a tool call as one line: the tool, what it was about, whether it failed.
func callLine(p *transcript.Part) string {
	line := "- " + p.ToolName
	if hint := recap.Hint(p); hint != "" {
		line += ": " + hint
	}
	if p.IsError {
		line += " (failed)"
	}
	return line
}

// cut keeps the first n bytes of a text at a rune boundary, with a note that it was cut.
func cut(text string, n int) string {
	note := "\n\n(cut here: the conversation was longer than a first prompt can carry)\n"
	n -= len(note)
	for n > 0 && n < len(text) && (text[n]&0xC0) == 0x80 {
		n--
	}
	if n < len(text) {
		text = text[:n]
	}
	return text + note
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
