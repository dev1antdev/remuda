package sessions

import (
	"strings"
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

// fakeCloser notes what was closed.
type fakeCloser struct{ closed []string }

func (f *fakeCloser) WorkspaceClose(id string) error {
	f.closed = append(f.closed, "workspace "+id)
	return nil
}

func (f *fakeCloser) TabClose(id string) error {
	f.closed = append(f.closed, "tab "+id)
	return nil
}

// A scout's place is taken down once it answered: never a tab the person added to the workspace made for it meanwhile,
// nor a tab that no longer holds the scout's pane.
func TestClosePlaceTakesDownOnlyWhatTheAgentHolds(t *testing.T) {
	made := Place{Pane: "w9:p1", Tab: "w9:t1", Workspace: "w9", MadeWorkspace: true}
	alone := &herdr.Snapshot{
		Tabs:  []herdr.TabInfo{{TabID: "w9:t1", WorkspaceID: "w9"}},
		Panes: []herdr.PaneInfo{{PaneID: "w9:p1", TabID: "w9:t1", WorkspaceID: "w9"}},
	}
	withTheirs := &herdr.Snapshot{
		Tabs:  []herdr.TabInfo{{TabID: "w9:t1", WorkspaceID: "w9"}, {TabID: "w9:t2", WorkspaceID: "w9"}},
		Panes: []herdr.PaneInfo{{PaneID: "w9:p1", TabID: "w9:t1", WorkspaceID: "w9"}, {PaneID: "w9:p2", TabID: "w9:t2", WorkspaceID: "w9"}},
	}
	moved := &herdr.Snapshot{
		Tabs:  []herdr.TabInfo{{TabID: "w9:t1", WorkspaceID: "w9"}, {TabID: "w9:t3", WorkspaceID: "w9"}},
		Panes: []herdr.PaneInfo{{PaneID: "w9:p1", TabID: "w9:t3", WorkspaceID: "w9"}},
	}
	cases := []struct {
		name  string
		snap  *herdr.Snapshot
		place Place
		want  string
	}{
		{"the workspace made for it, holding it alone", alone, made, "workspace w9"},
		{"the workspace made for it, with the person's tab beside", withTheirs, made, "tab w9:t1"},
		{"a tab of the person's workspace", withTheirs, Place{Pane: "w9:p1", Tab: "w9:t1", Workspace: "w9"}, "tab w9:t1"},
		{"the pane gone already", &herdr.Snapshot{}, made, ""},
		{"no snapshot yet", nil, made, "workspace w9"},
	}
	for _, c := range cases {
		closer := &fakeCloser{}
		if err := closePlace(closer, c.snap, c.place); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if got := strings.Join(closer.closed, ", "); got != c.want {
			t.Errorf("%s: closed %q, want %q", c.name, got, c.want)
		}
	}
	closer := &fakeCloser{}
	if err := closePlace(closer, moved, made); err == nil || len(closer.closed) != 0 {
		t.Errorf("a tab that no longer holds the pane was closed: %v %v", closer.closed, err)
	}
}
