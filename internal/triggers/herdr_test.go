package triggers

import (
	"os"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
)

// TestHerdrReadsTheBuiltInWatches: the two patterns are herdr's to run, in its Rust dialect; one it refused would
// refuse every machine's stream. A throwaway herdr takes them and tells a match of each as a pane prints it.
func TestHerdrReadsTheBuiltInWatches(t *testing.T) {
	sock := herdrtest.Server(t)
	client := herdr.NewClient(sock)
	made, err := client.WorkspaceCreate(os.TempDir(), "triggers")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	pane := made.RootPane.PaneID
	stream, err := client.Subscribe([]herdr.Subscription{
		{Type: herdr.EventOutputMatched, PaneID: pane, Source: "recent", Lines: lines, Match: &herdr.OutputMatch{Type: "regex", Value: ServePattern}},
		{Type: herdr.EventOutputMatched, PaneID: pane, Source: "recent", Lines: lines, Match: &herdr.OutputMatch{Type: "regex", Value: TestsPattern}},
	})
	if err != nil {
		t.Fatalf("herdr refused the built-in watches: %v", err)
	}
	defer stream.Close()
	// the rows go out through printf, so the command line typed shows neither the dev server's words nor the test's
	if err := client.PaneSendText(pane, `printf '%s:   http://localhost:%s/\n%s: TestLogin (0.01s)\n' '  ➜  Local' 5173 '--- FAIL'`+"\r"); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	deadline := time.After(5 * time.Second)
	for len(got) < 2 {
		done := make(chan herdr.Event, 1)
		go func() {
			if e, err := stream.Next(); err == nil {
				done <- e
			}
		}()
		select {
		case e := <-done:
			got[e.MatchedLine] = true
		case <-deadline:
			t.Fatalf("herdr told %v", got)
		}
	}
	if !got["  ➜  Local:   http://localhost:5173/"] || !got["--- FAIL: TestLogin (0.01s)"] {
		t.Fatalf("matches: %v", got)
	}
}
