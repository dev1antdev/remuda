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
	if err := trySoon(doorTries, refusedTwice, func() bool { return true }); err != nil || asked != 3 {
		t.Fatalf("asked %d times, %v: want the third ask to hold", asked, err)
	}

	asked = 0
	refused := func() error { asked++; return errors.New("remote port forwarding failed for listen port 0") }
	if err := trySoon(doorTries, refused, func() bool { return true }); err == nil || asked != 4 {
		t.Fatalf("asked %d times, %v: want every try spent and the refusal kept", asked, err)
	}

	asked = 0
	if err := trySoon(doorTries, refused, func() bool { return false }); err == nil || asked != 0 {
		t.Fatalf("asked %d times for a connection gone: want none", asked)
	}
}

// sshdAtTheDoor is what the machine does in doorOverSSH.
type sshdAtTheDoor struct {
	heldFor   int    // how many asks for 7318 its sshd refuses
	script    string // what DoorScript answers there, placed by default
	busyFor   int    // how many runs of it answer busy first
	forbidden bool   // its sshd refuses a port of its own choice too
	noKey     bool   // this remuda cannot make a key
	goneAtKey bool   // the connection is replaced while the key is made, which then fails
	letGoing  bool   // the connection before this one is still letting go of its forward
	pauseAt   string // the step that waits, asking for 7318 or placing, for the test to look
	goneThere bool   // the connection is replaced while that step waits
}

// doorRun is what came of opening the door: the scripts that placed it, and the forwards cancelled.
type doorRun struct {
	conn            *sshx.Conn
	placed, cancels string
	keys            int // how many keys were asked for
}

