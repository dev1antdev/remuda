package preview

import (
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// line is one line of the script's answer
func line(fields ...string) string { return strings.Join(fields, "\x1f") + "\n" }

// a Mac's answer: lsof lists a socket per fd and an IPv6 one on its own; ControlCenter runs in /
var macAnswer = line("5000", "*", "632", "ControlCenter", "/", "/", "/System/Library/CoreServices/ControlCenter.app/Contents/MacOS/ControlCenter") +
	line("5173", "[::1]", "4100", "node", "/Users/me/.herdr/worktrees/app/task-1", "/Users/me/.herdr/worktrees/app/task-1", "node /Users/me/.herdr/worktrees/app/task-1/node_modules/.bin/vite") +
	line("5173", "[::1]", "4100", "node", "/Users/me/.herdr/worktrees/app/task-1", "/Users/me/.herdr/worktrees/app/task-1", "node …/vite") +
	line("3000", "127.0.0.1", "4200", "node", "/Users/me/code/app/web", "/Users/me/code/app", "node node_modules/.bin/next dev") +
	line("3000", "[::1]", "4200", "node", "/Users/me/code/app/web", "/Users/me/code/app", "node node_modules/.bin/next dev") +
	line("7320", "127.0.0.1", "1334", "node", "/Users/me", "/Users/me", "code-server") +
	line("7317", "127.0.0.1", "900", "remuda", "/Users/me", "/", "remuda -addr 127.0.0.1:7317") +
	line("7318", "127.0.0.1", "950", "sshd-sess", "", "", "sshd-session: me") +
	line("8000", "*", "4300", "python3", "", "/Users/me/code/notes", "python3 -m http.server 8000") +
	line("22", "*", "1", "launchd", "/", "/", "launchd") +
	line("9100", "192.168.1.5", "4400", "node", "/Users/me/code/app", "", "node server.js")

// a Linux answer: ss reads `local` whole, an IPv6 wildcard as [::]
var linuxAnswer = line("5173", "[::]", "200", "node", "/home/me/app", "/home/me/app", "node vite --host") +
	line("8080", "0.0.0.0", "300", "uvicorn", "/home/me/api", "/home/me", "/usr/bin/python3 -m uvicorn main:app --port 8080") +
	line("6000", "127.0.0.1", "310", "node", "/home/me/x", "", "node x.js")

func ports(list []Listener) string {
	var out []string
	for _, l := range list {
		out = append(out, strconv.Itoa(l.Port)+"@"+l.Dial+"="+l.Label)
	}
	return strings.Join(out, " ")
}

func TestParseKeepsTheAppsServersOnly(t *testing.T) {
	fake := &shell.Fake{Name: "gog", Remote: true, Answer: func(string) (string, error) { return macAnswer, nil }}
	list, err := Find(fake, map[int]bool{7317: true, 7318: true, 7320: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ports(list), "3000@127.0.0.1=next 5173@::1=vite 8000@127.0.0.1=http.server 9100@192.168.1.5="; got != want {
		t.Fatalf("listeners %s, want %s", got, want)
	}
	if list[2].Dir != "/Users/me/code/notes" {
		t.Errorf("a server with no folder of its own goes by its parent's: %q", list[2].Dir)
	}
	if list[0].Address != "127.0.0.1" || list[1].Address != "::1" || list[2].Address != "*" {
		t.Errorf("addresses %q %q %q", list[0].Address, list[1].Address, list[2].Address)
	}
	if got := ports(Parse(linuxAnswer, nil)); got != "5173@::1=vite 6000@127.0.0.1= 8080@127.0.0.1=uvicorn" {
		t.Fatalf("linux listeners %s", got)
	}
}

func TestMatchNamesTheDeepestWorkspace(t *testing.T) {
	list := Match(Parse(macAnswer, nil), map[string]string{
		"w1": "/Users/me/code/app",
		"w2": "/Users/me/.herdr/worktrees/app/task-1",
		"w3": "/Users/me/.herdr/worktrees/app",
		"w4": "/",
	})
	got := map[int]string{}
	for _, l := range list {
		got[l.Port] = l.Workspace
	}
	if got[3000] != "w1" || got[5173] != "w2" || got[8000] != "" || got[9100] != "w1" {
		t.Fatalf("workspaces %v", got)
	}
	real := Match([]Listener{{Port: 1, Dir: "/private/tmp/e2e/app"}, {Port: 2, Dir: "/private/etc/x"}, {Port: 3, Dir: "/tmp/e2e/notes"}}, map[string]string{"w1": "/tmp/e2e/app", "w2": "/etc", "w3": "/private/tmp/e2e/notes"})
	if real[0].Workspace != "w1" || real[1].Workspace != "" || real[2].Workspace != "w3" {
		t.Fatalf("a Mac's real /private/tmp path, on either side: %+v", real)
	}
}

// serveHere starts python3's http.server in a folder of its own on a free loopback port, skipping the test where
// neither python3 nor a way to list the ports is here
func serveHere(t *testing.T) (port, pid int, dir string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	_, lsofErr := exec.LookPath("lsof")
	_, ssErr := exec.LookPath("ss")
	if lsofErr != nil && ssErr != nil {
		t.Skip("neither lsof nor ss")
	}
	dir, err = filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port = probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	server := exec.Command(python, "-m", "http.server", strconv.Itoa(port), "--bind", "127.0.0.1")
	server.Dir = dir
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Process.Kill(); _ = server.Wait() })
	for i := 0; i < 50; i++ {
		if c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
			c.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return port, server.Process.Pid, dir
}

func TestTheScriptFindsAServerRunningHere(t *testing.T) {
	port, pid, dir := serveHere(t)
	list, err := Find(shell.Local{Name: "local"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range list {
		if l.Port == port {
			if l.Dir != dir || l.Label != "http.server" || l.Dial != "127.0.0.1" || l.PID != pid {
				t.Fatalf("found %+v, want the folder %s", l, dir)
			}
			return
		}
	}
	t.Fatalf("port %d not found among %s", port, ports(list))
}
