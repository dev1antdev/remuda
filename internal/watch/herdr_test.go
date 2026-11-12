package watch

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
	"github.com/dev1antdev/remuda/internal/machines"
)

// TestTheWatcherHearsAThrowawayHerdr: the watcher on a herdr of the test's own, its poll slowed past the test's length,
// so only the stream can tell it: a pane made joins the stream, and its agent's every status reaches the bus at once —
// the first one too, said before the new stream stood.
func TestTheWatcherHearsAThrowawayHerdr(t *testing.T) {
	sock := herdrtest.Server(t)
	saved := []time.Duration{localEvery, localStreamingEvery}
	localEvery, localStreamingEvery = time.Minute, time.Minute
	t.Cleanup(func() { localEvery, localStreamingEvery = saved[0], saved[1] })
	fleet := machines.NewManager(t.TempDir(), sock, "", nil)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	var mu sync.Mutex
	var moves []string
	events.Subscribe(bus, func(e events.PaneStatus) {
		mu.Lock()
		moves = append(moves, e.Pane.Pane+" "+e.To)
		mu.Unlock()
	})
	heard := func(want string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, m := range moves {
			if m == want {
				return true
			}
		}
		return false
	}
	watcher := New(fleet, bus)
	done := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		watcher.run(mc, done)
	}()
	defer func() {
		close(done)
		<-ended
	}()
	waitFor(t, "the stream", func() bool {
		watcher.mu.Lock()
		defer watcher.mu.Unlock()
		return watcher.stateFor(machines.LocalID).streaming.Load()
	})

	client := herdr.NewClient(sock)
	made, err := client.WorkspaceCreate(os.TempDir(), "watched")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	pane := made.RootPane.PaneID
	waitFor(t, "the new pane in a snapshot", func() bool {
		snap, _ := watcher.Snapshot(machines.LocalID)
		return snap != nil && snap.Pane(pane) != nil
	})
	for _, state := range []string{"working", "blocked", "working", "idle"} {
		mu.Lock()
		moves = nil
		mu.Unlock()
		asked := time.Now()
		if err := client.Call("pane.report_agent", map[string]any{"pane_id": pane, "source": "remuda-test", "agent": "claude", "state": state}, nil); err != nil {
			t.Fatal(err)
		}
		want := pane + " " + state
		waitFor(t, want, func() bool { return heard(want) || state == "idle" && heard(pane+" done") }) // a turn ended reads done
		if took := time.Since(asked); took > 2*time.Second {
			t.Fatalf("%s took %s with the poll a minute apart", want, took)
		}
	}
}
