package core

import "strings"

// ScreenText is a `terminal.read {format: "vt"}` answer read as herdr's `pane.read` (format ansi) gives a screen: the
// same rows of text and SGR, less the blank rows under the last one that shows anything, which herdr leaves out. The
// screen views keep a screen's bottom rows when room runs out, and the question, mode and progress readers look at its
// last rows: blank rows under the text would push it out of both. A finished terminal's plain text passes the same way.
func ScreenText(vt string) string {
	rows := strings.Split(vt, "\n")
	end := len(rows)
	for end > 0 && (rows[end-1] == "" || rows[end-1] == blankRow) {
		end--
	}
	return strings.Join(rows[:end], "\n")
}

// blankRow is a vt row that shows nothing: every row opens with a style reset, and a blank one is the reset alone (a
// row of coloured spaces carries its colour and shows it).
const blankRow = "\x1b[0m"
