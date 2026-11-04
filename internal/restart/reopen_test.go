package restart

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// slowPlacer holds the tab it makes until let go, as a remote over a slow link does.
type slowPlacer struct {
	fakePlacer
	mu      sync.Mutex
	asked   chan struct{}
	release chan struct{}
}

func (f *slowPlacer) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	f.asked <- struct{}{}
	<-f.release
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fakePlacer.TabCreate(workspaceID, label)
}

func (f *slowPlacer) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fakePlacer.AgentStartWith(paneID, kind, name, args, timeout)
}

func TestAReopenAskedTwiceStartsTheAgentOnce(t *testing.T) {
	r := &Restarter{Memory: OpenMemory("")}
	c := closedOne(t, r)
	client := &slowPlacer{asked: make(chan struct{}, 2), release: make(chan struct{})}
	client.snap = &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/work/api"}}}}
	first := make(chan error, 1)
	go func() {
		_, err := r.reopenWith(client, "local", c)
		first <- err
	}()
	<-client.asked
	if _, err := r.reopenWith(client, "local", c); !errors.Is(err, ErrReopening) {
		t.Fatalf("a second reopen while the first runs: %v", err)
	}
	close(client.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if _, err := r.reopenWith(client, "local", c); !errors.Is(err, ErrNoSuchClosed) {
		t.Fatalf("a reopen that came late, its entry in hand: %v", err)
	}
	if len(client.started) != 1 {
		t.Fatalf("started %v", client.started)
	}
}
