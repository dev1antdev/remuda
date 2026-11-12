package herdr

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// throwawayServer starts a herdr server on a socket and HOME of its own and stops it the same way, never the person's.
func throwawayServer(t *testing.T) *Client {
	t.Helper()
	bin, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("no herdr on PATH")
	}
	home, err := os.MkdirTemp("", "hm") // short: a unix socket path has ~100 bytes
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
		cmd.Env = append(os.Environ(), "HERDR_SOCKET="+sock, "HERDR_SOCKET_PATH="+sock)
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
	startMu.Lock()
	spacing := lastStart
	startMu.Unlock()
	err = StartServerNow(bin, sock)
	startMu.Lock()
	lastStart = spacing // a throwaway start says nothing of the two minutes EnsureServer keeps between real ones
	startMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return NewClient(sock)
}

func text(s string) *string { return &s }

func paneOf(t *testing.T, c *Client, paneID string) (*Snapshot, *PaneInfo) {
	t.Helper()
	snap, err := c.SessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	pane := snap.Pane(paneID)
	if pane == nil {
		t.Fatalf("no pane %s in the snapshot", paneID)
	}
	return snap, pane
}

// TestMetadataOnAThrowawayServer writes a pane's and a workspace's display values, reads them back off the snapshot,
// clears one, sees a TTL lapse, and asks for a toast herdr's default config does not show.
func TestMetadataOnAThrowawayServer(t *testing.T) {
	c := throwawayServer(t)
	made, err := c.WorkspaceCreate(os.Getenv("HOME"), "scratch")
	if err != nil {
		t.Fatal(err)
	}
	pane, ws := made.RootPane.PaneID, made.Workspace.WorkspaceID

	err = c.PaneReportMetadata(PaneMetadata{
		Pane: pane, Source: "remuda",
		Tokens: map[string]*string{"remuda_cost": text("$4.20"), "remuda_context": text("62%")},
		Labels: map[string]string{StatusWorking: "stuck 25 min"},
		TTL:    time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WorkspaceReportMetadata(WorkspaceMetadata{Workspace: ws, Source: "remuda", Tokens: map[string]*string{"remuda_review": text("3 to review")}, TTL: 1500 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	snap, info := paneOf(t, c, pane)
	if info.Tokens["remuda_cost"] != "$4.20" || info.Tokens["remuda_context"] != "62%" || info.StateLabels[StatusWorking] != "stuck 25 min" {
		t.Fatalf("pane read back as tokens %v labels %v", info.Tokens, info.StateLabels)
	}
	if got := snap.Workspace(ws).Tokens["remuda_review"]; got != "3 to review" {
		t.Fatalf("workspace token read back as %q", got)
	}

	if err := c.PaneReportMetadata(PaneMetadata{Pane: pane, Source: "remuda", Tokens: map[string]*string{"remuda_context": nil}, ClearLabels: true}); err != nil {
		t.Fatal(err)
	}
	_, info = paneOf(t, c, pane)
	if _, still := info.Tokens["remuda_context"]; still || info.Tokens["remuda_cost"] != "$4.20" || len(info.StateLabels) != 0 {
		t.Fatalf("after clearing one token and the labels: tokens %v labels %v", info.Tokens, info.StateLabels)
	}

	said := "  the answer\twith a tab \n" + strings.Repeat("é", 100)
	if err := c.PaneReportMetadata(PaneMetadata{Pane: pane, Source: "remuda", Tokens: map[string]*string{"remuda_said": text(said)}}); err != nil {
		t.Fatal(err)
	}
	if _, info = paneOf(t, c, pane); info.Tokens["remuda_said"] != DisplayText(said, MaxValueRunes) {
		t.Fatalf("herdr stored %q, DisplayText says %q", info.Tokens["remuda_said"], DisplayText(said, MaxValueRunes))
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		snap, _ := paneOf(t, c, pane)
		if _, still := snap.Workspace(ws).Tokens["remuda_review"]; !still {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the workspace token outlived its TTL")
		}
		time.Sleep(250 * time.Millisecond)
	}

	if err := c.PaneReportMetadata(PaneMetadata{Pane: "w999:p9", Source: "remuda", Tokens: map[string]*string{"remuda_cost": text("$1")}}); !IsCode(err, "pane_not_found") {
		t.Fatalf("a pane gone: %v", err)
	}

	shown, err := c.NotificationShow(Toast{Title: "remuda", Body: "a test", Sound: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if shown.Shown || shown.Reason != "disabled" {
		t.Fatalf("herdr's default config keeps toasts off, yet it answered %+v", shown)
	}
}

func TestDisplayText(t *testing.T) {
	for in, want := range map[string]string{
		"  $4.20 ":             "$4.20",
		"a\tb\nc":              "abc",
		strings.Repeat("x", 9): "xxxxx",
		"ab   cdef":            "ab",
	} {
		if got := DisplayText(in, 5); got != want {
			t.Errorf("DisplayText(%q) = %q, want %q", in, got, want)
		}
	}
}
