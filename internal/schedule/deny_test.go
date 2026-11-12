package schedule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/transcript"
)

var claudeScout = Schedule{Kind: KindAgent, Machine: "local", Dir: "/home/me/remuda", Agent: "claude", Cron: "*/10 9-16 * * 1-5", Prompt: "Look for what could be better", Close: true, Enabled: true}

func on(b bool) *bool { return &b }

func TestANewAgentScheduleDeniesPromptsAndASavedOneKeepsItsWord(t *testing.T) {
	st, _ := scoutStore(t, t.TempDir())
	s, err := st.Put(claudeScout)
	if err != nil {
		t.Fatal(err)
	}
	if s.DenyPrompts == nil || !*s.DenyPrompts || !s.Denies() {
		t.Fatalf("a new schedule starts with the switch on: %+v", s.DenyPrompts)
	}
	start, err := s.Start()
	if err != nil || !start.DenyPrompts {
		t.Fatalf("its agent starts denying: %+v %v", start, err)
	}
	s.DenyPrompts = nil // a page from before the switch saves without it
	if kept, err := st.Put(s); err != nil || kept.DenyPrompts == nil || !*kept.DenyPrompts {
		t.Fatalf("a save that says nothing keeps the switch: %+v %v", kept.DenyPrompts, err)
	}
	s.DenyPrompts = on(false)
	off, err := st.Put(s)
	if err != nil || off.Denies() {
		t.Fatalf("switched off: %+v %v", off.DenyPrompts, err)
	}
	off.DenyPrompts = nil
	if still, _ := st.Put(off); still.Denies() {
		t.Fatal("a save that says nothing leaves it off")
	}
}

func TestAScheduleSavedBeforeTheSwitchReadsAsOff(t *testing.T) {
	dir := t.TempDir()
	saved := `[{"id":"old1","kind":"agent","machine":"local","dir":"/home/me/remuda","label":"Scout","cron":"0 9 * * *","prompt":"Look around","agent":"claude","close":true,"enabled":true,"last_ok":true}]`
	if err := os.WriteFile(filepath.Join(dir, file), []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	st, _ := scoutStore(t, dir)
	s := st.List()[0]
	if s.DenyPrompts != nil || s.Denies() {
		t.Fatalf("a record from before is off: %+v", s.DenyPrompts)
	}
	if again, err := st.Put(s); err != nil || again.Denies() {
		t.Fatalf("saved again without the switch, still off: %+v %v", again.DenyPrompts, err)
	}
	s.DenyPrompts = on(true)
	if flipped, _ := st.Put(s); !flipped.Denies() {
		t.Fatal("flipped on by the person")
	}
}

func TestOnlyClaudeCodeAgentSchedulesDeny(t *testing.T) {
	st, _ := scoutStore(t, t.TempDir())
	codex, err := st.Put(scout) // Codex: the switch is kept, and means nothing for it
	if err != nil {
		t.Fatal(err)
	}
	if codex.Denies() {
		t.Error("Codex has no such setting")
	}
	if start, err := codex.Start(); err != nil || start.DenyPrompts || strings.Join(start.Args, " ") != "--full-auto" {
		t.Errorf("Codex starts as it did: %+v %v", start, err)
	}
	prompt, err := st.Put(Schedule{Kind: KindPrompt, Machine: "local", Dir: "/x", Cron: "0 9 * * *", Prompt: "hi", DenyPrompts: on(true), Enabled: true})
	if err != nil || prompt.DenyPrompts != nil {
		t.Errorf("a prompt schedule starts no agent: %+v %v", prompt.DenyPrompts, err)
	}
	made := claudeScout
	made.ID = prompt.ID // the prompt schedule made an agent one: as new as one, switch on
	if agent, err := st.Put(made); err != nil || !agent.Denies() {
		t.Errorf("a prompt schedule made an agent one starts denying: %+v %v", agent.DenyPrompts, err)
	}
}

func TestAnAgentSchedulesOwnSettingsFileIsRefusedWhileTheSwitchIsOn(t *testing.T) {
	st, _ := scoutStore(t, t.TempDir())
	s := claudeScout
	s.Args = "--settings ~/.claude/night.json"
	if _, err := st.Put(s); err == nil || !strings.Contains(err.Error(), "night.json") {
		t.Fatalf("a settings file cannot take the setting: %v", err)
	}
	s.DenyPrompts = on(false)
	if _, err := st.Put(s); err != nil {
		t.Fatalf("with the switch off the file is the person's business: %v", err)
	}
	s.DenyPrompts = on(true)
	s.Args = `--settings '{"model":"opus"}'`
	merged, err := st.Put(s)
	if err != nil {
		t.Fatal(err)
	}
	start, _ := merged.Start()
	args, err := sessions.StartArgs(merged.Agent, start)
	if err != nil || len(args) != 2 || !strings.Contains(args[1], `"denyInsteadOfPrompting":true`) || !strings.Contains(args[1], `"model":"opus"`) {
		t.Errorf("merged into the person's JSON: %q %v", args, err)
	}
}

func TestASettledRunSaysWhatItWasDenied(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	st.Agents.Denied = func(machine, pane string) []transcript.Denial {
		return []transcript.Denial{{Tool: "Bash", What: "touch probe.txt"}, {Tool: "Write", What: "/home/me/remuda/notes.txt"}}
	}
	s, err := st.Put(claudeScout)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Run(s.ID); err != nil {
		t.Fatal(err)
	}
	st.OnStatus(status("w5:p2", "idle", "working"))
	st.OnStatus(status("w5:p2", "working", "done"))
	st.Wait()
	after := st.List()[0]
	if after.LastResult != "answered: Made three tasks. · 2 calls denied" || len(after.LastDenied) != 2 || after.LastDenied[1].Tool != "Write" {
		t.Fatalf("after the answer: %q %+v", after.LastResult, after.LastDenied)
	}
	if len(door.pushed) != 1 || !strings.Contains(door.pushed[0].Body, "2 calls denied: Bash touch probe.txt; Write /home/me/remuda/notes.txt") {
		t.Fatalf("pushed %+v", door.pushed)
	}
	if again, _ := st.Run(s.ID); again.LastDenied != nil {
		t.Errorf("a new run starts with nothing denied: %+v", again.LastDenied)
	}
}
