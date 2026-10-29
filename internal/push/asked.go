package push

import (
	"strings"
	"unicode/utf8"
)

// Asked is the dialog on a pane's screen when a question push goes out: its words and its choices in order.
type Asked struct {
	Text    string
	Choices []string
}

// Two buttons is what a notification shows; a choice's words are cut so both fit on a phone.
const (
	maxChoiceActions = 2
	choiceTitleRunes = 24
	questionBodyMax  = 120
)

// asked dresses a question push with the dialog the pane shows at that moment; without a Question input, or with no
// dialog on the screen, the push goes as built.
func (s *Service) asked(p Payload) Payload {
	if s.Question == nil {
		return p
	}
	asked, ok := s.Question(p.Machine, p.Pane)
	return askedPayload(p, asked, ok)
}

// askedPayload gives a question push its choices as buttons and the dialog's first line as its body, so the
// person answers from the notification; a dialog with one choice or none keeps the plain push.
func askedPayload(p Payload, asked Asked, ok bool) Payload {
	if !ok {
		return p
	}
	if body := questionBody(asked.Text); body != "" {
		p.Body = body
	}
	if len(asked.Choices) < 2 {
		return p
	}
	for i, choice := range asked.Choices[:maxChoiceActions] {
		p.Actions = append(p.Actions, Action{Action: "choice-" + string(rune('0'+i)), Title: cutRunes(choice, choiceTitleRunes)})
	}
	p.Choices = append([]string(nil), asked.Choices...)
	return p
}

// questionBody is the dialog's first line and, after it, the first line of its detail: "Bash command · rm -rf build".
func questionBody(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	switch len(lines) {
	case 0:
		return ""
	case 1:
		return cutRunes(lines[0], questionBodyMax)
	}
	return cutRunes(lines[0]+" · "+lines[1], questionBodyMax)
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:n-1])) + "…"
}
