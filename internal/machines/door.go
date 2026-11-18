package machines

import (
	"errors"
	"fmt"
	"log"
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

// DoorScript places the door at url in the machine's state dir, private to the user: the dir remuda keeps there
// (`state.Dir()` computed in sh), the key read on stdin, never on a command line, each file replaced whole since a
// running `remuda mcp` reads them for every call. Cockpits sharing the machine go one at a time under the locks of this
// half minute and the next: none is taken while one of now or later stands, so a clock set back waits for it, those of a
// script that died lapse by themselves, and one whose clock left them meanwhile places nothing (sh has no lock that dies
// with its process on both macOS and Linux).
// A door that answers stays — asked straight, past any proxy, and when nobody
// can ask too: one on a port of its own (agent-door-moved names it, so a cockpit on the machine itself is none) against
// any newcomer, any other against a door that would move. The port of a connection the network lost says nothing.
func DoorScript(url string) string {
	return "umask 077\n" + script.StateDir("d") + `
url=` + script.Quote(url) + `
usual=` + script.Quote(doorURL(DoorPort)) + `
mkdir -p "$d" && chmod 700 "$d" || exit 1
now=$(($(date +%s) / 30))
for held in "$d"/agent-door.lock.*; do
  [ -d "$held" ] || continue
  if [ "${held##*.}" -ge "$now" ] 2>/dev/null; then echo ` + doorBusy + `; exit 0; fi
  rmdir "$held" 2>/dev/null
done
mkdir "$d/agent-door.lock.$now" 2>/dev/null || { echo ` + doorBusy + `; exit 0; }
mkdir "$d/agent-door.lock.$((now + 1))" 2>/dev/null || { rmdir "$d/agent-door.lock.$now"; echo ` + doorBusy + `; exit 0; }
trap 'rmdir "$d/agent-door.lock.$now" "$d/agent-door.lock.$((now + 1))" 2>/dev/null' EXIT
was=$(head -n 1 "$d/agent-url" 2>/dev/null)
moved=$(head -n 1 "$d/agent-door-moved" 2>/dev/null)
if [ -n "$was" ] && [ "$was" != "$url" ] && { [ "$url" != "$usual" ] || [ "$was" = "$moved" ]; }; then
  command -v curl >/dev/null 2>&1 || { echo ` + doorTaken + `; exit 0; }
  case "$(curl -q -s -m 2 --noproxy '*' "$was/api/health" 2>/dev/null)" in *'"ok":true'*) echo ` + doorTaken + `; exit 0 ;; esac
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
// connection is the machine's. doorPortAsks is how many asks DoorPort gets before anything else is tried: after this
// remuda's own restart the machine's sshd holds it a moment for the connection that just ended.
var doorTries, doorPause, doorPortAsks = 13, 5 * time.Second, 3

// trySoon runs the step until it holds, the tries run out or the connection is no longer current.
func trySoon(step func() error, current func() bool) error {
	var err error
	for try := 0; try < doorTries; try++ {
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
	var port, asked int
	forward := func() (err error) {
		asked++
		port, err = doorForward(conn, door.LocalPort, asked)
		return err
	}
	if err := trySoon(forward, current); err != nil {
		log.Printf("%s: no door for its agents to the tasks here: %v", mc.Cfg.Host, err)
		return
	}
	placed := false
	defer func() {
		if placed {
			return
		}
		forwarded := port
		if port != DoorPort {
			forwarded = 0
		}
		conn.CancelForwardBack(forwarded, door.LocalPort)
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
		fields := strings.Fields(string(out))
		if len(fields) == 0 {
			return errors.New("the door script said nothing")
		}
		if answer = fields[len(fields)-1]; answer == doorBusy {
			return errors.New("another cockpit is placing its door")
		}
		return nil
	}
	err = trySoon(place, current)
	if placed = err == nil && answer == doorPlaced; placed {
		return
	}
	if err == nil {
		err = errors.New("a live door stands there, or it could not be looked at, so it stays as it is")
	}
	log.Printf("%s: the door files for its agents were not placed: %v", mc.Cfg.Host, err)
}
