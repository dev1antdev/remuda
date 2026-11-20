package core_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// standInAgent puts a `claude` on the PATH the next core starts with: it reports itself through OSC 7501 as Claude
// Code does, echoes each line it reads and works a moment on it; a line "ask" leaves it asking, as a dialog would.
func standInAgent(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
say() { printf '\033]7501;state=%s:app=claude-code\033\\' "$1"; }
say idle
while IFS= read -r line; do
  printf 'got: %s\n' "$line"
  if [ "$line" = ask ]; then say blocked; continue; fi
  if [ "$line" = leaving ]; then say clear; continue; fi
  say working
  sleep 0.2
  say idle
done
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// screenSays reads the pane's screen until it holds the text.
func screenSays(t *testing.T, adapter *core.Adapter, pane, text string) {
	t.Helper()
	var screen string
	for start := time.Now(); time.Since(start) < 10*time.Second; time.Sleep(100 * time.Millisecond) {
		screen, _ = adapter.PaneRead(pane)
		if strings.Contains(screen, text) {
			return
		}
	}
	t.Fatalf("the screen never said %q:\n%s", text, screen)
}

func tabsOf(snap *herdr.Snapshot, space string) []herdr.TabInfo {
	var tabs []herdr.TabInfo
	for _, tab := range snap.Tabs {
		if tab.WorkspaceID == space {
			tabs = append(tabs, tab)
		}
	}
	return tabs
}

func TestWorkspacesTabsAndPanesAreMadeRenamedAndClosedOnACore(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	folder := t.TempDir()

	made, err := adapter.WorkspaceCreate(folder, "")
	if err != nil {
		t.Fatal(err)
	}
	space, first := made.Workspace.WorkspaceID, made.RootPane.PaneID
	if made.Workspace.Label != filepath.Base(folder) || made.RootPane.WorkspaceID != space {
		t.Fatalf("made %+v", made)
	}
	snap, _ := adapter.SessionSnapshot()
	if ws := snap.Workspace(space); ws == nil || ws.Folder != folder || snap.Pane(first) == nil {
		t.Fatalf("the new workspace is not in the snapshot at once: %+v", snap)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool {
		at, _ := adapter.PaneAtPrompt(first)
		return at
	})

	tab, err := adapter.TabCreate(space, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if tab.Tab.Number != 2 || tab.RootPane.TabID != tab.Tab.TabID {
		t.Fatalf("tab %+v", tab)
	}
	if err := adapter.WorkspaceRename(space, "api"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.TabRename(tab.Tab.TabID, "logs"); err != nil {
		t.Fatal(err)
	}
	beside, err := adapter.PaneSplit(tab.RootPane.PaneID, "right", "")
	if err != nil {
		t.Fatal(err)
	}
	snap = eventually(t, adapter, func(s *herdr.Snapshot) bool {
		p := s.Pane(beside.PaneID)
		return p != nil && p.TabID == tab.Tab.TabID && s.Workspace(space).Label == "api"
	})
	if tabs := tabsOf(snap, space); len(tabs) != 2 || tabs[1].Label != "logs" {
		t.Fatalf("tabs %+v", tabs)
	}
	if beside.CWD != folder {
		t.Fatalf("the split's folder is %q, not the pane's", beside.CWD)
	}

	if err := adapter.PaneClose(beside.PaneID); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool {
		return s.Pane(beside.PaneID) == nil && s.Pane(tab.RootPane.PaneID) != nil
	})
	if err := adapter.TabClose(tab.Tab.TabID); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return len(tabsOf(s, space)) == 1 })
	if err := adapter.WorkspaceClose(space); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Workspace(space) == nil && len(s.Panes) == 0 })
	doc, err := client.DocGet(within(t, 5*time.Second), core.DocName)
	if err != nil || strings.Contains(string(doc.Value), space) {
		t.Fatalf("the document still names the closed workspace: %s %v", doc.Value, err)
	}
	if err := adapter.TabClose("tab-gone"); !herdr.IsCode(err, "tab_not_found") {
		t.Fatalf("closing a tab the machine lacks: %v", err)
	}
}

func TestTheLastPaneOfATabTakesItAndItsWorkspaceAlong(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	made, err := adapter.WorkspaceCreate(t.TempDir(), "solo")
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.PaneClose(made.RootPane.PaneID); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Workspace(made.Workspace.WorkspaceID) == nil })
	doc, _ := client.DocGet(within(t, 5*time.Second), core.DocName)
	if strings.Contains(string(doc.Value), made.Workspace.WorkspaceID) {
		t.Fatalf("the document keeps the empty workspace: %s", doc.Value)
	}
}

