package restart

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestKindsCountNames(t *testing.T) {
	if k := Kinds("agents"); len(k) != 4 {
		t.Fatalf("agents: %v", k)
	}
	if k := Kinds("herdr"); k != nil {
		t.Fatalf("herdr: %v", k)
	}
	sid := &herdr.AgentSession{Kind: "id", Value: "abc"}
	snap := &herdr.Snapshot{Panes: []herdr.PaneInfo{
		{PaneID: "wA:p1", Agent: "claude", AgentStatus: "idle", AgentSession: sid},
		{PaneID: "wA:p2", Agent: "claude", AgentStatus: "working", AgentSession: sid},
		{PaneID: "wA:p3", Agent: "claude", AgentStatus: "idle"}, // no session: not a chat to restart
		{PaneID: "wB:p1", Agent: "codex", AgentStatus: "done", AgentSession: sid},
		{PaneID: "wB:p2", Agent: "", AgentStatus: ""},
	}}
	if r, b := Count(snap, []string{"claude"}); r != 1 || b != 1 {
		t.Fatalf("claude: %d %d", r, b)
	}
	if r, b := Count(snap, Kinds("agents")); r != 2 || b != 1 {
		t.Fatalf("agents: %d %d", r, b)
	}
	if n := agentName("claude", "wD:p5"); n != "claude-wd-p5" {
		t.Fatalf("name: %s", n)
	}
	if Resume("codex", "abc") != "codex resume abc" || Resume("claude", "abc") != "claude --resume abc" || Resume("gemini", "abc") != "gemini --resume abc" || Resume("kimi", "abc") != "kimi --session abc" {
		t.Fatal("resume commands")
	}
	if Exit("claude") != "/exit" || Exit("codex") != "/quit" || Exit("kimi") != "/exit" || Exit("herdr") != "" {
		t.Fatal("exit commands")
	}
}

// TestResumableRefusesWhatCannotBeRestarted: the one check behind a restart by hand, on the snapshot before the job and on a fresh look inside it.
func TestResumableRefusesWhatCannotBeRestarted(t *testing.T) {
	sid := &herdr.AgentSession{Kind: "id", Value: "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"}
	for _, c := range []struct {
		name string
		pane herdr.PaneInfo
		want string
	}{
		{"shell", herdr.PaneInfo{PaneID: "w1:p1", Title: "zsh"}, "is a shell"},
		{"unknown agent", herdr.PaneInfo{PaneID: "w1:p1", Agent: "skynet", AgentStatus: "idle", AgentSession: sid}, "cannot restart"},
		{"no session", herdr.PaneInfo{PaneID: "w1:p1", Agent: "claude", AgentStatus: "idle"}, "no session id"},
		{"bad session", herdr.PaneInfo{PaneID: "w1:p1", Agent: "claude", AgentStatus: "idle", AgentSession: &herdr.AgentSession{Kind: "id", Value: "--help"}}, "no session id"},
		{"question open", herdr.PaneInfo{PaneID: "w1:p1", Agent: "claude", AgentStatus: "blocked", AgentSession: sid}, "question open"},
		{"working", herdr.PaneInfo{PaneID: "w1:p1", Agent: "codex", AgentStatus: "working", AgentSession: sid}, "still working"},
		{"unknown state", herdr.PaneInfo{PaneID: "w1:p1", Agent: "claude", AgentStatus: "unknown", AgentSession: sid}, "unknown right now"},
	} {
		if _, _, err := Resumable(c.pane); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Resumable = %v, want %q", c.name, err, c.want)
		}
	}
	for _, status := range []string{"idle", "done"} {
		kind, session, err := Resumable(herdr.PaneInfo{PaneID: "w1:p1", Agent: "kimi", AgentStatus: status, AgentSession: sid})
		if err != nil || kind != "kimi" || session != sid.Value {
			t.Errorf("%s: Resumable = %q %q %v", status, kind, session, err)
		}
	}
	snap := &herdr.Snapshot{Panes: []herdr.PaneInfo{{PaneID: "w1:p1", Agent: "claude"}, {PaneID: "w2:p1"}}}
	if p, ok := Lookup(snap, "w2:p1"); !ok || p.PaneID != "w2:p1" {
		t.Fatalf("lookup: %+v %v", p, ok)
	}
	if _, ok := Lookup(snap, "w3:p1"); ok {
		t.Fatal("a pane that is not there")
	}
	if _, ok := Lookup(nil, "w1:p1"); ok {
		t.Fatal("no snapshot, no pane")
	}
}

func TestSessionOfRefusesWhatIsNotAnID(t *testing.T) {
	// the id goes after --resume: a flag, a path or a shell word in it is no session at all
	for _, v := range []string{"--help", "-x", "a b", "../x", "", "x;rm", "x$(y)", ".hidden"} {
		if got := sessionOf(&herdr.PaneInfo{AgentSession: &herdr.AgentSession{Kind: "id", Value: v}}); got != "" {
			t.Errorf("sessionOf(%q) = %q, want none", v, got)
		}
	}
	for _, v := range []string{"9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d", "abc", "s_1.2"} {
		if sessionOf(&herdr.PaneInfo{AgentSession: &herdr.AgentSession{Kind: "id", Value: v}}) != v {
			t.Errorf("sessionOf(%q) refused a plain id", v)
		}
	}
	if sessionOf(&herdr.PaneInfo{AgentSession: &herdr.AgentSession{Kind: "path", Value: "/x.jsonl"}}) != "" {
		t.Error("a path is not an id")
	}
}
