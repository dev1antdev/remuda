package ports

import (
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestStopEndsAServerThisTestStarted(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	_, lsofErr := exec.LookPath("lsof")
	_, ssErr := exec.LookPath("ss")
	if lsofErr != nil && ssErr != nil {
		t.Skip("neither lsof nor ss")
	}
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	server := exec.Command(python, "-m", "http.server", strconv.Itoa(port), "--bind", "127.0.0.1")
	server.Dir = t.TempDir()
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	go func() { ended <- server.Wait() }()
	t.Cleanup(func() { _ = server.Process.Kill() })
	for i := 0; i < 50; i++ {
		if c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
			c.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stopped, err := Stop(shell.Local{Name: "local"}, nil, server.Process.Pid, port)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Port != port || stopped.Label != "http.server" {
		t.Fatalf("stopped %+v", stopped)
	}
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("the server is still running after SIGTERM")
	}
}
