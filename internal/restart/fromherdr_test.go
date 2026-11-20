package restart

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

const otherSid = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"

// onCore is the memory of the fleet once it moved to remuda-core.
func onCore(m *Memory) *Memory {
	m.Core = func() bool { return true }
	return m
}

// TestHerdrsChatsCarryOnOnTheCore: the chats remembered in herdr, their panes gone with it, wait in the stopped list
// from herdr — not the closed one — once, through restarts of remuda, until an agent of the core is on their session.
func TestHerdrsChatsCarryOnOnTheCore(t *testing.T) {
	noGrace(t)
	dir := t.TempDir()
	inHerdr := OpenMemory(dir)
	codex := agentPane("w1:p2", "codex", "/work/api")
	codex.AgentSession = &herdr.AgentSession{Kind: "id", Value: otherSid}
	observe(inHerdr, "gog", true, agentPane("w1:p1", "claude", "/work/api"), codex)

	gainedAny := false
	m := onCore(OpenMemory(dir))
	m.OnStopped = func(string) { gainedAny = true }
	observe(m, "gog", true, shellPane("t1", "/home/me"))
	stopped := m.Stopped("gog")
	if len(stopped) != 2 || stopped[0].Pane != "w1:p1" || stopped[0].From != FromHerdr || stopped[1].Agent != "codex" || stopped[1].Title != "api" || stopped[1].Command != "codex resume "+otherSid {
		t.Fatalf("herdr's chats on the core: %+v", stopped)
	}
	if len(m.Closed()) != 0 {
		t.Fatalf("herdr's chats are no closed agents: %+v", m.Closed())
	}
	if gainedAny {
		t.Fatal("nothing resumes herdr's agents by itself")
	}
	observe(m, "gog", false, shellPane("t1", "/home/me"))
	if n := len(m.Stopped("gog")); n != 2 {
		t.Fatalf("listed once while they wait: %d", n)
	}

	again := onCore(OpenMemory(dir))
	observe(again, "gog", true, shellPane("t1", "/home/me"))
	observe(again, "gog", false, shellPane("t1", "/home/me"))
	if n := len(again.Stopped("gog")); n != 2 || len(again.Closed()) != 0 {
		t.Fatalf("after remuda's own restart, the same two and nothing closed: %+v %+v", again.Stopped("gog"), again.Closed())
	}

	// Claude Code carries on in a pane of the core, by hand: its herdr chat is done with, the core's remembered as such
	core := agentPane("t2", "claude", "/work/api")
	observe(again, "gog", false, shellPane("t1", "/home/me"), core)
	stopped = again.Stopped("gog")
	if len(stopped) != 1 || stopped[0].Pane != "w1:p2" {
		t.Fatalf("the one an agent of the core is on leaves the list: %+v", stopped)
	}
	if c := again.chats["gog|t2"]; !c.Core || c.Session != sid {
		t.Fatalf("a chat remembered off the core is marked so: %+v", c)
	}
	if _, ok := again.chats["gog|w1:p1"]; ok {
		t.Fatal("herdr's chat whose session the core runs is forgotten")
	}
}

// TestOnHerdrTheMemoryIsAsItWas: before the move, a pane gone is a closed agent as ever.
func TestOnHerdrTheMemoryIsAsItWas(t *testing.T) {
	m := OpenMemory("")
	m.Core = func() bool { return false }
	observe(m, "gog", true, agentPane("w1:p1", "claude", "/work/api"))
	observe(m, "gog", false)
	if len(m.Stopped("gog")) != 0 || len(m.Closed()) != 1 {
		t.Fatalf("closed, not from herdr: %+v %+v", m.Stopped("gog"), m.Closed())
	}
}

// TestAdoptListsHerdrsSleepersAtOnce: a workspace that slept in herdr hands its agents over; they are listed from
// herdr before the next snapshot, once however often they are handed over.
func TestAdoptListsHerdrsSleepersAtOnce(t *testing.T) {
	m := onCore(OpenMemory(""))
	sleepers := map[string]Chat{"w4:p1": {Kind: "claude", Session: sid, Dir: "/work/web", Title: "web"}}
	m.Adopt("gog", sleepers)
	m.Adopt("gog", sleepers)
	stopped := m.Stopped("gog")
	if len(stopped) != 1 || stopped[0].From != FromHerdr || stopped[0].Title != "web" || stopped[0].Dir != "/work/web" {
		t.Fatalf("adopted: %+v", stopped)
	}
	observe(m, "gog", false, shellPane("t1", "/home/me"))
	if n := len(m.Stopped("gog")); n != 1 {
		t.Fatalf("still once after a snapshot: %d", n)
	}
}

