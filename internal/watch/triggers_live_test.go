package watch_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/triggers"
	"github.com/dev1antdev/remuda/internal/watch"
)

// firesOnce: the watcher and the triggers on the fleet's local machine, whatever it speaks. A pane made while watched
// prints failed tests and a dev server's address: one push, one offer; then the streams are opened anew — a rule
// added, twice — and the server tells the rows still standing on each, which is no news: no second push or offer.
// The quiet spells are off, so only the dedupe of what stood keeps it to one. Last, a rule past Rust's compiled-size
// limit is refused as invalid_regex and the stream stands again without the person's watches.
func firesOnce(t *testing.T, fleet *machines.Manager) {
	savedQuiet, savedRule := triggers.TestsQuiet, triggers.RuleQuiet
	triggers.TestsQuiet, triggers.RuleQuiet = 0, 0
	t.Cleanup(func() { triggers.TestsQuiet, triggers.RuleQuiet = savedQuiet, savedRule })

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
	var refused error
	watcher.Refused = func(_ string, err error) {
		if err != nil {
			mu.Lock()
			refused = err
			mu.Unlock()
		}
	}
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

	client, _, err := mc.Ready()
	if err != nil {
		t.Fatal(err)
	}
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

	// A rule past Rust's compiled-size limit passes the page's checks and is refused by the terminal's server as
	// invalid_regex: the stream stands again without the person's watches.
	if _, err := service.Replace([]triggers.Rule{{Name: "huge", Match: "regex", Pattern: `\w{1000}\w{1000}`, Action: triggers.ActionFeed, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the refusal", func() bool { mu.Lock(); defer mu.Unlock(); return herdr.IsCode(refused, "invalid_regex") })
	eventually(t, "the stream without them", func() bool { return watcher.Streaming(machines.LocalID) })
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
