package restart

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// TestASleepingPaneIsNeitherStoppedNorClosed: the person put the workspace to sleep, so its shells are the sleep's to
// wake — not a reboot's chats to offer back, and a pane of it closed is no agent to reopen.
func TestASleepingPaneIsNeitherStoppedNorClosed(t *testing.T) {
	noGrace(t)
	m := OpenMemory("")
	asleep := map[string]bool{"w1:p1": true, "w1:p2": true}
	m.Asleep = func(machineID, workspaceID, paneID string) bool {
		return machineID == "local" && workspaceID == "w1" && asleep[paneID]
	}
	observe(m, "local", false, agentPane("w1:p1", "claude", "/work/api"), agentPane("w1:p2", "codex", "/work/api"), agentPane("w1:p3", "claude", "/work/api"))
	if _, ok := m.chats["local|w1:p1"]; ok {
		t.Fatal("a sleeping pane is not remembered")
	}
	// herdr comes back after a reboot: the sleeping panes are shells as the sleep left them, the third was stopped
	observe(m, "local", true, shellPane("w1:p1", "/work/api"), shellPane("w1:p2", "/work/api"), shellPane("w1:p3", "/work/api"))
	stopped := m.Stopped("local")
	if len(stopped) != 1 || stopped[0].Pane != "w1:p3" {
		t.Fatalf("only the chat the reboot stopped is listed: %+v", stopped)
	}
	// the person closes a sleeping pane
	observe(m, "local", false, shellPane("w1:p1", "/work/api"), shellPane("w1:p3", "/work/api"))
	if closed := m.Closed(); len(closed) != 0 {
		t.Fatalf("a sleeping pane closed is no closed agent: %+v", closed)
	}
	// woken, it is remembered again
	delete(asleep, "w1:p1")
	observe(m, "local", false, agentPane("w1:p1", "claude", "/work/api"), shellPane("w1:p3", "/work/api"))
	if m.chats["local|w1:p1"].Session != sid {
		t.Fatal("a woken pane is remembered")
	}
}

// TestOnStoppedHearsAListThatGrew: what resumes the chats by itself hears a machine whose stopped list gained a pane,
// once, and not the snapshots that only keep it.
func TestOnStoppedHearsAListThatGrew(t *testing.T) {
	noGrace(t)
	m := OpenMemory("")
	var heard []string
	m.OnStopped = func(machineID string) { heard = append(heard, machineID) }
	observe(m, "gog", true, agentPane("w1:p1", "claude", "/work/api"), agentPane("w1:p2", "codex", "/work/api"))
	observe(m, "gog", true, shellPane("w1:p1", "/work/api"), shellPane("w1:p2", "/work/api"))
	observe(m, "gog", false, shellPane("w1:p1", "/work/api"), shellPane("w1:p2", "/work/api"))
	if len(heard) != 1 || heard[0] != "gog" {
		t.Fatalf("heard: %v", heard)
	}
	r := &Restarter{Memory: m}
	if picked := r.Picked("gog", []string{"w1:p2", "w9:p9"}); len(picked) != 1 || picked[0].Pane != "w1:p2" || picked[0].Command != "codex resume "+sid || picked[0].Dir != "/work/api" {
		t.Fatalf("picked: %+v", picked)
	}
	if picked := r.Picked("gog", nil); len(picked) != 2 {
		t.Fatalf("none picked is every one: %+v", picked)
	}
	if !r.claimRecovery("gog") || r.claimRecovery("gog") || !r.Recovering("gog") {
		t.Fatal("one recovery of a machine at a time")
	}
	r.releaseRecovery("gog")
	if r.Recovering("gog") {
		t.Fatal("released")
	}
}

// TestLeaveWaitsForTheShellAndTriesCtrlC: the leave shared by the restarts and the sleep.
func TestLeaveWaitsForTheShellAndTriesCtrlC(t *testing.T) {
	was := LeavePoll
	LeavePoll = time.Millisecond
	t.Cleanup(func() { LeavePoll = was })
	client := &leavingHerdr{snap: snapshot(agentPane("w1:p1", "claude", "/work/api")), lingers: true}
	var typed []string
	state, err := Leave(client, "w1:p1", "claude", func(text string) { typed = append(typed, text) }, func(string) {})
	if err != nil || state != LeftShell {
		t.Fatalf("leave: %v %v", state, err)
	}
	if len(typed) != 1 || typed[0] != "/exit" || len(client.keys) != 1 || client.keys[0] != "ctrl+c ctrl+c" {
		t.Fatalf("typed %v, keys %v", typed, client.keys)
	}
}

// leavingHerdr is an agent that leaves on its exit, or only after a Ctrl-C when it lingers.
type leavingHerdr struct {
	snap    *herdr.Snapshot
	lingers bool
	keys    []string
}

func (f *leavingHerdr) SessionSnapshot() (*herdr.Snapshot, error) { return f.snap, nil }
func (f *leavingHerdr) AgentPrompt(target, text string) error {
	if !f.lingers {
		f.snap.Panes[0].Agent = ""
	}
	return nil
}
func (f *leavingHerdr) PaneSendKeys(paneID string, keys []string) error {
	f.keys = append(f.keys, keys[0]+" "+keys[1])
	f.snap.Panes[0].Agent = ""
	return nil
}
