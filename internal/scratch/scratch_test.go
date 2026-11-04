package scratch

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

type fakeHerdr struct {
	made   []string // the cwd of every workspace made
	closed []string
	fail   error
	next   int
}

func (f *fakeHerdr) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	if label != Label {
		return nil, errors.New("unexpected label " + label)
	}
	f.next++
	f.made = append(f.made, cwd)
	id := "w" + string(rune('0'+f.next))
	return &herdr.WorkspaceCreateResult{Workspace: herdr.WorkspaceInfo{WorkspaceID: id}, RootPane: herdr.PaneInfo{PaneID: id + ":p1", WorkspaceID: id}}, nil
}

func (f *fakeHerdr) WorkspaceClose(id string) error {
	f.closed = append(f.closed, id)
	return nil
}

// at is a snapshot as Ensure reads it.
func at(snap *herdr.Snapshot) func() (*herdr.Snapshot, error) {
	return func() (*herdr.Snapshot, error) { return snap, nil }
}

// snapshotWith is a snapshot holding one workspace with the given panes, the first tab active
func snapshotWith(workspace string, panes ...herdr.PaneInfo) *herdr.Snapshot {
	snap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: workspace, ActiveTabID: workspace + ":t1"}}}
	snap.Panes = panes
	return snap
}

func TestEnsureMakesTheShellOnceAndFindsItAgain(t *testing.T) {
	dir := t.TempDir()
	client := &fakeHerdr{}
	store := Open(dir)
	shell, err := store.Ensure("gog", client, at(&herdr.Snapshot{}), "/Users/me")
	if err != nil || !shell.Made || shell.Workspace != "w1" || shell.Pane != "w1:p1" || len(client.made) != 1 || client.made[0] != "/Users/me" {
		t.Fatalf("first use: %+v %v %v", shell, err, client.made)
	}
	if !store.Is("gog", "w1") || store.Is("local", "w1") || store.Is("gog", "") {
		t.Fatal("the store does not know its own shell")
	}

	snap := snapshotWith("w1",
		herdr.PaneInfo{PaneID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t0"},
		herdr.PaneInfo{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1"},
		herdr.PaneInfo{PaneID: "w1:p3", WorkspaceID: "w1", TabID: "w1:t1", Focused: true},
	)
	again, err := Open(dir).Ensure("gog", client, at(snap), "/Users/me")
	if err != nil || again.Made || again.Workspace != "w1" || again.Pane != "w1:p3" || len(client.made) != 1 {
		t.Fatalf("kept shell, reopened store: %+v %v made %v", again, err, client.made)
	}
}

func TestEnsureMakesAFreshShellWhenTheKeptOneIsGone(t *testing.T) {
	client := &fakeHerdr{}
	store := Open(t.TempDir())
	if _, err := store.Ensure("gog", client, at(&herdr.Snapshot{}), "/h"); err != nil {
		t.Fatal(err)
	}
	shell, err := store.Ensure("gog", client, at(snapshotWith("w9")), "/h")
	if err != nil || !shell.Made || shell.Workspace != "w2" || !store.Is("gog", "w2") || store.Is("gog", "w1") {
		t.Fatalf("gone shell: %+v %v", shell, err)
	}
}

func TestEnsureSaysWhyHerdrRefused(t *testing.T) {
	client := &fakeHerdr{fail: errors.New("herdr is down")}
	store := Open(t.TempDir())
	if _, err := store.Ensure("gog", client, at(&herdr.Snapshot{}), "/h"); err == nil || store.Is("gog", "") {
		t.Fatalf("a refusal was kept: %v", err)
	}
}

func TestCloseEndsTheShellAndForgetsIt(t *testing.T) {
	dir := t.TempDir()
	client := &fakeHerdr{}
	store := Open(dir)
	if err := store.Close("gog", client, nil); !errors.Is(err, ErrNone) {
		t.Fatalf("closing nothing: %v", err)
	}
	if _, err := store.Ensure("gog", client, at(&herdr.Snapshot{}), "/h"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close("gog", client, snapshotWith("w1", herdr.PaneInfo{PaneID: "w1:p1", WorkspaceID: "w1"})); err != nil {
		t.Fatal(err)
	}
	if len(client.closed) != 1 || client.closed[0] != "w1" || store.Is("gog", "w1") || Open(dir).Is("gog", "w1") {
		t.Fatalf("after close: closed %v", client.closed)
	}

	if _, err := store.Ensure("gog", client, at(&herdr.Snapshot{}), "/h"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close("gog", client, &herdr.Snapshot{}); err != nil || len(client.closed) != 1 || store.Is("gog", "w2") {
		t.Fatalf("a shell herdr lost is only forgotten: %v %v", err, client.closed)
	}
}

func TestADamagedFileStartsAfresh(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Open(dir)
	store.Forget("gog")
	if store.Is("gog", "w1") {
		t.Fatal("a damaged file named a shell")
	}
}

// slowHerdr holds every workspace it makes until let go, as a remote over a slow link does.
type slowHerdr struct {
	mu      sync.Mutex
	made    []string
	asked   chan struct{}
	release chan struct{}
}

func (f *slowHerdr) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	f.asked <- struct{}{}
	<-f.release
	f.mu.Lock()
	defer f.mu.Unlock()
	id := "w" + string(rune('1'+len(f.made)))
	f.made = append(f.made, id)
	return &herdr.WorkspaceCreateResult{Workspace: herdr.WorkspaceInfo{WorkspaceID: id}, RootPane: herdr.PaneInfo{PaneID: id + ":p1", WorkspaceID: id}}, nil
}

func (f *slowHerdr) WorkspaceClose(string) error { return nil }

// snapshot is what herdr holds now: every workspace made so far, a pane each.
func (f *slowHerdr) snapshot() (*herdr.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	snap := &herdr.Snapshot{}
	for _, id := range f.made {
		snap.Workspaces = append(snap.Workspaces, herdr.WorkspaceInfo{WorkspaceID: id})
		snap.Panes = append(snap.Panes, herdr.PaneInfo{PaneID: id + ":p1", WorkspaceID: id})
	}
	return snap, nil
}

func TestTwoAsksAtOnceMakeOneShellAndHoldNoOtherMachine(t *testing.T) {
	store := Open(t.TempDir())
	slow := &slowHerdr{asked: make(chan struct{}, 2), release: make(chan struct{})}
	shells := make(chan Shell, 2)
	for i := 0; i < 2; i++ {
		go func() {
			shell, err := store.Ensure("gog", slow, slow.snapshot, "/h")
			if err != nil {
				t.Error(err)
			}
			shells <- shell
		}()
	}
	<-slow.asked
	answered := make(chan bool, 1)
	go func() { answered <- store.Is("local", "w1") }()
	select {
	case <-answered:
	case <-time.After(2 * time.Second):
		t.Fatal("the roster's question waited on herdr making a shell")
	}
	close(slow.release)
	first, second := <-shells, <-shells
	if len(slow.made) != 1 || first.Workspace != "w1" || second.Workspace != "w1" {
		t.Fatalf("one machine got %v: %+v, %+v", slow.made, first, second)
	}
}
