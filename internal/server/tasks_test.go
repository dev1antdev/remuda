package server

import "testing"

func TestTrustPromptMatchesAWrappedScreen(t *testing.T) {
	screen := "╭────────────────────────────────────────╮\n" +
		"│ Accessing workspace:                   │\n" +
		"│                                        │\n" +
		"│ /Users/dev1antdev/.herdr/worktrees/run │\n" +
		"│ repo/task-test-hello                   │\n" +
		"│                                        │\n" +
		"│ Quick safety check: Is this a project  │\n" +
		"│ ❯ No, exit                             │\n" +
		"│   Yes, I trust this folder             │\n" +
		"╰────────────────────────────────────────╯\n"
	dir := "/Users/dev1antdev/.herdr/worktrees/runrepo/task-test-hello"
	if !isTrustPrompt(screen, dir, "/Users/dev1antdev") {
		t.Fatal("a wrapped path was not recognised")
	}
	if isTrustPrompt(screen, "/Users/dev1antdev/other", "/Users/dev1antdev") {
		t.Fatal("another folder's prompt was taken")
	}
	tilde := "Accessing workspace:\n~/.herdr/worktrees/runrepo/task-test-hello\n❯ No, exit\n  Yes, I trust this folder\n"
	if !isTrustPrompt(tilde, dir, "/Users/dev1antdev") {
		t.Fatal("a ~ path was not recognised")
	}
	if isTrustPrompt("Do you want to proceed?\n❯ Yes\n  No", dir, "") {
		t.Fatal("a permission prompt was taken for the trust one")
	}
	if cursorOnYes(screen) {
		t.Fatal("the cursor is on No")
	}
	if !cursorOnYes("❯ Yes, I trust this folder\n  No, exit") {
		t.Fatal("the cursor on Yes was missed")
	}
}
