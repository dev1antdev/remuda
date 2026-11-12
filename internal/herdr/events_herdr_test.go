package herdr_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
)

// TestEventsFromAThrowawayHerdr is the protocol as herdr 0.9.5 speaks it: a pane made shows up on a global
// subscription, its agent's status and an output match on the ones bound to it.
func TestEventsFromAThrowawayHerdr(t *testing.T) {
	sock := herdrtest.Server(t)
	client := herdr.NewClient(sock)
	global, err := client.Subscribe([]herdr.Subscription{{Type: "workspace.created"}, {Type: "pane.created"}, {Type: "pane.closed"}})
	if err != nil {
		t.Fatal(err)
	}
	defer global.Close()
	made, err := client.WorkspaceCreate(os.TempDir(), "events")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	pane := made.RootPane.PaneID
	kinds := next(t, global, 2)
	if !strings.Contains(strings.Join(kinds, " "), "pane_created "+pane) {
		t.Fatalf("the new pane on the global stream: %v", kinds)
	}

	bound, err := client.Subscribe([]herdr.Subscription{
		{Type: herdr.EventAgentStatus, PaneID: pane},
		{Type: herdr.EventOutputMatched, PaneID: pane, Source: "recent", Lines: 40, Match: &herdr.OutputMatch{Type: "regex", Value: `^\s*Local:\s+https?://`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if err := client.Call("pane.report_agent", map[string]any{"pane_id": pane, "source": "remuda-test", "agent": "claude", "state": "working"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := next(t, bound, 1); got[0] != herdr.EventAgentStatus+" "+pane+" working" {
		t.Fatalf("the agent's status: %v", got)
	}
	if err := client.PaneSendText(pane, `printf '\n  Local:   http://localhost:5173/\n'`+"\r"); err != nil {
		t.Fatal(err)
	}
	if got := next(t, bound, 1); got[0] != herdr.EventOutputMatched+" "+pane+"   Local:   http://localhost:5173/" {
		t.Fatalf("the output match: %v", got)
	}

	if _, err := client.Subscribe([]herdr.Subscription{{Type: herdr.EventAgentStatus, PaneID: "w999:p9"}}); !herdr.IsCode(err, "pane_not_found") {
		t.Fatalf("a pane that is not there refuses the subscription: %v", err)
	}
	if _, err := client.Subscribe([]herdr.Subscription{{Type: herdr.EventOutputMatched, PaneID: pane, Source: "recent", Match: &herdr.OutputMatch{Type: "regex", Value: `(?=x)`}}}); !herdr.IsCode(err, "invalid_regex") {
		t.Fatalf("a regex herdr cannot read refuses the subscription: %v", err)
	}
}

// next reads n events within a few seconds, each as "kind pane [status | matched line]".
func next(t *testing.T, s *herdr.Stream, n int) []string {
	t.Helper()
	got := make(chan string, n)
	go func() {
		for i := 0; i < n; i++ {
			e, err := s.Next()
			if err != nil {
				got <- "error " + err.Error()
				return
			}
			word := e.Kind + " " + e.PaneID
			if e.AgentStatus != "" {
				word += " " + e.AgentStatus
			}
			if e.MatchedLine != "" {
				word += " " + e.MatchedLine
			}
			got <- word
		}
	}()
	var out []string
	for len(out) < n {
		select {
		case word := <-got:
			out = append(out, word)
		case <-time.After(5 * time.Second):
			t.Fatalf("waited for %d events, got %v", n, out)
		}
	}
	return out
}
