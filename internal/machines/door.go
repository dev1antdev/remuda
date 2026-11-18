package machines

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/sshx"
)

// The door is how a remote machine's agents reach this remuda's task list: this remuda's own port forwarded back
// over the ssh master to the machine's loopback, and two files placed in the machine's state dir, the tasks-only key
// and that address, which `remuda mcp` and `remuda tasks` read there.

// DoorPort is where, on a remote machine, this remuda answers while the port is free there (doorForward).
const DoorPort = 7318

// Door is what opening one takes: this remuda's port, and a tasks-only key for a machine (the one it already holds
// when that still passes, else a new one).
type Door struct {
	LocalPort int
	Key       func(name, have string) (string, error)
}

// What DoorScript answers on its last line: the door is placed, another one stays, or another cockpit is placing its own.
const (
	doorPlaced = "placed"
	doorTaken  = "taken"
	doorBusy   = "busy"
)

func doorURL(port int) string { return fmt.Sprintf("http://127.0.0.1:%d", port) }

// DoorScript places the door at url in the machine's state dir under a lock, the key read on stdin, each file replaced
// whole, and answers how it went: a door that answers, or that cannot be asked, may stay (CLAUDE.md, "Cockpits sharing a
// machine"). sh has no lock that dies with its process on both macOS and Linux, hence locks named by the time.
func DoorScript(url string) string {
	return "umask 077\n" + script.StateDir("d") + `
url=` + script.Quote(url) + `
usual=` + script.Quote(doorURL(DoorPort)) + `
mkdir -p "$d" && chmod 700 "$d" || exit 1
now=$(($(date +%s) / 30))
for held in "$d"/agent-door.lock.*; do
  [ -d "$held" ] || continue
  slot=${held##*.}
  if [ "$slot" -ge "$now" ] 2>/dev/null && [ "$slot" -le $((now + ` + strconv.Itoa(doorLockAhead) + `)) ]; then echo ` + doorBusy + `; exit 0; fi
  rmdir "$held" 2>/dev/null
done
mkdir "$d/agent-door.lock.$now" 2>/dev/null || { echo ` + doorBusy + `; exit 0; }
mkdir "$d/agent-door.lock.$((now + 1))" 2>/dev/null || { rmdir "$d/agent-door.lock.$now"; echo ` + doorBusy + `; exit 0; }
trap 'rmdir "$d/agent-door.lock.$now" "$d/agent-door.lock.$((now + 1))" 2>/dev/null' EXIT
was=$(head -n 1 "$d/agent-url" 2>/dev/null)
moved=$(head -n 1 "$d/agent-door-moved" 2>/dev/null)
if [ -n "$was" ] && [ "$was" != "$url" ] && { [ "$url" != "$usual" ] || [ "$was" = "$moved" ]; }; then
  command -v curl >/dev/null 2>&1 || { echo ` + doorTaken + `; exit 0; }
  case "$(` + script.Health(`"$was"`) + `)" in *'"ok":true'*) echo ` + doorTaken + `; exit 0 ;; esac
fi
still=$(($(date +%s) / 30))
[ "$still" -ge "$now" ] && [ "$still" -le $((now + 1)) ] || { echo ` + doorBusy + `; exit 0; }
if [ "$url" = "$usual" ]; then
  rm -f "$d/agent-door-moved"
else
  printf '%s\n' "$url" > "$d/agent-door-moved.new" && mv -f "$d/agent-door-moved.new" "$d/agent-door-moved" || exit 1
fi
cat > "$d/agent-key.new" && mv -f "$d/agent-key.new" "$d/agent-key" && printf '%s\n' "$url" > "$d/agent-url.new" && mv -f "$d/agent-url.new" "$d/agent-url" && chmod 600 "$d/agent-key" "$d/agent-url" || exit 1
echo ` + doorPlaced
}

