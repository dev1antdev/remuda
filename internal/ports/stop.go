package ports

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/preview"
	"github.com/dev1antdev/remuda/internal/shell"
)

// StopTimeout bounds the look and the signal.
var StopTimeout = 20 * time.Second

// ErrNotListening is a stop of a port the process asked about does not listen on now — gone already, or never a
// listener the person could stop: remuda's, herdr's, ssh's, Tailscale's, or another user's.
var ErrNotListening = errors.New("nothing of yours listens there now")

// Stop sends SIGTERM to the process listening on the machine's port, once a fresh look finds that very pid there
// among the listeners the person may stop — never one in skip, never remuda's, herdr's, ssh's or Tailscale's — and
// answers what it stopped.
func Stop(host shell.Shell, skip map[int]bool, pid, port int) (preview.Listener, error) {
	if pid <= 1 {
		return preview.Listener{}, ErrNotListening
	}
	list, err := preview.Find(host, skip)
	if err != nil {
		return preview.Listener{}, err
	}
	var found *preview.Listener
	for i := range list {
		if list[i].Port == port && list[i].PID == pid {
			found = &list[i]
		}
	}
	if found == nil {
		return preview.Listener{}, fmt.Errorf("%w: no process %d on port %d", ErrNotListening, pid, port)
	}
	out, err := shell.Text(host, "kill -TERM "+strconv.Itoa(pid)+" 2>&1", StopTimeout)
	if err != nil {
		if words := strings.TrimSpace(out); words != "" {
			return *found, fmt.Errorf("kill %d: %s", pid, words)
		}
		return *found, err
	}
	return *found, nil
}
