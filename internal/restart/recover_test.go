package restart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

const sid = "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"

func agentPane(id, kind, dir string) herdr.PaneInfo {
	return herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", Agent: kind, AgentStatus: "idle", CWD: dir, Title: "✳ Claude Code", AgentSession: &herdr.AgentSession{Kind: "id", Value: sid}}
}

func shellPane(id, dir string) herdr.PaneInfo {
	return herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", CWD: dir, Title: "zsh"}
}

func snapshot(panes ...herdr.PaneInfo) *herdr.Snapshot {
	return &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}}, Panes: panes}
}

func observe(m *Memory, machine string, first bool, panes ...herdr.PaneInfo) {
	m.Observe(events.Snapshot{Machine: machine, Snap: snapshot(panes...), At: time.Now(), First: first})
}

// noGrace lists a shell as stopped at once, so the rules can be read without a clock.
func noGrace(t *testing.T) {
	was := stoppedGrace
	stoppedGrace = 0
	t.Cleanup(func() { stoppedGrace = was })
}

// TestStoppedWaitsOutHerdrsOwnRelaunch: herdr brings the agents it started back a second after restoring their
// panes, so a shell counts as stopped only once it has stood for the grace.
func TestStoppedWaitsOutHerdrsOwnRelaunch(t *testing.T) {
	m := OpenMemory("")
	at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	snap := func(first bool, when time.Duration, panes ...herdr.PaneInfo) {
		m.Observe(events.Snapshot{Machine: "local", Snap: snapshot(panes...), At: at.Add(when), First: first})
	}
	snap(true, 0, agentPane("w1:p1", "claude", "/work/api"), agentPane("w2:p1", "codex", "/work/web"), agentPane("w3:p1", "claude", "/work/cli"), agentPane("w4:p1", "kimi", "/work/old"))
	// the first snapshot after the reboot: every pane a bare shell, the last two still reading the server's folder as their own
	snap(true, time.Minute, shellPane("w1:p1", "/work/api"), shellPane("w2:p1", "/work/web"), shellPane("w3:p1", "/home/me"), shellPane("w4:p1", "/home/me"))
	if n := len(m.Stopped("local")); n != 0 {
		t.Fatalf("listed before the grace: %d", n)
	}
	// herdr relaunched Codex itself; the third shell now reads its folder; the fourth really is somewhere else
	later := func(when time.Duration) {
		snap(false, when, shellPane("w1:p1", "/work/api"), agentPane("w2:p1", "codex", "/work/web"), shellPane("w3:p1", "/work/cli"), shellPane("w4:p1", "/home/me"))
	}
	later(time.Minute + 2*time.Second)
	if n := len(m.Stopped("local")); n != 0 || len(m.chats) != 4 {
		t.Fatalf("within the grace nothing is listed or forgotten: %d stopped, %d chats", n, len(m.chats))
	}
	later(time.Minute + stoppedGrace)
	stopped := m.Stopped("local")
	if len(stopped) != 2 || stopped[0].Pane != "w1:p1" || stopped[1].Pane != "w3:p1" {
		t.Fatalf("after the grace: %+v", stopped)
	}
	if _, ok := m.shells["local|w2:p1"]; ok {
		t.Fatal("a pane whose agent came back is no shell on its way to the list")
	}
	if _, ok := m.chats["local|w4:p1"]; ok {
		t.Fatal("a shell still elsewhere after the grace is not the chat's: forgotten")
	}
}

// TestMemoryTellsARebootFromAnExit: a shell where a chat was counts as stopped only when the change was not watched happening.
func TestMemoryTellsARebootFromAnExit(t *testing.T) {
	noGrace(t)
	m := OpenMemory("")
	observe(m, "local", true, agentPane("w1:p1", "claude", "/work/api"), agentPane("w1:p2", "codex", "/work/api"), shellPane("w1:p3", "/work/api"))
	if n := len(m.Stopped("local")); n != 0 {
		t.Fatalf("agents running: %d stopped", n)
	}
	// the person left Claude Code: watched happening, so the chat is theirs to end
	observe(m, "local", false, shellPane("w1:p1", "/work/api"), agentPane("w1:p2", "codex", "/work/api"))
	if n := len(m.Stopped("local")); n != 0 {
		t.Fatalf("an exit the watcher saw: %d stopped", n)
	}
	if _, ok := m.chats["local|w1:p1"]; ok {
		t.Fatal("a chat the person ended is forgotten")
	}
	// the Codex chat is still remembered; herdr goes away and comes back with its pane a bare shell
	observe(m, "local", true, shellPane("w1:p1", "/work/api"), shellPane("w1:p2", "/work/api"))
	stopped := m.Stopped("local")
	if len(stopped) != 1 || stopped[0].Pane != "w1:p2" || stopped[0].Agent != "codex" || stopped[0].Title != "api" {
		t.Fatalf("stopped after the come-back: %+v", stopped)
	}
	// it stays on the list through the ordinary snapshots that follow
	observe(m, "local", false, shellPane("w1:p1", "/work/api"), shellPane("w1:p2", "/work/api"))
	if len(m.Stopped("local")) != 1 {
		t.Fatal("a stopped chat is kept while its shell stands")
	}
	// the person did something else in the pane: no longer theirs to resume
	observe(m, "local", false, shellPane("w1:p1", "/work/api"), shellPane("w1:p2", "/home/me"))
	if len(m.Stopped("local")) != 0 {
		t.Fatal("a shell that moved folders is not the chat's")
	}
	if _, ok := m.chats["local|w1:p2"]; ok {
		t.Fatal("forgotten with it")
	}
}

