package machines

import (
	"errors"
	"fmt"
	"log"
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

// DoorScript writes the key read on stdin and the address into the machine's state dir, private to the user:
// the dir remuda keeps there (`state.Dir()` computed in sh), the key never on a command line, each file replaced whole
// since a running `remuda mcp` reads them for every call.
func DoorScript(url string) string {
	return "umask 077\n" + script.StateDir("d") + `
mkdir -p "$d" && chmod 700 "$d" && cat > "$d/agent-key.new" && mv -f "$d/agent-key.new" "$d/agent-key" && printf '%s\n' ` + script.Quote(url) + ` > "$d/agent-url.new" && mv -f "$d/agent-url.new" "$d/agent-url" && chmod 600 "$d/agent-key" "$d/agent-url"`
}

// doorTries and doorPause: a step of opening the door that failed is asked again, a minute in all, while this
// connection is the machine's. doorPortAsks is how many asks DoorPort gets before anything else is tried: after this
// remuda's own restart the machine's sshd holds it a moment for the connection that just ended.
var doorTries, doorPause, doorPortAsks = 13, 5 * time.Second, 3

// errDoorTaken leaves the machine's door to the remuda whose door it is.
var errDoorTaken = errors.New("the door its agent-url names answers, so it stays as it is")

// doorAnswersScript succeeds when the door the machine's agent-url names answers: a live remuda's, where the port of
// a connection the network lost takes the request and says nothing.
func doorAnswersScript() string {
	return script.StateDir("d") + `
url=$(head -n 1 "$d/agent-url" 2>/dev/null)
[ -n "$url" ] || exit 1
case "$(curl -s -m 2 "$url/api/health" 2>/dev/null)" in *'"ok":true'*) exit 0 ;; esac
exit 1`
}

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

// doorForward opens the door on DoorPort or, once that was refused doorPortAsks times and no live door stands on the
// machine, on a port the sshd picks: a connection the network lost holds DoorPort for hours, sshd not noticing.
func doorForward(conn *sshx.Conn, localPort, asked int) (int, error) {
	port, refused := conn.ForwardBack(DoorPort, localPort)
	if refused == nil {
		return port, nil
	}
	if asked < doorPortAsks {
		return 0, refused
	}
	if _, err := conn.Run(doorAnswersScript(), 10*time.Second); err == nil {
		return 0, errDoorTaken
	}
	if port, err := conn.ForwardBack(0, localPort); err == nil {
		return port, nil
	}
	return 0, refused
}

// openDoor lets the machine's agents reach this remuda's task list, once per connection; a sshd that forbids the
// reverse forward, or a state dir that cannot be written, is a log line, never a failure of the connection.
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
	place := func() error {
		_, err := conn.RunWithInput(DoorScript(fmt.Sprintf("http://127.0.0.1:%d", port)), []byte(key+"\n"), 15*time.Second)
		return err
	}
	if err := trySoon(place, current); err != nil {
		log.Printf("%s: the door files for its agents could not be placed: %v", mc.Cfg.Host, err)
	}
}
