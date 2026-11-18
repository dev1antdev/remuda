package sshx

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
)

// limitedSSH stands in for ssh: it notes how many sessions run together, a word in the command says what it does.
func limitedSSH(t *testing.T) (peak func() int) {
	t.Helper()
	dir := t.TempDir()
	running, log := filepath.Join(dir, "running"), filepath.Join(dir, "log")
	if err := os.Mkdir(running, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\nme=" + script.Quote(running) + "/$$\n: > \"$me\"\n" +
		"ls " + script.Quote(running) + " | wc -l >> " + script.Quote(log) + "\n" +
		"trap 'rm -f \"$me\"' EXIT\n" +
		"case \"$*\" in\n" +
		"*refuse*) echo 'mux_client_request_session: session request failed: Session open refused by peer' >&2; exit 255 ;;\n" +
		"*quick*) echo done ;;\n" +
		"*hold*) rm -f \"$me\"; exec sleep 30 ;;\n" +
		"*) sleep 0.6 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		most := 0
		for _, field := range strings.Fields(string(data)) {
			if n, _ := strconv.Atoi(field); n > most {
				most = n
			}
		}
		return most
	}
}

func limited() *Conn { return &Conn{Host: "gog.remote", ctl: "/tmp/remuda-ssh-test.ctl"} }

func TestScriptsWaitForAPlace(t *testing.T) {
	peak := limitedSSH(t)
	c := limited()
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Run("work", 20*time.Second)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if got := peak(); got > maxScripts {
		t.Errorf("%d scripts ran at once, the limit is %d", got, maxScripts)
	}
}

// TestScriptsLeaveRoomForRPC: herdr's requests go on however long the scripts take.
func TestScriptsLeaveRoomForRPC(t *testing.T) {
	limitedSSH(t)
	c := limited()
	var wg sync.WaitGroup
	for range maxScripts + 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Run("work", 20*time.Second)
		}()
	}
	defer wg.Wait()
	time.Sleep(100 * time.Millisecond)
	conn, err := c.DialExec("quick", 200*time.Millisecond) // a place at once, with every script's taken
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}

// TestAPlaceIsHeldUntilTheConnectionCloses, and waiting for one is bounded and said.
func TestAPlaceIsHeldUntilTheConnectionCloses(t *testing.T) {
	limitedSSH(t)
	c := limited()
	var held []io.Closer
	for range maxSessions {
		conn, err := c.DialExec("hold", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
	}
	defer func() {
		for _, conn := range held {
			conn.Close()
		}
	}()
	if _, err := c.DialExec("quick", 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "no session free") {
		t.Fatalf("a request past the limit: %v", err)
	}
	if _, err := c.Run("quick", 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "no session free") {
		t.Fatalf("a script past the limit: %v", err)
	}
	held[0].Close()
	held[0].Close() // closing twice gives one place back
	if _, err := c.Run("quick", 2*time.Second); err != nil {
		t.Fatalf("a place given back: %v", err)
	}
	again, err := c.DialExec("quick", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("the place was not free again: %v", err)
	}
	held = append(held, again)
}

// TestTheWaitComesOutOfTheTimeout: a caller's bound holds, queue included.
func TestTheWaitComesOutOfTheTimeout(t *testing.T) {
	limitedSSH(t)
	c := limited()
	leave := make([]func(), 0, maxScripts)
	for range maxScripts {
		out, _, err := c.enter(true, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		leave = append(leave, out)
	}
	time.AfterFunc(300*time.Millisecond, leave[0])
	defer func() {
		for _, out := range leave[1:] {
			out()
		}
	}()
	began := time.Now()
	if _, err := c.Run("hold", 600*time.Millisecond); err == nil {
		t.Fatal("the script outlived its timeout")
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("a 600ms call took %s", took)
	}
}

func TestARefusedSessionIsSaidInWords(t *testing.T) {
	limitedSSH(t)
	c := limited()
	_, err := c.Run("refuse", 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "MaxSessions") {
		t.Errorf("a script: %v", err)
	}
	if err := c.Stream("refuse", 5*time.Second, func([]byte) {}); err == nil || !strings.Contains(err.Error(), "MaxSessions") {
		t.Errorf("a stream: %v", err)
	}
	conn, err := c.DialExec("refuse", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Read(make([]byte, 8)); err == nil {
		t.Fatal("no error")
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := conn.Read(make([]byte, 8)); err == nil || !strings.Contains(err.Error(), "MaxSessions") {
		t.Errorf("a request: %v", err)
	}
}
