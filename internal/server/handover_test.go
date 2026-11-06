package server

import (
	"net/http"
	"strings"
	"testing"
)

// The handover route refuses a bad request before it looks at any machine: the paths a page or a script can get wrong.
func TestHandoverRefusesBadRequests(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.handoverRoutes(mux)
	cases := map[string]struct {
		body string
		want string
	}{
		"no pane":           {`{"agent":"codex"}`, "a pane and an agent"},
		"no agent":          {`{"pane":"w1:p1"}`, "a pane and an agent"},
		"a shell":           {`{"pane":"w1:p1","agent":"shell"}`, "a pane and an agent"},
		"unknown agent":     {`{"pane":"w1:p1","agent":"gpt"}`, `unknown agent "gpt"`},
		"bad session":       {`{"pane":"w1:p1","agent":"codex","session":"../x"}`, "bad session id"},
		"a model it lacks":  {`{"pane":"w1:p1","agent":"pi","model":"x"}`, "model"},
		"unclosed argument": {`{"pane":"w1:p1","agent":"codex","args":"--flag 'x"}`, "quote"},
	}
	for name, c := range cases {
		code, out := taskCall(mux, "POST", "/api/handover", "", c.body)
		msg, _ := out["error"].(string)
		if code != 400 || !strings.Contains(msg, c.want) {
			t.Errorf("%s: %d %q, want 400 with %q", name, code, msg, c.want)
		}
	}
}

func TestHandoverWords(t *testing.T) {
	for n, want := range map[int]string{-2: "from Claude Code · the recap · 2 KB", 0: "from Claude Code · the recap · 2 KB", 1: "from Claude Code · the recap and the last exchange · 2 KB", 3: "from Claude Code · the recap and the last 3 exchanges · 2 KB"} {
		if got := handoverWords("Claude Code", n, 12, 2000); got != want {
			t.Errorf("handoverWords(%d) = %q, want %q", n, got, want)
		}
	}
	// the default 3 over a conversation of one exchange carries that one; 50 asked carries MaxLastN
	if got := handoverWords("Codex", 3, 1, 2000); got != "from Codex · the recap and the last exchange · 2 KB" {
		t.Errorf("a short conversation: %q", got)
	}
	if got := handoverWords("Codex", 50, 40, 2000); got != "from Codex · the recap and the last 10 exchanges · 2 KB" {
		t.Errorf("more than a handover carries: %q", got)
	}
	if agentLabel("codex") != "Codex" || agentLabel("odd") != "odd" {
		t.Errorf("agentLabel: %q %q", agentLabel("codex"), agentLabel("odd"))
	}
}
