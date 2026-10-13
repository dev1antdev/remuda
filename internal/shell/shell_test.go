package shell

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLocalRunsAndReportsTheExit(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	local := Local{Name: "local"}
	if local.ID() != "local" || !local.Local() || local.Home() == "" {
		t.Fatal("identity")
	}
	out, err := Text(local, "printf 'a b'", time.Second)
	if err != nil || out != "a b" {
		t.Fatalf("run: %q %v", out, err)
	}
	_, err = local.Run("echo oops >&2; exit 3", time.Second)
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 3 || exit.Stderr != "oops" {
		t.Fatalf("exit: %v", err)
	}
	if _, err := local.Run("sleep 5", 50*time.Millisecond); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout: %v", err)
	}
	var streamed []string
	err = local.Stream("echo one; echo two >&2; exit 2", time.Second, func(b []byte) { streamed = append(streamed, string(b)) })
	if !errors.As(err, &exit) || exit.Code != 2 || !strings.Contains(strings.Join(streamed, ""), "one\n") || !strings.Contains(strings.Join(streamed, ""), "two\n") {
		t.Fatalf("stream: %q %v", streamed, err)
	}
}

func TestFakeAnswersAndRemembers(t *testing.T) {
	fake := &Fake{Name: "box", Dir: "/home/x", Remote: true, Answer: func(script string) (string, error) {
		if strings.Contains(script, "fail") {
			return "", &ExitError{Code: 1}
		}
		return "answer to " + script, nil
	}}
	if fake.Local() || fake.Home() != "/home/x" || fake.ID() != "box" {
		t.Fatal("identity")
	}
	if out, err := Text(fake, "ls", time.Second); err != nil || out != "answer to ls" {
		t.Fatalf("run: %q %v", out, err)
	}
	var got string
	if err := fake.Stream("fail", time.Second, func(b []byte) { got += string(b) }); err == nil || got != "" {
		t.Fatalf("stream: %q %v", got, err)
	}
	if s := fake.Scripts(); len(s) != 2 || s[0] != "ls" || s[1] != "fail" {
		t.Fatalf("scripts: %q", s)
	}
}
