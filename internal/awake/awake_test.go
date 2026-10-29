package awake

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/shell"
)

type fakeHolder struct {
	mu       sync.Mutex
	holds    map[string]int
	releases map[string]int
	fail     error
}

func newFakeHolder() *fakeHolder {
	return &fakeHolder{holds: map[string]int{}, releases: map[string]int{}}
}

func (f *fakeHolder) Hold(host shell.Host) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.holds[host.ID()]++
	return nil
}

func (f *fakeHolder) Release(host shell.Host) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releases[host.ID()]++
	return nil
}

func (f *fakeHolder) counts(id string) (holds, releases int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holds[id], f.releases[id]
}

func quick(t *testing.T) (*Service, *fakeHolder) {
	t.Helper()
	grace, reassert := Grace, Reassert
	Grace, Reassert = 40*time.Millisecond, 60*time.Millisecond
	t.Cleanup(func() { Grace, Reassert = grace, reassert })
	holder := newFakeHolder()
	s := New(t.TempDir(), func(id string) (shell.Host, bool) { return &shell.Fake{Name: id}, true }, holder)
	t.Cleanup(s.Close)
	return s, holder
}

func TestSwitchOffHoldsNothing(t *testing.T) {
	s, holder := quick(t)
	s.Observe("m", 2)
	s.Wait()
	if holds, _ := holder.counts("m"); holds != 0 {
		t.Fatalf("held with the switch off: %d", holds)
	}
	if st := s.Status("m"); st.On || st.Held || st.Working != 2 {
		t.Fatalf("status %+v", st)
	}
}

func TestHoldWhileWorkingLetGoAfterTheGrace(t *testing.T) {
	s, holder := quick(t)
	if err := s.Set("m", true); err != nil {
		t.Fatal(err)
	}
	s.Observe("m", 1)
	s.Wait()
	if st := s.Status("m"); !st.Held || st.Working != 1 {
		t.Fatalf("not held while working: %+v", st)
	}
	s.Observe("m", 0)
	if st := s.Status("m"); !st.Held {
		t.Fatal("let go the moment the agent settled; the grace should keep it")
	}
	time.Sleep(3 * Grace)
	s.Wait()
	if st := s.Status("m"); st.Held {
		t.Fatalf("still held after the grace: %+v", st)
	}
	if holds, releases := holder.counts("m"); holds != 1 || releases != 1 {
		t.Fatalf("holds %d releases %d", holds, releases)
	}
	s.Observe("m", 1)
	s.Wait()
	if holds, _ := holder.counts("m"); holds != 2 {
		t.Fatalf("an agent working again after a release should take the hold again: holds %d", holds)
	}
}

func TestWorkAgainWithinTheGraceKeepsTheHold(t *testing.T) {
	s, holder := quick(t)
	_ = s.Set("m", true)
	s.Observe("m", 1)
	s.Wait()
	s.Observe("m", 0)
	time.Sleep(Grace / 2)
	s.Observe("m", 1)
	time.Sleep(2 * Grace)
	s.Wait()
	if holds, releases := holder.counts("m"); holds != 1 || releases != 0 {
		t.Fatalf("holds %d releases %d: the hold should have stood through the pause", holds, releases)
	}
}

func TestSwitchOffLetsGoAtOnce(t *testing.T) {
	s, holder := quick(t)
	_ = s.Set("m", true)
	s.Observe("m", 1)
	s.Wait()
	_ = s.Set("m", false)
	s.Wait()
	if st := s.Status("m"); st.Held || st.On {
		t.Fatalf("status %+v", st)
	}
	if _, releases := holder.counts("m"); releases != 1 {
		t.Fatalf("releases %d", releases)
	}
	s.Observe("m", 3)
	s.Wait()
	if holds, _ := holder.counts("m"); holds != 1 {
		t.Fatalf("held again with the switch off: %d", holds)
	}
}

func TestCloseLetsEveryHoldGo(t *testing.T) {
	s, holder := quick(t)
	_ = s.Set("a", true)
	_ = s.Set("b", true)
	s.Observe("a", 1)
	s.Observe("b", 2)
	s.Wait()
	s.Close()
	for _, id := range []string{"a", "b"} {
		if _, releases := holder.counts(id); releases != 1 {
			t.Fatalf("%s: releases %d", id, releases)
		}
	}
}

func TestForgetLetsTheHoldGo(t *testing.T) {
	s, holder := quick(t)
	_ = s.Set("m", true)
	s.Observe("m", 1)
	s.Wait()
	s.Forget("m")
	if _, releases := holder.counts("m"); releases != 1 {
		t.Fatalf("releases %d", releases)
	}
	if st := s.Status("m"); st.On || st.Held {
		t.Fatalf("still known: %+v", st)
	}
}

func TestAFailedHoldIsReportedAndTriedAgainLater(t *testing.T) {
	s, holder := quick(t)
	holder.fail = errors.New("no sleep inhibitor on m")
	_ = s.Set("m", true)
	s.Observe("m", 1)
	s.Wait()
	if st := s.Status("m"); st.Held || st.Error != "no sleep inhibitor on m" {
		t.Fatalf("status %+v", st)
	}
	holder.mu.Lock()
	holder.fail = nil
	holder.mu.Unlock()
	s.Observe("m", 1)
	s.Wait()
	if st := s.Status("m"); st.Held {
		t.Fatal("tried again at once; a failure waits")
	}
	time.Sleep(Reassert + 10*time.Millisecond)
	s.Observe("m", 1)
	s.Wait()
	if st := s.Status("m"); !st.Held || st.Error != "" {
		t.Fatalf("not held after the wait: %+v", st)
	}
}

func TestAStandingHoldIsTakenAgainWhileAgentsWork(t *testing.T) {
	s, holder := quick(t)
	_ = s.Set("m", true)
	s.Observe("m", 1)
	s.Wait()
	time.Sleep(Reassert + 10*time.Millisecond)
	s.Observe("m", 1)
	s.Wait()
	if holds, _ := holder.counts("m"); holds != 2 {
		t.Fatalf("holds %d: a hold that stands is taken again every Reassert, so a dead inhibitor comes back", holds)
	}
}

func TestSwitchesAreKept(t *testing.T) {
	dir := t.TempDir()
	hosts := func(id string) (shell.Host, bool) { return &shell.Fake{Name: id}, true }
	s := New(dir, hosts, newFakeHolder())
	if err := s.Set("a", true); err != nil {
		t.Fatal(err)
	}
	_ = s.Set("b", true)
	_ = s.Set("b", false)
	s.Close()
	again := New(dir, hosts, newFakeHolder())
	defer again.Close()
	if !again.Status("a").On || again.Status("b").On {
		t.Fatalf("a %+v b %+v", again.Status("a"), again.Status("b"))
	}
}

func TestWorkingCountsWorkingAgentPanes(t *testing.T) {
	snap := &herdr.Snapshot{Panes: []herdr.PaneInfo{
		{PaneID: "w1:p1", Agent: "claude", AgentStatus: herdr.StatusWorking},
		{PaneID: "w1:p2", Agent: "codex", AgentStatus: herdr.StatusIdle},
		{PaneID: "w2:p1", AgentStatus: herdr.StatusWorking},
		{PaneID: "w3:p1", Agent: "claude", AgentStatus: herdr.StatusWorking},
	}}
	if n := Working(snap); n != 2 {
		t.Fatalf("working %d", n)
	}
	if n := Working(nil); n != 0 {
		t.Fatalf("working %d for no snapshot", n)
	}
}
