package sshx

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// fakeDropbear puts an `ssh` first on PATH that runs its last word here, stdin and all, and refuses a command past
// 9000 bytes as Dropbear does — by dropping the connection; it keeps every command line it was given.
func fakeDropbear(t *testing.T) (lines string) {
	t.Helper()
	dir := t.TempDir()
	lines = filepath.Join(dir, "lines")
	stub := `#!/bin/sh
for line; do :; done
printf '%s\n' "${#line}" >> ` + script.Quote(lines) + `
if [ "${#line}" -gt 9000 ]; then echo "Exit: String too long" >&2; exit 255; fi
exec sh -c "$line"
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

// TestAShortScriptStaysOnTheCommandLine: what fits travels as before, and a script with data for its stdin keeps the
// command line, the data being the stdin's.
func TestAShortScriptStaysOnTheCommandLine(t *testing.T) {
	lines := fakeDropbear(t)
	c := &Conn{Host: "roofr.remote", ctl: filepath.Join(t.TempDir(), "ctl")}
	short := `printf '%s' "$HOME" >/dev/null; echo short`
	if out, err := c.Run(short, 10*time.Second); err != nil || string(out) != "short\n" {
		t.Fatalf("the short script: %q, %v", out, err)
	}
	if out, err := c.RunWithInput(`IFS= read -r key; echo "got $key"`, []byte("k-1\n"), 10*time.Second); err != nil || string(out) != "got k-1\n" {
		t.Fatalf("the script with input: %q, %v", out, err)
	}
	got := lengths(t, lines)
	if len(got) != 2 || got[0] != strconv.Itoa(len(RemoteCommand(short))) {
		t.Fatalf("command lines of %v bytes", got)
	}
	line, stdin := Remote(strings.Repeat("y", MaxCommandLine-len(RemoteCommand(""))))
	if len(line) != MaxCommandLine || stdin != nil {
		t.Fatalf("a script just fitting left the command line: %d bytes", len(line))
	}
	if line, stdin := Remote(strings.Repeat("y", MaxCommandLine)); line != readScript || stdin == nil {
		t.Fatalf("a script past the length stayed on the command line")
	}
}