// fakeCore is remuda-core's adapter on paper: workspaces and tabs made into its own snapshot, the starts noted.
type fakeCore struct {
	snap    *herdr.Snapshot
	refuse  error
	started []string
	named   []string // the workspaces made, by name
}

func (f *fakeCore) SessionSnapshot() (*herdr.Snapshot, error) { return f.snap, nil }
func (f *fakeCore) AgentStart(paneID, kind string) error {
	return f.AgentStartWith(paneID, kind, kind, nil, 0)
}

func (f *fakeCore) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	if f.refuse != nil {
		return f.refuse
	}
	f.started = append(f.started, paneID+" "+kind+" "+strings.Join(args, " "))
	return nil
}

func (f *fakeCore) shell(workspaceID, cwd string) herdr.PaneInfo {
	p := herdr.PaneInfo{PaneID: fmt.Sprintf("t%d", len(f.snap.Panes)+1), WorkspaceID: workspaceID, TabID: fmt.Sprintf("tab-%d", len(f.snap.Panes)+1), CWD: cwd}
	f.snap.Panes = append(f.snap.Panes, p)
	return p
}

func (f *fakeCore) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	ws := herdr.WorkspaceInfo{WorkspaceID: fmt.Sprintf("ws-%d", len(f.snap.Workspaces)+1), Label: label, Folder: cwd}
	f.snap.Workspaces = append(f.snap.Workspaces, ws)
	f.named = append(f.named, label)
	return &herdr.WorkspaceCreateResult{Workspace: ws, RootPane: f.shell(ws.WorkspaceID, cwd)}, nil
}

func (f *fakeCore) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	p := f.shell(workspaceID, f.snap.WorkspaceDir(f.snap.Workspace(workspaceID)))
	return &herdr.TabCreateResult{Tab: herdr.TabInfo{TabID: p.TabID}, RootPane: p}, nil
}

// TestCarryOnMakesHerdrsWorkspaceAgainWithItsAgentsAsTabs: the agents of one herdr workspace come back as tabs of one
// workspace of the core, named as herdr's was; a session already on the core, or twice on the list, starts nothing; a
// folder gone forgets its agent; a start the core refuses keeps it listed.
func TestCarryOnMakesHerdrsWorkspaceAgainWithItsAgentsAsTabs(t *testing.T) {
	m := onCore(OpenMemory(""))
	r := New(nil)
	r.Memory = m
	const third, fourth = "2b3c4d5e-6f70-4b8c-9d0e-1f2a3b4c5d6e", "3c4d5e6f-7081-4c9d-8e0f-2a3b4c5d6e7f"
	m.Adopt("gog", map[string]Chat{
		"w1:p1": {Kind: "claude", Session: sid, Dir: "/work/api", Title: "api"},
		"w1:p2": {Kind: "codex", Session: otherSid, Dir: "/work/api", Title: "api"},
		"w1:p3": {Kind: "claude", Session: sid, Dir: "/work/api", Title: "api"}, // the same conversation twice
		"w2:p1": {Kind: "claude", Session: third, Dir: "/work/gone", Title: "old"},
		"w3:p1": {Kind: "claude", Session: fourth, Dir: "/work/web", Title: "web"},
	})
	running := agentPane("t9", "claude", "/work/web")
	running.AgentSession = &herdr.AgentSession{Kind: "id", Value: fourth}
	client := &fakeCore{snap: &herdr.Snapshot{Panes: []herdr.PaneInfo{running}}}
	checks := func(s Stopped, _ string) error {
		if s.chat.Dir == "/work/gone" {
			return errors.New("its folder /work/gone is gone — nothing to resume into")
		}
		return nil
	}
	var log []string
	out := r.carryOn(client, "gog", m.Stopped("gog"), checks, func(line string) { log = append(log, line) })
	if len(client.named) != 1 || client.named[0] != "api" || len(client.snap.Workspaces) != 1 {
		t.Fatalf("one workspace, named as herdr's: %v %+v", client.named, client.snap.Workspaces)
	}
	want := []string{"t2 claude --resume " + sid, "t3 codex resume " + otherSid}
	if strings.Join(client.started, "|") != strings.Join(want, "|") {
		t.Fatalf("started %v, want %v", client.started, want)
	}
	if client.snap.Panes[1].WorkspaceID != client.snap.Panes[2].WorkspaceID {
		t.Fatalf("the second agent is a tab of the first's workspace: %+v", client.snap.Panes)
	}
	if back := out.back(); len(back) != 4 {
		t.Fatalf("back: %v\n%s", back, strings.Join(log, "\n"))
	}
	if len(m.Stopped("gog")) != 0 || len(m.chats) != 0 {
		t.Fatalf("every herdr chat dealt with is forgotten: %+v %+v", m.Stopped("gog"), m.chats)
	}

	m.Adopt("gog", map[string]Chat{"w5:p1": {Kind: "claude", Session: sid, Dir: "/work/api", Title: "api"}})
	refusing := &fakeCore{snap: &herdr.Snapshot{}, refuse: &herdr.Error{Code: "agent_start_failed", Message: "no such binary"}}
	if out := r.carryOn(refusing, "gog", m.Stopped("gog"), checks, func(string) {}); len(out.failed()) != 1 || len(m.Stopped("gog")) != 1 {
		t.Fatalf("a refused start stays listed: %+v %+v", out, m.Stopped("gog"))
	}
}

