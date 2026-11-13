package herdr_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
)

// TestLayoutsOnAThrowawayHerdr is layout.export, layout.apply, pane.process_info and pane.close as herdr 0.9.5 speaks
// them: a tab's tree read back, laid out again as a new tab and in place of one, and a pane closed with its tab.
func TestLayoutsOnAThrowawayHerdr(t *testing.T) {
	sock := herdrtest.Server(t)
	client := herdr.NewClient(sock)
	dir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	made, err := client.WorkspaceCreate(dir, "layouts")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	workspace, tab := made.Workspace.WorkspaceID, made.RootPane.TabID

	tree := herdr.LayoutNode{Type: "split", Direction: "right", Ratio: 0.6,
		First:  &herdr.LayoutNode{Type: "pane", CWD: dir, Label: "editor"},
		Second: &herdr.LayoutNode{Type: "pane", CWD: dir},
	}
	applied, err := client.LayoutApply(workspace, "", "dev", tree)
	if err != nil {
		t.Fatal(err)
	}
	panes := applied.Root.Panes()
	if applied.WorkspaceID != workspace || applied.TabID == tab || len(panes) != 2 || panes[0].Label != "editor" || panes[0].PaneID == "" {
		t.Fatalf("a new tab of the workspace with the two panes: %+v", applied)
	}
	exported, err := client.LayoutExport(applied.TabID)
	if err != nil {
		t.Fatal(err)
	}
	if exported.Root.Type != "split" || exported.Root.Direction != "right" || exported.Root.Ratio < 0.59 || exported.Root.Ratio > 0.61 || exported.Root.First.CWD != dir {
		t.Fatalf("the tree read back: %+v", exported.Root)
	}

	replaced, err := client.LayoutApply("", tab, "main", herdr.LayoutNode{Type: "split", Direction: "down", Ratio: 0.5,
		First: &herdr.LayoutNode{Type: "pane", CWD: dir}, Second: &herdr.LayoutNode{Type: "pane", CWD: dir}})
	if err != nil {
		t.Fatal(err)
	}
	if len(replaced.Root.Panes()) != 2 || replaced.WorkspaceID != workspace {
		t.Fatalf("the workspace's first tab laid out again: %+v", replaced)
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Pane(made.RootPane.PaneID) != nil {
		t.Fatal("a tab laid out in place lets its old panes go")
	}

	pane := replaced.Root.Panes()[0].PaneID
	if at, err := client.PaneAtPrompt(pane); err != nil || !at {
		t.Fatalf("a fresh shell is at its prompt: %v %v", at, err)
	}
	if pid, err := client.PaneShellPID(pane); err != nil || pid <= 1 {
		t.Fatalf("herdr names the pane's shell: %d %v", pid, err)
	}
	if err := client.PaneClose(pane); err != nil {
		t.Fatal(err)
	}
	if snap, err = client.SessionSnapshot(); err != nil || snap.Pane(pane) != nil {
		t.Fatalf("the pane is closed: %v", err)
	}
}
