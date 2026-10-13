package machines

import (
	"os"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// A Machine is a shell.Host: scripts run here through sh -c, or over the ssh master.

// ID is the machine's id in the registry.
func (mc *Machine) ID() string { return mc.Cfg.ID }

// Local reports whether this is the machine remuda runs on.
func (mc *Machine) Local() bool { return mc.local }

// Home is the user's home directory on this machine.
func (mc *Machine) Home() string {
	if mc.local {
		home, _ := os.UserHomeDir()
		return home
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.home
}

// shell is where scripts run: here, or the ssh master while it is up.
func (mc *Machine) shell() (shell.Shell, error) {
	if mc.local {
		return shell.Local{Name: mc.Cfg.ID}, nil
	}
	return mc.remoteConn()
}

func (mc *Machine) Run(script string, timeout time.Duration) ([]byte, error) {
	sh, err := mc.shell()
	if err != nil {
		return nil, err
	}
	return sh.Run(script, timeout)
}

func (mc *Machine) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	sh, err := mc.shell()
	if err != nil {
		return err
	}
	return sh.Stream(script, timeout, sink)
}

// text is Run with the output as text.
func (mc *Machine) text(script string, timeout time.Duration) (string, error) {
	return shell.Text(mc, script, timeout)
}