// TestHerdrsAgentsWaitForItsServer: while herdr's server answers, Resume of its agents is refused before anything
// starts; the chats a reboot stopped are not held by it, and never resume herdr's by themselves.
func TestHerdrsAgentsWaitForItsServer(t *testing.T) {
	fleet := machines.NewCoreManager(t.TempDir(), "", nil, true)
	t.Cleanup(fleet.Close)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	r := New(fleet)
	up := true
	r.Serving = func(*machines.Machine) (bool, error) { return up, nil }
	stopped := []Stopped{{Pane: "t1", Agent: "claude"}, {Pane: "w1:p1", Agent: "claude", From: FromHerdr}}
	var running *HerdrRunning
	if err := r.MayCarryOn(mc, stopped); !errors.As(err, &running) || !strings.Contains(err.Error(), "Stop herdr's server") {
		t.Fatalf("herdr still up: %v", err)
	}
	if err := r.MayCarryOn(mc, stopped[:1]); err != nil {
		t.Fatalf("a reboot's chats alone: %v", err)
	}
	up = false
	if err := r.MayCarryOn(mc, stopped); err != nil {
		t.Fatalf("herdr stopped: %v", err)
	}
	r.Serving = func(*machines.Machine) (bool, error) { return false, errors.New("ssh: the master is gone") }
	if err := r.MayCarryOn(mc, stopped); err == nil {
		t.Fatal("unknown is no yes")
	}
	if rebooted := Rebooted(stopped); len(rebooted) != 1 || rebooted[0].Pane != "t1" {
		t.Fatalf("Rebooted: %+v", rebooted)
	}
}

// TestCarryOnStartsWhereTheAgentRan: a chat whose agent ran in another folder than its pane's carries on there, where
// its session belongs.
func TestCarryOnStartsWhereTheAgentRan(t *testing.T) {
	m := onCore(OpenMemory(""))
	r := New(nil)
	r.Memory = m
	m.Adopt("gog", map[string]Chat{"w1:p1": {Kind: "claude", Session: sid, Dir: "/work/api", Ran: "/work/api/cli", Title: "api"}})
	client := &fakeCore{snap: &herdr.Snapshot{}}
	checked := ""
	r.carryOn(client, "gog", m.Stopped("gog"), func(_ Stopped, dir string) error { checked = dir; return nil }, func(string) {})
	if checked != "/work/api/cli" || len(client.snap.Workspaces) != 1 || client.snap.Workspaces[0].Folder != "/work/api/cli" {
		t.Fatalf("checked %q, made %+v", checked, client.snap.Workspaces)
	}
}
