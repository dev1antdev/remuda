package sessions

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// A message that names a picture, and the Enter Claude Code loses of it.
//
// herdr's own submit writes the text as a paste and Enter right after it. Claude Code swallows an Enter that arrives
// while it takes a paste in, and presses it again itself once the paste is in — unless the paste names a picture
// (a composer's attachment, "@.remuda/uploads/shot.png"): then it reads the file first, and the Enter is gone for
// good. The message sits in the prompt box as "[Pasted text]", unsent (read off Claude Code 2.1.325's paste hook).
// So the text still goes by herdr's submit, the one way that makes sure an agent holds the terminal; the paste is
// waited out — the footer says "Pasting…" while it lasts — and Enter is pressed again after it, alone, only while the
// prompt box still shows the paste unsent.

// Settler is what the Enter after the paste needs of herdr's client beyond a Prompter's part.
type Settler interface {
	AgentGet(target string) (herdr.AgentInfo, error)
	PaneRead(paneID string) (string, error)
	PaneAtPrompt(paneID string) (bool, error)
}

// The paste's wait: a first pause for Claude Code to take the paste up, then a look at its footer every SettlePoll
// until it no longer says it is pasting, SettleFor at most. Vars, so a test does not wait.
var (
	SettleFirst = 250 * time.Millisecond
	SettlePoll  = 150 * time.Millisecond
	SettleFor   = 10 * time.Second
)

// ErrAskedMeanwhile: the agent put a question up between the paste and the Enter; the Enter would have answered it.
var ErrAskedMeanwhile = errors.New("the agent asked something while your message was on its way: it is in the agent's prompt box, not sent — answer the prompt, then press Enter in its terminal")

// ErrLeftMeanwhile: the agent left the terminal just after herdr gave it the text, and no Enter follows.
var ErrLeftMeanwhile = errors.New("the agent left just as your message reached it: it may not have been sent")

// ErrNotEntered: the text reached the agent, and whether it went could not be made sure of or has to wait for its turn.
var ErrNotEntered = errors.New("your message reached the agent but may still be unsent in its prompt box: look there, and press Enter if it is")

// NotDelivered says the text did not surely reach the agent as a message, so the person keeps it and is told why.
func NotDelivered(err error) bool {
	for _, known := range []error{ErrAgentBusy, ErrNoAgent, ErrAskedMeanwhile, ErrLeftMeanwhile, ErrNotEntered} {
		if errors.Is(err, known) {
			return true
		}
	}
	return false
}

const (
	claudeAgent   = "claude"
	pastingWords  = "Pasting"
	footerRows    = 6
	statusBlocked = "blocked"
)

// pictureRe is a file name Claude Code may take for a picture in a paste.
var pictureRe = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp|bmp|tiff?|heic|heif|avif)\b`)

// unsentRe is the prompt box holding a paste: its marker right after the prompt sign, a border before it at most.
var unsentRe = regexp.MustCompile(`^[\s│|]*[>❯]\s*\[Pasted text`)

// NamesPicture says the text names a picture file, the paste Claude Code takes in at its own pace.
func NamesPicture(text string) bool {
	return pictureRe.MatchString(text)
}

// stillPasting reads Claude Code's footer off a screen: true while it says the paste is being taken in.
func stillPasting(screen string) bool {
	return slices.ContainsFunc(footer(screen), func(row string) bool { return strings.Contains(row, pastingWords) })
}

// stillUnsent reads the prompt box off a screen: true while the paste sits in it, waiting for its Enter.
func stillUnsent(screen string) bool { return slices.ContainsFunc(footer(screen), unsentRe.MatchString) }

// footer is the last rows of a screen, where Claude Code keeps its prompt box and what it says under it.
func footer(screen string) []string {
	rows := strings.Split(strings.TrimRight(screen, "\n \t"), "\n")
	if len(rows) > footerRows {
		rows = rows[len(rows)-footerRows:]
	}
	return rows
}

// enterAfterPaste presses the Enter Claude Code lost of a message herdr's submit just gave it. Any other agent took the
// submit whole.
func enterAfterPaste(client interface {
	Settler
	Prompter
}, pane string) error {
	agent, err := client.AgentGet(pane)
	if err != nil {
		return ErrLeftMeanwhile
	}
	if agent.Agent != claudeAgent {
		return nil
	}
	time.Sleep(SettleFirst)
	for waited := SettleFirst; waited < SettleFor; waited += SettlePoll {
		screen, err := client.PaneRead(pane)
		if err != nil || !stillPasting(screen) {
			break
		}
		time.Sleep(SettlePoll)
	}
	// the Enter goes to the agent or nowhere: a question put up meanwhile would take it as its answer
	agent, err = client.AgentGet(pane)
	if err != nil || agent.Agent != claudeAgent {
		return ErrLeftMeanwhile
	}
	if agent.Status == statusBlocked {
		return ErrAskedMeanwhile
	}
	if atPrompt, err := client.PaneAtPrompt(pane); err != nil || atPrompt {
		return ErrLeftMeanwhile
	}
	screen, err := client.PaneRead(pane)
	if err != nil {
		return ErrNotEntered
	}
	if !stillUnsent(screen) { // sent already, or something else is up: an Enter could only answer that
		return nil
	}
	if agent.Status == herdr.StatusWorking { // a turn can put a question up at any moment, and the Enter would answer it
		return ErrNotEntered
	}
	return client.PaneSendKeys(pane, []string{"enter"})
}
