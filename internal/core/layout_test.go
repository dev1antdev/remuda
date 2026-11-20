package core_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/scenes"
)

// threePanes is a scene's kind of tree: an editor on the left, a shell and a running command stacked on the right.
func threePanes(sub string) herdr.LayoutNode {
	return herdr.LayoutNode{Type: "split", Direction: "right", Ratio: 0.6,
		First: &herdr.LayoutNode{Type: "pane", CWD: sub, Label: "editor"},
		Second: &herdr.LayoutNode{Type: "split", Direction: "down", Ratio: 0.5,
			First:  &herdr.LayoutNode{Type: "pane"},
			Second: &herdr.LayoutNode{Type: "pane", Command: []string{"sh", "-c", "echo laid; sleep 300"}},
		},
	}
}

// sameShape says whether two trees split the same way with the same labels and commands, whatever their pane ids.
func sameShape(a, b *herdr.LayoutNode) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type != b.Type || a.Label != b.Label || !slices.Equal(a.Command, b.Command) {
		return false
	}
	if a.Type != "split" {
		return true
	}
	return a.Direction == b.Direction && a.Ratio == b.Ratio && sameShape(a.First, b.First) && sameShape(a.Second, b.Second)
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestALayoutIsLaidOutAsANewTabAndExportedAsItWasGiven(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	folder := t.TempDir()
	sub := filepath.Join(folder, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	made, err := adapter.WorkspaceCreate(folder, "")
	if err != nil {
		t.Fatal(err)
	}
	space, first := made.Workspace.WorkspaceID, made.Workspace.ActiveTabID

	tree := threePanes(sub)
	laid, err := adapter.LayoutApply(space, "", "Dev", tree)
	if err != nil {
		t.Fatal(err)
	}
	panes := laid.Root.Panes()
	if laid.WorkspaceID != space || laid.TabID == first || len(panes) != 3 || laid.FocusedPaneID != panes[0].PaneID || !sameShape(&laid.Root, &tree) {
		t.Fatalf("laid %+v", laid)
	}
	snap := eventually(t, adapter, func(s *herdr.Snapshot) bool {
		tab := s.Tab(laid.TabID)
		return tab != nil && tab.Label == "Dev" && len(panesOfTab(s, laid.TabID)) == 3
	})
	if ws := snap.Workspace(space); ws.ActiveTabID != first {
		t.Fatalf("the laid-out tab took the front: %+v", ws)
	}
	screenSays(t, adapter, panes[2].PaneID, "laid")

	exported, err := adapter.LayoutExport(laid.TabID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameShape(&exported.Root, &tree) || exported.TabID != laid.TabID || exported.WorkspaceID != space {
		t.Fatalf("exported %+v, laid out %+v", exported, tree)
	}
	got := exported.Root.Panes()
	if realPath(t, got[0].CWD) != realPath(t, sub) || realPath(t, got[1].CWD) != realPath(t, folder) {
		t.Fatalf("the panes' folders are %q and %q", got[0].CWD, got[1].CWD)
	}
}

func TestALayoutLaidInPlaceOfATabEndsItsOldPanes(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	tab, old := made.Workspace.ActiveTabID, made.RootPane.PaneID
	tree := herdr.LayoutNode{Type: "split", Direction: "down", Ratio: 0.3, First: &herdr.LayoutNode{Type: "pane"}, Second: &herdr.LayoutNode{Type: "pane"}}
	laid, err := adapter.LayoutApply("", tab, "Two", tree)
	if err != nil {
		t.Fatal(err)
	}
	if laid.TabID != tab || len(laid.Root.Panes()) != 2 {
		t.Fatalf("laid %+v", laid)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool {
		shown := s.Tab(tab)
		return shown != nil && shown.Label == "Two" && s.Pane(old) == nil && len(panesOfTab(s, tab)) == 2
	})
	exported, err := adapter.LayoutExport(tab)
	if err != nil || !sameShape(&exported.Root, &tree) {
		t.Fatalf("exported %+v, %v", exported, err)
	}
}

func TestAPaneTheTreeLacksIsExportedBesideItAndAnEndedOneIsLeftOut(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	space, tab := made.Workspace.WorkspaceID, made.Workspace.ActiveTabID
	laid, err := adapter.LayoutApply("", tab, "", herdr.LayoutNode{Type: "split", Direction: "down", Ratio: 0.5, First: &herdr.LayoutNode{Type: "pane"}, Second: &herdr.LayoutNode{Type: "pane"}})
	if err != nil {
		t.Fatal(err)
	}
	stray, err := client.Run(t.Context(), core.RunParams{Labels: map[string]string{core.LabelSpace: space, core.LabelTab: tab}, Rm: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.PaneClose(laid.Root.Panes()[0].PaneID); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool {
		return s.Pane(stray.Address) != nil && len(panesOfTab(s, tab)) == 2
	})
	exported, err := adapter.LayoutExport(tab)
	if err != nil {
		t.Fatal(err)
	}
	want := herdr.LayoutNode{Type: "split", Direction: "right", Ratio: 0.5, First: &herdr.LayoutNode{Type: "pane"}, Second: &herdr.LayoutNode{Type: "pane"}}
	if got := exported.Root.Panes(); !sameShape(&exported.Root, &want) || got[0].PaneID != laid.Root.Panes()[1].PaneID || got[1].PaneID != stray.Address {
		t.Fatalf("exported %+v", exported.Root)
	}
}

func TestALayoutTheCoreCannotLayOutIsRefused(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	pane := herdr.LayoutNode{Type: "pane"}
	stray, err := client.Run(t.Context(), core.RunParams{Labels: map[string]string{core.LabelSpace: "ws-labelled", core.LabelTab: "tab-labelled"}, Rm: true})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Workspace("ws-labelled") != nil })
	for _, c := range []struct {
		name, space, tab string
		tree             herdr.LayoutNode
		code             string
	}{
		{"a split of nothing", made.Workspace.WorkspaceID, "", herdr.LayoutNode{Type: "split", Direction: "right"}, "invalid_request"},
		{"a workspace only labelled, not in the document", "ws-labelled", "", pane, "workspace_not_found"},
		{"no such workspace", "ws-none", "", pane, "workspace_not_found"},
		{"no such tab", "", "tab-none", pane, "tab_not_found"},
	} {
		if _, err := adapter.LayoutApply(c.space, c.tab, "", c.tree); !herdr.IsCode(err, c.code) {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if _, err := adapter.LayoutExport("tab-none"); !herdr.IsCode(err, "tab_not_found") {
		t.Errorf("export of no tab: %v", err)
	}
	snap, _ := adapter.SessionSnapshot()
	if len(snap.Panes) != 2 || snap.Pane(stray.Address) == nil {
		t.Fatalf("a refused layout left terminals behind: %+v", snap.Panes)
	}
}

func TestALayoutWhosePaneCommandCannotStartFailsAndLeavesNothing(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	space := made.Workspace.WorkspaceID
	tree := herdr.LayoutNode{Type: "split", Direction: "right", Ratio: 0.5, First: &herdr.LayoutNode{Type: "pane"},
		Second: &herdr.LayoutNode{Type: "pane", Command: []string{"no-such-program-anywhere", "--dev"}}}
	if _, err := adapter.LayoutApply(space, "", "", tree); err == nil {
		t.Fatal("a layout whose command is missing was laid out")
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return len(s.Panes) == 1 && len(tabsOf(s, space)) == 1 })
}

func TestASceneCapturedOnACoreLaysItsLayoutOutInAnotherFolder(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	from, to := realPath(t, t.TempDir()), realPath(t, t.TempDir())
	for _, dir := range []string{from, to} {
		if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	made, err := adapter.WorkspaceCreate(from, "")
	if err != nil {
		t.Fatal(err)
	}
	laid, err := adapter.LayoutApply("", made.Workspace.ActiveTabID, "", threePanes(filepath.Join(from, "sub")))
	if err != nil {
		t.Fatal(err)
	}
	exported, err := adapter.LayoutExport(laid.TabID)
	if err != nil {
		t.Fatal(err)
	}
	kept := scenes.Capture(exported, from, laid.Root.Panes()[1].PaneID)
	if kept.Agent != 1 || kept.Root.First.Dir != "sub" || kept.Root.Second.First.Dir != "" {
		t.Fatalf("captured %+v", kept)
	}

	there, err := adapter.WorkspaceCreate(to, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := adapter.LayoutApply("", there.Workspace.ActiveTabID, "Again", kept.Tree(to))
	if err != nil {
		t.Fatal(err)
	}
	agent := kept.AgentPane(&again.Root)
	snap := eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(agent) != nil })
	if pane := snap.Pane(agent); pane.TabID != again.TabID || realPath(t, again.Root.Panes()[0].CWD) != filepath.Join(to, "sub") {
		t.Fatalf("the agent's pane %+v, the tree %+v", pane, again.Root)
	}
}

func panesOfTab(snap *herdr.Snapshot, tab string) []string {
	var out []string
	for _, p := range snap.Panes {
		if p.TabID == tab {
			out = append(out, p.PaneID)
		}
	}
	return out
}

func TestALayoutWhoseWorkspaceClosedWhileItStartedDoesNotBringItBack(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	space := made.Workspace.WorkspaceID
	core.BeforeLayoutWrite(t, func() {
		if err := adapter.WorkspaceClose(space); err != nil {
			t.Error(err)
		}
	})
	if _, err := adapter.LayoutApply(space, "", "", threePanes("")); err == nil {
		t.Fatal("a layout of a workspace closed meanwhile was laid out")
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Workspace(space) == nil && len(s.Panes) == 0 })
	if left, err := client.List(t.Context(), false, map[string]string{core.LabelSpace: space}); err != nil || len(left) > 0 {
		t.Fatalf("terminals of the closed workspace still run: %+v, %v", left, err)
	}
}

func TestALayoutLaidInPlaceOfATabClosedMeanwhileFails(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := adapter.TabCreate(made.Workspace.WorkspaceID, "")
	if err != nil {
		t.Fatal(err)
	}
	core.BeforeLayoutWrite(t, func() {
		if err := adapter.TabClose(other.Tab.TabID); err != nil {
			t.Error(err)
		}
	})
	if _, err := adapter.LayoutApply("", other.Tab.TabID, "", threePanes("")); err == nil {
		t.Fatal("a tab closed meanwhile was laid out again")
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Tab(other.Tab.TabID) == nil && len(s.Panes) == 1 })
}

