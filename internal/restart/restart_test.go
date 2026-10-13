package restart

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestKindsCountNames(t *testing.T) {
	if k := Kinds("agents"); len(k) != 3 {
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
	if Resume("codex", "abc") != "codex resume abc" || Resume("claude", "abc") != "claude --resume abc" || Resume("gemini", "abc") != "gemini --resume abc" {
		t.Fatal("resume commands")
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
