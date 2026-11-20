package core

import "strings"

// ScreenText is a `terminal.read {format: "vt"}` answer read as herdr's `pane.read` (format ansi) gives a screen: the
// same rows of text and SGR, less the blank rows under the last one that shows anything, which herdr leaves out. The
// screen views keep a screen's bottom rows when room runs out, and the question, mode and progress readers look at its
// last rows: blank rows under the text would push it out of both. A finished terminal's plain text passes the same way.
func ScreenText(vt string) string {
	rows := strings.Split(vt, "\n")
	end := len(rows)
	for end > 0 && blankRow(rows[end-1]) {
		end--
	}
	return strings.Join(rows[:end], "\n")
}

// blankRow says a row shows nothing: nothing is left past its escape sequences. A row of coloured spaces shows its
// colour and is not blank.
func blankRow(row string) bool {
	for i := 0; i < len(row); i++ {
		if row[i] != 0x1b {
			return false
		}
		j := i + 1
		if j < len(row) && row[j] == '[' {
			for j++; j < len(row) && row[j] >= 0x20 && row[j] <= 0x3f; j++ {
			}
		}
		i = j
	}
	return true
}
