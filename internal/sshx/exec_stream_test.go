package sshx

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestAnExecConnCarriesAStreamPastItsDeadline is the `nc -U` bridge holding herdr's event stream: the deadline the
// dial set is cleared once the stream stands, and lines keep coming long after it would have ended the command.
func TestAnExecConnCarriesAStreamPastItsDeadline(t *testing.T) {
	if _, err := exec.LookPath("nc"); err != nil {
		t.Skip("no nc")
	}
	dir, err := os.MkdirTemp("", "nc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		bufio.NewReader(conn).ReadBytes('\n')
		conn.Write([]byte("started\n"))
		for i := 0; i < 3; i++ {
			time.Sleep(150 * time.Millisecond)
			conn.Write([]byte("event\n"))
		}
	}()
	conn, err := newCmdConn(exec.Command("nc", "-U", sock), 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("subscribe\n"))
	reader := bufio.NewReader(conn)
	if line, err := reader.ReadString('\n'); err != nil || line != "started\n" {
		t.Fatalf("the answer: %q %v", line, err)
	}
	conn.SetDeadline(time.Time{})
	for i := 0; i < 3; i++ {
		if line, err := reader.ReadString('\n'); err != nil || line != "event\n" {
			t.Fatalf("event %d, past the dial's deadline: %q %v", i, line, err)
		}
	}
}
