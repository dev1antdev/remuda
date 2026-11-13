package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/ports"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/usage"
)

// A workspace carries the ports its processes listen on, the contract the sidebar's card and the status bar share;
// one with none carries no field at all.
func TestRosterCarriesTheWorkspacesPorts(t *testing.T) {
	answer := strings.Join([]string{"5173", "[::1]", "4100", "node", "/home/me/app", "/home/me/app", "node node_modules/.bin/vite"}, "\x1f") + "\n"
	host := &shell.Fake{Name: "gog", Remote: true, Answer: func(string) (string, error) { return answer, nil }}
	listening := ports.New(func() []ports.Target {
		return []ports.Target{{ID: "local", Host: host, Folders: map[string]string{"w1": "/home/me/app", "w2": "/home/me/notes"}}}
	}, func() map[int]bool { return nil })
	listening.Touch()
	listening.Sweep()
	deadline := time.Now().Add(2 * time.Second)
	for listening.For("local", "w1") == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "app"}, {WorkspaceID: "w2", Label: "notes"}},
		Tabs:       []herdr.TabInfo{{TabID: "w1:t1", WorkspaceID: "w1", Number: 1}, {TabID: "w2:t1", WorkspaceID: "w2", Number: 1}},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", TabID: "w1:t1", WorkspaceID: "w1", CWD: "/home/me/app"},
			{PaneID: "w2:p1", TabID: "w2:t1", WorkspaceID: "w2", CWD: "/home/me/notes"},
		},
	}
	s := &Server{Radar: radar.New(t.TempDir(), nil), Usage: usage.NewTracker(), Review: review.New(t.TempDir()), Ports: listening}
	got := map[string]string{}
	for _, session := range s.sessionsFromSnapshot("local", snap) {
		body, err := json.Marshal(session)
		if err != nil {
			t.Fatal(err)
		}
		got[session.ID] = string(body)
	}
	if !strings.Contains(got["w1"], `"ports":[{"port":5173,"process":"node","label":"vite","pid":4100,"address":"localhost:5173"}]`) {
		t.Fatalf("the workspace with a server: %s", got["w1"])
	}
	if strings.Contains(got["w2"], `"ports"`) {
		t.Fatalf("a workspace with none carries the field: %s", got["w2"])
	}
}