func TestMemoryFollowsTheAgentBackAndForgetsWhatIsGone(t *testing.T) {
	noGrace(t)
	m := OpenMemory("")
	observe(m, "gog", true, agentPane("w1:p1", "claude", "/work/api"))
	observe(m, "gog", true, shellPane("w1:p1", "/work/api"))
	if len(m.Stopped("gog")) != 1 {
		t.Fatal("stopped")
	}
	// the person ran `claude --resume` by hand: an agent again, remembered anew, not stopped
	observe(m, "gog", false, agentPane("w1:p1", "claude", "/work/api"))
	if len(m.Stopped("gog")) != 0 || m.chats["gog|w1:p1"].Session != sid {
		t.Fatalf("back by hand: %+v %+v", m.Stopped("gog"), m.chats)
	}
	// the pane closed: forgotten
	observe(m, "gog", false)
	if len(m.chats) != 0 {
		t.Fatalf("a closed pane is forgotten: %+v", m.chats)
	}
	// another machine's memory is its own
	observe(m, "gog", true, agentPane("w1:p1", "kimi", "/work/api"))
	observe(m, "roofr", true, agentPane("w1:p1", "gemini", "/work/web"))
	observe(m, "roofr", true, shellPane("w1:p1", "/work/web"))
	if len(m.Stopped("gog")) != 0 || len(m.Stopped("roofr")) != 1 {
		t.Fatal("per machine")
	}
	m.Forget("roofr")
	if len(m.Stopped("roofr")) != 0 || len(m.chats) != 1 {
		t.Fatalf("Forget: %+v", m.chats)
	}
}

func TestMemoryIsPausedWhileRemudaRestartsHerdrItself(t *testing.T) {
	noGrace(t)
	m := OpenMemory("")
	observe(m, "local", true, agentPane("w1:p1", "claude", "/work/api"))
	m.Pause("local")
	observe(m, "local", true, shellPane("w1:p1", "/work/api"))
	if len(m.Stopped("local")) != 0 {
		t.Fatal("paused: the server restart brings its own chats back")
	}
	if m.chats["local|w1:p1"].Kind != "claude" {
		t.Fatal("paused: the memory stands")
	}
	m.Unpause("local")
	observe(m, "local", false, agentPane("w1:p1", "claude", "/work/api"))
	if len(m.Stopped("local")) != 0 || len(m.chats) != 1 {
		t.Fatal("after the pause")
	}
}

func TestMemoryIsKeptAcrossARestartOfRemuda(t *testing.T) {
	noGrace(t)
	dir := t.TempDir()
	m := OpenMemory(dir)
	observe(m, "local", true, agentPane("w1:p1", "claude", "/work/api"), agentPane("w2:p1", "codex", "/work/web"))
	if _, err := os.Stat(filepath.Join(dir, chatsFile)); err != nil {
		t.Fatalf("chats.json: %v", err)
	}
	// remuda starts again after the reboot: its first snapshot shows the restored panes as shells
	again := OpenMemory(dir)
	observe(again, "local", true, shellPane("w1:p1", "/work/api"), shellPane("w2:p1", "/work/web"))
	stopped := again.Stopped("local")
	if len(stopped) != 2 || stopped[0].Pane != "w1:p1" || stopped[1].Pane != "w2:p1" || stopped[1].chat.Session != sid {
		t.Fatalf("stopped after remuda's own start: %+v", stopped)
	}
	// a damaged file starts empty, loudly in the log, not with a crash
	if err := os.WriteFile(filepath.Join(dir, chatsFile), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := len(OpenMemory(dir).chats); n != 0 {
		t.Fatalf("damaged file: %d chats", n)
	}
}

// fakeHerdr answers the recovery's two questions: what is in the panes now, and whether a start takes.
type fakeHerdr struct {
	snap    *herdr.Snapshot
	refuse  map[string]error
	started []string
}

func (f *fakeHerdr) SessionSnapshot() (*herdr.Snapshot, error) { return f.snap, nil }
func (f *fakeHerdr) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	if err := f.refuse[paneID]; err != nil {
		return err
	}
	f.started = append(f.started, fmt.Sprintf("%s %s %s %s", paneID, kind, name, strings.Join(args, " ")))
	return nil
}

