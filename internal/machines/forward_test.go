package machines

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/sshx"
)

// TestTwoFirstAsksOfAPortMakeOneForward: a page that opens the editor, a preview or a peer's terminal asks for the
// same port from several requests at once; each that missed the cache made a forward of its own, and every one but
// the last stayed on the master, listening on a port of the cockpit's, until the master went.
func TestTwoFirstAsksOfAPortMakeOneForward(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + script.Quote(calls) + "\nsleep 0.1\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	mc := newRemote(Config{ID: "gog", Label: "GOG", Host: "gog.remote"})
	mc.conn = &sshx.Conn{Host: "gog.remote"}
	var wg sync.WaitGroup
	addrs := make([]string, 8)
	for i := range addrs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addr, err := mc.ForwardHostAddr("127.0.0.1", 3000)
			if err != nil {
				t.Error(err)
			}
			addrs[i] = addr
		}()
	}
	wg.Wait()
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if forwards := strings.Count(string(b), "-O forward -L"); forwards != 1 {
		t.Fatalf("%d forwards made for one port, want one", forwards)
	}
	for _, addr := range addrs {
		if addr != addrs[0] {
			t.Fatalf("the asks got different addresses: %v", addrs)
		}
	}
}
