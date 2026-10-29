package sessions

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// fakePlacer is a herdr client that makes tabs and workspaces on paper.
type fakePlacer struct {
	fakeClient
	tabs       []string // the workspaces tabs were made in
	workspaces []string // the folders workspaces were made on
}

func (f *fakePlacer) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	f.workspaces = append(f.workspaces, cwd)
	return &herdr.WorkspaceCreateResult{Workspace: herdr.WorkspaceInfo{WorkspaceID: "w9"}, RootPane: herdr.PaneInfo{PaneID: "w9:p1", TabID: "w9:t1", WorkspaceID: "w9"}}, nil
}

func (f *fakePlacer) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	f.tabs = append(f.tabs, workspaceID)
	return &herdr.TabCreateResult{Tab: herdr.TabInfo{TabID: "w2:t3"}, RootPane: herdr.PaneInfo{PaneID: "w2:p3", TabID: "w2:t3", WorkspaceID: workspaceID}}, nil
}

func TestStartInUsesTheFoldersWorkspaceElseMakesOne(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w2"}},
		Panes:      []herdr.PaneInfo{{PaneID: "w2:p1", WorkspaceID: "w2", CWD: "/home/me/remuda"}},
	}
	client := &fakePlacer{}
	place, err := StartIn(client, snap, "/home/me/remuda", "scout", "codex", Start{Model: "gpt-5", Args: []string{"--full-auto"}})
	if err != nil {
		t.Fatal(err)
	}
	if place.MadeWorkspace || place.Workspace != "w2" || place.Tab != "w2:t3" || place.Pane != "w2:p3" || len(client.tabs) != 1 || client.tabs[0] != "w2" {
		t.Fatalf("a tab in the folder's workspace: %+v tabs %v", place, client.tabs)
	}
	want := []string{"codex", "codex", "--model", "gpt-5", "--full-auto"}
	if len(client.withArgs) != len(want) {
		t.Fatalf("args %v", client.withArgs)
	}
	for i := range want {
		if client.withArgs[i] != want[i] {
			t.Fatalf("args %v, want %v", client.withArgs, want)
		}
	}

	elsewhere, err := StartIn(client, snap, "/home/me/other", "scout", "codex", Start{})
	if err != nil {
		t.Fatal(err)
	}
	if !elsewhere.MadeWorkspace || elsewhere.Workspace != "w9" || elsewhere.Pane != "w9:p1" || len(client.workspaces) != 1 || client.workspaces[0] != "/home/me/other" {
		t.Fatalf("a workspace on a folder without one: %+v made %v", elsewhere, client.workspaces)
	}
	if _, err := StartIn(client, nil, "/home/me/other", "", "shell", Start{}); err == nil {
		t.Fatal("a shell started")
	}
	if _, err := StartIn(client, nil, "/home/me/other", "", "nobody", Start{}); err == nil {
		t.Fatal("an unknown agent started")
	}
}

func TestStartInKeepsThePlaceWhenTheAgentRefuses(t *testing.T) {
	client := &fakePlacer{fakeClient: fakeClient{startErrs: []*herdr.Error{{Code: "agent_kind_unknown"}}}}
	place, err := StartIn(client, nil, "/x", "", "claude", Start{})
	if err == nil || place.Pane != "w9:p1" || !place.MadeWorkspace {
		t.Fatalf("err %v place %+v", err, place)
	}
}
