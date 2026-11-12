package concierge

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

const session = "0a1b2c3d-1111-2222-3333-444455556666"

// fakeHerdr is a machine's herdr: the snapshot it holds, the workspaces made and closed, the agents started, the keys
// pressed and the screens a pane shows, in turn.
type fakeHerdr struct {
	mu      sync.Mutex
	snap    *herdr.Snapshot
	made    []string // folder|label
	closed  []string
	started []string // pane: the arguments, joined by |
	keys    []string
	screens []string
}

func (f *fakeHerdr) AgentStart(pane, kind string) error {
	f.started = append(f.started, pane+":")
	return nil
}

func (f *fakeHerdr) AgentStartWith(pane, kind, name string, args []string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, pane+":"+strings.Join(args, "|"))
	for i := range f.snap.Panes {
		if f.snap.Panes[i].PaneID == pane {
			f.snap.Panes[i].Agent = kind
		}
	}
	return nil
}

func (f *fakeHerdr) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	f.made = append(f.made, cwd+"|"+label)
	id := "w9"
	f.snap.Workspaces = append(f.snap.Workspaces, herdr.WorkspaceInfo{WorkspaceID: id, Label: label, ActiveTabID: id + ":t1"})
	f.snap.Panes = append(f.snap.Panes, herdr.PaneInfo{PaneID: id + ":p1", TabID: id + ":t1", WorkspaceID: id, CWD: cwd})
	return &herdr.WorkspaceCreateResult{Workspace: herdr.WorkspaceInfo{WorkspaceID: id}, RootPane: herdr.PaneInfo{PaneID: id + ":p1"}}, nil
}

func (f *fakeHerdr) WorkspaceClose(id string) error {
	f.closed = append(f.closed, id)
	return nil
}

func (f *fakeHerdr) PaneRead(string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.screens) == 0 {
		return "", nil
	}
	screen := f.screens[0]
	f.screens = f.screens[1:]
	return screen, nil
}

func (f *fakeHerdr) PaneSendKeys(_ string, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = append(f.keys, keys...)
	return nil
}

// standing is a snapshot where w5 is the concierge's workspace, its pane running Claude Code on a session
func standing(agent string) *herdr.Snapshot {
	return &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api", ActiveTabID: "w1:t1"}, {WorkspaceID: "w5", Label: Label, ActiveTabID: "w5:t1"}},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", TabID: "w1:t1", WorkspaceID: "w1", Agent: "claude", AgentSession: &herdr.AgentSession{Agent: "claude", Kind: "id", Value: "99999999-1111-2222-3333-444455556666"}},
			{PaneID: "w5:p1", TabID: "w5:t1", WorkspaceID: "w5", Agent: agent, CWD: "/state/concierge"},
		},
	}
}

func hostOn(client *fakeHerdr, resumable bool) Host {
	return Host{
		ID: "mac", Home: "/Users/me", Client: client, Remuda: "/Users/me/.local/bin/remuda",
		Snapshot:  func() (*herdr.Snapshot, error) { return client.snap, nil },
		Prepare:   func() (string, error) { return "/state/concierge", nil },
		Resumable: func(folder, id string) bool { return resumable && folder == "/state/concierge" && id == session },
	}
}

func noTrustLook(t *testing.T) {
	was := TrustWait
	TrustWait = 0
	t.Cleanup(func() { TrustWait = was })
}

func TestAFirstStartMakesItsWorkspaceInItsFolderAndKeepsIt(t *testing.T) {
	noTrustLook(t)
	dir := t.TempDir()
	s := Open(dir)
	client := &fakeHerdr{snap: &herdr.Snapshot{}}
	run, err := s.Ensure(hostOn(client, true))
	if err != nil {
		t.Fatal(err)
	}
	if len(client.made) != 1 || client.made[0] != "/state/concierge|concierge" {
		t.Errorf("one workspace, in its folder: %q", client.made)
	}
	if run.Workspace != "w9" || run.Pane != "w9:p1" || run.Dir != "/state/concierge" || !run.Started || run.Resumed {
		t.Errorf("the run: %+v", run)
	}
	if len(client.started) != 1 || client.started[0] != "w9:p1:"+strings.Join(Args("/Users/me/.local/bin/remuda"), "|") {
		t.Errorf("a fresh agent on its settings and its one server: %q", client.started)
	}
	if !s.Is("mac", "w9") || s.Is("mac", "w1") || s.Is("gog", "w9") {
		t.Error("the workspace is the concierge's on that machine alone")
	}
	if again := Open(dir); !again.Is("mac", "w9") {
		t.Error("the workspace is kept across a restart")
	}
}

func TestAStandingConciergeIsFoundAgainAndNothingStarts(t *testing.T) {
	noTrustLook(t)
	s := Open(t.TempDir())
	s.update("mac", func(p *Place) { p.Workspace = "w5" })
	client := &fakeHerdr{snap: standing("claude")}
	run, err := s.Ensure(hostOn(client, true))
	if err != nil || run.Pane != "w5:p1" || run.Started || len(client.made)+len(client.started) != 0 {
		t.Errorf("found again, nothing made or started: %+v %v made %q started %q", run, err, client.made, client.started)
	}
}

func TestAPaneBackAsABareShellResumesTheConversation(t *testing.T) {
	noTrustLook(t)
	s := Open(t.TempDir())
	s.update("mac", func(p *Place) { *p = Place{Workspace: "w5", Session: session} })
	client := &fakeHerdr{snap: standing("")}
	run, err := s.Ensure(hostOn(client, true))
	if err != nil || !run.Resumed || run.Pane != "w5:p1" || len(client.made) != 0 {
		t.Fatalf("resumed in its own pane: %+v %v made %q", run, err, client.made)
	}
	if len(client.started) != 1 || !strings.HasPrefix(client.started[0], "w5:p1:--resume|"+session+"|--settings|") {
		t.Errorf("the conversation kept comes back: %q", client.started)
	}
}

