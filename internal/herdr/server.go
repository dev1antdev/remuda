package herdr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dev1antdev/remuda/internal/sshx"
)

var (
	startMu   sync.Mutex
	lastStart time.Time
)

// EnsureServer pings the herdr server at socket and starts one when none answers, at most every two minutes.
func EnsureServer(bin, socket string) error {
	if _, _, err := NewClient(socket).Ping(); err == nil {
		return nil
	}
	startMu.Lock()
	defer startMu.Unlock()
	if _, _, err := NewClient(socket).Ping(); err == nil {
		return nil
	}
	if time.Since(lastStart) < 2*time.Minute {
		return fmt.Errorf("herdr server started %s ago and does not answer yet", time.Since(lastStart).Round(time.Second))
	}
	return start(bin, socket)
}

// StartServerNow is EnsureServer without the two-minute spacing: the person asked for the restart.
func StartServerNow(bin, socket string) error {
	startMu.Lock()
	defer startMu.Unlock()
	if _, _, err := NewClient(socket).Ping(); err == nil {
		return nil
	}
	return start(bin, socket)
}

// start runs the server detached and waits for it to answer; startMu held.
func start(bin, socket string) error {
	lastStart = time.Now()
	home, _ := os.UserHomeDir()
	cmd := serverCommand(bin, socket, home)
	logPath := filepath.Join(filepath.Dir(socket), "herdr-server.out")
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
		defer f.Close()
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start herdr server: %w", err)
	}
	go cmd.Wait()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		if _, _, err := NewClient(socket).Ping(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("herdr server started but does not answer at %s yet", socket)
}

// serverCommand is `bin server` as the user's terminal would start it: login shell, home, a login's environment.
// Every pane inherits the server's environment, and herdr listens where HERDR_SOCKET_PATH says, ignoring HOME.
func serverCommand(bin, socket, home string) *exec.Cmd {
	var cmd *exec.Cmd
	if sh := os.Getenv("SHELL"); loginShell(sh) {
		cmd = exec.Command(sh, "-l", "-c", "exec "+sshx.Quote(bin)+" server")
	} else {
		cmd = exec.Command(bin, "server")
	}
	cmd.Env = append(cleanEnv(), "HERDR_SOCKET="+socket, "HERDR_SOCKET_PATH="+socket)
	if home != "" {
		cmd.Dir = home
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // its own session: it stays when remuda exits
	return cmd
}

func loginShell(sh string) bool {
	if sh == "" || !filepath.IsAbs(sh) {
		return false
	}
	st, err := os.Stat(sh)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// cleanEnv is what a fresh login has; the service's own variables stay behind.
func cleanEnv() []string {
	keep := map[string]bool{"HOME": true, "PATH": true, "USER": true, "LOGNAME": true, "SHELL": true, "TMPDIR": true, "LANG": true, "SSH_AUTH_SOCK": true, "TERM": true}
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if keep[k] || strings.HasPrefix(k, "LC_") || strings.HasPrefix(k, "XDG_") {
			out = append(out, kv)
		}
	}
	return out
}

// StartServerScript is the same start as a POSIX script over ssh: unless a server runs, start one and wait for it.
func StartServerScript(bin, socket string) string {
	q := sshx.Quote(bin)
	s := sshx.Quote(socket)
	up := `HERDR_SOCKET_PATH=` + s + ` ` + q + ` status server 2>/dev/null | grep -q "status: running"`
	return `if ` + up + `; then exit 0; fi; ` +
		`mkdir -p "$(dirname ` + s + `)"; cd "$HOME" 2>/dev/null; export HERDR_SOCKET=` + s + ` HERDR_SOCKET_PATH=` + s + `; unset HERDR_ENV HERDR_PANE_ID TS_AUTHKEY; ` +
		`if [ -n "${SHELL:-}" ] && [ -x "$SHELL" ]; then nohup "$SHELL" -l -c ` + sshx.Quote("exec "+q+" server") + ` >> "$(dirname ` + s + `)/herdr-server.out" 2>&1 < /dev/null & ` +
		`else nohup ` + q + ` server >> "$(dirname ` + s + `)/herdr-server.out" 2>&1 < /dev/null & fi; ` +
		`i=0; while [ $i -lt 30 ]; do if ` + up + `; then exit 0; fi; sleep 0.5; i=$((i+1)); done; exit 1`
}
