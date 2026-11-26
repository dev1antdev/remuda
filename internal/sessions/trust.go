package sessions

import "strings"

// TrustPrompt recognises Claude Code's "do you trust this folder" screen for the given folder, nothing else.
// The dialog is boxed and wrapped at the pane's width, so whitespace and box runes are dropped; the path may start with ~.
func TrustPrompt(screen, dir, home string) bool {
	flat := squash(screen)
	if !strings.Contains(flat, "trustthisfolder") || !strings.Contains(flat, "Accessingworkspace") {
		return false
	}
	if dir == "" {
		return true
	}
	want := []string{squash(dir)}
	if home != "" && (dir == home || strings.HasPrefix(dir, home+"/")) {
		want = append(want, squash("~"+strings.TrimPrefix(dir, home)))
	}
	for _, w := range want {
		if strings.Contains(flat, w) {
			return true
		}
	}
	return false
}

// CursorOnYes says whether the trust dialog's cursor already sits on its yes line.
func CursorOnYes(screen string) bool {
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "❯") && strings.Contains(line, "Yes, I trust") {
			return true
		}
	}
	return false
}

// squash drops whitespace and box-drawing runes.
func squash(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' || (r >= 0x2500 && r <= 0x257F) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
