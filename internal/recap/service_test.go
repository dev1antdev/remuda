package recap

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestTheStandupReportsOnAgentsOutsideRemudasOwnWorkspaces(t *testing.T) {
	s := &Service{Skip: func(machine, workspace string) bool { return machine == "gog" && workspace == "w9" }}
	for _, c := range []struct {
		machine string
		pane    herdr.PaneInfo
		want    bool
	}{
		{"gog", herdr.PaneInfo{WorkspaceID: "w1", Agent: "claude"}, true},
		{"gog", herdr.PaneInfo{WorkspaceID: "w1"}, false},
		{"gog", herdr.PaneInfo{WorkspaceID: "w9", Agent: "claude"}, false},
		{"local", herdr.PaneInfo{WorkspaceID: "w9", Agent: "codex"}, true},
	} {
		if got := s.reports(c.machine, &c.pane); got != c.want {
			t.Errorf("%s %+v: %v, want %v", c.machine, c.pane, got, c.want)
		}
	}
	if !(&Service{}).reports("gog", &herdr.PaneInfo{WorkspaceID: "w9", Agent: "claude"}) {
		t.Error("without a Skip every agent counts")
	}
}
