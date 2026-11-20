package herdr

import (
	"errors"
	"os/exec"
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
		{answer: "", err: errors.New("ssh: the master is gone"), fails: true},
		{answer: "sh: syntax error\n", fails: true},
	} {
		host := &shell.Fake{Answer: func(string) (string, error) { return try.answer, try.err }}
		up, err := Serving(host)
		if up != try.up || (err != nil) != try.fails {
			t.Errorf("%q %v: up %v, err %v", try.answer, try.err, up, err)
		}
		run := host.Scripts()[0]
		if !strings.Contains(run, `status server`) || strings.Contains(run, "server stop") || strings.Contains(run, "HERDR_SOCKET_PATH=") {
			t.Fatalf("the status script asks herdr on its own socket and stops nothing:\n%s", run)
		}
	}
}

// The scripts are read by sh without being run: a test never asks or stops a herdr of the person's.
func TestTheServingScriptsAreShell(t *testing.T) {
	for _, s := range []string{servingScript, stopScript} {
		if out, err := exec.Command("sh", "-n", "-c", s).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s\n%s", err, out, s)
		}
	}
}

func TestStopServingStopsHerdrsServerAndWaitsForIt(t *testing.T) {
	host := &shell.Fake{Answer: func(string) (string, error) { return "herdr server stopped\nstopped\n", nil }}
	if err := StopServing(host); err != nil {
		t.Fatal(err)
	}
	run := host.Scripts()[0]
	if !strings.Contains(run, `"$h" server stop`) || !strings.Contains(run, "while up") {
		t.Fatalf("the stop script stops the server and waits for it:\n%s", run)
	}
	if err := StopServing(&shell.Fake{Answer: func(string) (string, error) { return "none\n", nil }}); err != nil {
		t.Fatalf("no herdr there is nothing to stop: %v", err)
	}
	lingers := StopServing(&shell.Fake{Answer: func(string) (string, error) { return "running\n", nil }})
	if lingers == nil || !strings.Contains(lingers.Error(), "still answers") {
		t.Fatalf("a server outliving the wait: %v", lingers)
	}
	if err := StopServing(&shell.Fake{Answer: func(string) (string, error) { return "", errors.New("exit status 255") }}); err == nil || !strings.Contains(err.Error(), "exit status 255") {
		t.Fatalf("a script that failed: %v", err)
	}
}
