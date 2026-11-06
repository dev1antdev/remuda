package auth

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/state"
)

// The door is how the agents on a machine reach this remuda's task list: two files in that machine's state dir,
// the tasks-only key and the address, read by `remuda mcp` and `remuda tasks` there.
// On a remote machine the address is the reverse forward of this remuda's port over the ssh master (machines.DoorPort).
const (
	DoorKeyFile = "agent-key"
	DoorURLFile = "agent-url"
)

// EnsureDoor keeps the cockpit's own door files: the key file holds a key of the name given (minted when the file is
// missing or holds a key the store no longer knows), the url file the loopback address.
func EnsureDoor(dir string, keys *Keys, name, url string) error {
	keyPath := filepath.Join(dir, DoorKeyFile)
	have, _ := os.ReadFile(keyPath)
	current := strings.TrimSpace(string(have))
	if rec, ok := keys.Check(current); !ok || rec.Name != name {
		if ok {
			keys.Revoke(rec.ID) // the machine was renamed: the key under the old name goes with the file's
		}
		key, _, err := keys.Mint(name)
		if err != nil {
			return fmt.Errorf("door: %w", err)
		}
		if err := state.WriteFile(keyPath, []byte(key+"\n")); err != nil {
			return fmt.Errorf("door: %w", err)
		}
	}
	urlPath := filepath.Join(dir, DoorURLFile)
	if was, _ := os.ReadFile(urlPath); strings.TrimSpace(string(was)) != url {
		if err := state.WriteFile(urlPath, []byte(url+"\n")); err != nil {
			return fmt.Errorf("door: %w", err)
		}
	}
	return nil
}

// LoopbackURL is the address the agents on this machine reach the server at: its own port on 127.0.0.1, whatever host -addr named.
func LoopbackURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "http://" + addr
	}
	return "http://127.0.0.1:" + port
}

// PortOf is the port -addr listens on, 0 when it names none.
func PortOf(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
