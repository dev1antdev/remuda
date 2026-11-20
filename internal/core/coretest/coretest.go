// Package coretest starts a remuda-core of a test's own, for the tests that speak its protocol for real.
package coretest

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
)

// Core is a throwaway core: its socket, state, config and HOME all in a temporary folder.
type Core struct {
	Socket string
	// Env is the environment it runs under, for a `remuda-core` command aimed at it.
	Env []string
	Bin string

	root    string
	process *os.Process
	exited  chan struct{}
}

// Server starts a core and stops it when the test ends, skipping the test without the binary: RC_BIN, else
// remuda-core on PATH. It never reaches a core of the person's: RC_SESSION, RC_SOCKET and the XDG folders are its
// own, and RC_LAUNCHD_DIRECT keeps macOS from starting it as a launchd job of the person's session instead.
func Server(t testing.TB) *Core {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("coretest runs a core on a unix socket")
	}
	bin := os.Getenv("RC_BIN")
	if bin == "" {
		found, err := exec.LookPath("remuda-core")
		if err != nil {
			t.Skip("no remuda-core on PATH and no RC_BIN")
		}
		bin = found
	}
	root, err := os.MkdirTemp("", "rc") // short: a unix socket path has ~100 bytes
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"run", "state", "config"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	c := &Core{Socket: filepath.Join(root, "c.sock"), Bin: bin, root: root}
	c.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + root,
		"SHELL=/bin/sh",
		"RC_SESSION=test",
		"RC_SOCKET=" + c.Socket,
		"XDG_RUNTIME_DIR=" + filepath.Join(root, "run"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
		"RC_LAUNCHD_DIRECT=" + strconv.Itoa(os.Getpid()),
		"RC_HEARTBEAT_MS=300",
	}
	t.Cleanup(func() {
		c.stop(t)
		if t.Failed() {
			said, _ := os.ReadFile(c.logPath())
			t.Logf("core.log:\n%s", said)
		}
		os.RemoveAll(root)
	})
	c.serve(t)
	return c
}

// Restart stops the core and starts it again on the same state, as a reboot or the core's own stop would: its kept
// terminals come back with new shells.
func (c *Core) Restart(t testing.TB) {
	t.Helper()
	c.stop(t)
	c.serve(t)
}

func (c *Core) logPath() string { return filepath.Join(c.root, "core.log") }

// serve starts `remuda-core serve` and waits for its socket.
func (c *Core) serve(t testing.TB) {
	t.Helper()
	log, err := os.OpenFile(c.logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	serve := exec.Command(c.Bin, "serve")
	serve.Env, serve.Dir, serve.Stdout, serve.Stderr = c.Env, c.root, log, log
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		serve.Wait()
		log.Close()
		close(exited)
	}()
	c.process, c.exited = serve.Process, exited
	for start := time.Now(); ; time.Sleep(20 * time.Millisecond) {
		if probe, err := net.Dial("unix", c.Socket); err == nil {
			probe.Close()
			return
		}
		select {
		case <-exited:
			said, _ := os.ReadFile(c.logPath())
			t.Fatalf("remuda-core serve exited: %s", said)
		default:
		}
		if time.Since(start) > 15*time.Second {
			t.Fatal("remuda-core did not start within 15 s")
		}
	}
}

// Use aims the test process's own `remuda-core` commands at the core, as a remuda started with -core reaches its
// machine's: the core's environment set for the test, its binary first on PATH.
func (c *Core) Use(t testing.TB) {
	for _, kv := range c.Env {
		name, value, _ := strings.Cut(kv, "=")
		t.Setenv(name, value)
	}
	t.Setenv("PATH", filepath.Dir(c.Bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Dial reaches the core's socket.
func (c *Core) Dial(ctx context.Context) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "unix", c.Socket)
}

// Client is a client of the core, closed when the test ends.
func (c *Core) Client(t testing.TB) *core.Client {
	client := core.New(c.Dial)
	t.Cleanup(func() { client.Close() })
	return client
}

// stop asks the core to stop on a plain connection of its own, which ends every terminal it holds (a kept one waits
// for the next start), and waits for it; one that does not stop in time is killed.
func (c *Core) stop(t testing.TB) {
	exited, process := c.exited, c.process
	if exited == nil {
		return
	}
	c.exited = nil
	if plain, err := net.DialTimeout("unix", c.Socket, 5*time.Second); err == nil {
		plain.SetDeadline(time.Now().Add(10 * time.Second))
		plain.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"server.stop","params":{}}` + "\n"))
		bufio.NewReader(plain).ReadString('\n')
		plain.Close()
	}
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		t.Errorf("remuda-core did not stop within 15 s: killed")
		process.Kill()
		<-exited
	}
}
