package chat

import "strings"

// Claude Code names its permission mode in the footer under the input box — "⏵⏵ accept edits on (shift+tab to cycle)",
// "⏸ plan mode on …", "⏵⏵ auto mode on …", "⏸ manual mode on" for the default, "⏵⏵ bypass permissions on …" — and
// shift+tab cycles it. The words were read off a 2.1.312 pane; a narrow pane cuts the line's tail, never the words.

// modeRows is how many rows up from the screen's bottom the footer is looked for: the input box and a status line sit there.
const modeRows = 12

// modeWords maps the footer's words to the mode the page names.
var modeWords = []struct{ words, mode string }{
	{"bypass permissions on", "bypass"},
	{"accept edits on", "acceptEdits"},
	{"plan mode on", "plan"},
	{"auto mode on", "auto"},
	{"manual mode on", "default"},
}

// ParseMode reads the permission mode off a pane's visible screen: a footer row starting with the mode glyph in the last
// rows, so a quote of the words in the conversation never counts; false when no footer is on screen (a dialog, another agent).
func ParseMode(screen string) (string, bool) {
	rows := strings.Split(strings.ReplaceAll(screen, "\r\n", "\n"), "\n")
	if len(rows) > modeRows {
		rows = rows[len(rows)-modeRows:]
	}
	for i := len(rows) - 1; i >= 0; i-- {
		line := strings.TrimSpace(rows[i])
		if !strings.HasPrefix(line, "⏵") && !strings.HasPrefix(line, "⏸") {
			continue
		}
		for _, m := range modeWords {
			if strings.Contains(line, m.words) {
				return m.mode, true
			}
		}
	}
	return "", false
}
