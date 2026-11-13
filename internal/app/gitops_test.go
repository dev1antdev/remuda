package app

import "testing"

func TestAnAgentInAWorktreeNestedInTheCheckoutIsNotInTheCheckout(t *testing.T) {
	worktrees := []string{"/src/app", "/src/app/.claude/worktrees/agent-1", "/home/me/.herdr/worktrees/app/feature"}
	cases := map[string]bool{
		"/src/app":                               true,
		"/src/app/web":                           true,
		"/src/app/.claude/worktrees/agent-1":     false,
		"/src/app/.claude/worktrees/agent-1/web": false,
		"/home/me/.herdr/worktrees/app/feature":  false,
		"/src/application":                       false,
		"":                                       false,
	}
	for dir, want := range cases {
		if got := checkoutOf(dir, "/src/app", worktrees); got != want {
			t.Errorf("%q in the checkout: %v", dir, got)
		}
	}
	if !checkoutOf(`C:\Users\me\app\src`, "C:/Users/me/app", []string{"C:/Users/me/app"}) {
		t.Error("a Windows folder under its checkout's top, spelt with backslashes, is not in it")
	}
}
