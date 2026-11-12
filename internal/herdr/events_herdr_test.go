package herdr

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// throwawayServer starts a herdr server of the test's own: its socket and its HOME both temporary, since herdr
// restores the saved session of the HOME it runs under — the person's workspaces, their agents resumed — and saves its
// own over it. It is stopped through its socket when the test ends.
func throwawayServer(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("no herdr on PATH")
	}
	home, err := os.MkdirTemp("", "hs") // short: a unix socket path has ~100 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	sock := filepath.Join(home, "herdr.sock")
	t.Setenv("HERDR_SOCKET", sock)
	t.Setenv("HERDR_SOCKET_PATH", sock)
	t.Cleanup(func() {
		defer os.RemoveAll(home)
		cmd := exec.Command(bin, "server", "stop")
		cmd.Env = append(os.Environ(), "HOME="+home, "HERDR_SOCKET="+sock, "HERDR_SOCKET_PATH="+sock)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("stop: %v %s", err, out)
		}
		for i := 0; i < 20; i++ {
			if _, _, err := NewClient(sock).Ping(); err != nil {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Errorf("the throwaway server still answers at %s", sock)
	})
	if err := StartServerNow(bin, sock); err != nil {
		t.Fatal(err)
	}
	return sock
}

// TestEventsFromAThrowawayHerdr is the protocol as herdr 0.9.5 speaks it: a pane made shows up on a global
// subscription, its agent's status and an output match on the ones bound to it.
func TestEventsFromAThrowawayHerdr(t *testing.T) {
	sock := throwawayServer(t)
	client := NewClient(sock)
	global, err := client.Subscribe([]Subscription{{Type: "workspace.created"}, {Type: "pane.created"}, {Type: "pane.closed"}})
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

	bound, err := client.Subscribe([]Subscription{
		{Type: EventAgentStatus, PaneID: pane},
		{Type: EventOutputMatched, PaneID: pane, Source: "recent", Lines: 40, Match: &OutputMatch{Type: "regex", Value: `^\s*Local:\s+https?://`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if err := client.Call("pane.report_agent", map[string]any{"pane_id": pane, "source": "remuda-test", "agent": "claude", "state": "working"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := next(t, bound, 1); got[0] != EventAgentStatus+" "+pane+" working" {
		t.Fatalf("the agent's status: %v", got)
	}
	if err := client.PaneSendText(pane, `printf '\n  Local:   http://localhost:5173/\n'`+"\r"); err != nil {
		t.Fatal(err)
	}
	if got := next(t, bound, 1); got[0] != EventOutputMatched+" "+pane+"   Local:   http://localhost:5173/" {
		t.Fatalf("the output match: %v", got)
	}

	if _, err := client.Subscribe([]Subscription{{Type: EventAgentStatus, PaneID: "w999:p9"}}); !IsCode(err, "pane_not_found") {
		t.Fatalf("a pane that is not there refuses the subscription: %v", err)
	}
	if _, err := client.Subscribe([]Subscription{{Type: EventOutputMatched, PaneID: pane, Source: "recent", Match: &OutputMatch{Type: "regex", Value: `(?=x)`}}}); !IsCode(err, "invalid_regex") {
		t.Fatalf("a regex herdr cannot read refuses the subscription: %v", err)
	}
}

// next reads n events within a few seconds, each as "kind pane [status | matched line]".
func next(t *testing.T, s *Stream, n int) []string {
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
