package restart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/coreapi"
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
	codex.AgentSession = &coreapi.AgentSession{Kind: "id", Value: otherSid}
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

// remembered puts chats remembered in herdr into the memory of a fleet now on the core, and lets a core snapshot list
// them from herdr.
func remembered(m *Memory, machine string, chats map[string]Chat) {
	m.mu.Lock()
	for pane, c := range chats {
		m.chats[machine+"|"+pane] = c
	}
	m.mu.Unlock()
	observe(m, machine, false)
}

// TestHerdrsSleepersWaitWithTheRest: once on the core, a workspace that slept in herdr offers its agents from herdr,
// listed once by pane however many snapshots come; one carried on by hand is woken in the sleep, and one dealt with by
// Resume too. Before the move they are the sleep's alone.
func TestHerdrsSleepersWaitWithTheRest(t *testing.T) {
	sleepers := map[string]Chat{
		"w4:p1": {Kind: "claude", Session: sid, Dir: "/work/web", Title: "web"},
		"w4:p2": {Kind: "codex", Session: otherSid, Dir: "/work/web", Title: "web"},
	}
	var woken []string
	hooked := func(m *Memory) *Memory {
		m.Sleepers = func(string) map[string]Chat { return sleepers }
		m.Woken = func(machine, pane string) { woken = append(woken, machine+"|"+pane); delete(sleepers, pane) }
		return m
	}
	before := hooked(OpenMemory(""))
	observe(before, "gog", false, shellPane("w9:p1", "/home/me"))
	if len(before.Stopped("gog")) != 0 {
		t.Fatalf("on herdr a sleeper is the sleep's: %+v", before.Stopped("gog"))
	}

	m := onCore(hooked(OpenMemory("")))
	observe(m, "gog", false, shellPane("t1", "/home/me"))
	observe(m, "gog", false, shellPane("t1", "/home/me"))
	stopped := m.Stopped("gog")
	if len(stopped) != 2 || stopped[0].Pane != "w4:p1" || stopped[0].From != FromHerdr || stopped[1].Title != "web" || stopped[1].Command != "codex resume "+otherSid {
		t.Fatalf("herdr's sleepers: %+v", stopped)
	}
	codex := agentPane("t2", "codex", "/work/web")
	codex.AgentSession = &coreapi.AgentSession{Kind: "id", Value: otherSid}
	observe(m, "gog", false, shellPane("t1", "/home/me"), codex)
	if stopped := m.Stopped("gog"); len(stopped) != 1 || len(woken) != 1 || woken[0] != "gog|w4:p2" {
		t.Fatalf("one on its session in the core again: %+v, woken %v", stopped, woken)
	}
	r := New(nil)
	r.Memory = m
	r.carryOn(&fakeCore{snap: &coreapi.Snapshot{}}, "gog", m.Stopped("gog"), func(Stopped, string) error { return nil }, func(string) {})
	if len(m.Stopped("gog")) != 0 || len(woken) != 2 || woken[1] != "gog|w4:p1" {
		t.Fatalf("carried on by Resume, the sleep is told: %+v, woken %v", m.Stopped("gog"), woken)
	}
}

// fakeCore is remuda-core's adapter on paper: workspaces and tabs made into its own snapshot, the starts noted.
type fakeCore struct {
	snap    *coreapi.Snapshot
	refuse  error
	started []string
	named   []string     // the workspaces made, by name
	reread  func() error // what reading the agents' sessions again does, nil for nothing
	looks   int          // the snapshots asked for
}

func (f *fakeCore) RereadSessions() error {
	if f.reread == nil {
		return nil
	}
	return f.reread()
}

func (f *fakeCore) SessionSnapshot() (*coreapi.Snapshot, error) {
	f.looks++
	return f.snap, nil
}
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

