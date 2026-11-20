package machines

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core/coretest"
)

// remoteOnCore connects a remote on remuda-core through an `ssh` stand-in first on PATH: a master that stays, and every
// command it carries run here, under the test's environment, as the machine would run it.
func remoteOnCore(t *testing.T) *Machine {
	t.Helper()
	dir := t.TempDir()
	stub := `#!/bin/sh
case " $* " in
*" -G "*) exit 1 ;;
*" -M "*) while [ "$1" != -S ]; do shift; done; : > "$2"; exec sleep 30 ;;
*" -O "*) exit 0 ;;
esac
for line; do :; done
eval "$line"
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mc := newRemote(Config{ID: "studio", Label: "Studio", Host: "core-test-" + filepath.Base(dir)})
	mc.speaksCore = true
	mc.connect()
	t.Cleanup(func() { mc.fail(errors.New("the test is over")) })
	return mc
}

// A machine the fleet moved to remuda-core before its core was installed keeps its master: the vitals and Updates'
// Install remuda-core reach it, and Ready says why no workspace can.
func TestARemoteWithoutRemudaCoreStaysConnected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mc := remoteOnCore(t)
	if state, why := mc.State(); state != StateConnected {
		t.Fatalf("state = %s (%s), want connected over the master", state, why)
	}
	api, terminals, err := mc.Ready()
	if err == nil || api != nil || terminals != nil {
		t.Fatalf("Ready without a core = %v, %v, %v", api, terminals, err)
	}
	if !strings.Contains(err.Error(), "remuda-core is not installed on") || !strings.Contains(err.Error(), "Install remuda-core") {
		t.Fatalf("Ready says %q", err)
	}
	if out, err := mc.Run("echo vitals", 10*time.Second); err != nil || strings.TrimSpace(string(out)) != "vitals" {
		t.Fatalf("a script over the master = %q, %v", out, err)
	}
	mc.RetryCore()
	if state, _ := mc.State(); state != StateConnected {
		t.Fatalf("looking for the core again left the machine %s", state)
	}
	if _, _, again := mc.Ready(); again == nil || again.Error() != err.Error() {
		t.Fatalf("Ready after another look = %v", again)
	}
}

// Once a job installed it, the core is followed at once over the same master.
func TestARemoteFollowsRemudaCoreOnceItIsInstalled(t *testing.T) {
	c := coretest.Server(t)
	home := ""
	for _, kv := range c.Env {
		key, value, _ := strings.Cut(kv, "=")
		t.Setenv(key, value)
		if key == "HOME" {
			home = value
		}
	}
	mc := remoteOnCore(t)
	if _, _, err := mc.Ready(); err == nil {
		t.Fatal("a core was followed before one was installed")
	}
	mc.mu.Lock()
	master := mc.conn
	mc.mu.Unlock()

	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(c.Bin, filepath.Join(bin, "remuda-core")); err != nil {
		t.Fatal(err)
	}
	mc.RetryCore()
	api, terminals, err := mc.Ready()
	if err != nil || api == nil || terminals == nil {
		t.Fatalf("Ready once the core is there = %v, %v, %v", api, terminals, err)
	}
	if _, err := api.SessionSnapshot(); err != nil {
		t.Fatalf("the core's snapshot: %v", err)
	}
	if conn, _ := mc.RemoteConn(); conn != master {
		t.Fatal("following the core dialed another master")
	}
}
