package herdrollback

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestServingReadsTheStatusScriptsLastWord(t *testing.T) {
	for _, try := range []struct {
		answer string
		err    error
		up     bool
		fails  bool
	}{
		{answer: "running\n", up: true},
		{answer: "stopped\n"},
		{answer: "none\n"}, // herdr is not installed there
		{answer: "sh: herdr: Permission denied\nunknown\n", fails: true},
		{answer: "", err: errors.New("ssh: the master is gone"), fails: true},
	} {
		host := &shell.Fake{Answer: func(string) (string, error) { return try.answer, try.err }}
		up, err := Serving(host, "", "")
		if up != try.up || (err != nil) != try.fails {
			t.Errorf("%q %v: up %v, err %v", try.answer, try.err, up, err)
		}
		run := host.Scripts()[0]
		if !strings.Contains(run, `status server`) || strings.Contains(run, "server stop") || strings.Contains(run, "HERDR_SOCKET_PATH=") {
			t.Fatalf("the status script asks herdr on its own socket and stops nothing:\n%s", run)
		}
	}
	host := &shell.Fake{Answer: func(string) (string, error) { return "stopped\n", nil }}
	if _, err := Serving(host, "/opt/herdr/bin/herdr", "/run/herdr me.sock"); err != nil {
		t.Fatal(err)
	}
	if run := host.Scripts()[0]; !strings.Contains(run, "h='/opt/herdr/bin/herdr'") || !strings.Contains(run, "export HERDR_SOCKET_PATH='/run/herdr me.sock'") || strings.Contains(run, "command -v herdr") {
		t.Fatalf("the herdr remuda reached there, by its binary and socket:\n%s", run)
	}
}

// standIn is a herdr of the test's own at an explicit path, so the scripts run for real in sh and never find a herdr
// of the person's: it says its server's state from a file and its stop writes that file.
func standIn(t *testing.T, state string) (bin, file string) {
	t.Helper()
	dir := t.TempDir()
	file = filepath.Join(dir, "state")
	if err := os.WriteFile(file, []byte(state+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin = filepath.Join(dir, "herdr")
	script := "#!/bin/sh\ncase \"$1 $2\" in\n\"status server\") cat '" + file + "' ;;\n\"server stop\") echo 'status: not running' > '" + file + "'; echo 'server stopped' ;;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, file
}

func TestTheScriptsGoByHerdrsOwnWordInSh(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "herdr.sock")
	bin, file := standIn(t, "status: running")
	if up, err := Serving(shell.Local{}, bin, socket); !up || err != nil {
		t.Fatalf("running: %v %v", up, err)
	}
	if err := StopServing(shell.Local{}, bin, socket); err != nil {
		t.Fatal(err)
	}
	if said, _ := os.ReadFile(file); !strings.Contains(string(said), "not running") {
		t.Fatalf("the stop was not asked: %s", said)
	}
	if up, err := Serving(shell.Local{}, bin, socket); up || err != nil {
		t.Fatalf("after the stop: %v %v", up, err)
	}

	gone := filepath.Join(t.TempDir(), "herdr")
	if _, err := Serving(shell.Local{}, gone, socket); err == nil {
		t.Fatal("a herdr that would not run is no server stopped")
	}
	if err := StopServing(shell.Local{}, gone, socket); err == nil || !strings.Contains(err.Error(), "cannot tell") {
		t.Fatalf("a stop nobody could confirm: %v", err)
	}
	mumbling, _ := standIn(t, "error: the socket is not a socket")
	if _, err := Serving(shell.Local{}, mumbling, socket); err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Fatalf("herdr saying neither: %v", err)
	}
}

func TestStopServingSaysWhatCameOfIt(t *testing.T) {
	host := &shell.Fake{Answer: func(string) (string, error) { return "herdr server stopped\nstopped\n", nil }}
	if err := StopServing(host, "", ""); err != nil {
		t.Fatal(err)
	}
	if run := host.Scripts()[0]; !strings.Contains(run, `"$h" server stop`) || !strings.Contains(run, "while state") {
		t.Fatalf("the stop script stops the server and waits for it:\n%s", run)
	}
	if err := StopServing(&shell.Fake{Answer: func(string) (string, error) { return "none\n", nil }}, "", ""); err != nil {
		t.Fatalf("no herdr there is nothing to stop: %v", err)
	}
	lingers := StopServing(&shell.Fake{Answer: func(string) (string, error) { return "running\n", nil }}, "", "")
	if lingers == nil || !strings.Contains(lingers.Error(), "still answers") {
		t.Fatalf("a server outliving the wait: %v", lingers)
	}
	if err := StopServing(&shell.Fake{Answer: func(string) (string, error) { return "", errors.New("exit status 255") }}, "", ""); err == nil || !strings.Contains(err.Error(), "exit status 255") {
		t.Fatalf("a script that failed: %v", err)
	}
}
