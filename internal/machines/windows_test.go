//go:build windows

package machines

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/sshx"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsTheMachineIsReadAsTheConnectionReadsIt runs the connection's probe through the runner's own OpenSSH, Git
// Bash its shell, as a cockpit's master would carry it.
func TestOnWindowsTheMachineIsReadAsTheConnectionReadsIt(t *testing.T) {
	wintest.Live(t)
	answer := wintest.Text(t, wintest.Remote(t), probeScript()+coreLookup)
	wintest.Record(t, "probe.txt", answer)
	found := parseProbe(answer)
	home, _ := os.UserHomeDir()
	if found.os != "windows" || found.home != home {
		t.Fatalf("the probe read %+v; the home is %s", found, home)
	}
	if !strings.HasSuffix(found.core, "/.local/bin/remuda-core") && !strings.HasSuffix(found.core, "/.local/bin/remuda-core.exe") {
		t.Fatalf("the probe did not find remuda-core where its installer puts it: %q", found.core)
	}
}

// TestOnWindowsALongScriptTravelsOnStdin: a script goes on stdin under a short line (sshx.Remote), and Git Bash behind
// Windows' OpenSSH runs it as it would have run it on that line.
func TestOnWindowsALongScriptTravelsOnStdin(t *testing.T) {
	wintest.Live(t)
	long := "# " + strings.Repeat("x", 20000) + "\n" +
		`say() { printf '%s|' "$1"; }; say "it's"; say "$(uname -s | cut -c1-5)"` + "\n" +
		"cat <<'EOF'\nheredoc $HOME\nEOF\n" +
		`if read -r rest; then echo "read $rest"; else echo "stdin ended"; fi` + "\n"
	if got := wintest.Text(t, wintest.Remote(t), long); got != "it's|MINGW|heredoc $HOME\nstdin ended\n" {
		t.Fatalf("the long script said %q", got)
	}
}

// TestOnWindowsRemudaCoreIsReachedOverSSH is a cockpit's whole way to a Windows PC now: `remuda-core.exe connect` over
// the runner's OpenSSH, Git Bash its ssh shell; a terminal started in it, the snapshot read through the adapter every
// package takes, and the terminal attached as a viewer.
func TestOnWindowsRemudaCoreIsReachedOverSSH(t *testing.T) {
	wintest.Live(t)
	remote := wintest.Remote(t)
	found := parseProbe(wintest.Text(t, remote, probeScript()+coreLookup))
	if found.core == "" {
		t.Fatal("remuda-core is not installed: run remuda core install there")
	}
	t.Logf("remuda-core at %s", found.core)
	command := sshx.RemoteCommand("exec " + script.Quote(found.core) + " connect")
	client := core.New(func(context.Context) (net.Conn, error) {
		return remote.Dial(command, 20*time.Second)
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	hello, err := client.Hello(ctx)
	if err != nil && strings.Contains(err.Error(), "elevated Windows core over SSH") {
		// this runner's sshd admits only administrators, and a core refuses to start in such a session while a desktop one
		// runs: it is started here the way its app does, and the ssh connection then follows it as it would on a PC whose
		// sshd is the person's own
		t.Logf("the runner's ssh is elevated; starting the core in this session first: %v", err)
		starting := exec.Command(found.core, "server", "start")
		if err := starting.Start(); err != nil {
			t.Fatalf("start the core in this session: %v", err)
		}
		go starting.Wait()
		for begin := time.Now(); err != nil && time.Since(begin) < 30*time.Second; time.Sleep(time.Second) {
			hello, err = client.Hello(ctx)
		}
	}
	if err != nil {
		t.Fatalf("hello through remuda-core.exe connect over ssh: %v", err)
	}
	t.Logf("remuda-core %s answers", hello.Version)

	running, err := client.Run(ctx, core.RunParams{Argv: []string{"cmd.exe", "/k", "echo rmd-core-ok"}, Size: &core.Size{Cols: 100, Rows: 30}})
	if err != nil {
		t.Fatalf("a terminal in the core: %v", err)
	}
	defer client.Kill(context.Background(), running.Address)
	if !strings.Contains(running.Cwd, `:\`) {
		t.Fatalf("the core names a Windows folder in Windows' spelling, got %q", running.Cwd)
	}

	adapter := core.NewAdapter(client, wintest.Host(t), core.AdapterHooks{})
	defer adapter.Close()
	snap, err := adapter.SessionSnapshot()
	if err != nil {
		t.Fatalf("the snapshot through the adapter: %v", err)
	}
	if snap.Pane(running.Address) == nil {
		t.Fatalf("the snapshot lacks %s", running.Address)
	}

	viewer, err := core.NewTerminals(client).Open(running.Address, herdr.View{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	defer viewer.Close()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case out, ok := <-viewer.Out():
			if ok && len(out.Data) > 0 {
				t.Logf("the attach painted %d bytes", len(out.Data))
				return
			}
			if !ok {
				t.Fatal("the attach ended before it painted")
			}
		case <-deadline:
			t.Fatal("the attach painted nothing in 20 s")
		}
	}
}
