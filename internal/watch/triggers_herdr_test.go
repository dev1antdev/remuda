package watch_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/triggers"
	"github.com/dev1antdev/remuda/internal/watch"
)

// TestATriggerFiresOnceThroughNewStreams: the watcher and the triggers on a herdr of the test's own. A pane made while
// watched prints failed tests and a dev server's address: one push, one offer; then the streams are opened anew —
// a rule added, twice — and herdr tells the rows still standing on each, which is no news: no second push or offer.
// The quiet spells are off, so only the dedupe of what stood keeps it to one.
func TestATriggerFiresOnceThroughNewStreams(t *testing.T) {
	sock := herdrtest.Server(t)
	savedQuiet, savedRule := triggers.TestsQuiet, triggers.RuleQuiet
	triggers.TestsQuiet, triggers.RuleQuiet = 0, 0
	t.Cleanup(func() { triggers.TestsQuiet, triggers.RuleQuiet = savedQuiet, savedRule })

	fleet := machines.NewManager(t.TempDir(), sock, "", nil)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	watcher := watch.New(fleet, bus)
	service := triggers.New(t.TempDir())
	var mu sync.Mutex
	var pushes []push.Payload
	offers := 0
	service.Notify = func(p push.Payload) { mu.Lock(); pushes = append(pushes, p); mu.Unlock() }
	service.Listening = func(string, int) (string, bool) { return "vite", true }
	service.Changed = func() { mu.Lock(); offers++; mu.Unlock() }
	service.Resubscribe = watcher.Resubscribe
	watcher.Outputs = service.Watches
	events.Subscribe(bus, service.Observe)
	events.Subscribe(bus, service.OnMatch)
	counts := func() (int, int) {
		mu.Lock()
		defer mu.Unlock()
		return len(pushes), offers
	}
	done := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		watcher.RunUntil(mc, done)
	}()
	defer func() {
		close(done)
		<-ended
	}()
	eventually(t, "the stream", func() bool { return watcher.Streaming(machines.LocalID) })

	client := herdr.NewClient(sock)
	made, err := client.WorkspaceCreate(os.TempDir(), "api")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	pane := made.RootPane.PaneID
	eventually(t, "the pane in a snapshot", func() bool {
		snap, _ := watcher.Snapshot(machines.LocalID)
		return snap != nil && snap.Pane(pane) != nil
	})
	time.Sleep(time.Second) // the stream with the pane's watches stands
	// printf, so the command line typed shows neither the test's words nor the server's
	if err := client.PaneSendText(pane, `printf '%s: TestLogin (0.01s)\n%s:   http://localhost:%s/\n' '--- FAIL' '  ➜  Local' 5173`+"\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a push and an offer", func() bool { p, o := counts(); return p == 1 && o == 1 })
	mu.Lock()
	first := pushes[0]
	mu.Unlock()
	if first.Kind != "tests" || first.Title != "Tests failed in api" {
		t.Fatalf("the push: %+v", first)
	}
	for i, pattern := range []string{"never printed", "nor this"} {
		if _, err := service.Replace([]triggers.Rule{{Name: "rule", Match: "substring", Pattern: pattern, Action: triggers.ActionFeed, Enabled: true}}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1500 * time.Millisecond) // a new stream; herdr tells it the rows standing
		if p, o := counts(); p != 1 || o != 1 {
			t.Fatalf("after new stream %d, the rows that stood are no news: %d pushes, %d offers", i+1, p, o)
		}
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
