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

func TestInPlaceIsClaudeCodesAlone(t *testing.T) {
	if got := InPlace("claude"); got != "/restart" {
		t.Fatalf("claude: %q", got)
	}
	for _, kind := range []string{"codex", "gemini", "kimi", "shell"} {
		if got := InPlace(kind); got != "" {
			t.Fatalf("%s: %q", kind, got)
		}
	}
}

func TestOutcomeReadsTheScreenPastTheNewestEcho(t *testing.T) {
	resumed := " ▐▛███▛█   Claude Code v2.1.312\n▝▜██████▀  Fable 5.1 · Claude Max\n❯ Reply with PROBE-OK.\n⏺ PROBE-OK\n❯ /restart\n  ⎿  Session resumed\n───\n❯\n───\n  Model: Fable 5.1 | Context: 46k/1.0M (5%)"
	unknown := "⏺ PROBE-OK\n❯ /restart\n⏺ Unknown command: /restart\n❯"
	away := "⏺ PROBE-OK\n❯ /restart\n  ⎿  /restart isn't available over Remote Control.\n❯"
	refused := "⏺ PROBE-OK\n❯ /restart\n  ⎿  Can't restart this session. It isn't saved to disk, so a restart would lose it. Nothing was changed.\n❯"
	typing := "⏺ PROBE-OK\n───\n❯ /restart\n───"
	cases := []struct {
		name   string
		screen string
		before int
		want   Verdict
		line   string
	}{
		{"resumed", resumed, 0, Resumed, ""},
		{"unknown command", unknown, 0, Unknown, "Unknown command: /restart"},
		{"not available here", away, 0, Unknown, "/restart isn't available over Remote Control."},
		{"refused", refused, 0, Refused, "Can't restart this session. It isn't saved to disk, so a restart would lose it. Nothing was changed."},
		{"still typing", typing, 0, Pending, ""},
		{"an older echo does not count", resumed, 1, Pending, ""},
		{"no echo yet", "⏺ PROBE-OK\n❯", 0, Pending, ""},
	}
	for _, c := range cases {
		got, line := Outcome(c.screen, c.before)
		if got != c.want || line != c.line {
			t.Errorf("%s: got %v %q, want %v %q", c.name, got, line, c.want, c.line)
		}
	}
	if n := Echoes(resumed); n != 1 {
		t.Errorf("echoes: %d", n)
	}
	if v := Version(resumed); v != "2.1.312" {
		t.Errorf("version: %q", v)
	}
	if v := Version(unknown); v != "" {
		t.Errorf("version without a banner: %q", v)
	}
}
