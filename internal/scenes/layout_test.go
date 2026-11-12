package scenes

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// exported is a tab as herdr's layout.export answers it: an editor in the folder, a dev server below it in web/, a
// shell on the right in another folder, the agent in the second pane.
func exported() *herdr.LayoutDescription {
	return &herdr.LayoutDescription{WorkspaceID: "w1", TabID: "w1:t1", Root: herdr.LayoutNode{Type: "split", Direction: "right", Ratio: 0.6,
		First: &herdr.LayoutNode{Type: "split", Direction: "down", Ratio: 0.7,
			First:  &herdr.LayoutNode{Type: "pane", PaneID: "w1:p1", CWD: "/home/me/api", Label: "editor"},
			Second: &herdr.LayoutNode{Type: "pane", PaneID: "w1:p2", CWD: "/home/me/api/web", Command: []string{"npm", "run", "dev"}},
		},
		Second: &herdr.LayoutNode{Type: "pane", PaneID: "w1:p3", CWD: "/tmp", Env: map[string]string{"TOKEN": "secret"}},
	}}
}

func TestACapturedLayoutKeepsItsFoldersRelativeAndFindsTheAgent(t *testing.T) {
	layout := Capture(exported(), "/home/me/api/", "w1:p3")
	panes := layout.Root.panes()
	if len(panes) != 3 || panes[0].Dir != "" || panes[1].Dir != "web" || panes[2].Dir != "/tmp" || panes[0].Label != "editor" || panes[1].Command[2] != "dev" {
		t.Fatalf("the panes: %+v %+v %+v", panes[0], panes[1], panes[2])
	}
	if layout.Agent != 2 {
		t.Fatalf("the agent's pane: %d", layout.Agent)
	}
	if Capture(exported(), "/home/me/api", "w9:p9").Agent != 0 {
		t.Fatal("an agent not in the tab starts in the first pane")
	}
	tree := layout.Tree("/srv/api")
	laid := tree.Panes()
	if laid[0].CWD != "/srv/api" || laid[1].CWD != "/srv/api/web" || laid[2].CWD != "/tmp" || laid[2].Env != nil || laid[0].PaneID != "" {
		t.Fatalf("laid out again under a moved folder: %+v %+v %+v", laid[0], laid[1], laid[2])
	}
	if tree.Direction != "right" || tree.Ratio != 0.6 || tree.First.Direction != "down" {
		t.Fatalf("the splits: %+v", tree)
	}
	if err := checkLayout(layout); err != nil {
		t.Fatal(err)
	}
}

func TestTheAgentsPaneOfALaidOutTab(t *testing.T) {
	laid := exported().Root
	if (Layout{Agent: 1}).AgentPane(&laid) != "w1:p2" || (Layout{Agent: 7}).AgentPane(&laid) != "w1:p1" {
		t.Fatal("the pane at the layout's index, else the first")
	}
}

func TestALayoutHerdrCouldNotLayOutIsRefused(t *testing.T) {
	pane := func(dir string) *Node { return &Node{Type: "pane", Dir: dir} }
	split := func(a, b *Node) *Node {
		return &Node{Type: "split", Direction: "right", Ratio: 0.5, First: a, Second: b}
	}
	wide := pane("")
	for range MaxPanes {
		wide = split(wide, pane(""))
	}
	for name, layout := range map[string]Layout{
		"too many panes":               {Root: *wide},
		"a split going up":             {Root: Node{Type: "split", Direction: "up", Ratio: 0.5, First: pane(""), Second: pane("")}},
		"a split all to one side":      {Root: Node{Type: "split", Direction: "down", Ratio: 1, First: pane(""), Second: pane("")}},
		"a split with one side":        {Root: Node{Type: "split", Direction: "down", Ratio: 0.5, First: pane("")}},
		"a folder that climbs out":     {Root: *split(pane("../secrets"), pane(""))},
		"an unknown node":              {Root: Node{Type: "window"}},
		"an agent pane past them":      {Root: *split(pane(""), pane("")), Agent: 2},
		"an agent pane with a command": {Root: Node{Type: "split", Direction: "down", Ratio: 0.5, First: &Node{Type: "pane", Command: []string{"top"}}, Second: pane("")}},
		"a command with a NUL":         {Root: Node{Type: "pane", Command: []string{"a\x00b"}}},
	} {
		if err := checkLayout(layout); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := checkLayout(Layout{Root: *split(pane("web"), pane("/elsewhere"))}); err != nil {
		t.Fatalf("folders below the step's and absolute ones: %v", err)
	}
}