func TestAConversationWhoseFileIsGoneStartsFresh(t *testing.T) {
	noTrustLook(t)
	s := Open(t.TempDir())
	s.update("mac", func(p *Place) { *p = Place{Workspace: "w5", Session: session} })
	client := &fakeHerdr{snap: standing("")}
	run, err := s.Ensure(hostOn(client, false))
	if err != nil || run.Resumed || strings.Contains(client.started[0], "--resume") {
		t.Fatalf("a fresh start: %+v %v %q", run, err, client.started)
	}
	if s.Place("mac").Session != "" {
		t.Error("the gone conversation is forgotten")
	}
}

func TestAWorkspaceHerdrNoLongerHasIsMadeAgainOnTheKeptConversation(t *testing.T) {
	noTrustLook(t)
	s := Open(t.TempDir())
	s.update("mac", func(p *Place) { *p = Place{Workspace: "w7", Session: session} })
	client := &fakeHerdr{snap: standing("claude")}
	run, err := s.Ensure(hostOn(client, true))
	if err != nil || run.Workspace != "w9" || !run.Resumed || len(client.made) != 1 {
		t.Errorf("a new workspace, the conversation resumed in it: %+v %v %q", run, err, client.made)
	}
}

func TestObserveKeepsTheConversationItsAgentIsOn(t *testing.T) {
	s := Open(t.TempDir())
	s.update("mac", func(p *Place) { p.Workspace = "w5" })
	snap := standing("claude")
	s.Observe("mac", snap)
	if s.Place("mac").Session != "" {
		t.Error("no session reported yet, none kept; another workspace's is not taken")
	}
	snap.Panes[1].AgentSession = &herdr.AgentSession{Agent: "claude", Kind: "path", Value: "/x.jsonl"}
	s.Observe("mac", snap)
	if s.Place("mac").Session != "" {
		t.Error("a path is no session id")
	}
	snap.Panes[1].AgentSession = &herdr.AgentSession{Agent: "claude", Kind: "id", Value: "../../etc"}
	s.Observe("mac", snap)
	if s.Place("mac").Session != "" {
		t.Error("an id that does not look like one is not kept")
	}
	snap.Panes[1].AgentSession = &herdr.AgentSession{Agent: "claude", Kind: "id", Value: session}
	s.Observe("mac", snap)
	s.Observe("gog", snap)
	if s.Place("mac").Session != session || s.Place("gog").Session != "" {
		t.Errorf("the session kept for its machine alone: %+v %+v", s.Place("mac"), s.Place("gog"))
	}
}

func TestCloseKeepsTheConversationAndANewOneForgetsIt(t *testing.T) {
	s := Open(t.TempDir())
	s.update("mac", func(p *Place) { *p = Place{Workspace: "w5", Session: session} })
	client := &fakeHerdr{snap: standing("claude")}
	if err := s.Close("mac", client, client.snap, false); err != nil {
		t.Fatal(err)
	}
	if len(client.closed) != 1 || client.closed[0] != "w5" || s.Place("mac") != (Place{Session: session}) {
		t.Errorf("closed, the conversation kept for the next start: %q %+v", client.closed, s.Place("mac"))
	}
	if !s.Is("mac", "w5") {
		t.Error("a snapshot read before the close still shows its workspace: it stays hidden")
	}
	if err := s.Close("mac", client, client.snap, false); err != ErrNone {
		t.Errorf("nothing standing: %v", err)
	}
	if err := s.Close("mac", client, client.snap, true); err != nil || s.Place("mac") != (Place{}) {
		t.Errorf("a new conversation forgets the old one: %v %+v", err, s.Place("mac"))
	}
	s.update("gog", func(p *Place) { p.Workspace = "w42" })
	if err := s.Close("gog", client, client.snap, false); err != nil || len(client.closed) != 1 {
		t.Errorf("a workspace herdr no longer has is only forgotten: %v %q", err, client.closed)
	}
	if got := s.Standing(""); len(got) != 0 {
		t.Errorf("nothing stands: %q", got)
	}
}

func TestTheTrustQuestionForItsOwnFolderIsAnswered(t *testing.T) {
	other := "Accessing workspace:\n/Users/me/elsewhere\n❯ No, exit\n  Yes, I trust this folder\n"
	ours := "Accessing workspace:\n~/Library/Application Support/remuda/concierge\n❯ No, exit\n  Yes, I trust this folder\n"
	client := &fakeHerdr{snap: &herdr.Snapshot{}, screens: []string{"", other, ours}}
	var locked bool
	h := hostOn(client, false)
	h.Lock = func(string) sync.Locker { locked = true; return &sync.Mutex{} }
	answerTrust(h, "w9:p1", "/Users/me/Library/Application Support/remuda/concierge", 2*time.Second, time.Millisecond)
	if strings.Join(client.keys, " ") != "down enter" || !locked {
		t.Errorf("the cursor moved to Yes and Enter pressed under the pane's lock: %q %v", client.keys, locked)
	}
	client = &fakeHerdr{snap: &herdr.Snapshot{}, screens: []string{other, other, other}}
	answerTrust(hostOn(client, false), "w9:p1", "/state/concierge", 50*time.Millisecond, time.Millisecond)
	if len(client.keys) != 0 {
		t.Errorf("another folder's question is never answered: %q", client.keys)
	}
}
