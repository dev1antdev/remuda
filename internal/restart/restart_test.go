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
