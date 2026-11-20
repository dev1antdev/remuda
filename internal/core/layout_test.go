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

// threePanes is a scene's kind of tree: an editor on the left, two shells stacked on the right.
func threePanes(sub string) herdr.LayoutNode {
	return herdr.LayoutNode{Type: "split", Direction: "right", Ratio: 0.6,
		First: &herdr.LayoutNode{Type: "pane", CWD: sub, Label: "editor"},
		Second: &herdr.LayoutNode{Type: "split", Direction: "down", Ratio: 0.5,
			First:  &herdr.LayoutNode{Type: "pane"},
			Second: &herdr.LayoutNode{Type: "pane", Label: "server"},
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
	core.HideRunLiteral(t)
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
		{"a pane with a command, on a core that cannot start it as it is", made.Workspace.WorkspaceID, "", herdr.LayoutNode{Type: "split", Direction: "right", First: &pane,
			Second: &herdr.LayoutNode{Type: "pane", Command: []string{"npm", "run", "dev", "--token=s3cret-sentinel"}}}, "invalid_request"},
		{"a workspace only labelled, not in the document", "ws-labelled", "", pane, "workspace_not_found"},
		{"no such workspace", "ws-none", "", pane, "workspace_not_found"},
		{"no such tab", "", "tab-none", pane, "tab_not_found"},
	} {
		_, err := adapter.LayoutApply(c.space, c.tab, "", c.tree)
		if !herdr.IsCode(err, c.code) {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
		if err != nil && strings.Contains(err.Error(), "s3cret-sentinel") {
			t.Errorf("%s: the refusal repeats a pane's arguments: %v", c.name, err)
		}
	}
	if _, err := adapter.LayoutExport("tab-none"); !herdr.IsCode(err, "tab_not_found") {
		t.Errorf("export of no tab: %v", err)
	}
	if _, err := client.Run(t.Context(), core.RunParams{Argv: []string{"sh"}, Literal: true}); !core.IsKind(err, "needs_update") {
		t.Errorf("a literal run on a core that cannot: %v", err)
	}
	snap, _ := adapter.SessionSnapshot()
	if len(snap.Panes) != 2 || snap.Pane(stray.Address) == nil {
		t.Fatalf("a refused layout left terminals behind: %+v", snap.Panes)
	}
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

// templateNamed puts a template of that name in the core's config, its command saying so if it ever runs.
func templateNamed(t *testing.T, c *coretest.Core, name string) {
	t.Helper()
	var config string
	for _, kv := range c.Env {
		if dir, ok := strings.CutPrefix(kv, "XDG_CONFIG_HOME="); ok {
			config = dir
		}
	}
	dir := filepath.Join(config, "remuda-core", "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte("---\ncommand: sh -c 'echo TEMPLATE-RAN; sleep 60'\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestASceneCommandIsLaidOutAsSavedThoughATemplateHasItsProgramsName(t *testing.T) {
	c := coretest.Server(t)
	client := c.Client(t)
	hello, err := client.Hello(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !hello.Has("run_literal") {
		t.Skipf("remuda-core %s cannot start a program as it is (run_literal)", hello.Version)
	}
	adapter := adapterOf(t, client, core.AdapterHooks{})
	templateNamed(t, c, "sh")
	templateNamed(t, c, "rc-no-such-program")
	var known struct{ Templates []struct{ Name string } }
	if err := client.Call(t.Context(), "templates.list", map[string]any{}, &known); err != nil || len(known.Templates) != 2 {
		t.Fatalf("the core knows the templates %+v (%v)", known, err)
	}
	folder := realPath(t, t.TempDir())
	made, err := adapter.WorkspaceCreate(folder, "")
	if err != nil {
		t.Fatal(err)
	}
	space := made.Workspace.WorkspaceID
	sceneWith := func(command ...string) herdr.LayoutNode {
		scene := scenes.Layout{Root: scenes.Node{Type: "split", Direction: "right", Ratio: 0.5,
			First: &scenes.Node{Type: "pane"}, Second: &scenes.Node{Type: "pane", Label: "server", Command: command}}}
		return scene.Tree(folder)
	}

	command := []string{"sh", "-c", `printf '<%s>' "$@"; sleep 60`, "remuda", "two words", "$HOME", "%PATH%", `"quoted"`}
	tree := sceneWith(command...)
	laid, err := adapter.LayoutApply(space, "", "Dev", tree)
	if err != nil {
		t.Fatal(err)
	}
	server := laid.Root.Panes()[1].PaneID
	screenSays(t, adapter, server, `<two words><$HOME><%PATH%><"quoted">`)
	if ran, err := client.Get(t.Context(), server); err != nil || !slices.Equal(ran.Argv, command) {
		t.Fatalf("the core runs %q, the scene saved %q (%v)", ran.Argv, command, err)
	}
	if exported, err := adapter.LayoutExport(laid.TabID); err != nil || !sameShape(&exported.Root, &tree) {
		t.Fatalf("exported %+v, laid out %+v (%v)", exported, tree, err)
	}

	if _, err := adapter.LayoutApply(space, "", "", sceneWith("rc-no-such-program", "--flag")); !herdr.IsCode(err, "spawn_failed") {
		t.Fatalf("a layout of a missing program: %v", err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return len(s.Panes) == 3 })
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
