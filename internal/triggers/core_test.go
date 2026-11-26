package triggers

import (
	"context"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/coreapi"
)

// TestCoreReadsTheBuiltInWatches: the two patterns are remuda-core's to run, in its own dialect; one it refused would
// refuse every machine's stream. A throwaway core takes them and tells a match of each as a pane prints it.
func TestCoreReadsTheBuiltInWatches(t *testing.T) {
	c := coretest.Server(t)
	client := c.Client(t)
	adapter := core.NewAdapter(client, nil, core.AdapterHooks{})
	t.Cleanup(func() { adapter.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	running, err := client.Run(ctx, core.RunParams{Argv: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	pane := running.Address
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		if snap, err := adapter.SessionSnapshot(); err == nil && snap.Pane(pane) != nil {
			break
		}
		if time.Since(start) > 10*time.Second {
			t.Fatal("the machine never showed the pane")
		}
	}
	stream, err := adapter.Subscribe([]coreapi.Subscription{
		{Type: coreapi.EventOutputMatched, PaneID: pane, Source: "recent", Lines: lines, Match: &coreapi.OutputMatch{Type: "regex", Value: ServePattern}},
		{Type: coreapi.EventOutputMatched, PaneID: pane, Source: "recent", Lines: lines, Match: &coreapi.OutputMatch{Type: "regex", Value: TestsPattern}},
	})
	if err != nil {
		t.Fatalf("remuda-core refused the built-in watches: %v", err)
	}
	defer stream.Close()
	// the rows go out through printf, so the command line typed shows neither the dev server's words nor the test's
	if err := adapter.PaneSendText(pane, `printf '%s:   http://localhost:%s/\n%s: TestLogin (0.01s)\n' '  ➜  Local' 5173 '--- FAIL'`+"\r"); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	deadline := time.After(10 * time.Second)
	for len(got) < 2 {
		done := make(chan coreapi.Event, 1)
		go func() {
			for {
				e, err := stream.Next()
				if err != nil {
					return
				}
				if e.Kind == coreapi.EventOutputMatched {
					done <- e
					return
				}
			}
		}()
		select {
		case e := <-done:
			got[e.MatchedLine] = true
		case <-deadline:
			t.Fatalf("remuda-core told %v", got)
		}
	}
	if !got["  ➜  Local:   http://localhost:5173/"] || !got["--- FAIL: TestLogin (0.01s)"] {
		t.Fatalf("matches: %v", got)
	}
}
