package machines

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// canonicalWithin bounds the read of a folder's own spelling.
const canonicalWithin = 5 * time.Second

// A Machine is a shell.Host: scripts run here through sh -c, or over the ssh master.

// ID is the machine's id in the registry.
func (mc *Machine) ID() string { return mc.Cfg.ID }

// Local reports whether this is the machine remuda runs on.
func (mc *Machine) Local() bool { return mc.local }

// OS is the machine's system: darwin, linux or windows, "" for a remote before its first connect.
func (mc *Machine) OS() string {
	if mc.local {
		return runtime.GOOS
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.os
}

// Windows says the machine is a Windows PC: its paths spelled C:\Users\me, its herdr reached through remuda.exe.
func (mc *Machine) Windows() bool { return mc.OS() == "windows" }

// Paths is how the machine spells its paths.
func (mc *Machine) Paths() rpath.Style { return rpath.Style{Windows: mc.Windows()} }

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

// Canonical is a folder the way the machine spells it — every link resolved before a .. in the path too (`cd -P`),
// Windows' own separators and drive letter — so two spellings of one folder meet in remuda's keys: the documents on
// its core, the triggers and the repository settings alike. The folder as given when the machine cannot say (away, or
// no such folder): a folder remuda cannot reach is not silently moved to another.
func (mc *Machine) Canonical(dir string) string {
	if dir == "" {
		return ""
	}
	style := mc.Paths()
	here := `$(pwd -P)`
	if style.Windows { // Git Bash answers /cygdrive/c/…, which is Windows' own spelling only through cygpath
		here = `$(cygpath -w "$(pwd -P)")`
	}
	out, err := shell.Text(mc, "cd -P -- "+script.Quote(style.Slash(dir))+` 2>/dev/null && printf '%s\n' "`+here+`"`, canonicalWithin)
	if said := strings.TrimSuffix(strings.TrimSuffix(out, "\n"), "\r"); err == nil && strings.TrimSpace(said) != "" {
		return style.Clean(said)
	}
	return dir
}

// StateDir is the folder remuda keeps on this machine, "" for a remote before its first connect.
func (mc *Machine) StateDir() string {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.stateDir
}

// shell is where scripts run: here, or the ssh master while it is up.
func (mc *Machine) shell() (shell.Shell, error) {
	if mc.local {
		return shell.Local{Name: mc.ID()}, nil
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

// PutFile writes data whole to a file on the machine, readable by its user alone: here directly, over the ssh master on a remote.
func (mc *Machine) PutFile(path string, data []byte, timeout time.Duration) error {
	if mc.local {
		return os.WriteFile(path, data, 0o600)
	}
	conn, err := mc.remoteConn()
	if err != nil {
		return err
	}
	return conn.Put(path, data, timeout)
}

// text is Run with the output as text.
func (mc *Machine) text(script string, timeout time.Duration) (string, error) {
	return shell.Text(mc, script, timeout)
}
