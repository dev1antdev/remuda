//go:build windows

package machines

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/coreapi"
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
	// a core of its own, as coretest.Server gives a test core: its own session, its own named pipe and its own config and
	// state folders, so a socket the person's config names, or an RC_* a desktop shell carries, can reach no core of theirs
	session := fmt.Sprintf("remuda-wintest-%d", os.Getpid())
	conf, state := t.TempDir(), t.TempDir()
	// RC_SOCKET empty, said by a shell that inherits one for a core of its own: the core ignores an empty value and takes
	// the pipe its session names
	own := []string{"RC_SESSION=" + session, "RC_SOCKET=", "XDG_CONFIG_HOME=" + conf, "XDG_STATE_HOME=" + state}
	envWords := func(env []string) string {
		words := "exec env"
		for _, kv := range env {
			k, v, _ := strings.Cut(kv, "=")
			words += " " + k + "=" + script.Quote(v)
		}
		return words
	}
	isolated := func(env []string) []string {
		kept := make([]string, 0, len(env)+len(own))
		for _, kv := range env {
			switch {
			case strings.HasPrefix(kv, "RC_SESSION="), strings.HasPrefix(kv, "RC_SOCKET="),
				strings.HasPrefix(kv, "XDG_CONFIG_HOME="), strings.HasPrefix(kv, "XDG_STATE_HOME="):
				continue
			}
			kept = append(kept, kv)
		}
		return append(kept, own...)
	}
	stopCommand := envWords(own) + " " + script.Quote(found.core) + " server stop"
	defer func() {
		// the core is the test's, whichever way it started: it is stopped here, before this session goes, and the stop
		// waits for its own ssh to end
		if out, err := remote.Run(stopCommand, 30*time.Second); err != nil {
			t.Logf("stopping the test's core: %v\n%s", err, out)
		}
	}()
	command := sshx.RemoteCommand(envWords(own) + " " + script.Quote(found.core) + " connect")
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
		// Go needs Windows' own spelling of the binary, Git Bash's /c/... spelling is not an executable path to it
		winPath, pathErr := remote.Raw("cygpath -w " + script.Quote(found.core))
		if pathErr != nil || strings.TrimSpace(winPath) == "" {
			t.Fatalf("remuda-core's Windows path: %v %q", pathErr, winPath)
		}
		core := strings.TrimSpace(winPath)
		var said strings.Builder
		starting := exec.Command(core, "server", "start")
		starting.Env = isolated(os.Environ())
		starting.Stdout, starting.Stderr = &said, &said
		if err := starting.Start(); err != nil {
			t.Fatalf("start the core in this session: %v", err)
		}
		go starting.Wait()
		for begin := time.Now(); err != nil && time.Since(begin) < 30*time.Second; time.Sleep(time.Second) {
			hello, err = client.Hello(ctx)
		}
		if err != nil {
			t.Fatalf("the core started in this session did not answer over ssh: %v\n%s", err, said.String())
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

	viewer, err := core.NewTerminals(client).Open(running.Address, coreapi.View{Cols: 80, Rows: 24})
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
