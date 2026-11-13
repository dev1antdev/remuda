package worktrees

import "testing"

func TestAWindowsMachinesWorktreesAreKnownInItsSpelling(t *testing.T) {
	home := `C:\Users\me`
	if got := Root(home); got != `C:\Users\me\.herdr\worktrees` {
		t.Fatalf("Root = %q", got)
	}
	for folder, want := range map[string]bool{
		`C:\Users\me\.herdr\worktrees\app\feature-x`:  true,
		`C:\Users\me\.herdr\worktrees\app\feature-x\`: true, // herdr may end a folder with its separator
		`c:/users/me/.herdr/worktrees/app/feature-x`:  true,
		`C:\Users\me\.herdr\worktrees\app`:            false,
		`C:\Users\me\app`:                             false,
	} {
		if got := Made(home, folder); got != want {
			t.Errorf("Made(%q) = %v, want %v", folder, got, want)
		}
	}
	answer := "C:/Users/me/.herdr/worktrees/app/feature-x\x1ffeature-x\x1f\x1f120\r\n" +
		"C:/Users/me/.herdr/worktrees/app/open-one\x1fopen-one\x1f M a.go\x1f8\n"
	open := []string{`C:\Users\me\.herdr\worktrees\app\open-one\src\`}
	left := ParseLeftovers(Root(home), answer, open)
	if len(left) != 1 || left[0].Path != `C:\Users\me\.herdr\worktrees\app\feature-x` || left[0].Repo != "app" || left[0].Name != "feature-x" || left[0].KB != 120 {
		t.Fatalf("the leftovers of a Windows machine: %+v", left)
	}
}
