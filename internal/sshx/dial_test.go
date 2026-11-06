package sshx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
)

// fakeMaster puts an `ssh` first on PATH whose master signs in after a pause — its control socket appears then, as
// OpenSSH puts it in place — having said a great deal on its stderr, as a master does over days of refused channels.
func fakeMaster(t *testing.T) (calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	stub := `#!/bin/sh
printf '%s\n' "$*" >> ` + script.Quote(calls) + `
ctl="$2"
case "$*" in
  *"-O check"*) [ -e "$ctl" ] && exit 0; exit 255 ;;
  *" -M -N "*) (sleep 0.8; : > "$ctl") </dev/null >/dev/null 2>&1 &
    head -c 300000 /dev/zero | tr '\0' x >&2
    exec sleep 30 ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

// TestDialWaitsForTheSocketAndKeepsLittleOfWhatTheMasterSays: until the master has signed in there is no control
// socket to ask, and an `ssh -O check` five times a second asked nothing for as long as a host stayed silent; and
// the master's stderr, read only for why a connect failed, was kept whole for as long as the master lived.
func TestDialWaitsForTheSocketAndKeepsLittleOfWhatTheMasterSays(t *testing.T) {
	defer func(d string) { SocketDir = d }(SocketDir)
	dir, err := os.MkdirTemp("", "sx") // short: a control path holds 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	SocketDir = dir
	calls := fakeMaster(t)

	c, err := Dial("gog.remote")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	checks := 0
	for _, line := range recorded(t, calls) {
		if strings.Contains(line, "-O check") {
			checks++
		}
	}
	if checks != 1 {
		t.Errorf("the master was asked %d times whether it answers, want once its socket is there", checks)
	}
	time.Sleep(200 * time.Millisecond) // whatever the master printed has been copied by now
	if kept := len(c.masterErr.String()); kept > masterErrMax {
		t.Errorf("%d bytes of the master's stderr kept, want %d at most", kept, masterErrMax)
	}
}
