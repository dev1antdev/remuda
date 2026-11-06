package sshx

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestAnExecConnTellsWhatTheCommandSaid: a failing read carries the command's stderr — written by exec's own
// goroutine while the read fails, so behind a lock — and the deadline ends the command.
func TestAnExecConnTellsWhatTheCommandSaid(t *testing.T) {
	conn, err := newCmdConn(exec.Command("sh", "-c", "echo nope >&2; sleep 0.2; exit 1"), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	buf := make([]byte, 64)
	if _, err := conn.Read(buf); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Read = %v, want the command's words", err)
	}

	slow, err := newCmdConn(exec.Command("sh", "-c", "sleep 5"), 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	if _, err := slow.Read(buf); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Read past the deadline = %v, want os.ErrDeadlineExceeded", err)
	}
}