func TestAPaneWhoseProgramSharesATemplatesNameRunsTheProgram(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "sleeper"), []byte("#!/bin/sh\necho the program ran \"$@\"\nsleep 300\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	server := coretest.Server(t)
	client := server.Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	config := ""
	for _, kv := range server.Env {
		if value, ok := strings.CutPrefix(kv, "XDG_CONFIG_HOME="); ok {
			config = value
		}
	}
	templates := filepath.Join(config, "remuda-core", "templates")
	if err := os.MkdirAll(templates, 0o755); err != nil {
		t.Fatal(err)
	}
	template := "---\ndescription: a stand-in\ncommand: echo the template ran; sleep 300\n---\n"
	if err := os.WriteFile(filepath.Join(templates, "sleeper.md"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	laid, err := adapter.LayoutApply(made.Workspace.WorkspaceID, "", "", herdr.LayoutNode{Type: "pane", Command: []string{"sleeper", "run", "dev"}})
	if err != nil {
		t.Fatal(err)
	}
	screenSays(t, adapter, laid.Root.PaneID, "the program ran run dev")
}

func TestAWorkspaceClosedWhileALayoutIsWrittenEndsTheLayoutsPanesToo(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	space := made.Workspace.WorkspaceID
	core.BeforeCloseWrite(t, func() {
		if _, err := adapter.LayoutApply(space, "", "", threePanes("")); err != nil {
			t.Error(err)
		}
	})
	if err := adapter.WorkspaceClose(space); err != nil {
		t.Fatal(err)
	}
	if left, err := client.List(t.Context(), false, map[string]string{core.LabelSpace: space}); err != nil || len(left) > 0 {
		t.Fatalf("the layout's panes outlived the close: %+v, %v", left, err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Workspace(space) == nil && len(s.Panes) == 0 })
}
