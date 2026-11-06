package machines

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// TestDoorScriptPlacesTheFilesPrivately: the script finds remuda's state dir the way state.Dir does on either system,
// takes the key on stdin and leaves both files readable by the user alone.
func TestDoorScriptPlacesTheFilesPrivately(t *testing.T) {
	host := &shell.Fake{Name: "gog", Dir: "/home/me", Remote: true}
	if _, err := host.Run(DoorScript("http://127.0.0.1:7318"), 0); err != nil {
		t.Fatal(err)
	}
	ran := host.Scripts()
	if len(ran) != 1 {
		t.Fatalf("%d scripts", len(ran))
	}
	s := ran[0]
	for _, want := range []string{
		`umask 077`,
		`Darwin) d="$HOME/Library/Application Support/remuda"`,
		`d="${XDG_CONFIG_HOME:-$HOME/.config}/remuda"`,
		`chmod 700 "$d"`,
		`cat > "$d/agent-key.new"`,
		`mv -f "$d/agent-key.new" "$d/agent-key"`,
		`printf '%s\n' 'http://127.0.0.1:7318' > "$d/agent-url"`,
		`chmod 600 "$d/agent-key" "$d/agent-url"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the script lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "rk_") {
		t.Error("a key must never be in the script itself")
	}
}

// TestADoorRefusedWhileTheOldConnectionHoldsThePortIsAskedAgain: after remuda's own restart the machine's sshd may
// still hold 7318 for the connection that just ended, and the new forward is refused; it is asked again until it
// holds, and never for a connection that is no longer the machine's.
func TestADoorRefusedWhileTheOldConnectionHoldsThePortIsAskedAgain(t *testing.T) {
	tries, pause := doorTries, doorPause
	doorTries, doorPause = 4, time.Millisecond
	t.Cleanup(func() { doorTries, doorPause = tries, pause })

	asked := 0
	refusedTwice := func() error {
		asked++
		if asked <= 2 {
			return errors.New("remote port forwarding failed for listen port 7318")
		}
		return nil
	}
	if err := forwardSoon(refusedTwice, func() bool { return true }); err != nil || asked != 3 {
		t.Fatalf("asked %d times, %v: want the third ask to hold", asked, err)
	}

	asked = 0
	refused := func() error { asked++; return errors.New("remote port forwarding failed for listen port 7318") }
	if err := forwardSoon(refused, func() bool { return true }); err == nil || asked != 4 {
		t.Fatalf("asked %d times, %v: want every try spent and the refusal kept", asked, err)
	}

	asked = 0
	if err := forwardSoon(refused, func() bool { return false }); err == nil || asked != 1 {
		t.Fatalf("asked %d times for a connection gone: want one", asked)
	}
}
