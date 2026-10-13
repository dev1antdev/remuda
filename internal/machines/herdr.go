package machines

import (
	"regexp"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/script"
)

// herdr on the machine: its binary and socket, the server started, stopped and asked for its version.
// HerdrBin is herdr's binary on the machine, "" before a remote's first connect.
func (mc *Machine) HerdrBin() string {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.herdrBin
}

// ServerVersions reads the installed herdr's version and the running server's ("" when none runs), freshly.
func (mc *Machine) ServerVersions() (installed, running string, err error) {
	bin, sock := mc.herdrPaths()
	out, err := mc.text(`LC_ALL=C; `+script.Quote(bin)+` --version 2>/dev/null | head -1; HERDR_SOCKET_PATH=`+script.Quote(sock)+` `+script.Quote(bin)+` status server 2>/dev/null | sed -n 's/^version: //p' | head -1`, 20*time.Second)
	if err != nil && strings.TrimSpace(out) == "" {
		return "", "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	installed = herdrSemver.FindString(lines[0])
	if len(lines) > 1 {
		running = herdrSemver.FindString(lines[1])
	}
	return installed, running, nil
}

var herdrSemver = regexp.MustCompile(`\d+\.\d+\.\d+`)

// StopServer stops herdr's server on the machine, and every pane process with it.
func (mc *Machine) StopServer() (string, error) {
	bin, sock := mc.herdrPaths()
	return mc.text(`HERDR_SOCKET_PATH=`+script.Quote(sock)+` `+script.Quote(bin)+` server stop 2>&1`, 30*time.Second)
}

// StartServer starts herdr's server on the machine now, without the spacing of the automatic starts.
func (mc *Machine) StartServer() error {
	bin, sock := mc.herdrPaths()
	if mc.local {
		return herdr.StartServerNow(bin, sock)
	}
	conn, err := mc.remoteConn()
	if err != nil {
		return err
	}
	mc.mu.Lock()
	mc.serverStart = time.Now()
	mc.mu.Unlock()
	_, err = conn.Run(herdr.StartServerScript(bin, sock), 25*time.Second)
	return err
}

func (mc *Machine) herdrPaths() (bin, sock string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	bin, sock = mc.herdrBin, mc.socket
	if bin == "" {
		bin = "herdr"
	}
	return bin, sock
}

// mayStartServer allows one `herdr server` start per two minutes, noting the one it allows.
func (m *Machine) mayStartServer() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.serverStart) < 2*time.Minute {
		return false
	}
	m.serverStart = time.Now()
	return true
}
