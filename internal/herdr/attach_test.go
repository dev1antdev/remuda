package herdr

import (
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAttach is an attach command of the test's own: what it prints into the PTY, and whether it then exits as herdr
// does after a refusal or stays up as a live attach does.
func fakeAttach(prints string, exits bool) func(string) *exec.Cmd {
	tail := "; sleep 30"
	if exits {
		tail = "; exit 1"
	}
	return func(string) *exec.Cmd {
		return exec.Command("sh", "-c", "printf '%s' "+shellWord(prints)+tail)
	}
}

func shellWord(s string) string { return "'" + s + "'" }

// TestAScreenSayingAttachFailedIsNoRefusal: herdr prints its refusal into the PTY and exits at once, so a pane whose
// screen merely shows those words — an agent reading this very file — is a live attach, not herdr declining.
func TestAScreenSayingAttachFailedIsNoRefusal(t *testing.T) {
	defer func(d time.Duration) { refusalExitWait = d }(refusalExitWait)
	refusalExitWait = 300 * time.Millisecond

	screen := "$ go test ./internal/herdr\\r\\nattach_test.go: terminal attach failed: already has an attached client\\r\\n$ "
	m := NewAttachManager(fakeAttach(screen, false))
	ch, cancel, err := m.Subscribe("t1", 80, 24)
	if err != nil {
		t.Fatalf("a live attach whose screen shows the words was refused: %v", err)
	}
	defer func() {
		cancel()
		if a, err := m.get("t1"); err == nil {
			m.kill(a)
		}
	}()
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("the attach was killed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no output reached the viewer")
	}

	refused := NewAttachManager(fakeAttach("herdr: server shut down: terminal attach failed: terminal t2 already has an attached client; retry with --takeover\\r\\n", true))
	if _, _, err := refused.Subscribe("t2", 80, 24); !Refused(err) {
		t.Fatalf("herdr's refusal, printed and followed by the exit, must be one: %v", err)
	}
}

// TestAnAttachHoldsItsPlaceAndItsCarrierCanRefuse: the machine counts it while it stands, and says why ssh would not carry it.
func TestAnAttachHoldsItsPlaceAndItsCarrierCanRefuse(t *testing.T) {
	var held, released atomic.Int32
	m := NewAttachManager(fakeAttach("screen", false))
	m.Hold = func() func() {
		held.Add(1)
		return func() { released.Add(1) }
	}
	_, cancel, err := m.Subscribe("t1", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if held.Load() != 1 || released.Load() != 0 {
		t.Fatalf("held %d, released %d", held.Load(), released.Load())
	}
	cancel()
	m.mu.Lock()
	a := m.attaches["t1"]
	m.mu.Unlock()
	m.kill(a)
	if released.Load() != 1 {
		t.Errorf("released %d", released.Load())
	}
	refused := NewAttachManager(fakeAttach("no more sessions", true))
	refused.ReadRefusal = func(first string) (string, bool) { return "the machine is full", strings.Contains(first, "no more") }
	if _, _, err := refused.Subscribe("t2", 80, 24); err == nil || err.Error() != "the machine is full" {
		t.Errorf("a refusal by what carries the attach: %v", err)
	}
}
