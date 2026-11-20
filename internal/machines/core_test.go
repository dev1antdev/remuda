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
		cmd := exec.Command("sh", "-c", probeScript(false)+coreLookup)
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
	m := NewCoreManager(t.TempDir(), "", nil, true)
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

	m := NewCoreManager(t.TempDir(), "", nil, false)
	defer m.Close()
	mc, err := m.Get(LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if !mc.OnCore() || mc.Peer() != nil {
		t.Fatal("the local machine of a -core remuda is on remuda-core, with no peer to relay through")
	}
	if _, err := mc.StopServer(); !errors.Is(err, ErrOnCore) {
		t.Fatalf("herdr's server stop on remuda-core = %v, want ErrOnCore", err)
	}
	if err := mc.StartServer(); !errors.Is(err, ErrOnCore) {
		t.Fatalf("herdr's server start on remuda-core = %v, want ErrOnCore", err)
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
	var refused *herdr.RefusedError
	if errors.As(err, &refused) {
		t.Fatal("a missing core is no refusal of herdr's")
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

	m := NewCoreManager(t.TempDir(), "", nil, false)
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
