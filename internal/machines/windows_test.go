//go:build windows

package machines

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsTheMachineIsReadAsTheConnectionReadsIt runs the connection's probe through the runner's own OpenSSH, Git
// Bash its shell, as a cockpit's master would carry it.
func TestOnWindowsTheMachineIsReadAsTheConnectionReadsIt(t *testing.T) {
	wintest.Live(t)
	answer := wintest.Text(t, wintest.Remote(t), probeScript(true))
	wintest.Record(t, "probe.txt", answer)
	found := parseProbe(answer)
	home, _ := os.UserHomeDir()
	if found.os != "windows" || found.home != home {
		t.Fatalf("the probe read %+v; the home is %s", found, home)
	}
	if found.appData != os.Getenv("APPDATA") || !strings.HasSuffix(found.remuda, "/.local/bin/remuda.exe") || !strings.HasSuffix(found.herdr, "/Programs/Herdr/bin/herdr.exe") {
		t.Fatalf("the probe read %+v", found)
	}
}

// TestOnWindowsHerdrIsReachedThroughRemudaBridgeOverSSH is a cockpit's whole RPC to a Windows herdr: every request one exec of
// remuda.exe bridge over ssh, the event stream one that stays, a shell workspace made, typed into, read and closed.
func TestOnWindowsHerdrIsReachedThroughRemudaBridgeOverSSH(t *testing.T) {
	wintest.Live(t)
	remote := wintest.Remote(t)
	found := parseProbe(wintest.Text(t, remote, probeScript(true)))
	sock := windowsSocket(found.appData)
	client := herdr.NewClient(sock)
	client.Dial = func(timeout time.Duration) (net.Conn, error) {
		return remote.Dial(BridgeCommand(found.remuda, sock), timeout)
	}
	version, protocol, err := client.Ping()
	if err != nil {
		t.Fatalf("ping through the bridge: %v", err)
	}
	t.Logf("herdr %s, protocol %d, at %s", version, protocol, sock)

	stream, err := client.Subscribe([]herdr.Subscription{{Type: "workspace.created"}, {Type: "pane.created"}})
	if err != nil {
		t.Fatalf("the event stream through the bridge: %v", err)
	}
	defer stream.Close()
	made, err := client.WorkspaceCreate(t.TempDir(), "remuda-windows-check")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	pane := made.RootPane.PaneID
	if !strings.HasPrefix(made.RootPane.CWD, `C:\`) && !strings.HasPrefix(made.RootPane.CWD, `D:\`) {
		t.Fatalf("herdr names a Windows folder in Windows' spelling, got %q", made.RootPane.CWD)
	}
	heard := make(chan string, 4)
	go func() {
		for i := 0; i < 2; i++ {
			e, err := stream.Next()
			if err != nil {
				heard <- "error " + err.Error()
				return
			}
			heard <- e.Kind + " " + e.PaneID
		}
	}()
	select {
	case got := <-heard:
		t.Logf("heard %s", got)
	case <-time.After(10 * time.Second):
		t.Fatal("nothing heard on the event stream through the bridge")
	}

	if err := client.PaneSendText(pane, "echo remuda-was-here\r"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		text, err := client.PaneRead(pane)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(text, "remuda-was-here") >= 2 {
			wintest.Record(t, "pane-read.txt", text)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the pane never showed the echo:\n%s", text)
		}
		time.Sleep(300 * time.Millisecond)
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Pane(pane) == nil {
		t.Fatalf("the snapshot through the bridge lacks %s", pane)
	}
}
