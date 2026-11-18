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
	"sync"
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
		`url='http://127.0.0.1:7318'`,
		`cat > "$d/agent-key.new"`,
		`mv -f "$d/agent-key.new" "$d/agent-key"`,
		`printf '%s\n' "$url" > "$d/agent-url.new"`,
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

// sshdAtTheDoor is what the machine does in doorOverSSH.
type sshdAtTheDoor struct {
	heldFor   int    // how many asks for 7318 its sshd refuses
	script    string // what DoorScript answers there, placed by default
	forbidden bool   // its sshd refuses a port of its own choice too
	noKey     bool   // this remuda cannot make a key
}

// doorRun is what came of opening the door: the scripts that placed it, and the forwards cancelled.
type doorRun struct {
	conn            *sshx.Conn
	placed, cancels string
}

// doorOverSSH opens the door over an `ssh` stand-in first on PATH, a master that stays. A script is not run: it is kept
// with its stdin when it is to answer placed.
func doorOverSSH(t *testing.T, sshd sshdAtTheDoor) doorRun {
	t.Helper()
	tries, pause := doorTries, doorPause
	doorTries, doorPause = 6, time.Millisecond
	t.Cleanup(func() { doorTries, doorPause = tries, pause })
	dir := t.TempDir()
	kept, asks, cancels := filepath.Join(dir, "scripts"), filepath.Join(dir, "asks"), filepath.Join(dir, "cancels")
	stub := `#!/bin/sh
case " $* " in
*" -M "*) while [ "$1" != -S ]; do shift; done; : > "$2"; exec sleep 30 ;;
*" -O cancel "*) echo "$*" >> ` + script.Quote(cancels) + `; exit 0 ;;
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
if [ "${DOOR_SCRIPT:-placed}" = placed ]; then { printf '%s\n' "$line"; cat; } >> ` + script.Quote(kept) + `; fi
echo "${DOOR_SCRIPT:-placed}"
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOOR_HELD_FOR", strconv.Itoa(sshd.heldFor))
	t.Setenv("DOOR_SCRIPT", sshd.script)
	t.Setenv("DOOR_FORBIDDEN", map[bool]string{true: "1"}[sshd.forbidden])
	conn, err := sshx.Dial("door-test-" + filepath.Base(dir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	key := func(string, string) (string, error) {
		if sshd.noKey {
			return "", errors.New("the disk is full")
		}
		return "rk_door", nil
	}
	mc := &Machine{conn: conn, door: &Door{LocalPort: 7317, Key: key}}
	mc.openDoor(conn)
	placed, _ := os.ReadFile(kept)
	cancelled, _ := os.ReadFile(cancels)
	return doorRun{conn, string(placed), string(cancelled)}
}

const heldForGood = 1000

func TestTheDoorStaysOnItsPortWhileThatIsFree(t *testing.T) {
	run := doorOverSSH(t, sshdAtTheDoor{})
	if !strings.Contains(run.placed, "http://127.0.0.1:7318") || !strings.Contains(run.placed, "rk_door") {
		t.Fatalf("placed: %s", run.placed)
	}
}

// TestADoorKeepsItsPortThroughARestart: sshd lets go of 7318 a moment after the connection that just ended.
func TestADoorKeepsItsPortThroughARestart(t *testing.T) {
	run := doorOverSSH(t, sshdAtTheDoor{heldFor: doorPortAsks - 1})
	if !strings.Contains(run.placed, "url='\\''http://127.0.0.1:7318'\\''") && !strings.Contains(run.placed, "url='http://127.0.0.1:7318'") {
		t.Fatalf("placed: %s", run.placed)
	}
	if strings.Contains(run.placed, "50236") {
		t.Fatalf("moved: %s", run.placed)
	}
}

// TestADoorWhosePortIsHeldOpensOnAnother: a connection the network lost holds 7318, and the door opens on the port the
// sshd picked, the address in the files saying so.
func TestADoorWhosePortIsHeldOpensOnAnother(t *testing.T) {
	run := doorOverSSH(t, sshdAtTheDoor{heldFor: heldForGood})
	if !strings.Contains(run.placed, "http://127.0.0.1:50236") || !strings.Contains(run.placed, "rk_door") {
		t.Fatalf("placed: %s", run.placed)
	}
	if strings.Count(run.cancels, "127.0.0.1:0:") != 1 {
		t.Fatalf("the door it stands on was let go: %s", run.cancels)
	}
}

// TestAForwardNoDoorCameOfIsLetGo: the machine kept another door, or stayed busy, and the forward goes.
func TestAForwardNoDoorCameOfIsLetGo(t *testing.T) {
	for name, sshd := range map[string]sshdAtTheDoor{
		"taken, on 7318":               {script: doorTaken},
		"busy, on 7318":                {script: doorBusy},
		"taken, on a port of its own":  {heldFor: heldForGood, script: doorTaken},
		"no key, on 7318":              {noKey: true},
		"no key, on a port of its own": {heldFor: heldForGood, noKey: true},
	} {
		t.Run(name, func(t *testing.T) {
			run := doorOverSSH(t, sshd)
			if run.placed != "" {
				t.Fatalf("placed: %s", run.placed)
			}
			spec := "127.0.0.1:7318:"
			if sshd.heldFor > 0 {
				spec = "127.0.0.1:0:"
			}
			if strings.Count(run.cancels, spec) != 2 { // the one before asking, and letting it go
				t.Fatalf("cancelled: %s", run.cancels)
			}
		})
	}
}

func TestADoorForbiddenOnEveryPortKeepsTheRefusal(t *testing.T) {
	run := doorOverSSH(t, sshdAtTheDoor{heldFor: heldForGood, forbidden: true})
	if run.placed != "" {
		t.Fatalf("placed: %s", run.placed)
	}
	if _, err := doorForward(run.conn, 7317, doorPortAsks); err == nil || !strings.Contains(err.Error(), "listen port 7318") {
		t.Fatalf("%v", err)
	}
}

// doorHome is a home of its own for DoorScript to run in on this machine, every proxy pointing nowhere.
type doorHome struct {
	t   *testing.T
	dir string
}

func newDoorHome(t *testing.T) doorHome {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, name := range []string{"http_proxy", "HTTP_PROXY", "all_proxy", "ALL_PROXY"} {
		t.Setenv(name, "http://127.0.0.1:9")
	}
	t.Setenv("no_proxy", "")
	t.Setenv("NO_PROXY", "")
	out, err := exec.Command("sh", "-c", script.StateDir("d")+`
mkdir -p "$d" && printf '%s' "$d"`).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return doorHome{t, string(out)}
}

func (h doorHome) write(name, text string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, name), []byte(text+"\n"), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h doorHome) read(name string) string {
	data, _ := os.ReadFile(filepath.Join(h.dir, name))
	return strings.TrimSpace(string(data))
}

func (h doorHome) clear() {
	h.t.Helper()
	for _, name := range []string{"agent-url", "agent-key", "agent-door-moved"} {
		os.Remove(filepath.Join(h.dir, name))
	}
}

func (h doorHome) place(url string) string {
	h.t.Helper()
	cmd := exec.Command("sh", "-c", DoorScript(url))
	cmd.Stdin = strings.NewReader("rk_new\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("%v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func liveDoor(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.Write([]byte(`{"ok":true,"server":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestDoorScriptKeepsALiveDoor runs the script itself: who may replace what stands at the address.
func TestDoorScriptKeepsALiveDoor(t *testing.T) {
	home := newDoorHome(t)
	usual, elsewhere := doorURL(DoorPort), doorURL(50236)
	live := liveDoor(t)
	release := make(chan struct{})
	silent := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(silent.Close)
	t.Cleanup(func() { close(release) })
	for _, c := range []struct {
		name, was string
		moved     bool
		url, want string
	}{
		{"nothing there", "", false, usual, doorPlaced},
		{"the usual door again", usual, false, usual, doorPlaced},
		{"a cockpit on the machine, on its own port, gives way to 7318", live, false, usual, doorPlaced},
		{"a live door on a port of its own keeps 7318 out", live, true, usual, doorTaken},
		{"a silent door on a port of its own gives way to 7318", silent.URL, true, usual, doorPlaced},
		{"a live door keeps a moving one out", live, false, elsewhere, doorTaken},
		{"a silent door gives way to a moving one", silent.URL, false, elsewhere, doorPlaced},
		{"a closed port gives way to a moving one", "http://127.0.0.1:9", false, elsewhere, doorPlaced},
		{"its own moved door again", live, true, live, doorPlaced},
	} {
		home.clear()
		home.write("agent-key", "rk_old")
		if c.was != "" {
			home.write("agent-url", c.was)
		}
		if c.moved {
			home.write("agent-door-moved", c.was)
		}
		if got := home.place(c.url); got != c.want {
			t.Errorf("%s: answered %s, want %s", c.name, got, c.want)
			continue
		}
		wantURL, wantKey, wantMoved := c.url, "rk_new", ""
		if c.url != usual {
			wantMoved = c.url
		}
		if c.want == doorTaken {
			wantURL, wantKey, wantMoved = c.was, "rk_old", ""
			if c.moved {
				wantMoved = c.was
			}
		}
		if url, key, moved := home.read("agent-url"), home.read("agent-key"), home.read("agent-door-moved"); url != wantURL || key != wantKey || moved != wantMoved {
			t.Errorf("%s: url %q key %q moved %q, want %q %q %q", c.name, url, key, moved, wantURL, wantKey, wantMoved)
		}
	}
}

// TestDoorScriptGoesOneAtATime: of cockpits placing their live doors together exactly one wins, the others finding its
// door.
func TestDoorScriptGoesOneAtATime(t *testing.T) {
	home := newDoorHome(t)
	urls := []string{liveDoor(t), liveDoor(t), liveDoor(t), liveDoor(t)}
	answers := make([]string, len(urls))
	var cockpits sync.WaitGroup
	for i, url := range urls {
		cockpits.Add(1)
		go func() {
			defer cockpits.Done()
			for {
				cmd := exec.Command("sh", "-c", DoorScript(url))
				cmd.Stdin = strings.NewReader("rk_new\n")
				out, err := cmd.Output()
				if answers[i] = strings.TrimSpace(string(out)); err != nil || answers[i] != doorBusy {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}
	cockpits.Wait()
	winner := ""
	for i, answer := range answers {
		if answer == doorPlaced {
			if winner != "" {
				t.Fatalf("two doors placed: %v", answers)
			}
			winner = urls[i]
		} else if answer != doorTaken {
			t.Fatalf("answers: %v", answers)
		}
	}
	if winner == "" || home.read("agent-url") != winner || home.read("agent-door-moved") != winner {
		t.Fatalf("winner %q, url %q, moved %q", winner, home.read("agent-url"), home.read("agent-door-moved"))
	}
}

// doorClock puts a `date` first on PATH that reads the clock it answers with, and a `curl` that sets it to afterLook:
// what the clock did while the script looked at the door.
func doorClock(t *testing.T, seconds, afterLook int) (set func(int)) {
	t.Helper()
	tools := t.TempDir()
	clock := filepath.Join(tools, "clock")
	set = func(to int) {
		t.Helper()
		if err := os.WriteFile(clock, []byte(strconv.Itoa(to)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	set(seconds)
	for name, stub := range map[string]string{
		"date": "#!/bin/sh\ncat " + script.Quote(clock) + "\n",
		"curl": "#!/bin/sh\necho " + strconv.Itoa(afterLook) + " > " + script.Quote(clock) + "\n",
	} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	return set
}

// TestDoorScriptWaitsForTheLocksStanding: at second 3000 the half minute is 100. A lock of now, of the next one or of
// later — a clock set back — says busy; one of the past was left by a dead script and lapsed.
func TestDoorScriptWaitsForTheLocksStanding(t *testing.T) {
	home := newDoorHome(t)
	doorClock(t, 3000, 3000)
	for halfMinute, want := range map[int]string{100: doorBusy, 101: doorBusy, 140: doorBusy, 99: doorPlaced, 40: doorPlaced} {
		lock := filepath.Join(home.dir, "agent-door.lock."+strconv.Itoa(halfMinute))
		if err := os.Mkdir(lock, 0o700); err != nil {
			t.Fatal(err)
		}
		if got := home.place(doorURL(DoorPort)); got != want {
			t.Errorf("a lock of %d: answered %s, want %s", halfMinute, got, want)
		}
		if want == doorBusy {
			os.Remove(lock)
		}
		if left, _ := filepath.Glob(filepath.Join(home.dir, "agent-door.lock.*")); len(left) != 0 {
			t.Errorf("a lock of %d: left %v", halfMinute, left)
		}
	}
}

// TestDoorScriptPlacesNothingOnceItsLocksAreLeftBehind: the clock went past them or back before them during the look,
// so another cockpit may hold the door by now.
func TestDoorScriptPlacesNothingOnceItsLocksAreLeftBehind(t *testing.T) {
	for afterLook, want := range map[int]string{3120: doorBusy, 2940: doorBusy, 3030: doorPlaced} {
		home := newDoorHome(t)
		doorClock(t, 3000, afterLook)
		home.write("agent-url", "http://127.0.0.1:9")
		home.write("agent-key", "rk_old")
		if got := home.place(doorURL(50236)); got != want {
			t.Errorf("the clock at %d after the look: answered %s, want %s", afterLook, got, want)
		}
		if want == doorBusy && (home.read("agent-url") != "http://127.0.0.1:9" || home.read("agent-key") != "rk_old") {
			t.Errorf("the clock at %d after the look: the files changed", afterLook)
		}
	}
}

// TestDoorScriptWithoutCurlLeavesTheDoor: a door nobody can ask about may be alive.
func TestDoorScriptWithoutCurlLeavesTheDoor(t *testing.T) {
	home := newDoorHome(t)
	home.write("agent-url", "http://127.0.0.1:50236")
	home.write("agent-door-moved", "http://127.0.0.1:50236")
	tools := t.TempDir()
	for _, tool := range []string{"cat", "chmod", "date", "head", "mkdir", "mv", "rm", "rmdir", "uname"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(tools, tool)); err != nil {
			t.Fatal(err)
		}
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools)
	out, err := exec.Command(shell, "-c", DoorScript(doorURL(DoorPort))).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != doorTaken {
		t.Fatalf("%q, %v", out, err)
	}
}