func TestResumeStartsEachStoppedChatOnItsSessionAndSaysWhyNot(t *testing.T) {
	noGrace(t)
	m := OpenMemory("")
	r := New(nil)
	r.Memory = m
	var running []herdr.PaneInfo
	for _, id := range []string{"w1:p1", "w1:p2", "w1:p3", "w1:p4", "w1:p5", "w1:p6", "w1:p7"} {
		running = append(running, agentPane(id, "claude", "/work/api"))
	}
	observe(m, "local", true, running...)
	observe(m, "local", true,
		shellPane("w1:p1", "/work/api"), shellPane("w1:p2", "/work/api"), shellPane("w1:p3", "/work/api"),
		shellPane("w1:p4", "/work/api"), shellPane("w1:p5", "/work/api"), shellPane("w1:p6", "/work/api"), shellPane("w1:p7", "/work/api"))
	if len(m.Stopped("local")) != 7 {
		t.Fatalf("stopped: %d", len(m.Stopped("local")))
	}
	// by the time Resume is pressed the panes moved on in every way they can
	fresh := snapshot(
		shellPane("w1:p1", "/work/api"),                  // resumed
		shellPane("w1:p2", "/work/api"),                  // the start fails: stays on the list
		shellPane("w1:p3", "/home/me"),                   // the shell moved folders
		herdr.PaneInfo{PaneID: "w1:p4", Agent: "gemini"}, // something else runs there
		agentPane("w1:p5", "claude", "/work/api"),        // back already, by hand
		shellPane("w1:p6", "/work/api"),                  // its folder is gone (the checks say so)
		// w1:p7 is gone
	)
	client := &fakeHerdr{snap: fresh, refuse: map[string]error{"w1:p2": errors.New("agent_start_failed: no such binary")}}
	checks := func(s Stopped) error {
		if s.Pane == "w1:p6" {
			return errors.New("its folder /work/api is gone — nothing to resume into")
		}
		return nil
	}
	var log []string
	out := r.resume(client, "local", m.Stopped("local"), checks, func(line string) { log = append(log, line) })
	if len(client.started) != 1 || client.started[0] != "w1:p1 claude claude-w1-p1 --resume "+sid {
		t.Fatalf("started: %v", client.started)
	}
	if back := out.back(); len(back) != 2 { // p1 resumed, p5 back already
		t.Fatalf("back: %v", back)
	}
	if failed := out.failed(); len(failed) != 1 || failed[0] != "api" {
		t.Fatalf("failed: %v", failed)
	}
	stopped := m.Stopped("local")
	if len(stopped) != 1 || stopped[0].Pane != "w1:p2" {
		t.Fatalf("still on the list after the recovery: %+v", stopped)
	}
	for _, pane := range []string{"w1:p3", "w1:p4", "w1:p6", "w1:p7"} {
		if _, ok := m.chats["local|"+pane]; ok {
			t.Errorf("%s: a chat that will not come back is forgotten", pane)
		}
	}
	for _, pane := range []string{"w1:p1", "w1:p2", "w1:p5"} {
		if _, ok := m.chats["local|"+pane]; !ok {
			t.Errorf("%s: still remembered", pane)
		}
	}
	text := strings.Join(log, "\n")
	for _, want := range []string{"✓ Claude Code is back in api, session 9b1c2d3e resumed", "no such binary", "run `claude --resume " + sid + "` there", "the shell is in /home/me now", `runs "gemini" now`, "is back already", "its folder /work/api is gone", "the pane is gone"} {
		if !strings.Contains(text, want) {
			t.Errorf("the log lacks %q:\n%s", want, text)
		}
	}
	if chatsWord(1) != "1 chat" || chatsWord(3) != "3 chats" {
		t.Fatal("chatsWord")
	}
}
