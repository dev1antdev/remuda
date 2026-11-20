package core_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
)

func adapterOf(t *testing.T, client *core.Client, hooks core.AdapterHooks) *core.Adapter {
	adapter := core.NewAdapter(client, hooks)
	t.Cleanup(func() { adapter.Close() })
	return adapter
}

// eventually asks for a snapshot until it says what want looks for.
func eventually(t *testing.T, adapter *core.Adapter, want func(*herdr.Snapshot) bool) *herdr.Snapshot {
	t.Helper()
	for start := time.Now(); time.Since(start) < 10*time.Second; time.Sleep(50 * time.Millisecond) {
		snap, err := adapter.SessionSnapshot()
		if err == nil && want(snap) {
			return snap
		}
	}
	snap, err := adapter.SessionSnapshot()
	t.Fatalf("the snapshot never came: %+v %v", snap, err)
	return nil
}

func TestTheAdapterShowsTheDocumentsWorkspacesAndEveryTerminal(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	agentTab, err := client.Run(ctx, core.RunParams{Argv: []string{"sleep", "60"}, Labels: map[string]string{core.LabelSpace: "w1", core.LabelTab: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	loose, err := client.Run(ctx, core.RunParams{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	doc := core.Document{Workspaces: []core.DocWorkspace{{ID: "w1", Label: "api", Tabs: []core.DocTab{{ID: "a", Label: "agents"}}}}}
	if _, err := client.DocSet(ctx, core.DocName, doc, 0); err != nil {
		t.Fatal(err)
	}
	adapter := adapterOf(t, client, core.AdapterHooks{})
	snap := eventually(t, adapter, func(s *herdr.Snapshot) bool { return len(s.Panes) == 2 && len(s.Workspaces) == 2 })
	if ws := snap.Workspaces[0]; ws.WorkspaceID != "w1" || ws.Label != "api" || ws.ActiveTabID != "a" {
		t.Fatalf("first workspace %+v", ws)
	}
	if p := snap.Pane(agentTab.Address); p == nil || p.TabID != "a" || p.WorkspaceID != "w1" {
		t.Fatalf("the labelled terminal is %+v", p)
	}
	if p := snap.Pane(loose.Address); p == nil || p.WorkspaceID == "w1" {
		t.Fatalf("the loose terminal is %+v", p)
	}

	doc.Workspaces[0].Label = "api renamed"
	if _, err := client.DocSet(ctx, core.DocName, doc, 1); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Workspaces[0].Label == "api renamed" })
	if _, err := client.Kill(ctx, loose.Address); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(loose.Address) == nil })
}

func nextEvent(t *testing.T, stream herdr.EventStream, want func(herdr.Event) bool) herdr.Event {
	t.Helper()
	got := make(chan herdr.Event, 1)
	go func() {
		for {
			ev, err := stream.Next()
			if err != nil {
				return
			}
			if want(ev) {
				got <- ev
				return
			}
		}
	}()
	select {
	case ev := <-got:
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("the event never came")
		return herdr.Event{}
	}
}

func TestAStatusTheProgramReportsReachesTheStreamAndTheSnapshot(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	stream, err := adapter.Subscribe(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	agent, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", `sleep 0.3; printf '\033]7501;state=working:app=claude-code\033\\'; sleep 60`}})
	if err != nil {
		t.Fatal(err)
	}
	told := nextEvent(t, stream, func(ev herdr.Event) bool { return ev.Kind == herdr.EventAgentStatus && ev.PaneID == agent.Address })
	if told.AgentStatus != herdr.StatusWorking {
		t.Fatalf("told %+v", told)
	}
	snap := eventually(t, adapter, func(s *herdr.Snapshot) bool {
		p := s.Pane(agent.Address)
		return p != nil && p.Agent == "claude" && p.AgentStatus == herdr.StatusWorking
	})
	if got, err := adapter.AgentGet(agent.Address); err != nil || got.Agent != "claude" || got.Status != herdr.StatusWorking {
		t.Fatalf("agent.get %+v %v in %+v", got, err, snap)
	}
}

func TestAnOutputWatchComesAsHerdrsMatch(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	builder, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", `sleep 1; echo "--- FAIL: TestThing (0.01s)"; sleep 60`}})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(builder.Address) != nil })
	stream, err := adapter.Subscribe([]herdr.Subscription{
		{Type: herdr.EventAgentStatus, PaneID: builder.Address},
		{Type: herdr.EventOutputMatched, PaneID: builder.Address, Source: "recent", Lines: 40, Match: &herdr.OutputMatch{Type: "regex", Value: `--- FAIL: \w+`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	told := nextEvent(t, stream, func(ev herdr.Event) bool { return ev.Kind == herdr.EventOutputMatched })
	if told.PaneID != builder.Address || !strings.Contains(told.MatchedLine, "--- FAIL: TestThing") {
		t.Fatalf("told %+v", told)
	}

	_, err = adapter.Subscribe([]herdr.Subscription{{Type: herdr.EventOutputMatched, PaneID: builder.Address, Match: &herdr.OutputMatch{Type: "regex", Value: `(unclosed`}}})
	if !herdr.IsCode(err, "invalid_regex") {
		t.Fatalf("a regex the core cannot read: %v", err)
	}
}

func TestReadsAnswerInHerdrsWords(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{})
	printer, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", `seq 1 50; printf '\033[31mred\033[0m\n'; sleep 60`}})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(printer.Address) != nil })
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		recent, _, err := adapter.PaneRecent(printer.Address, 3)
		if err == nil && recent == "49\n50\nred" {
			break
		}
		if time.Since(start) > 10*time.Second {
			t.Fatalf("recent %q %v", recent, err)
		}
	}
	screen, err := adapter.PaneScreen(printer.Address)
	if err != nil || !strings.Contains(screen.Text, "\x1b[") || !strings.Contains(screen.Text, "red") {
		t.Fatalf("screen %q %v", screen.Text, err)
	}
	if _, err := adapter.PaneRead("t999"); !herdr.IsCode(err, "pane_not_found") {
		t.Fatalf("a terminal that is not there: %v", err)
	}
	if _, err := adapter.AgentGet(printer.Address); !herdr.IsCode(err, "agent_not_found") {
		t.Fatalf("a terminal with no agent: %v", err)
	}
	if pid, err := adapter.PaneShellPID(printer.Address); err != nil || pid != printer.Pid {
		t.Fatalf("shell pid %d %v, want %d", pid, err, printer.Pid)
	}
	if err := adapter.AgentPrompt(printer.Address, "hi"); !herdr.IsCode(err, "invalid_request") {
		t.Fatalf("a request not on core yet: %v", err)
	}
}

// breakable dials the core and lets the test cut the connection it holds.
type breakable struct {
	dial core.Dialer
	mu   sync.Mutex
	last net.Conn
}

func (b *breakable) Dial(ctx context.Context) (net.Conn, error) {
	conn, err := b.dial(ctx)
	b.mu.Lock()
	b.last = conn
	b.mu.Unlock()
	return conn, err
}

func (b *breakable) cut() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last.Close()
}

func TestTheMirrorIsReadAgainAfterTheConnectionIsLost(t *testing.T) {
	ctx := within(t, 30*time.Second)
	server := coretest.Server(t)
	line := &breakable{dial: server.Dial}
	client := core.New(line.Dial)
	t.Cleanup(func() { client.Close() })
	var lost atomic.Int32
	adapter := adapterOf(t, client, core.AdapterHooks{TransportError: func(error) { lost.Add(1) }})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	stream, err := adapter.Subscribe(nil)
	if err != nil {
		t.Fatal(err)
	}
	line.cut()
	if _, err := stream.Next(); err == nil {
		for {
			if _, err = stream.Next(); err != nil {
				break
			}
		}
	}
	other := server.Client(t)
	started, err := other.Run(ctx, core.RunParams{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(started.Address) != nil })
	if lost.Load() == 0 {
		t.Fatal("the machine never heard the connection was lost")
	}
}
