package app

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// An agent's pane may stand in a checkout its workspace's folder is not in: a tab whose shell went elsewhere and
// started the agent there. Git's writes on that checkout wait for it as for any other.
func TestAnAgentWhosePaneIsInTheCheckoutHoldsItsWritesBack(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1"}},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", CWD: "/home/me"},
			{PaneID: "w1:p2", WorkspaceID: "w1", CWD: "/src/app/web", Agent: "claude", AgentStatus: herdr.StatusWorking},
		},
	}
	same := func(dir string) string { return dir }
	never := func(string) bool { return false }
	if who := busyCheckout(snap, "/src/app", []string{"/src/app"}, same, never); who == "" {
		t.Error("an agent at work in the checkout, its workspace's folder elsewhere, held nothing back")
	}
	if who := busyCheckout(snap, "/src/other", []string{"/src/other"}, same, never); who != "" {
		t.Errorf("an agent in another checkout held this one back: %q", who)
	}
	snap.Panes[1].AgentStatus = herdr.StatusIdle
	if who := busyCheckout(snap, "/src/app", []string{"/src/app"}, same, func(dir string) bool { return dir == "/home/me" }); who != "" {
		t.Errorf("a turn ending in another folder held the checkout back: %q", who)
	}
}

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