func TestTwoRemudasMakingWorkspacesAtOnceKeepEveryOne(t *testing.T) {
	server := coretest.Server(t)
	one := adapterOf(t, server.Client(t), core.AdapterHooks{})
	two := adapterOf(t, server.Client(t), core.AdapterHooks{})
	for _, a := range []*core.Adapter{one, two} {
		eventually(t, a, func(*herdr.Snapshot) bool { return true })
	}
	var made sync.Map
	var wg sync.WaitGroup
	for _, a := range []*core.Adapter{one, two} {
		wg.Go(func() {
			for range 3 {
				ws, err := a.WorkspaceCreate("", "")
				if err != nil {
					t.Error(err)
					return
				}
				made.Store(ws.Workspace.WorkspaceID, true)
			}
		})
	}
	wg.Wait()
	doc, err := server.Client(t).DocGet(within(t, 5*time.Second), core.DocName)
	if err != nil {
		t.Fatal(err)
	}
	made.Range(func(id, _ any) bool {
		if !strings.Contains(string(doc.Value), id.(string)) {
			t.Errorf("workspace %s was lost from the document %s", id, doc.Value)
		}
		return true
	})
}

func TestAnAgentStartsInTheShellAndTakesPromptsAndKeys(t *testing.T) {
	standInAgent(t)
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	pane := made.RootPane.PaneID
	if err := adapter.AgentPrompt(pane, "hello"); !herdr.IsCode(err, "agent_not_found") {
		t.Fatalf("a prompt to a shell: %v", err)
	}
	var started error
	for start := time.Now(); time.Since(start) < 10*time.Second; time.Sleep(200 * time.Millisecond) {
		if started = adapter.AgentStartWith(pane, "claude", "claude", []string{"--model", "it's opus"}, 20*time.Second); !herdr.IsCode(started, "agent_pane_busy") {
			break
		}
	}
	if started != nil {
		t.Fatal(started)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool {
		p := s.Pane(pane)
		return p != nil && p.Agent == "claude" && p.AgentStatus == herdr.StatusIdle
	})
	if err := adapter.AgentStart(pane, "claude"); !herdr.IsCode(err, "agent_pane_busy") {
		t.Fatalf("a second agent in a pane its agent holds: %v", err)
	}

	if err := adapter.AgentPrompt(pane, "hello there"); err != nil {
		t.Fatal(err)
	}
	screenSays(t, adapter, pane, "got: hello there")

	if err := adapter.PaneSendKeys(pane, []string{"x", "enter"}); err != nil {
		t.Fatal(err)
	}
	screenSays(t, adapter, pane, "got: x")
	if err := adapter.PaneSendText(pane, "typed raw"); !herdr.IsCode(err, "agent_pane_busy") {
		t.Fatalf("raw typing into an agent's terminal: %v", err)
	}

	if err := adapter.AgentPrompt(pane, "ask"); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(pane).AgentStatus == herdr.StatusBlocked })
	if err := adapter.AgentPrompt(pane, "an answer typed into its dialog"); !herdr.IsCode(err, "agent_blocked") {
		t.Fatalf("a prompt to an agent asking something: %v", err)
	}
	if err := adapter.PaneSendKeys(pane, []string{"l", "e", "a", "v", "i", "n", "g", "enter"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(pane).AgentStatus == herdr.StatusUnknown })
	if err := adapter.AgentPrompt(pane, "echo typed into the shell"); err == nil {
		t.Fatal("a prompt went to an agent that cleared its status, as one leaving does")
	}
	if err := adapter.PaneSendInput(pane, "two\nlines", nil); err == nil {
		t.Fatal("a paste of several lines typed raw")
	}
	if pid, err := adapter.PaneShellPID(pane); err != nil || pid == 0 {
		t.Fatalf("shell pid %d %v", pid, err)
	}
	if !slices.ContainsFunc(eventually(t, adapter, func(*herdr.Snapshot) bool { return true }).Panes, func(p herdr.PaneInfo) bool { return p.PaneID == pane }) {
		t.Fatal("the pane is gone")
	}
}

func TestRemudaTypesACommandLineIntoAShell(t *testing.T) {
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	pane := made.RootPane.PaneID
	eventually(t, adapter, func(*herdr.Snapshot) bool { at, _ := adapter.PaneAtPrompt(pane); return at })
	if err := adapter.PaneSendText(pane, "echo typed-$((40+2))"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.PaneSendKeys(pane, []string{"enter"}); err != nil {
		t.Fatal(err)
	}
	screenSays(t, adapter, pane, "typed-42")
}

func TestAnAgentThatCannotRunFailsItsStart(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin") // no amp on it
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	made, err := adapter.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	var started error
	for start := time.Now(); time.Since(start) < 10*time.Second; time.Sleep(200 * time.Millisecond) {
		if started = adapter.AgentStart(made.RootPane.PaneID, "amp"); !herdr.IsCode(started, "agent_pane_busy") {
			break
		}
	}
	if started == nil || !strings.Contains(started.Error(), "did not start") {
		t.Fatalf("an agent that is not there: %v", started)
	}
	if err := adapter.AgentStart(made.RootPane.PaneID, "no-such-agent"); !herdr.IsCode(err, "agent_kind_unknown") {
		t.Fatalf("an agent remuda does not know: %v", err)
	}
}
