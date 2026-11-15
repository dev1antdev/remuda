package sessions

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// A message that names a picture, sent to Claude Code in two steps.
//
// herdr's own submit writes the text as a paste and Enter right after it. Claude Code swallows an Enter that arrives
// while it takes a paste in, and presses it again itself once the paste is in — unless the paste names a picture
// (a composer's attachment, "@.remuda/uploads/shot.png"): then it reads the file first, and the Enter is gone for
// good. The message sits in the prompt box as "[Pasted text]", unsent (read off Claude Code 2.1.325's paste hook).
// So such a message is pasted alone, the paste is waited out — the footer says "Pasting…" while it lasts — and Enter
// is pressed after it.

// Settler is what the two steps need of herdr's client beyond a Prompter's part.
type Settler interface {
	AgentGet(target string) (herdr.AgentInfo, error)
	PaneSendInput(paneID, text string, keys []string) error
	PaneRead(paneID string) (string, error)
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

const (
	claudeAgent   = "claude"
	pastingWords  = "Pasting"
	footerRows    = 6
	statusBlocked = "blocked"
)

// pictureRe is a file name Claude Code may take for a picture in a paste.
var pictureRe = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp|bmp|tiff?|heic|heif|avif)\b`)

// NamesPicture says the text names a picture file, the paste Claude Code takes in at its own pace.
func NamesPicture(text string) bool {
	return pictureRe.MatchString(text)
}

// stillPasting reads Claude Code's footer off a screen: true while it says the paste is being taken in.
func stillPasting(screen string) bool {
	rows := strings.Split(strings.TrimRight(screen, "\n \t"), "\n")
	if len(rows) > footerRows {
		rows = rows[len(rows)-footerRows:]
	}
	for _, row := range rows {
		if strings.Contains(row, pastingWords) {
			return true
		}
	}
	return false
}

// pasteThenEnter delivers a message to a Claude Code pane in two steps. handled is false when the pane is not one —
// another agent, a shell, an agent herdr cannot name — and nothing was sent: herdr's own submit takes it then.
func pasteThenEnter(client interface {
	Settler
	Prompter
}, pane, text string) (handled bool, err error) {
	agent, err := client.AgentGet(pane)
	if err != nil || agent.Agent != claudeAgent {
		return false, nil
	}
	if agent.Status == statusBlocked {
		return true, ErrAgentBusy
	}
	if err := client.PaneSendInput(pane, text, nil); err != nil {
		return true, err
	}
	time.Sleep(SettleFirst)
	for waited := SettleFirst; waited < SettleFor; waited += SettlePoll {
		screen, err := client.PaneRead(pane)
		if err != nil || !stillPasting(screen) {
			break
		}
		time.Sleep(SettlePoll)
	}
	// a question put up meanwhile must not get the Enter: it would answer it
	if agent, err := client.AgentGet(pane); err == nil && agent.Status == statusBlocked {
		return true, ErrAskedMeanwhile
	}
	return true, client.PaneSendKeys(pane, []string{"enter"})
}
