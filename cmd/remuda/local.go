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

// socketsUnder keeps the ssh control and forward sockets in the state dir (0700):
// in the shared temp dir another user of the machine could leave a socket at the name.
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

// confirm asks a yes/no question at the terminal; anything but y is no, and so is nobody to ask.
// /dev/null is a character device too, so only the read tells whether anyone is there.
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

// openPage shows the page in the person's browser: on a Mac at the desk only.
// The address carries the token, so it reaches osascript on stdin, never a command line ps would show.
func openPage(page string) bool {
	if runtime.GOOS != "darwin" || os.Getenv("SSH_CONNECTION") != "" {
		return false
	}
	cmd := exec.Command("osascript")
	cmd.Stdin = strings.NewReader(openScript(page))
	return cmd.Run() == nil
}

// openScript is the AppleScript that opens page as one string literal, its quotes and backslashes escaped.
func openScript(page string) string {
	return `open location "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(page) + `"` + "\n"
}