func (f *fakeCore) shell(workspaceID, cwd string) coreapi.PaneInfo {
	p := coreapi.PaneInfo{PaneID: fmt.Sprintf("t%d", len(f.snap.Panes)+1), WorkspaceID: workspaceID, TabID: fmt.Sprintf("tab-%d", len(f.snap.Panes)+1), CWD: cwd}
	f.snap.Panes = append(f.snap.Panes, p)
	return p
}

func (f *fakeCore) WorkspaceCreate(cwd, label string) (*coreapi.WorkspaceCreateResult, error) {
	ws := coreapi.WorkspaceInfo{WorkspaceID: fmt.Sprintf("ws-%d", len(f.snap.Workspaces)+1), Label: label, Folder: cwd}
	f.snap.Workspaces = append(f.snap.Workspaces, ws)
	f.named = append(f.named, label)
	return &coreapi.WorkspaceCreateResult{Workspace: ws, RootPane: f.shell(ws.WorkspaceID, cwd)}, nil
}

func (f *fakeCore) TabCreate(workspaceID, label string) (*coreapi.TabCreateResult, error) {
	p := f.shell(workspaceID, f.snap.WorkspaceDir(f.snap.Workspace(workspaceID)))
	return &coreapi.TabCreateResult{Tab: coreapi.TabInfo{TabID: p.TabID}, RootPane: p}, nil
}

// TestCarryOnMakesHerdrsWorkspaceAgainWithItsAgentsAsTabs: the agents of one herdr workspace come back as tabs of one
// workspace of the core, named as herdr's was; a session already on the core, or twice on the list, starts nothing; a
// folder gone forgets its agent; a start the core refuses keeps it listed.
func TestCarryOnMakesHerdrsWorkspaceAgainWithItsAgentsAsTabs(t *testing.T) {
	m := onCore(OpenMemory(""))
	r := New(nil)
	r.Memory = m
	const third, fourth = "2b3c4d5e-6f70-4b8c-9d0e-1f2a3b4c5d6e", "3c4d5e6f-7081-4c9d-8e0f-2a3b4c5d6e7f"
	remembered(m, "gog", map[string]Chat{
		"w1:p1": {Kind: "claude", Session: sid, Dir: "/work/api", Title: "api"},
		"w1:p2": {Kind: "codex", Session: otherSid, Dir: "/work/api", Title: "api"},
		"w1:p3": {Kind: "claude", Session: sid, Dir: "/work/api", Title: "api"}, // the same conversation twice
		"w2:p1": {Kind: "claude", Session: third, Dir: "/work/gone", Title: "old"},
		"w3:p1": {Kind: "claude", Session: fourth, Dir: "/work/web", Title: "web"},
	})
	running := agentPane("t9", "claude", "/work/web")
	running.AgentSession = &coreapi.AgentSession{Kind: "id", Value: fourth}
	client := &fakeCore{snap: &coreapi.Snapshot{Panes: []coreapi.PaneInfo{running}}}
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

	remembered(m, "gog", map[string]Chat{"w5:p1": {Kind: "claude", Session: sid, Dir: "/work/api", Title: "api"}})
	refusing := &fakeCore{snap: &coreapi.Snapshot{}, refuse: &coreapi.Error{Code: "agent_start_failed", Message: "no such binary"}}
	if out := r.carryOn(refusing, "gog", m.Stopped("gog"), checks, func(string) {}); len(out.failed()) != 1 || len(m.Stopped("gog")) != 1 {
		t.Fatalf("a refused start stays listed: %+v %+v", out, m.Stopped("gog"))
	}
}

// TestHerdrsAgentsWaitForItsServer: while herdr's server answers, Resume of its agents is refused before anything
// starts; the chats a reboot stopped are not held by it, and never resume herdr's by themselves.
func TestHerdrsAgentsWaitForItsServer(t *testing.T) {
	fleet := machines.NewManager(t.TempDir(), nil, true)
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
	remembered(m, "gog", map[string]Chat{"w1:p1": {Kind: "claude", Session: sid, Dir: "/work/api", Ran: "/work/api/cli", Title: "api"}})
	client := &fakeCore{snap: &coreapi.Snapshot{}}
	checked := ""
	r.carryOn(client, "gog", m.Stopped("gog"), func(_ Stopped, dir string) error { checked = dir; return nil }, func(string) {})
	if checked != "/work/api/cli" || len(client.snap.Workspaces) != 1 || client.snap.Workspaces[0].Folder != "/work/api/cli" {
		t.Fatalf("checked %q, made %+v", checked, client.snap.Workspaces)
	}
}

