package gitsync

import (
	"errors"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestAScanKeepsToAFolderOfRepositories(t *testing.T) {
	for _, check := range []struct {
		home, folder string
		depth        int
		want, err    string
	}{
		{"/Users/me", "~/code", 3, "/Users/me/code", ""},
		{"/home/me", "/srv/git/", 1, "/srv/git", ""},
		{`C:\Users\me`, `C:\Users\me\source`, 5, `C:\Users\me\source`, ""},
		{`C:\Users\me`, "~/source", 2, `C:\Users\me\source`, ""},
		{"/Users/me", "code", 3, "", "full path"},
		{"/Users/me", "~", 3, "", "too big"},
		{"/Users/me", "/Users/me/", 3, "", "too big"},
		{"/Users/me", "/Users", 3, "", "too big"},
		{"/Users/me", "/", 3, "", "too big"},
		{"/Users/me", "/home/them", 3, "", "too big"},
		{"/Users/me", "/root", 3, "", "too big"},
		{`C:\Users\me`, `C:\Users`, 3, "", "too big"},
		{`C:\Users\me`, `C:\`, 3, "", "too big"},
		{"/Users/me", "~/code", 0, "", "1 to 5"},
		{"/Users/me", "~/code", 6, "", "1 to 5"},
		{"", "~/code", 3, "", "not known"},
	} {
		got, err := CheckScan(check.home, Scan{Folder: check.folder, Depth: check.depth})
		switch {
		case check.err != "" && (err == nil || !strings.Contains(err.Error(), check.err)):
			t.Errorf("%q in %q: %v, want an error with %q", check.folder, check.home, err, check.err)
		case check.err == "" && (err != nil || got.Folder != check.want || got.Depth != check.depth):
			t.Errorf("%q in %q: %+v %v, want %q", check.folder, check.home, got, err, check.want)
		}
	}
}

func TestTheScansAreKept(t *testing.T) {
	dir := t.TempDir()
	scans := OpenScans(dir)
	if err := scans.Set("gog", Scan{Folder: "/home/me/code", Depth: 2}); err != nil {
		t.Fatal(err)
	}
	if got, ok := OpenScans(dir).Of("gog"); !ok || got.Folder != "/home/me/code" || got.Depth != 2 {
		t.Fatalf("read back: %+v %v", got, ok)
	}
	OpenScans(dir).Forget("gog")
	if _, ok := OpenScans(dir).Of("gog"); ok {
		t.Fatal("a removed machine's scan stayed")
	}
}

func TestAPullWaitsOnlyForAnAgentInThatCheckout(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{
			{WorkspaceID: "w1", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/code/app", RepoRoot: "/code/app"}},
			{WorkspaceID: "w2", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/code/app/.claude/worktrees/x", IsLinkedWorktree: true, RepoRoot: "/code/app"}},
			{WorkspaceID: "w3"},
		},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusIdle, CWD: "/code/app"},
			{PaneID: "w2:p1", WorkspaceID: "w2", Agent: "codex", AgentStatus: herdr.StatusWorking, CWD: "/code/app/.claude/worktrees/x"},
			{PaneID: "w3:p1", WorkspaceID: "w3", Agent: "gemini", AgentStatus: herdr.StatusBlocked, CWD: "/code/web/src"},
		},
	}
	style := rpath.Style{}
	if agent, _ := AgentIn(snap, style, "/code/app", nil); agent != "" {
		t.Fatalf("an agent in a linked worktree held its repository's main folder back: %q", agent)
	}
	if agent, doing := AgentIn(snap, style, "/code/web", nil); agent != "gemini" || doing != DoingAsk {
		t.Fatalf("an agent asking in a folder of the checkout: %q %q", agent, doing)
	}
	if agent, doing := AgentIn(snap, style, "/code/app/.claude/worktrees/x", nil); agent != "codex" || doing != DoingWork {
		t.Fatalf("an agent at work in the worktree: %q %q", agent, doing)
	}
	going := func(dir string) bool { return dir == "/code/app" }
	if agent, doing := AgentIn(snap, style, "/code/app", going); agent != "claude" || doing != DoingEnding {
		t.Fatalf("a turn still ending: %q %q", agent, doing)
	}
	if agent, _ := AgentIn(nil, style, "/code/app", going); agent != "" {
		t.Fatal("no snapshot is no agent")
	}
}

func TestTheListReadsTheConnectedMachinesAndNamesTheAwayOnes(t *testing.T) {
	read := "\x1dw /home/me/app\x1e# branch.head main\x00# branch.upstream origin/main\x00# branch.ab +0 -2\x00\x1e0\x1f\x1forigin\x1f1763000000\n"
	gog := &shell.Fake{Name: "gog", Dir: "/home/me", Remote: true, Answer: func(string) (string, error) { return read, nil }}
	pi := &shell.Fake{Name: "pi", Dir: "/home/pi", Remote: true}
	broken := &shell.Fake{Name: "broken", Dir: "/home/b", Remote: true, Answer: func(string) (string, error) { return "", errors.New("connection lost") }}
	scans := OpenScans(t.TempDir())
	if err := scans.Set("gog", Scan{Folder: "/home/me/code", Depth: 2}); err != nil {
		t.Fatal(err)
	}
	place := &Service{
		Fleet:   func() []Machine { return []Machine{{Host: gog}, {Host: pi, Away: "not answering"}, {Host: broken}} },
		Folders: func(machineID string) []string { return []string{"/home/me/app"} },
		Scans:   scans,
	}
	listed := place.List(false)
	if len(listed) != 3 || len(listed[0].Repos) != 1 || listed[0].Repos[0].Behind != 2 || listed[0].Scan == nil || listed[0].Repos[0].Fetched.Unix() != 1763000000 {
		t.Fatalf("gog: %+v", listed[0])
	}
	if listed[1].Away != "not answering" || len(pi.Scripts()) != 0 || listed[2].Error != "connection lost" {
		t.Fatalf("the others: %+v %+v", listed[1], listed[2])
	}
	if script := gog.Scripts()[0]; !strings.Contains(script, "'/home/me/app'") || !strings.Contains(script, "find -H '/home/me/code' -mindepth 1 -maxdepth 3") {
		t.Fatalf("gog's script: %s", script)
	}
	place.List(false)
	if len(gog.Scripts()) != 1 {
		t.Fatal("a read within FreshFor was not reused")
	}
	place.List(true)
	if len(gog.Scripts()) != 2 {
		t.Fatal("a fresh list did not read again")
	}
}
