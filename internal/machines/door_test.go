package machines

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/sshx"
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
		`printf '%s\n' 'http://127.0.0.1:7318' > "$d/agent-url.new"`,
		`mv -f "$d/agent-url.new" "$d/agent-url"`,
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

// TestAFailedStepOfTheDoorIsAskedAgain: it is asked until it holds or the tries run out, and never for a connection
// that is no longer the machine's.
func TestAFailedStepOfTheDoorIsAskedAgain(t *testing.T) {
	tries, pause := doorTries, doorPause
	doorTries, doorPause = 4, time.Millisecond
	t.Cleanup(func() { doorTries, doorPause = tries, pause })

	asked := 0
	refusedTwice := func() error {
		asked++
		if asked <= 2 {
			return errors.New("remote port forwarding failed for listen port 0")
		}
		return nil
	}
	if err := trySoon(refusedTwice, func() bool { return true }); err != nil || asked != 3 {
		t.Fatalf("asked %d times, %v: want the third ask to hold", asked, err)
	}

	asked = 0
	refused := func() error { asked++; return errors.New("remote port forwarding failed for listen port 0") }
	if err := trySoon(refused, func() bool { return true }); err == nil || asked != 4 {
		t.Fatalf("asked %d times, %v: want every try spent and the refusal kept", asked, err)
	}

	asked = 0
	if err := trySoon(refused, func() bool { return false }); err == nil || asked != 1 {
		t.Fatalf("asked %d times for a connection gone: want one", asked)
	}
}

func TestADoorForbiddenOnEveryPortKeepsTheRefusal(t *testing.T) {
	refused := errors.New("remote port forwarding failed for listen port 7318")
	port, err := doorForward(func() error { return refused }, func() (int, error) { return 0, errors.New("no port") })
	if port != 0 || !errors.Is(err, refused) {
		t.Fatalf("port %d, %v", port, err)
	}
}

// doorOverSSH opens the door over an `ssh` stand-in first on PATH: a master that stays, 7318 refused while held says
// so, a port asked for as 0 answered 50236, and a script not run but kept with its stdin, which it answers with.
func doorOverSSH(t *testing.T, held bool) string {
	t.Helper()
	dir := t.TempDir()
	kept := filepath.Join(dir, "scripts")
	stub := `#!/bin/sh
case " $* " in
*" -M "*) while [ "$1" != -S ]; do shift; done; : > "$2"; exec sleep 30 ;;
*" -O forward -R 127.0.0.1:0:"*) echo 50236; exit 0 ;;
*" -O forward -R 127.0.0.1:7318:"*)
  [ -z "$DOOR_HELD" ] && exit 0
  echo "remote port forwarding failed for listen port 7318" >&2; exit 255 ;;
*" -O "*) exit 0 ;;
esac
for line; do :; done
{ printf '%s\n' "$line"; cat; } >> ` + script.Quote(kept) + `
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOOR_HELD", "")
	if held {
		t.Setenv("DOOR_HELD", "1")
	}
	conn, err := sshx.Dial("door-test-" + filepath.Base(dir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	mc := &Machine{conn: conn, door: &Door{LocalPort: 7317, Key: func(string, string) (string, error) { return "rk_door", nil }}}
	mc.openDoor(conn)
	placed, err := os.ReadFile(kept)
	if err != nil {
		t.Fatalf("the door files were not placed: %v", err)
	}
	return string(placed)
}

func TestTheDoorStaysOnItsPortWhileThatIsFree(t *testing.T) {
	placed := doorOverSSH(t, false)
	if !strings.Contains(placed, "http://127.0.0.1:7318") || !strings.Contains(placed, "rk_door") {
		t.Fatalf("placed: %s", placed)
	}
}

// TestADoorWhosePortIsHeldOpensOnAnother: the connection the network lost holds 7318, and the door opens at once on
// the port the sshd picked, the address in the files saying so.
func TestADoorWhosePortIsHeldOpensOnAnother(t *testing.T) {
	placed := doorOverSSH(t, true)
	if !strings.Contains(placed, "http://127.0.0.1:50236") || strings.Contains(placed, "127.0.0.1:7318") || !strings.Contains(placed, "rk_door") {
		t.Fatalf("placed: %s", placed)
	}
}
