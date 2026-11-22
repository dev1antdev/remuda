package machines

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestTheProbeFindsRemudaCoreWhereItsInstallerPutsIt(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func() probe {
		cmd := exec.Command("sh", "-c", probeScript()+coreLookup)
		cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return parseProbe(string(out))
	}
	if found := run(); found.core != "" {
		t.Fatalf("found %q where there is none", found.core)
	}
	if err := os.WriteFile(filepath.Join(bin, "remuda-core"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if found := run(); found.core != filepath.Join(bin, "remuda-core") {
		t.Fatalf("core = %q", found.core)
	}
	if err := errNoCore("gog"); !strings.Contains(err.Error(), "remuda core -host gog install") {
		t.Fatalf("the reason names no way to install it: %v", err)
	}
}

// The local machine of a -core remuda reaches the core `remuda-core connect` finds under its environment: here a
// throwaway one, as a scratch instance beside the real cockpit would.
func TestAPageOnlyHostOnCoreStartsNoCoreOfItsOwn(t *testing.T) {
	m := NewManager(t.TempDir(), nil, true)
	defer m.Close()
	mc, err := m.Get(LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if mc.link != nil || !m.Server() {
		t.Fatal("a page-only host on remuda-core followed a core of its own")
	}
	if _, _, err := mc.Ready(); !errors.Is(err, ErrServerHost) {
		t.Fatalf("Ready on a page-only host = %v", err)
	}
}

func TestTheLocalMachineOnCoreShowsTheCoresTerminals(t *testing.T) {
	c := coretest.Server(t)
	c.Use(t)

	m := NewManager(t.TempDir(), nil, false)
	defer m.Close()
	mc, err := m.Get(LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if !mc.OnCore() {
		t.Fatal("the local machine of a -core remuda is on remuda-core")
	}
	api, terminals, err := mc.Ready()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	running, err := c.Client(t).Run(ctx, core.RunParams{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		snap, err := api.SessionSnapshot()
		if err == nil && snap.Pane(running.Address) != nil {
			break
		}
		if time.Since(start) > 10*time.Second {
			t.Fatalf("the snapshot never showed %s: %v %+v", running.Address, err, snap)
		}
	}
	viewer, err := terminals.Open(running.Address, herdr.View{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("attach on remuda-core: %v", err)
	}
	defer viewer.Close()
	if n := terminals.Viewers(running.Address); n != 1 {
		t.Fatalf("viewers = %d, want 1", n)
	}
	select {
	case first, ok := <-viewer.Out():
		if !ok || len(first.Data) == 0 {
			t.Fatal("the core's first paint never came")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no first paint within 10 s")
	}
}

func TestTheLocalMachineOnCoreSaysHowToInstallIt(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if found, err := herdr.FindBinary("remuda-core"); err == nil {
		t.Skipf("a remuda-core stands in a system folder (%s): dialing would reach this machine's real core", found)
	}
	_, err := dialLocalCore(context.Background())
	if err == nil || !strings.Contains(err.Error(), "remuda core install") {
		t.Fatalf("dial without remuda-core = %v", err)
	}
}

// A machine reached through remuda-core makes a worktree through its own shell: the adapter it is given was lent it.
func TestTheLocalMachineOnCoreMakesAWorktreeWithItsShell(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	c := coretest.Server(t)
	for _, kv := range c.Env {
		key, value, _ := strings.Cut(kv, "=")
		t.Setenv(key, value)
	}
	t.Setenv("PATH", filepath.Dir(c.Bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "no-config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := filepath.Join(home, "app")
	for _, args := range [][]string{{"init", "-q", "-b", "main", repo}, {"-C", repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}

	m := NewManager(t.TempDir(), nil, false)
	defer m.Close()
	mc, err := m.Get(LocalID)
	if err != nil {
		t.Fatal(err)
	}
	api, _, err := mc.Ready()
	if err != nil {
		t.Fatal(err)
	}
	made, err := api.WorkspaceCreate(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	created, err := api.WorktreeCreate(made.Workspace.WorkspaceID, "feat", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".herdr", "worktrees", "app", "feat"); created.Worktree.Path != want || created.Workspace.Worktree == nil || !created.Workspace.Worktree.IsLinkedWorktree {
		t.Fatalf("the worktree is %+v in %+v, want it at %s", created.Worktree, created.Workspace.Worktree, want)
	}
}

func TestTheHerdrBeforeTheMoveIsTheOneRemudaReached(t *testing.T) {
	m := NewManager(t.TempDir(), nil, true)
	t.Cleanup(m.Close)
	m.SetMovedHerdr("/opt/herdr/bin/herdr", "/run/me/herdr.sock")
	local, _ := m.Get(LocalID)
	if bin, socket := local.MovedHerdr(); bin != "/opt/herdr/bin/herdr" || socket != "/run/me/herdr.sock" {
		t.Fatalf("this machine's: %q %q", bin, socket)
	}
}

// A remote's herdr socket is the one remuda dials there, before the move and after it: its remote_socket, else herdr's
// default under the home — never one the ssh environment names; nothing before the machine's first connect.
func TestTheMovedHerdrOfARemoteIsTheSocketRemudaDials(t *testing.T) {
	posix := probe{os: "linux", home: "/home/me"}
	if got := herdrSocket(Config{}, posix); got != "/home/me/.config/herdr/herdr.sock" {
		t.Fatalf("a Mac or Linux: %q", got)
	}
	if got := herdrSocket(Config{RemoteSocket: "/run/h.sock"}, posix); got != "/run/h.sock" {
		t.Fatalf("remote_socket: %q", got)
	}
	redirected := probe{os: "windows", home: `C:\Users\me`, appData: `D:\Profiles\me\AppData\Roaming`}
	if got := herdrSocket(Config{}, redirected); got != `D:\Profiles\me\AppData\Roaming\herdr\herdr.sock` {
		t.Fatalf("a Windows profile kept on another drive: %q", got)
	}
	gog := &Machine{Cfg: Config{ID: "gog", HerdrBin: "/usr/local/bin/herdr"}}
	if bin, socket := gog.MovedHerdr(); bin != "/usr/local/bin/herdr" || socket != "" {
		t.Fatalf("before its first connect: %q %q", bin, socket)
	}
	gog.socket = herdrSocket(gog.Cfg, posix)
	if _, socket := gog.MovedHerdr(); socket != "/home/me/.config/herdr/herdr.sock" {
		t.Fatalf("once connected: %q", socket)
	}
}
