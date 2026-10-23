package chat

import (
	"regexp"
	"strconv"
	"strings"
)

// Question is what an agent asks on its screen while it waits: the dialog's words and the choices it offers, the cursor on one.
type Question struct {
	Text    string   `json:"text"`
	Options []Option `json:"options"`
}

// Option is one choice of a dialog: its number when the dialog shows one, else its place in the list.
type Option struct {
	N      int    `json:"n"`
	Label  string `json:"label"`
	Picked bool   `json:"picked"`
}

const (
	cursor        = "❯"
	borderRunes   = "│╭╮╰╯─┃┏┓┗┛━║╔╗╚╝═┌┐└┘├┤┬┴┼"
	boxTops       = "╭┌┏╔"
	questionLines = 40 // how far above the choices the dialog's words are read
)

var numbered = regexp.MustCompile(`^(\d+)\.\s+(.*)$`)

// screenLine is one row of the screen: its words with the box borders stripped, and where on the row they start.
type screenLine struct {
	text   string
	col    int  // the column of the first rune that is not a border or a space; -1 on a blank row
	boxed  bool // the row sits inside a box: it starts with a border
	boxTop bool // the row is a box's top edge
}

// ParseQuestion reads a dialog off a pane's visible screen: Claude Code's permission prompts, its trust prompt and
// an AskUserQuestion's choices — a cursor (❯) on one line of a list, numbered or aligned inside a box, with the
// dialog's words above it. Anything else is no question: a plain prompt, a shell, a numbered list in an answer.
func ParseQuestion(screen string) (Question, bool) {
	lines := screenLines(screen)
	at := -1
	for i, l := range lines {
		if strings.HasPrefix(l.text, cursor+" ") && strings.TrimSpace(l.text) != cursor {
			at = i // the last cursor with words after it: the dialog sits at the bottom, a bare prompt is no choice
		}
	}
	if at < 0 {
		return Question{}, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(lines[at].text, cursor))
	isNumbered := numbered.MatchString(rest)
	if !isNumbered && !lines[at].boxed {
		return Question{}, false
	}
	left := lines[at].col + 2 // where every other choice starts: under the cursor line's words
	isOption := func(l screenLine) bool {
		if strings.HasPrefix(l.text, cursor+" ") {
			return !isNumbered || numbered.MatchString(strings.TrimSpace(strings.TrimPrefix(l.text, cursor)))
		}
		if l.col != left {
			return false
		}
		return !isNumbered || numbered.MatchString(l.text)
	}
	isFollowOn := func(l screenLine) bool { // a wrapped choice, or the line under it that describes it
		if l.col < 0 || footer(l.text) || isOption(l) {
			return false
		}
		if isNumbered {
			return l.col >= left
		}
		return l.col > left
	}
	top, bottom := at, at
	for i := at - 1; i >= 0; i-- {
		if isOption(lines[i]) {
			top = i
		} else if !isFollowOn(lines[i]) {
			break
		}
	}
	for i := at + 1; i < len(lines); i++ {
		if !isOption(lines[i]) && !isFollowOn(lines[i]) {
			break
		}
		bottom = i
	}
	var options []Option
	for _, l := range lines[top : bottom+1] {
		if isOption(l) {
			options = append(options, option(l.text, len(options)+1, isNumbered))
		} else if len(options) > 0 {
			options[len(options)-1].Label += " " + l.text
		}
	}
	if len(options) < 2 || !consecutive(options) {
		return Question{}, false
	}
	return Question{Text: wordsAbove(lines, top), Options: options}, true
}

// option reads one choice line: the cursor, the number when the list has them, the words.
func option(text string, place int, isNumbered bool) Option {
	o := Option{N: place, Picked: strings.HasPrefix(text, cursor+" ")}
	label := strings.TrimSpace(strings.TrimPrefix(text, cursor))
	if isNumbered {
		m := numbered.FindStringSubmatch(label)
		o.N, _ = strconv.Atoi(m[1])
		label = m[2]
	}
	o.Label = strings.TrimSpace(label)
	return o
}

// consecutive says the choices are numbered 1, 2, 3… and exactly one carries the cursor.
func consecutive(options []Option) bool {
	picked := 0
	for i, o := range options {
		if o.N != i+1 {
			return false
		}
		if o.Picked {
			picked++
		}
	}
	return picked == 1
}

// wordsAbove is the dialog's text over its choices: the rows of the same box, or up to the blank row above an unboxed list.
func wordsAbove(lines []screenLine, top int) string {
	var rows []string
	for i := top - 1; i >= 0 && top-i <= questionLines; i-- {
		l := lines[i]
		if lines[top].boxed {
			if l.boxTop || !l.boxed {
				break
			}
		} else if l.col < 0 {
			break
		}
		rows = append(rows, l.text)
	}
	var out []string
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i] == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, rows[i])
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// footer is a dialog's last row, the keys it explains, never a choice.
func footer(text string) bool {
	return strings.Contains(text, "Enter to") || strings.Contains(text, "Esc to") || strings.HasPrefix(text, "Press ")
}

func screenLines(screen string) []screenLine {
	raw := strings.Split(strings.ReplaceAll(screen, "\r\n", "\n"), "\n")
	out := make([]screenLine, len(raw))
	for i, r := range raw {
		out[i] = screenLine{text: strings.TrimFunc(r, border), col: contentColumn(r)}
		lead := strings.TrimLeft(r, " ")
		out[i].boxed = strings.HasPrefix(lead, "│") || strings.HasPrefix(lead, "┃") || strings.HasPrefix(lead, "║")
		out[i].boxTop = lead != "" && strings.ContainsRune(boxTops, []rune(lead)[0])
	}
	return out
}

func border(r rune) bool { return r == ' ' || r == '\t' || strings.ContainsRune(borderRunes, r) }

func contentColumn(line string) int {
	for i, r := range []rune(line) {
		if !border(r) {
			return i
		}
	}
	return -1
}
