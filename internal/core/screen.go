package core

import "strings"

// ScreenText is a vt read as herdr's ansi screen, for /api/screen and the timelapse: without the blank rows under the text.
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
