package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/sshx"
)

// socketsUnder keeps the ssh control and forward sockets in the state dir
// (0700, the person's own) rather than the shared temp dir, where another
// user of the machine could leave a socket at the name.
func socketsUnder(dir string) {
	d := filepath.Join(dir, "ssh")
	if err := os.MkdirAll(d, 0o700); err == nil {
		sshx.SocketDir = d
	}
}

// interactive says a person is at the terminal: stderr is one.
func interactive() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// confirm asks a yes/no question at the terminal; anything but y is no.
// Where nobody can answer — stdin is a pipe, a file, or ends without a
// line (/dev/null is a character device too, so only the read tells) —
// the answer is no, and it says how to mean yes.
func confirm(question string) bool {
	const how = "nobody to ask here: say -yes to go ahead without being asked"
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		fmt.Println(how)
		return false
	}
	fmt.Print(question)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Println("\n" + how)
		return false
	}
	return yes(line)
}

func yes(answer string) bool {
	a := strings.ToLower(strings.TrimSpace(answer))
	return a == "y" || a == "yes"
}

// answers waits for the remuda at addr to say it is up.
func answers(addr string, wait time.Duration) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	for deadline := time.Now().Add(wait); ; time.Sleep(300 * time.Millisecond) {
		if res, err := client.Get("http://" + addr + "/api/health"); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

// openPage shows the page in the person's browser: on a Mac, at the desk.
// The address carries the token, so it reaches osascript on stdin — a
// command line is what ps shows to every user of the machine. On Linux
// the browser started for it would keep the address in its own arguments
// for as long as it runs, so there the person follows the printed link.
func openPage(page string) bool {
	if runtime.GOOS != "darwin" || os.Getenv("SSH_CONNECTION") != "" {
		return false
	}
	cmd := exec.Command("osascript")
	cmd.Stdin = strings.NewReader(openScript(page))
	return cmd.Run() == nil
}

// openScript is the AppleScript that opens page: one string literal, its
// quotes and backslashes escaped.
func openScript(page string) string {
	return `open location "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(page) + `"` + "\n"
}
