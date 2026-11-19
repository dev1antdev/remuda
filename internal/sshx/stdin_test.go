package sshx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// fakeDropbear puts an `ssh` first on PATH that runs its last word here, stdin and all, and refuses a command past
// 9000 bytes as Dropbear does — by dropping the connection; it keeps every command line it was given.
func fakeDropbear(t *testing.T) (lines string) {
	t.Helper()
	return fakeLogin(t, "sh")
}

// fakeLogin is fakeDropbear with the account's login shell given: sshd runs the line through it.
func fakeLogin(t *testing.T, login string) (lines string) {
	t.Helper()
	dir := t.TempDir()
	lines = filepath.Join(dir, "lines")
	stub := `#!/bin/sh
case "$*" in *" -G "*) exit 1 ;; esac
for line; do :; done
printf '%s\n' "${#line}" >> ` + script.Quote(lines) + `
if [ "${#line}" -gt 9000 ]; then echo "Exit: String too long" >&2; exit 255; fi
exec ` + script.Quote(login) + ` -c "$line"
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return lines
}

// longScript is a script past any command line Dropbear takes, with what a script on the command line has: quotes,
// a function, a here-document, its own exit, and a stdin at its end.
func longScript(exit int) string {
	return "# " + strings.Repeat("x", 20000) + "\n" +
		`say() { printf '%s|' "$1"; }; say "it's"; say "$(printf 'a b')"` + "\n" +
		"cat <<'EOF'\nheredoc $HOME\nEOF\n" +
		`if read -r rest; then echo "read $rest"; else echo "stdin ended"; fi` + "\n" +
		"exit " + strconv.Itoa(exit) + "\n"
}

func lengths(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

// TestALongScriptTravelsOnStdin: a script whose command line Dropbear would refuse — dropping the master and the event
// stream with it — goes on the session's stdin under a short command line, and runs as it would have on that line.
func TestALongScriptTravelsOnStdin(t *testing.T) {
	lines := fakeDropbear(t)
	c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
	out, err := c.Run(longScript(0), 10*time.Second)
	if err != nil {
		t.Fatalf("the long script failed: %v (%s)", err, out)
	}
	if string(out) != "it's|a b|heredoc $HOME\nstdin ended\n" {
		t.Fatalf("the long script said %q", out)
	}
	if got := lengths(t, lines); len(got) != 1 || got[0] != strconv.Itoa(len(readScript)) {
		t.Fatalf("command lines of %v bytes, want one of %d", got, len(readScript))
	}
	_, err = c.Run(longScript(3), 10*time.Second)
	var exit *shell.ExitError
	if !errors.As(err, &exit) || exit.Code != 3 {
		t.Fatalf("the script's own exit was lost: %v", err)
	}
}

// TestAStreamedLongScriptTravelsOnStdin: an update's job is a stream; past the command line's length it goes on stdin too.
func TestAStreamedLongScriptTravelsOnStdin(t *testing.T) {
	fakeDropbear(t)
	c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
	var said strings.Builder
	err := c.Stream(longScript(2)+"echo never", 10*time.Second, func(chunk []byte) { said.Write(chunk) })
	var exit *shell.ExitError
	if !errors.As(err, &exit) || exit.Code != 2 {
		t.Fatalf("the stream's exit: %v", err)
	}
	if said.String() != "it's|a b|heredoc $HOME\nstdin ended\n" {
		t.Fatalf("the stream said %q", said.String())
	}
}

// TestEveryScriptTravelsOnStdin: a short script and one with data for its stdin go on stdin too, under a line any login
// shell reads as written.
func TestEveryScriptTravelsOnStdin(t *testing.T) {
	lines := fakeDropbear(t)
	c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
	short := `printf '%s' "$HOME" >/dev/null; echo short`
	if out, err := c.Run(short, 10*time.Second); err != nil || string(out) != "short\n" {
		t.Fatalf("the short script: %q, %v", out, err)
	}
	withInput := `IFS= read -r key; echo "got $key"; cat`
	if out, err := c.RunWithInput(withInput, []byte("k-1\nrest\n"), 10*time.Second); err != nil || string(out) != "got k-1\nrest\n" {
		t.Fatalf("the script with input: %q, %v", out, err)
	}
	withLine, _ := RemoteWithInput(withInput, strings.NewReader(""))
	if got := lengths(t, lines); len(got) != 2 || got[0] != strconv.Itoa(len(readScript)) || got[1] != strconv.Itoa(len(withLine)) {
		t.Fatalf("command lines of %v bytes", got)
	}
	for _, line := range []string{readScript, withLine} {
		if strings.ContainsAny(line, "\\\n") || strings.Count(line, "'") != 2 {
			t.Errorf("a login shell may read %q otherwise", line)
		}
	}
}

// TestALoginShellOfAnotherKindRunsTheScriptAsWritten: sshd hands the line to the account's own shell; tcsh refuses a
// line break inside quotes and fish reads backslashes there, so a script on the line broke under either.
func TestALoginShellOfAnotherKindRunsTheScriptAsWritten(t *testing.T) {
	for _, login := range []string{"tcsh", "fish"} {
		path, err := exec.LookPath(login)
		if err != nil {
			continue
		}
		fakeLogin(t, path)
		c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
		written := "printf '%s\\n' 'a\\b' \"it's\"\nprintf '\\036x\\n' | od -An -c | tr -d ' \\n'; echo\n"
		out, err := c.Run(written, 10*time.Second)
		if err != nil || string(out) != "a\\b\nit's\n036x\\n\n" {
			t.Errorf("%s: %q, %v", login, out, err)
		}
		if out, err := c.RunWithInput("IFS= read -r key\necho \"got $key\"", []byte("k\\1\n"), 10*time.Second); err != nil || string(out) != "got k\\1\n" {
			t.Errorf("%s with input: %q, %v", login, out, err)
		}
	}
}

// TestAScriptThatNeverArrivesFailsInWords: an ssh config that discards the session's stdin (StdinNull) used to run an
// empty script, cleanly; now the machine says the script did not reach it.
func TestAScriptThatNeverArrivesFailsInWords(t *testing.T) {
	dir := t.TempDir()
	stub := "#!/bin/sh\ncase \"$*\" in *\" -G \"*) exit 1 ;; esac\nfor line; do :; done\nexec sh -c \"$line\" </dev/null\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
	for name, run := range map[string]func() error{
		"run":        func() error { _, err := c.Run("echo ran", 10*time.Second); return err },
		"with input": func() error { _, err := c.RunWithInput("cat", []byte("data"), 10*time.Second); return err },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "did not reach the machine whole") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestAHostThatDiscardsStdinStillGetsTheScript: an ssh that knows StdinNull is told to keep the session's stdin, which a
// host's config set to discard.
func TestAHostThatDiscardsStdinStillGetsTheScript(t *testing.T) {
	dir := t.TempDir()
	stub := "#!/bin/sh\ncase \"$*\" in *\" -G \"*) exit 0 ;; esac\nkeep=\nfor word; do case \"$word\" in StdinNull=no) keep=1 ;; esac; line=$word; done\n" +
		"if [ -n \"$keep\" ]; then exec sh -c \"$line\"; fi\nexec sh -c \"$line\" </dev/null\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
	if out, err := c.Run("echo ran", 10*time.Second); err != nil || string(out) != "ran\n" {
		t.Errorf("%q, %v", out, err)
	}
}

// TestTheOptionProbeReadsNoConfig: the probe of what this ssh knows reads no ssh config, so a Match exec there cannot
// hold a session back; and it ends within its time.
func TestTheOptionProbeReadsNoConfig(t *testing.T) {
	dir := t.TempDir()
	stub := "#!/bin/sh\ncase \"$*\" in *\"-F none -G\"*) exit 0 ;; *-G*) sleep 30 ;; esac\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	began := time.Now()
	if got, _ := keepStdin(0); len(got) != 4 || time.Since(began) > stdinProbe {
		t.Errorf("%q after %s", got, time.Since(began))
	}
}

// TestAProbeThatTimedOutIsAskedAgain: a slow first probe leaves the options off for that call only.
func TestAProbeThatTimedOutIsAskedAgain(t *testing.T) {
	dir := t.TempDir()
	slow := filepath.Join(dir, "slow")
	stub := "#!/bin/sh\ncase \"$*\" in *\" -G \"*) if [ ! -e " + script.Quote(slow) + " ]; then : > " + script.Quote(slow) + "; sleep 5; fi; exit 0 ;; esac\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	was := stdinProbe
	stdinProbe = 300 * time.Millisecond
	t.Cleanup(func() { stdinProbe = was })
	if got, _ := keepStdin(0); got != nil {
		t.Fatalf("a probe that timed out said %q", got)
	}
	if got, _ := keepStdin(0); len(got) != 4 {
		t.Errorf("asked again: %q", got)
	}
}

// TestCallersWaitingOnOneProbeShareItsAnswer: while a probe stalls, the callers waiting on it get its answer when it
// times out, and not one probe each.
func TestCallersWaitingOnOneProbeShareItsAnswer(t *testing.T) {
	dir := t.TempDir()
	probes := filepath.Join(dir, "probes")
	stub := "#!/bin/sh\ncase \"$*\" in *\" -G \"*) echo x >> " + script.Quote(probes) + "; sleep 5 ;; esac\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	was := stdinProbe
	stdinProbe = 300 * time.Millisecond
	t.Cleanup(func() { stdinProbe = was })
	began := time.Now()
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); keepStdin(0) }()
	}
	wg.Wait()
	if took := time.Since(began); took > 2*stdinProbe {
		t.Errorf("three callers took %s", took)
	}
	if got := lengths(t, probes); len(got) != 1 {
		t.Errorf("%d probes for one stall", len(got))
	}
	keepStdin(0)
	if got := lengths(t, probes); len(got) != 2 {
		t.Errorf("a later caller did not ask again: %d probes", len(got))
	}
}

// TestAScriptWithNoTimeLeftAfterTheProbeNeverStarts: what the probe took is the script's own time.
func TestAScriptWithNoTimeLeftAfterTheProbeNeverStarts(t *testing.T) {
	dir := t.TempDir()
	ran := filepath.Join(dir, "ran")
	stub := "#!/bin/sh\ncase \"$*\" in *\" -G \"*) sleep 5 ;; esac\n: > " + script.Quote(ran) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	was := stdinProbe
	stdinProbe = 300 * time.Millisecond
	t.Cleanup(func() { stdinProbe = was })
	c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
	began := time.Now()
	if _, err := c.Run("true", 100*time.Millisecond); err == nil {
		t.Error("a script ran past its time")
	}
	if took := time.Since(began); took >= stdinProbe {
		t.Errorf("the script waited %s for the probe, past its own time", took)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("ssh was started past the script's time")
	}
}

// TestAScriptHoldingANulNeverRuns: the shell would drop the byte and run another script than the one written.
func TestAScriptHoldingANulNeverRuns(t *testing.T) {
	fakeDropbear(t)
	c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
	if _, err := c.Run("cat /safe/a\x00/../../etc/passwd", 10*time.Second); err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Errorf("%v", err)
	}
	if out, err := c.RunWithInput("cat", []byte("a\x00b"), 10*time.Second); err != nil || string(out) != "a\x00b" {
		t.Errorf("data on stdin must stay whole: %q, %v", out, err)
	}
}
