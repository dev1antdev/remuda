package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// The fork route refuses a bad request before it looks at any machine.
func TestForkRefusesBadRequests(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.forkRoutes(mux)
	cases := map[string]struct {
		body string
		want string
	}{
		"no pane":     {`{}`, "a pane to fork"},
		"bad session": {`{"pane":"w1:p1","session":"../x"}`, "bad session id"},
	}
	for name, c := range cases {
		code, out := taskCall(mux, "POST", "/api/fork", "", c.body)
		msg, _ := out["error"].(string)
		if code != 400 || !strings.Contains(msg, c.want) {
			t.Errorf("%s: %d %q, want 400 with %q", name, code, msg, c.want)
		}
	}
}

func TestForkSession(t *testing.T) {
	live := &herdr.AgentSession{Kind: "id", Value: "0123-abcd"}
	cases := []struct {
		name  string
		pane  herdr.PaneInfo
		asked string
		want  string
		err   string
	}{
		{"the live one", herdr.PaneInfo{Agent: "claude", AgentSession: live}, "", "0123-abcd", ""},
		{"the one shown", herdr.PaneInfo{Agent: "codex", AgentSession: live}, "9999-ffff", "9999-ffff", ""},
		{"a shell", herdr.PaneInfo{}, "", "", "a shell pane"},
		{"an agent that cannot", herdr.PaneInfo{Agent: "gemini", AgentSession: live}, "", "", "Gemini cannot start on a copy"},
		{"no session yet", herdr.PaneInfo{Agent: "claude"}, "", "", "no session id yet"},
		{"a path, not an id", herdr.PaneInfo{Agent: "claude", AgentSession: &herdr.AgentSession{Kind: "path", Value: "/x.jsonl"}}, "", "", "no session id yet"},
	}
	for _, c := range cases {
		got, err := forkSession(&c.pane, c.asked)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: %q %v, want an error with %q", c.name, got, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v, want %q", c.name, got, err, c.want)
		}
	}
}
