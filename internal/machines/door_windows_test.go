//go:build windows

package machines_test

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/tasksapi"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsTheDoorIsWhereRemudaExeLooksForIt places the door files over the runner's OpenSSH, the key on stdin as a
// cockpit hands it over, and reads them back the way remuda.exe mcp and remuda.exe tasks do: state.Dir, %APPDATA%\remuda.
func TestOnWindowsTheDoorIsWhereRemudaExeLooksForIt(t *testing.T) {
	wintest.Live(t)
	key := "rk_door-check" // gitleaks:allow (no key, a word)
	if out, err := wintest.Remote(t).RunWithInput(machines.DoorScript(fmt.Sprintf("http://127.0.0.1:%d", machines.DoorPort)), []byte(key+"\n"), 30*time.Second); err != nil {
		t.Fatalf("the door script: %v\n%s", err, out)
	}
	door, err := tasksapi.FindDoor()
	if err != nil {
		t.Fatal(err)
	}
	if door.Key != key || door.URL != fmt.Sprintf("http://127.0.0.1:%d", machines.DoorPort) {
		t.Fatalf("remuda.exe reads the door as %+v", door)
	}
}

// TestOnWindowsTheDoorsPortReachesBackOverSSH is the reverse forward a cockpit asks of the machine's sshd: the
// machine's 127.0.0.1:7318 leading back to this remuda.
func TestOnWindowsTheDoorsPortReachesBackOverSSH(t *testing.T) {
	wintest.Live(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "the cockpit answers "+r.URL.Path)
	})}
	go server.Serve(listener)
	defer server.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	out, err := wintest.Remote(t).ForwardingBack(machines.DoorPort, port, fmt.Sprintf("curl -s -m 10 http://127.0.0.1:%d/api/tasks", machines.DoorPort))
	if err != nil || !strings.Contains(out, "the cockpit answers /api/tasks") {
		t.Fatalf("through the reverse forward: %v\n%s", err, out)
	}
}
