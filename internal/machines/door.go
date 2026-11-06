package machines

import (
	"fmt"
	"log"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/sshx"
)

// The door is how a remote machine's agents reach this remuda's task list: this remuda's own port forwarded back
// over the ssh master to DoorPort on the machine's loopback, and two files placed in the machine's state dir, the
// tasks-only key and that address, which `remuda mcp` and `remuda tasks` read there.

// DoorPort is where, on a remote machine, this remuda answers.
const DoorPort = 7318

// Door is what opening one takes: this remuda's port, and a tasks-only key for a machine (the one it already holds
// when that still passes, else a new one).
type Door struct {
	LocalPort int
	Key       func(name, have string) (string, error)
}

// DoorScript writes the key read on stdin and the address into the machine's state dir, private to the user:
// the dir remuda keeps there (`state.Dir()` computed in sh), the key never on a command line.
func DoorScript(url string) string {
	return `umask 077
case "$(uname -s)" in
  Darwin) d="$HOME/Library/Application Support/remuda" ;;
  *) d="${XDG_CONFIG_HOME:-$HOME/.config}/remuda" ;;
esac
mkdir -p "$d" && chmod 700 "$d" && cat > "$d/agent-key.new" && mv -f "$d/agent-key.new" "$d/agent-key" && printf '%s\n' ` + script.Quote(url) + ` > "$d/agent-url" && chmod 600 "$d/agent-key" "$d/agent-url"`
}

// openDoor lets the machine's agents reach this remuda's task list, once per connection; a sshd that forbids the
// reverse forward, or a state dir that cannot be written, is a log line, never a failure of the connection.
func (mc *Machine) openDoor(conn *sshx.Conn) {
	door := mc.door
	if door == nil || door.LocalPort == 0 || door.Key == nil {
		return
	}
	if err := conn.ForwardBack(DoorPort, door.LocalPort); err != nil {
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
	if _, err := conn.RunWithInput(DoorScript(fmt.Sprintf("http://127.0.0.1:%d", DoorPort)), []byte(key+"\n"), 15*time.Second); err != nil {
		log.Printf("%s: the door files for its agents could not be placed: %v", mc.Cfg.Host, err)
	}
}
