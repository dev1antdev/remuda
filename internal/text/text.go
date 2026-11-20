// Package text holds the few string helpers more than one package wants.
package text

import "strings"

// FirstLine is the first line of s, trimmed, cut to max runes with an ellipsis.
func FirstLine(s string, max int) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(line); len(r) > max {
		line = string(r[:max]) + "…"
	}
	return line
}

// Slug is a branch as herdr names its worktree's folder: lower case, a dash for every other run of characters, and
// "worktree" for a branch with no a-z or 0-9 in it (a Cyrillic one), never the folder it goes in itself.
func Slug(branch string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(branch) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	if b.Len() == 0 {
		return "worktree"
	}
	return b.String()
}

// LastLine is git's complaint: the last line it printed that is no hint, else the fallback.
func LastLine(out, fallback string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" && !strings.HasPrefix(line, "hint:") { // git's advice after the refusal it explains
			return line
		}
	}
	return fallback
}