// TestCarryOnForgetsNothingTheMachineCouldNotSay: a check the machine could not answer, or sessions it could not read
// again, keep the agent listed and the run failed, nothing started; an agent of the core resumed onto the session a
// moment before — seen by a look, then its session read again — counts as back.
func TestCarryOnForgetsNothingTheMachineCouldNotSay(t *testing.T) {
	m := onCore(OpenMemory(""))
	r := New(nil)
	r.Memory = m
	remembered(m, "gog", map[string]Chat{"w1:p1": {Kind: "claude", Session: sid, Dir: "/work/api", Title: "api"}})
	unasked := func(Stopped, string) error { return fmt.Errorf("%w: ssh: the master is gone", ErrUnasked) }
	client := &fakeCore{snap: &coreapi.Snapshot{}}
	if out := r.carryOn(client, "gog", m.Stopped("gog"), unasked, func(string) {}); len(out.failed()) != 1 || len(out.notBack()) != 1 || len(m.Stopped("gog")) != 1 || len(client.started) != 0 {
		t.Fatalf("a check unanswered: %+v %+v %v", out, m.Stopped("gog"), client.started)
	}
	fine := func(Stopped, string) error { return nil }
	client.reread = func() error { return errors.New("the process table could not be read") }
	if out := r.carryOn(client, "gog", m.Stopped("gog"), fine, func(string) {}); len(out.failed()) != 1 || len(m.Stopped("gog")) != 1 || len(client.started) != 0 {
		t.Fatalf("sessions unread: %+v %+v %v", out, m.Stopped("gog"), client.started)
	}
	resumed := agentPane("t7", "claude", "/work/api")
	client.looks = 0
	client.reread = func() error {
		if client.looks == 0 { // an agent started a moment ago is read only once a look has seen it
			return nil
		}
		resumed.AgentSession = &coreapi.AgentSession{Kind: "id", Value: sid}
		client.snap.Panes = []coreapi.PaneInfo{resumed}
		return nil
	}
	if out := r.carryOn(client, "gog", m.Stopped("gog"), fine, func(string) {}); len(out.back()) != 1 || len(client.started) != 0 || len(m.Stopped("gog")) != 0 {
		t.Fatalf("on its session after a /resume: %+v %v", out, client.started)
	}
}

// TestCanResumeTellsAFolderGoneFromOneNotAsked: a folder that is not there forgets its chat, one the machine would not
// show wraps ErrUnasked, so the chat stays.
func TestCanResumeTellsAFolderGoneFromOneNotAsked(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a folder whatever its mode")
	}
	fleet := machines.NewManager(t.TempDir(), nil, true)
	t.Cleanup(fleet.Close)
	mc, _ := fleet.Get(machines.LocalID)
	here := t.TempDir()
	locked := filepath.Join(here, "locked")
	if err := os.MkdirAll(filepath.Join(locked, "api"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })
	if err := CanResume(mc, "codex", sid, here); err != nil {
		t.Fatalf("a folder that is there: %v", err)
	}
	if err := CanResume(mc, "codex", sid, filepath.Join(here, "gone")); err == nil || errors.Is(err, ErrUnasked) || !strings.Contains(err.Error(), "is gone") {
		t.Fatalf("a folder gone: %v", err)
	}
	if err := CanResume(mc, "codex", sid, filepath.Join(locked, "api")); !errors.Is(err, ErrUnasked) {
		t.Fatalf("a folder nobody could look at: %v", err)
	}
}