// doorTries and doorPause: a step of opening the door that failed is asked again, a minute in all, while this
// connection is the machine's. doorPlaceTries is placing it, which outwaits the locks a dead script left: they stand
// for doorLockAhead half minutes at most, and this runs once per connection. doorPortAsks is how many asks DoorPort gets
// before anything else is tried: after this remuda's own restart the machine's sshd holds it a moment for the
// connection that just ended.
var doorTries, doorPlaceTries, doorPause, doorPortAsks = 13, 33, 5 * time.Second, 3

// doorLockAhead is how many half minutes ahead of the machine's clock a lock still counts, two minutes.
const doorLockAhead = 4

// trySoon runs the step until it holds, the tries run out or the connection is no longer current.
func trySoon(tries int, step func() error, current func() bool) error {
	var err error
	for try := 0; try < tries; try++ {
		if try > 0 {
			time.Sleep(doorPause)
			if !current() {
				return errors.New("the connection went away")
			}
		}
		if err = step(); err == nil {
			return nil
		}
	}
	return err
}

// doorForward opens the door on DoorPort or, once that was refused doorPortAsks times, on a port the sshd picks: a
// connection the network lost holds DoorPort for hours, sshd not noticing. DoorScript decides whether it may stand.
func doorForward(conn *sshx.Conn, localPort, asked int) (int, error) {
	port, refused := conn.ForwardBack(DoorPort, localPort)
	if refused == nil {
		return port, nil
	}
	if asked < doorPortAsks {
		return 0, refused
	}
	if port, err := conn.ForwardBack(0, localPort); err == nil {
		return port, nil
	}
	return 0, refused
}

// openDoor lets the machine's agents reach this remuda's task list, once per connection; a sshd that forbids the
// reverse forward, a state dir that cannot be written or another cockpit's live door is a log line, never a failure of
// the connection, and a forward no door came of is let go.
func (mc *Machine) openDoor(conn *sshx.Conn) {
	door := mc.door
	if door == nil || door.LocalPort == 0 || door.Key == nil {
		return
	}
	current := func() bool {
		mc.mu.Lock()
		defer mc.mu.Unlock()
		return mc.conn == conn
	}
	conn.CancelForwardBack(0, door.LocalPort) // one this cockpit left on a reused master is no other cockpit's live door
	var port, asked int
	forward := func() (err error) {
		asked++
		port, err = doorForward(conn, door.LocalPort, asked)
		return err
	}
	if err := trySoon(doorTries, forward, current); err != nil {
		log.Printf("%s: no door for its agents to the tasks here: %v", mc.Cfg.Host, err)
		return
	}
	placed := false
	defer func() {
		if placed || !current() { // a Conn reaches whichever master the host has now: never the next connection's door
			return
		}
		if port != DoorPort {
			port = 0
		}
		conn.CancelForwardBack(port, door.LocalPort)
	}()
	mc.mu.Lock()
	have := mc.doorKey
	mc.mu.Unlock()
	key, err := door.Key(mc.Label(), have)
	if err != nil {
		log.Printf("%s: no key for its agents: %v", mc.Cfg.Host, err)
		return
	}
	mc.mu.Lock()
	mc.doorKey = key
	mc.mu.Unlock()
	var answer string
	place := func() error {
		out, err := conn.RunWithInput(DoorScript(doorURL(port)), []byte(key+"\n"), 15*time.Second)
		if err != nil {
			return err
		}
		lines := script.Lines(string(out))
		if len(lines) == 0 {
			return errors.New("the door script said nothing")
		}
		if answer = strings.TrimSpace(lines[len(lines)-1]); answer == doorBusy {
			return errors.New("another cockpit is placing its door")
		}
		return nil
	}
	err = trySoon(doorPlaceTries, place, current)
	if placed = err == nil && answer == doorPlaced; placed {
		return
	}
	if err == nil {
		err = errors.New("a live door stands there, or it could not be looked at, so it stays as it is")
	}
	log.Printf("%s: the door files for its agents were not placed: %v", mc.Cfg.Host, err)
}
