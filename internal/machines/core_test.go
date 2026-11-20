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
	for _, kv := range c.Env {
		key, value, _ := strings.Cut(kv, "=")
		t.Setenv(key, value)
	}
	t.Setenv("PATH", filepath.Dir(c.Bin)+string(os.PathListSeparator)+os.Getenv("PATH"))

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
	if _, _, err := terminals.Subscribe("t1", 80, 24); err == nil || !strings.Contains(err.Error(), NoAttachOnCore) {
		t.Fatalf("attach on remuda-core = %v, want the refusal", err)
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
			return
		}
		if time.Since(start) > 10*time.Second {
			t.Fatalf("the snapshot never showed %s: %v %+v", running.Address, err, snap)
		}
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
