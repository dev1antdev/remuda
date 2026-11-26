package tasks

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/sessions"
)

func TestTrustPromptMatchesAWrappedScreen(t *testing.T) {
	screen := "╭────────────────────────────────────────╮\n" +
		"│ Accessing workspace:                   │\n" +
		"│                                        │\n" +
		"│ /Users/me/.herdr/worktrees/run │\n" +
		"│ repo/task-test-hello                   │\n" +
		"│                                        │\n" +
		"│ Quick safety check: Is this a project  │\n" +
		"│ ❯ No, exit                             │\n" +
		"│   Yes, I trust this folder             │\n" +
		"╰────────────────────────────────────────╯\n"
	dir := "/Users/me/.herdr/worktrees/runrepo/task-test-hello"
	if !sessions.TrustPrompt(screen, dir, "/Users/me") {
		t.Fatal("a wrapped path was not recognised")
	}
	if sessions.TrustPrompt(screen, "/Users/me/other", "/Users/me") {
		t.Fatal("another folder's prompt was taken")
	}
	tilde := "Accessing workspace:\n~/.herdr/worktrees/runrepo/task-test-hello\n❯ No, exit\n  Yes, I trust this folder\n"
	if !sessions.TrustPrompt(tilde, dir, "/Users/me") {
		t.Fatal("a ~ path was not recognised")
	}
	if sessions.TrustPrompt("Do you want to proceed?\n❯ Yes\n  No", dir, "") {
		t.Fatal("a permission prompt was taken for the trust one")
	}
	if sessions.CursorOnYes(screen) {
		t.Fatal("the cursor is on No")
	}
	if !sessions.CursorOnYes("❯ Yes, I trust this folder\n  No, exit") {
		t.Fatal("the cursor on Yes was missed")
	}
}
