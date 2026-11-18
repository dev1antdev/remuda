package machines

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// sshdAtTheDoor is what the machine's sshd does in doorOverSSH.
type sshdAtTheDoor struct {
	heldFor   int  // how many asks for 7318 it refuses
	answers   bool // the door agent-url names answers
	forbidden bool // it refuses a port of its own choice too
}

// doorOverSSH opens the door over an `ssh` stand-in first on PATH, a master that stays, and answers with the connection
// and the scripts placed: not run but kept with their stdin, the look at the door excepted.
func doorOverSSH(t *testing.T, sshd sshdAtTheDoor) (*sshx.Conn, string) {
	t.Helper()
	tries, pause := doorTries, doorPause
	doorTries, doorPause = 6, time.Millisecond
	t.Cleanup(func() { doorTries, doorPause = tries, pause })
	dir := t.TempDir()
	kept, asks := filepath.Join(dir, "scripts"), filepath.Join(dir, "asks")
	stub := `#!/bin/sh
case " $* " in
*" -M "*) while [ "$1" != -S ]; do shift; done; : > "$2"; exec sleep 30 ;;
*" -O forward -R 127.0.0.1:0:"*)
  [ -z "$DOOR_FORBIDDEN" ] || { echo "remote port forwarding failed for listen port 0" >&2; exit 255; }
  echo 50236; exit 0 ;;
*" -O forward -R 127.0.0.1:7318:"*)
  echo asked >> ` + script.Quote(asks) + `
  [ $(($(wc -l < ` + script.Quote(asks) + `))) -gt "$DOOR_HELD_FOR" ] && exit 0
  echo "remote port forwarding failed for listen port 7318" >&2; exit 255 ;;
*" -O "*) exit 0 ;;
esac
for line; do :; done
case "$line" in *api/health*) [ -n "$DOOR_ANSWERS" ]; exit ;; esac
{ printf '%s\n' "$line"; cat; } >> ` + script.Quote(kept) + `
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOOR_HELD_FOR", strconv.Itoa(sshd.heldFor))
	t.Setenv("DOOR_ANSWERS", map[bool]string{true: "1"}[sshd.answers])
	t.Setenv("DOOR_FORBIDDEN", map[bool]string{true: "1"}[sshd.forbidden])
	conn, err := sshx.Dial("door-test-" + filepath.Base(dir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	mc := &Machine{conn: conn, door: &Door{LocalPort: 7317, Key: func(string, string) (string, error) { return "rk_door", nil }}}
	mc.openDoor(conn)
	placed, _ := os.ReadFile(kept)
	return conn, string(placed)
}

const heldForGood = 1000

func TestTheDoorStaysOnItsPortWhileThatIsFree(t *testing.T) {
	_, placed := doorOverSSH(t, sshdAtTheDoor{})
	if !strings.Contains(placed, "http://127.0.0.1:7318") || !strings.Contains(placed, "rk_door") {
		t.Fatalf("placed: %s", placed)
	}
}

// TestADoorKeepsItsPortThroughARestart: sshd lets go of 7318 a moment after the connection that just ended.
func TestADoorKeepsItsPortThroughARestart(t *testing.T) {
	_, placed := doorOverSSH(t, sshdAtTheDoor{heldFor: doorPortAsks - 1})
	if !strings.Contains(placed, "http://127.0.0.1:7318") || strings.Contains(placed, "50236") {
		t.Fatalf("placed: %s", placed)
	}
}

// TestADoorWhosePortIsHeldOpensOnAnother: a connection the network lost holds 7318 and answers nothing, and the door
// opens on the port the sshd picked, the address in the files saying so.
func TestADoorWhosePortIsHeldOpensOnAnother(t *testing.T) {
	_, placed := doorOverSSH(t, sshdAtTheDoor{heldFor: heldForGood})
	if !strings.Contains(placed, "http://127.0.0.1:50236") || strings.Contains(placed, "127.0.0.1:7318") || !strings.Contains(placed, "rk_door") {
		t.Fatalf("placed: %s", placed)
	}
}

// TestALiveDoorIsLeftToItsCockpit: another cockpit's door answers on the machine, and its agents stay with it.
func TestALiveDoorIsLeftToItsCockpit(t *testing.T) {
	conn, placed := doorOverSSH(t, sshdAtTheDoor{heldFor: heldForGood, answers: true})
	if placed != "" {
		t.Fatalf("the files were replaced: %s", placed)
	}
	if _, err := doorForward(conn, 7317, doorPortAsks); !errors.Is(err, errDoorTaken) {
		t.Fatalf("%v", err)
	}
}

func TestADoorForbiddenOnEveryPortKeepsTheRefusal(t *testing.T) {
	conn, placed := doorOverSSH(t, sshdAtTheDoor{heldFor: heldForGood, forbidden: true})
	if placed != "" {
		t.Fatalf("placed: %s", placed)
	}
	if _, err := doorForward(conn, 7317, doorPortAsks); err == nil || !strings.Contains(err.Error(), "listen port 7318") {
		t.Fatalf("%v", err)
	}
}

// TestTheLookAtTheDoorTellsALiveOneFromAHeldPort runs the script on this machine, in a home of its own.
func TestTheLookAtTheDoorTellsALiveOneFromAHeldPort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	place := func(url string) {
		t.Helper()
		out, err := exec.Command("sh", "-c", script.StateDir("d")+`
mkdir -p "$d" && printf '%s\n' `+script.Quote(url)+` > "$d/agent-url"`).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	answers := func() bool { return exec.Command("sh", "-c", doorAnswersScript()).Run() == nil }
	if answers() {
		t.Error("no agent-url answered")
	}
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.Write([]byte(`{"ok":true,"server":true}`))
		}
	}))
	defer live.Close()
	place(live.URL)
	if !answers() {
		t.Error("a live door did not answer")
	}
	release := make(chan struct{})
	silent := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer silent.Close()
	defer close(release)
	place(silent.URL)
	if answers() {
		t.Error("a port that says nothing answered")
	}
}
