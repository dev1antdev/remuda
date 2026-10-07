// Package text holds the few string helpers more than one package wants.
package text

import "strings"

// FirstLine is the first line of s, trimmed, cut to max runes with an
// ellipsis — a label made from a prompt or a comment.
func FirstLine(s string, max int) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(line); len(r) > max {
		line = string(r[:max]) + "…"
	}
	return line
}