// doorOverSSH opens the door over an `ssh` stand-in first on PATH, a master that stays. A script is not run: it is kept
// with its stdin when it is to answer placed.
func doorOverSSH(t *testing.T, sshd sshdAtTheDoor) doorRun {
	t.Helper()
	tries, placeTries, pause := doorTries, doorPlaceTries, doorPause
	doorTries, doorPlaceTries, doorPause = 6, 12, time.Millisecond
	t.Cleanup(func() { doorTries, doorPlaceTries, doorPause = tries, placeTries, pause })
	dir := t.TempDir()
	runs, resume := filepath.Join(dir, "runs"), filepath.Join(dir, "resume")
	kept, asks, cancels := filepath.Join(dir, "scripts"), filepath.Join(dir, "asks"), filepath.Join(dir, "cancels")
	stub := `#!/bin/sh
case "$1" in -G) exit 1 ;; esac
case " $* " in
*" -M "*) while [ "$1" != -S ]; do shift; done; : > "$2"; exec sleep 30 ;;
*" -O cancel "*) echo "$*" >> ` + script.Quote(cancels) + `; exit 0 ;;
*" -O forward -R 127.0.0.1:0:"*)
  [ -z "$DOOR_FORBIDDEN" ] || { echo "remote port forwarding failed for listen port 0" >&2; exit 255; }
  echo 50236; exit 0 ;;
*" -O forward -R 127.0.0.1:7318:"*)
  echo asked >> ` + script.Quote(asks) + `
  [ "$DOOR_PAUSE_AT" != forward ] || while [ ! -e ` + script.Quote(resume) + ` ]; do sleep 0.01; done
  [ $(($(wc -l < ` + script.Quote(asks) + `))) -gt "$DOOR_HELD_FOR" ] && exit 0
  echo "remote port forwarding failed for listen port 7318" >&2; exit 255 ;;
*" -O "*) exit 0 ;;
esac
for line; do :; done
answer=${DOOR_SCRIPT:-placed}
echo ran >> ` + script.Quote(runs) + `
[ "$DOOR_PAUSE_AT" != place ] || while [ ! -e ` + script.Quote(resume) + ` ]; do sleep 0.01; done
[ $(($(wc -l < ` + script.Quote(runs) + `))) -gt "$DOOR_BUSY_FOR" ] || answer=busy
if [ "$answer" = placed ]; then { printf '%s\n' "$line"; cat; } >> ` + script.Quote(kept) + `; fi
echo "$answer"
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOOR_HELD_FOR", strconv.Itoa(sshd.heldFor))
	t.Setenv("DOOR_SCRIPT", sshd.script)
	t.Setenv("DOOR_BUSY_FOR", strconv.Itoa(sshd.busyFor))
	t.Setenv("DOOR_PAUSE_AT", sshd.pauseAt)
	t.Setenv("DOOR_FORBIDDEN", map[bool]string{true: "1"}[sshd.forbidden])
	conn, err := sshx.Dial("door-test-" + filepath.Base(dir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	mc := &Machine{conn: conn}
	keys := 0
	key := func(string, string) (string, error) {
		keys++
		if mc.doorMu.TryLock() {
			mc.doorMu.Unlock()
			t.Error("a key was made without the door's lock")
		}
		if sshd.goneAtKey {
			mc.mu.Lock()
			mc.conn = nil
			mc.mu.Unlock()
		}
		if sshd.noKey || sshd.goneAtKey {
			return "", errors.New("the disk is full")
		}
		return "rk_door", nil
	}
	mc.door = &Door{LocalPort: 7317, Key: key}
	if sshd.pauseAt != "" {
		reached := map[string]string{"forward": asks, "place": runs}[sshd.pauseAt]
		opened := make(chan struct{})
		go func() { mc.openDoor(conn); close(opened) }()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			if _, err := os.Stat(reached); err == nil {
				break
			}
		}
		if mc.doorMu.TryLock() {
			mc.doorMu.Unlock()
			t.Errorf("%s runs without the door's lock", sshd.pauseAt)
		}
		if sshd.goneThere {
			mc.mu.Lock()
			mc.conn = nil
			mc.mu.Unlock()
		}
		if err := os.WriteFile(resume, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		<-opened
	} else if sshd.letGoing {
		mc.doorMu.Lock()
		opened := make(chan struct{})
		go func() { mc.openDoor(conn); close(opened) }()
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(cancels); err == nil {
			t.Error("the door began while the connection before it was letting go")
		}
		mc.doorMu.Unlock()
		<-opened
	} else {
		mc.openDoor(conn)
	}
	placed, _ := os.ReadFile(kept)
	cancelled, _ := os.ReadFile(cancels)
	return doorRun{conn, string(placed), string(cancelled), keys}
}

const heldForGood = 1000

func TestTheDoorStaysOnItsPortWhileThatIsFree(t *testing.T) {
	run := doorOverSSH(t, sshdAtTheDoor{})
	if !strings.Contains(run.placed, "http://127.0.0.1:7318") || !strings.Contains(run.placed, "rk_door") {
		t.Fatalf("placed: %s", run.placed)
	}
	if first, _, _ := strings.Cut(run.cancels, "\n"); !strings.Contains(first, "127.0.0.1:0:") {
		t.Fatalf("what a reused master held on a picked port was not let go first: %s", run.cancels)
	}
}

// TestAConnectionGoneLeavesTheForwardsAlone: the next connection may have its door on the same port by now.
func TestAConnectionGoneLeavesTheForwardsAlone(t *testing.T) {
	run := doorOverSSH(t, sshdAtTheDoor{goneAtKey: true})
	if strings.Count(run.cancels, "127.0.0.1:7318:") != 1 { // the one before asking
		t.Fatalf("cancelled: %s", run.cancels)
	}
}

// TestTheNextDoorWaitsForTheForwardBeingLetGo: a cancel under way reaches the new master, so nothing of the new door
// may stand there yet.
func TestTheNextDoorWaitsForTheForwardBeingLetGo(t *testing.T) {
	if run := doorOverSSH(t, sshdAtTheDoor{letGoing: true}); !strings.Contains(run.placed, "rk_door") {
		t.Fatalf("placed: %q", run.placed)
	}
}

// TestEveryStepOfTheDoorHoldsItsLock: the next connection's door cannot begin while one is on its way to the master.
func TestEveryStepOfTheDoorHoldsItsLock(t *testing.T) {
	for _, step := range []string{"forward", "place"} {
		t.Run(step, func(t *testing.T) {
			if run := doorOverSSH(t, sshdAtTheDoor{pauseAt: step}); !strings.Contains(run.placed, "rk_door") {
				t.Fatalf("placed: %q", run.placed)
			}
		})
	}
}

// TestAConnectionGoneMakesNoKey: a new key would replace the one the next connection placed.
func TestAConnectionGoneMakesNoKey(t *testing.T) {
	if run := doorOverSSH(t, sshdAtTheDoor{pauseAt: "forward", goneThere: true}); run.keys != 0 || run.placed != "" {
		t.Fatalf("%d keys, placed: %q", run.keys, run.placed)
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
	if strings.Count(run.cancels, "127.0.0.1:0:") != 2 { // what a reused master held, and before asking
		t.Fatalf("the door it stands on was let go: %s", run.cancels)
	}
}

// TestPlacingTheDoorOutwaitsTheLocksLeftBehind: the machine stays busy past the tries of any other step, and placing
// goes on for longer than the last lock a dead script left can stand.
func TestPlacingTheDoorOutwaitsTheLocksLeftBehind(t *testing.T) {
	if waited, stands := time.Duration(doorPlaceTries-1)*doorPause, time.Duration(doorLockAhead+1)*30*time.Second; waited <= stands {
		t.Fatalf("placing is asked for %v, a lock stands for %v", waited, stands)
	}
	run := doorOverSSH(t, sshdAtTheDoor{busyFor: 8}) // doorOverSSH gives other steps 6 tries, placing 12
	if !strings.Contains(run.placed, "rk_door") {
		t.Fatalf("placed: %q, cancelled: %s", run.placed, run.cancels)
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
			spec, cancels := "127.0.0.1:7318:", 2 // before asking, and letting it go
			if sshd.heldFor > 0 {
				spec, cancels = "127.0.0.1:0:", 3 // and what a reused master held
			}
			if strings.Count(run.cancels, spec) != cancels {
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

// doorClock puts a `date` first on PATH answering seconds, and a `curl` that runs duringLook: what happened on the
// machine while the script looked at the door. It answers the folder they are in, for a test's own stand-ins.
func doorClock(t *testing.T, seconds int, duringLook string) string {
	t.Helper()
	tools := t.TempDir()
	for name, stub := range map[string]string{
		"date": "#!/bin/sh\ncase \"$1\" in -G) exit 1 ;; esac\necho " + strconv.Itoa(seconds) + "\n",
		"curl": "#!/bin/sh\ncase \"$1\" in -G) exit 1 ;; esac\n" + duringLook + "\n",
	} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	return tools
}

// TestDoorScriptWaitsForTheLocksStanding: at second 3000 the half minute is 100. A lock of now or up to two minutes
// ahead — a clock set back — says busy; one of the past lapsed, and one further ahead was left by a dead script under a
// clock since put right.
func TestDoorScriptWaitsForTheLocksStanding(t *testing.T) {
	home := newDoorHome(t)
	doorClock(t, 3000, ":")
	for halfMinute, want := range map[int]string{100: doorBusy, 101: doorBusy, 100 + doorLockAhead: doorBusy, 101 + doorLockAhead: doorPlaced, 140: doorPlaced, 99: doorPlaced, 40: doorPlaced} {
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

// TestDoorScriptLosingALockAsItTakesItIsBusy: a run of the next half minute clears the lock it finds of the last one, and
// that may be one this run has just made. A shell ends a script on a failed redirection of `:`, with no answer at all.
func TestDoorScriptLosingALockAsItTakesItIsBusy(t *testing.T) {
	home := newDoorHome(t)
	realMkdir, err := exec.LookPath("mkdir")
	if err != nil {
		t.Fatal(err)
	}
	tools := doorClock(t, 3000, ":")
	stub := "#!/bin/sh\ncase \"$1\" in -G) exit 1 ;; esac\n" + script.Quote(realMkdir) + ` "$@" || exit
case "$1" in *agent-door.lock.100) rm -rf "$1" ;; esac
`
	if err := os.WriteFile(filepath.Join(tools, "mkdir"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := home.place(doorURL(DoorPort)); got != doorBusy {
		t.Fatalf("answered %q", got)
	}
}

// TestDoorScriptPlacesNothingOnceItsLocksAreNotItsOwn: another run took them from it during the look — its clock set
// back past them, and put right again — and the locks that stand then are that run's, which this one leaves.
func TestDoorScriptPlacesNothingOnceItsLocksAreNotItsOwn(t *testing.T) {
	for name, c := range map[string]struct {
		duringLook, want string
		locksLeft        int
	}{
		"nothing happened":  {":", doorPlaced, 0},
		"locks taken away":  {`rm -rf "$DOOR_LOCKS"/agent-door.lock.*`, doorBusy, 0},
		"one taken away":    {`rm -rf "$DOOR_LOCKS"/agent-door.lock.101`, doorBusy, 0},
		"another run's now": {`rm -rf "$DOOR_LOCKS"/agent-door.lock.*; mkdir "$DOOR_LOCKS"/agent-door.lock.100 "$DOOR_LOCKS"/agent-door.lock.101`, doorBusy, 2},
	} {
		t.Run(name, func(t *testing.T) {
			home := newDoorHome(t)
			t.Setenv("DOOR_LOCKS", home.dir)
			doorClock(t, 3000, c.duringLook)
			home.write("agent-url", "http://127.0.0.1:9")
			home.write("agent-key", "rk_old")
			if got := home.place(doorURL(50236)); got != c.want {
				t.Fatalf("answered %s, want %s", got, c.want)
			}
			if c.want == doorBusy && (home.read("agent-url") != "http://127.0.0.1:9" || home.read("agent-key") != "rk_old") {
				t.Error("the files changed")
			}
			if left, _ := filepath.Glob(filepath.Join(home.dir, "agent-door.lock.*")); len(left) != c.locksLeft {
				t.Errorf("locks left: %v", left)
			}
		})
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
