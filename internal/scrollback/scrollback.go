// Package scrollback is the find over a pane's scrollback: herdr keeps the rows (the attach stream repaints the
// visible screen, so the browser's terminal never holds them), the server finds the query in the rows it reads,
// and the page scrolls the pane to a match and marks it on the screen.
package scrollback

import (
	"strings"
	"unicode/utf8"
)

// MaxMatches bounds one find: past it the count reads as the bound and the rest is left unfound.
const MaxMatches = 2000

// Match is one occurrence: the row counted from the top of the rows read, the column in characters, the length.
type Match struct {
	Row int `json:"row"`
	Col int `json:"col"`
	Len int `json:"len"`
}

// Rows splits what herdr read into rows, the trailing newline not a row of its own.
func Rows(text string) []string {
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// Find lists where query occurs in the rows, case-folded, left to right and never overlapping, at most MaxMatches.
func Find(rows []string, query string) []Match {
	if query == "" {
		return nil
	}
	needle := strings.ToLower(query)
	var out []Match
	for row, line := range rows {
		hay := strings.ToLower(line)
		if len(hay) != len(line) {
			// folding moved the offsets (İ, ß): compare rune by rune on the line as it is
			out = append(out, findRunes(row, line, query, needle, MaxMatches-len(out))...)
		} else {
			from := 0
			for {
				at := strings.Index(hay[from:], needle)
				if at < 0 {
					break
				}
				at += from
				out = append(out, Match{Row: row, Col: utf8.RuneCountInString(line[:at]), Len: utf8.RuneCountInString(query)})
				if len(out) >= MaxMatches {
					return out
				}
				from = at + len(needle)
			}
		}
		if len(out) >= MaxMatches {
			return out
		}
	}
	return out
}

func findRunes(row int, line, query, needle string, room int) []Match {
	runes := []rune(line)
	n := utf8.RuneCountInString(query)
	var out []Match
	for at := 0; at+n <= len(runes) && len(out) < room; at++ {
		if strings.ToLower(string(runes[at:at+n])) != needle {
			continue
		}
		out = append(out, Match{Row: row, Col: at, Len: n})
		at += n - 1
	}
	return out
}
