package tasks

// TrustPrompt says whether the screen is Claude Code's "do you trust this folder" question for that very folder:
// a worktree remuda made at the person's word, which the arena answers as a task's start does.
func TrustPrompt(screen, dir, home string) bool { return isTrustPrompt(screen, dir, home) }

// CursorOnYes says whether the trust dialog's cursor already sits on its yes line.
func CursorOnYes(screen string) bool { return cursorOnYes(screen) }
